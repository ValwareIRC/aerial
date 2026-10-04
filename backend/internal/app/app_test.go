package app

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/aerial-project/aerial/internal/audit"
	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/payment"
	"github.com/aerial-project/aerial/internal/timeline"
)

func seeded(t *testing.T) *App {
	t.Helper()
	a, err := New([]byte("test-salt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Seed(); err != nil {
		t.Fatal(err)
	}
	return a
}

func on(s string) timeline.Date { return timeline.MustParseDate(s) }

var serviceActor = audit.Actor{ID: "a.reyes", Role: audit.RoleService}

func TestRefundsArePricedForEveryReason(t *testing.T) {
	a := seeded(t)

	// Nadia Kaur's licence: £180.00 from 1 April 2026 to 1 April 2027.
	detail, err := a.LicenceDetail(licenceHeldBy(t, a, "PARTY1").ID, on("2026-10-01"))
	if err != nil {
		t.Fatal(err)
	}

	if detail.Refunds[string(domain.CancelIssuedInError)] != detail.Summary.FeePaid {
		t.Errorf("a licence issued in error should refund the whole fee, got %d of %d",
			detail.Refunds[string(domain.CancelIssuedInError)], detail.Summary.FeePaid)
	}
	// Six complete months left of twelve, on £180.00.
	if got := detail.Refunds[string(domain.CancelNoLongerWatch)]; got != 9000 {
		t.Errorf("ordinary refund = %d, want 9000", got)
	}
	if detail.Refund != detail.Refunds[string(domain.CancelNoLongerWatch)] {
		t.Error("the headline refund should be the ordinary reason's figure")
	}
	for _, reason := range domain.CancellationReasons() {
		if _, ok := detail.Refunds[string(reason)]; !ok {
			t.Errorf("no refund figure offered for %s", reason)
		}
	}
}

func TestRefundQuotedBeforehandIsTheRefundPaid(t *testing.T) {
	a := seeded(t)
	licence := licenceHeldBy(t, a, "PARTY1").ID

	detail, err := a.LicenceDetail(licence, on("2026-10-01"))
	if err != nil {
		t.Fatal(err)
	}
	quoted := detail.Refunds[string(domain.CancelMovedAbroad)]

	paid, err := a.CancelLicence(licence, domain.CancelMovedAbroad, on("2026-10-01"))
	if err != nil {
		t.Fatal(err)
	}
	if paid.Pence() != quoted {
		t.Errorf("quoted %d before cancelling but paid %d", quoted, paid.Pence())
	}
}

func TestInstalmentDueTodayIsDueNotMissed(t *testing.T) {
	a := seeded(t)
	licence := licenceHeldBy(t, a, "PARTY1").ID

	// Three monthly collections of £15.00 are seeded, from 1 April 2026.
	account, err := a.Account(licence, on("2026-07-01"))
	if err != nil {
		t.Fatal(err)
	}

	states := map[int]domain.InstalmentState{}
	for _, i := range account.Instalments {
		states[i.Seq] = i.State
	}
	if states[1] != domain.InstalmentPaid || states[3] != domain.InstalmentPaid {
		t.Errorf("the three collections taken should read as paid, got %v", states)
	}
	if states[4] != domain.InstalmentDue {
		t.Errorf("the instalment due today should read as due, got %s", states[4])
	}
	if states[5] != domain.InstalmentFuture {
		t.Errorf("next month's instalment should read as not due yet, got %s", states[5])
	}
}

func TestLaterInstalmentIsNotPaidWhileAnEarlierOneIsOutstanding(t *testing.T) {
	a := seeded(t)

	// A weekly plan puts the odd pence on the earliest instalments, so a
	// later one is a penny cheaper than the first. Paying nothing at all must
	// not mark the cheaper one paid.
	licence := licenceHeldBy(t, a, "PARTY6").ID
	account, err := a.Account(licence, on("2027-01-01"))
	if err != nil {
		t.Fatal(err)
	}
	if len(account.Instalments) != 52 {
		t.Fatalf("weekly plan has %d instalments, want 52", len(account.Instalments))
	}

	paidCount := 0
	for _, i := range account.Instalments {
		if i.State == domain.InstalmentPaid {
			paidCount++
		}
	}
	// Two payments of £1.74 are seeded, and the first two instalments are
	// £1.74. Nothing beyond those may read as paid.
	if paidCount != 2 {
		t.Errorf("%d instalments read as paid, want 2", paidCount)
	}
	if account.Instalments[2].State == domain.InstalmentPaid {
		t.Error("an unfunded instalment must not read as paid because it is cheaper")
	}
}

func TestTransferChecksTheTargetAddress(t *testing.T) {
	a := seeded(t)
	licence := licenceHeldBy(t, a, "PARTY1").ID

	if err := a.TransferLicence(licence, "P999", on("2026-10-01"), "a.reyes"); err == nil {
		t.Error("a transfer to an address that does not exist should be refused")
	}
	// P4 is a hotel room, covered by the hotel's own licence.
	if err := a.TransferLicence(licence, "P4", on("2026-10-01"), "a.reyes"); !errors.Is(err, domain.ErrNotLicensable) {
		t.Errorf("transfer to a non-licensable address = %v, want ErrNotLicensable", err)
	}

	if err := a.TransferLicence(licence, "P2", on("2026-10-01"), "a.reyes"); err != nil {
		t.Fatalf("transfer to a real address: %v", err)
	}
	summary, err := a.LicenceSummaryOf(licence, on("2026-10-02"))
	if err != nil {
		t.Fatal(err)
	}
	if summary.Premises != "P2" {
		t.Errorf("licence now covers %s, want P2", summary.Premises)
	}

	audited := false
	for _, e := range a.Audit.Entries() {
		if e.Action == "licence.transferred" && e.Subject == string(licence) {
			audited = true
		}
	}
	if !audited {
		t.Error("a transfer should be written to the access log")
	}
}

func TestADeclarationNeedsAnAddressThatExists(t *testing.T) {
	a := seeded(t)
	if _, err := a.Declare("P999", domain.DeclarationNLN, "PARTY1", on("2026-10-01")); err == nil {
		t.Error("a declaration against an unknown address should be refused")
	}
}

func TestAVisitNeedsAnAddressThatExists(t *testing.T) {
	a := seeded(t)
	if _, err := a.RecordVisit("P999", on("2026-10-01"), "J Okafor", domain.OutcomeNoAnswer, ""); err == nil {
		t.Error("a visit to an unknown address should be refused")
	}
}

func TestALicenceCannotBeIssuedToSomebodyWhoIsNotOnRecord(t *testing.T) {
	a := seeded(t)
	_, _, err := a.IssueWithPayment("PARTY999", "P2", domain.ClassColour, domain.ConcessionNone,
		on("2026-10-01"), domain.DefaultTermMonths, domain.MethodCard, payment.PlanAnnual)
	if err == nil {
		t.Error("issuing to an unknown person should be refused")
	}
}

func TestACancelledLicenceCannotBeRenewed(t *testing.T) {
	a := seeded(t)
	licence := licenceHeldBy(t, a, "PARTY1").ID
	if _, err := a.CancelLicence(licence, domain.CancelMovedAbroad, on("2026-10-01")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Renew(licence, domain.MethodCard, payment.PlanAnnual); !errors.Is(err, domain.ErrLicenceState) {
		t.Errorf("renewing a cancelled licence = %v, want ErrLicenceState", err)
	}
}

func TestAddressesSortTheWayPeopleReadThem(t *testing.T) {
	a := seeded(t)
	for _, line1 := range []string{"2 Rowan Close", "10 Rowan Close", "1 Rowan Close"} {
		if _, _, err := a.CreateAddress(domain.Address{Line1: line1, Town: "Dover", Postcode: "CT16 2QP"},
			domain.KindDwelling, "", 1, true, on("2026-10-01")); err != nil {
			t.Fatal(err)
		}
	}

	var order []string
	for _, p := range a.Search("Rowan Close", on("2026-10-01")) {
		order = append(order, p.Address.Line1)
	}
	want := []string{"1 Rowan Close", "2 Rowan Close", "10 Rowan Close", "12 Rowan Close"}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestOverviewCarriesTheListsBehindItsCounts(t *testing.T) {
	a := seeded(t)
	o := a.Overview(on("2026-10-01"))

	if o.Unlicensed != len(o.NoLicenceList) {
		t.Errorf("unlicensed count %d but list of %d", o.Unlicensed, len(o.NoLicenceList))
	}
	if o.AccessWithdrawn != len(o.DoNotVisitList) {
		t.Errorf("withdrawn count %d but list of %d", o.AccessWithdrawn, len(o.DoNotVisitList))
	}
	if o.DueForReview != len(o.ReviewList) {
		t.Errorf("review count %d but list of %d", o.DueForReview, len(o.ReviewList))
	}
	for _, p := range o.NoLicenceList {
		if p.Licensed || p.Declared || p.CoveredByParent {
			t.Errorf("%s does not belong on the no-licence list", p.ID)
		}
	}
}

func TestEveryLicenceHeldAtAnAddressAppearsInTheHistory(t *testing.T) {
	a := seeded(t)
	first := licenceHeldBy(t, a, "PARTY1").ID

	// A renewal starts the day the current licence expires, well after the
	// occupancy began. Both are this person's.
	renewed, err := a.Renew(first, domain.MethodCard, payment.PlanAnnual)
	if err != nil {
		t.Fatal(err)
	}

	history, err := a.Service.History(serviceActor, "PARTY1")
	if err != nil {
		t.Fatal(err)
	}
	found := map[ids.LicenceID]bool{}
	for _, id := range history.Licences {
		found[id] = true
	}
	if !found[first] || !found[renewed.ID] {
		t.Errorf("history has %v, want both %s and %s", history.Licences, first, renewed.ID)
	}
}

// TestConcurrentUseIsSafe is the reason the app takes a lock on every
// operation rather than only on reads. Run with -race.
func TestConcurrentUseIsSafe(t *testing.T) {
	a := seeded(t)
	licence := licenceHeldBy(t, a, "PARTY1").ID
	day := on("2026-10-01")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			a.Search("", day)
			a.Overview(day)
			a.Worklist("", day)
			a.ListLicences(day)
			_, _ = a.PersonDetail("PARTY1", day)
			_, _ = a.Account(licence, day)
			_, _ = a.SubjectAccess("PARTY1", day)
			_, _ = a.TakePayment(licence, money.FromPence(100), domain.MethodCard, day, "concurrent", "test")
			_, _ = a.AddNote("PARTY1", "concurrent note", "test")
			_, _, _ = a.CreateAddress(domain.Address{
				Line1:    "Flat " + string(rune('A'+n)) + ", 1 Concurrency Court",
				Town:     "Dover",
				Postcode: "CT16 2QP",
			}, domain.KindDwelling, "", 1, true, day)
			a.SendRenewalReminders(day)
		}(i)
	}
	wg.Wait()

	if got := len(a.Notes("PARTY1")); got < 8 {
		t.Errorf("only %d notes survived concurrent writes", got)
	}
}

func licenceHeldBy(t *testing.T, a *App, party ids.PartyID) LicenceSummary {
	t.Helper()
	for _, l := range a.ListLicences(on("2026-10-01")) {
		if l.Holder == party {
			return l
		}
	}
	t.Fatalf("no licence held by %s in the seeded data", party)
	return LicenceSummary{}
}
