package projection

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aerial-project/aerial/internal/audit"
	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/timeline"
)

func d(s string) timeline.Date { return timeline.MustParseDate(s) }

func clock() time.Time { return time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC) }

type fixture struct {
	licences     *domain.LicenceRegister
	declarations *domain.Register
	occupancy    *domain.OccupancyRegister
	log          *audit.Log
	enforcement  *Enforcement
	service      *Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		licences:     domain.NewLicenceRegister(),
		declarations: domain.NewRegister(),
		occupancy:    domain.NewOccupancyRegister(),
		log:          audit.New(clock),
	}
	var err error
	f.enforcement, err = NewEnforcement(f.licences, f.declarations, f.occupancy, f.log, []byte("test-salt"))
	if err != nil {
		t.Fatal(err)
	}
	f.service = NewService(f.licences, f.occupancy, f.log)
	return f
}

var (
	officer = audit.Actor{ID: "OFFICER1", Role: audit.RoleEnforcement}
	agent   = audit.Actor{ID: "AGENT1", Role: audit.RoleService}
)

// The architectural guarantee, asserted structurally: no exported method on the
// enforcement projection may accept or return a party identifier.
func TestEnforcementCannotBeAskedAboutAParty(t *testing.T) {
	partyType := reflect.TypeOf(ids.PartyID(""))
	et := reflect.TypeOf(&Enforcement{})

	for i := 0; i < et.NumMethod(); i++ {
		m := et.Method(i)
		// Parameter 0 is the receiver, which legitimately holds the registers.
		for j := 1; j < m.Type.NumIn(); j++ {
			if mentions(m.Type.In(j), partyType, map[reflect.Type]bool{}) {
				t.Errorf("%s takes a party identifier in parameter %d", m.Name, j)
			}
		}
		for j := 0; j < m.Type.NumOut(); j++ {
			if mentions(m.Type.Out(j), partyType, map[reflect.Type]bool{}) {
				t.Errorf("%s returns a party identifier in result %d", m.Name, j)
			}
		}
	}
}

func mentions(t, target reflect.Type, seen map[reflect.Type]bool) bool {
	if t == target {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true

	switch t.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Array, reflect.Chan:
		return mentions(t.Elem(), target, seen)
	case reflect.Map:
		return mentions(t.Key(), target, seen) || mentions(t.Elem(), target, seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if mentions(t.Field(i).Type, target, seen) {
				return true
			}
		}
	}
	return false
}

// The same person at two addresses must not be linkable from enforcement reads.
func TestOccupantTokensDoNotJoinAcrossPremises(t *testing.T) {
	f := newFixture(t)
	on := d("2026-06-01")
	period := timeline.From(d("2026-01-01"))

	for _, p := range []ids.PremisesID{"P1", "P2"} {
		if err := f.occupancy.Record(domain.Occupancy{Party: "SAME_PERSON", Premises: p, Period: period}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := f.enforcement.Snapshot(officer, "P1", on)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.enforcement.Snapshot(officer, "P2", on)
	if err != nil {
		t.Fatal(err)
	}

	if len(first.OccupantTokens) != 1 || len(second.OccupantTokens) != 1 {
		t.Fatalf("expected one token each, got %d and %d", len(first.OccupantTokens), len(second.OccupantTokens))
	}
	if first.OccupantTokens[0] == second.OccupantTokens[0] {
		t.Error("the same person produced the same token at two premises, which permits a cross-address join")
	}
}

func TestOccupantTokenIsStableAtOnePremises(t *testing.T) {
	f := newFixture(t)
	on := d("2026-06-01")
	if err := f.occupancy.Record(domain.Occupancy{Party: "P", Premises: "P1", Period: timeline.From(d("2026-01-01"))}); err != nil {
		t.Fatal(err)
	}

	a, _ := f.enforcement.Snapshot(officer, "P1", on)
	b, _ := f.enforcement.Snapshot(officer, "P1", on)
	if a.OccupantTokens[0] != b.OccupantTokens[0] {
		t.Error("tokens must be stable for the same premises so repeat visits are recognisable")
	}
}

func TestEnforcementRoleCannotReadPartyHistory(t *testing.T) {
	f := newFixture(t)
	if _, err := f.service.History(officer, "SOMEONE"); !errors.Is(err, ErrWrongRole) {
		t.Errorf("enforcement reading party history = %v, want ErrWrongRole", err)
	}
	if _, err := f.service.History(agent, "SOMEONE"); err != nil {
		t.Errorf("service reading party history should be allowed, got %v", err)
	}
}

func TestServiceRoleCannotUseTheEnforcementProjection(t *testing.T) {
	f := newFixture(t)
	if _, err := f.enforcement.Snapshot(agent, "P1", d("2026-06-01")); !errors.Is(err, ErrWrongRole) {
		t.Errorf("service actor on enforcement projection = %v, want ErrWrongRole", err)
	}
}

func TestSnapshotReportsLicenceAndDeclarationState(t *testing.T) {
	f := newFixture(t)

	l, err := domain.Issue("L1", "HOLDER", "P1", domain.ClassColour, domain.ConcessionNone, d("2026-04-01"), 12, money.MustFromPounds("180.00"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.licences.Add(l); err != nil {
		t.Fatal(err)
	}

	snap, err := f.enforcement.Snapshot(officer, "P1", d("2026-06-01"))
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Licensed {
		t.Error("premises should read as licensed")
	}
	if snap.LicenceExpiry == nil || snap.LicenceExpiry.String() != "2027-04-01" {
		t.Errorf("expiry = %v, want 2027-04-01", snap.LicenceExpiry)
	}
	if !snap.VisitPermitted {
		t.Error("visits should be permitted with no withdrawal in force")
	}
}

func TestSnapshotHonoursWithdrawalOfAccess(t *testing.T) {
	f := newFixture(t)
	w, err := domain.NewDeclaration("W1", "P1", domain.DeclarationWOIRA, "OCCUPIER", d("2026-01-01"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.declarations.Record(w); err != nil {
		t.Fatal(err)
	}

	snap, err := f.enforcement.Snapshot(officer, "P1", d("2030-06-01"))
	if err != nil {
		t.Fatal(err)
	}
	if snap.VisitPermitted {
		t.Error("a withdrawal of access must show as visits not permitted, indefinitely")
	}
	if err := f.enforcement.VisitPermitted(officer, "P1", d("2030-06-01")); !errors.Is(err, domain.ErrVisitProhibited) {
		t.Errorf("visit check = %v, want ErrVisitProhibited", err)
	}
}

func TestDeclaredNoLicenceShowsAsDeclaredNotAsSuspicion(t *testing.T) {
	f := newFixture(t)
	n, err := domain.NewDeclaration("N1", "P1", domain.DeclarationNLN, "OCCUPIER", d("2026-01-01"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.declarations.Record(n); err != nil {
		t.Fatal(err)
	}

	snap, err := f.enforcement.Snapshot(officer, "P1", d("2029-01-01"))
	if err != nil {
		t.Fatal(err)
	}
	if !snap.NoLicenceDeclared {
		t.Error("an unlicensed premises with a standing declaration must read as declared")
	}
	if !snap.DueForReview {
		t.Error("it should be flagged as due for review")
	}
}

func TestEveryEnforcementReadIsAudited(t *testing.T) {
	f := newFixture(t)
	if _, err := f.enforcement.Snapshot(officer, "P1", d("2026-06-01")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.enforcement.Snapshot(agent, "P1", d("2026-06-01")); err == nil {
		t.Fatal("expected the service actor to be refused")
	}

	entries := f.log.ByRole(audit.RoleEnforcement)
	if len(entries) != 1 {
		t.Fatalf("expected one enforcement entry, got %d", len(entries))
	}
	if !entries[0].Allowed || entries[0].Subject != "P1" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}

	refused := f.log.For("P1")
	if len(refused) != 2 {
		t.Fatalf("expected both the allowed and the refused read to be recorded, got %d", len(refused))
	}
	if refused[1].Allowed {
		t.Error("the refused read should be recorded as not allowed")
	}
}

func TestEnforcementRequiresASalt(t *testing.T) {
	f := newFixture(t)
	if _, err := NewEnforcement(f.licences, f.declarations, f.occupancy, f.log, nil); !errors.Is(err, ErrNoSalt) {
		t.Errorf("building without a salt = %v, want ErrNoSalt", err)
	}
}

func TestTheVisitCheckIsAlsoEnforcementOnly(t *testing.T) {
	f := newFixture(t)
	if err := f.enforcement.VisitPermitted(agent, "P1", d("2026-06-01")); !errors.Is(err, ErrWrongRole) {
		t.Errorf("a service actor using the enforcement projection = %v, want ErrWrongRole", err)
	}
	refused := false
	for _, e := range f.log.Entries() {
		if e.Action == "enforcement.visit_check" && !e.Allowed && e.Detail == "wrong role" {
			refused = true
		}
	}
	if !refused {
		t.Error("the refusal should be written to the audit log")
	}
}

func TestHistoryFindsALicenceBoughtPartWayThroughAnOccupancy(t *testing.T) {
	f := newFixture(t)
	if err := f.occupancy.Record(domain.Occupancy{
		Party: "PARTY1", Premises: "P1", Period: timeline.From(d("2024-01-01")),
	}); err != nil {
		t.Fatal(err)
	}

	// Bought eighteen months after moving in, and renewed a year later. Both
	// are this person's, and a history that reports neither is no use for a
	// transfer or a refund.
	first, err := domain.Issue("L1", "PARTY1", "P1", domain.ClassColour, domain.ConcessionNone,
		d("2025-07-01"), 12, money.MustFromPounds("174.50"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.licences.Add(first); err != nil {
		t.Fatal(err)
	}
	second, err := domain.Issue("L2", "PARTY1", "P1", domain.ClassColour, domain.ConcessionNone,
		d("2026-07-01"), 12, money.MustFromPounds("180.00"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.licences.Add(second); err != nil {
		t.Fatal(err)
	}

	history, err := f.service.History(agent, "PARTY1")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Licences) != 2 {
		t.Fatalf("history = %v, want both L1 and L2", history.Licences)
	}
}

func TestHistoryIgnoresSomebodyElsesLicenceAtTheSameAddress(t *testing.T) {
	f := newFixture(t)
	if err := f.occupancy.Record(domain.Occupancy{
		Party: "PARTY1", Premises: "P1", Period: timeline.Until(d("2026-01-01"), d("2026-06-01")),
	}); err != nil {
		t.Fatal(err)
	}
	theirs, err := domain.Issue("L9", "PARTY2", "P1", domain.ClassColour, domain.ConcessionNone,
		d("2026-02-01"), 12, money.MustFromPounds("180.00"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.licences.Add(theirs); err != nil {
		t.Fatal(err)
	}

	history, err := f.service.History(agent, "PARTY1")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Licences) != 0 {
		t.Errorf("history = %v, want nothing: the licence belongs to somebody else", history.Licences)
	}
}
