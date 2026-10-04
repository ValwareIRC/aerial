// Package app wires the domain into a single in-memory service. Persistence is
// not implemented; see QUESTIONS.md.
package app

import (
	"fmt"
	"sync"
	"time"

	"github.com/aerial-project/aerial/internal/audit"
	"github.com/aerial-project/aerial/internal/auth"
	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/equality"
	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/payment"
	"github.com/aerial-project/aerial/internal/projection"
	"github.com/aerial-project/aerial/internal/timeline"
)

// Name, Version and the licence notice below are the single place this
// deployment describes itself. The licence in LICENCE.md requires that
// attribution notices are retained and that the terms are described
// accurately: it is a source-available licence, not an open source one, and it
// grants nothing to the BBC, to Capita, or to any other administrator of the
// television licence fee. Keeping the notice in the running software means a
// deployment cannot quietly shed it.
const (
	Name    = "Aerial"
	Version = "0.1.0"

	Copyright = "Copyright (c) 2026 the Aerial Contributors"

	LicenceName = "The Aerial Public Licence, Version 1.0"

	// LicenceNotice is a plain-English summary. It is not the licence; the
	// licence is LICENCE.md, and that text governs.
	LicenceNotice = "Source-available, not open source. Free for anyone to use except the BBC, " +
		"Capita, and any other administrator of a television licence fee scheme, who need the " +
		"commercial licence in section 8. See LICENCE.md."
)

// App holds the whole caseload in memory. Every register it owns is
// unsynchronised on its own, so all access goes through mu: exported methods
// take the lock, and the unexported ...Locked forms are what the rest of the
// package calls once it is already held. The HTTP layer never touches a
// register directly.
type App struct {
	mu sync.Mutex

	Schedule     *domain.Schedule
	Licences     *domain.LicenceRegister
	Declarations *domain.Register
	Occupancy    *domain.OccupancyRegister
	Parties      *domain.PartyRegister
	Premises     map[ids.PremisesID]domain.Premises
	Audit        *audit.Log
	Enforcement  *projection.Enforcement
	Service      *projection.Service
	Equality     *equality.Monitor
	Users        *auth.Store
	Ledger       *domain.Ledger
	Records      *domain.Records

	seq             int
	licenceSeq      int
	provisionalUPRN uint64
}

func New(salt []byte) (*App, error) {
	schedule, err := domain.SeedUKSchedule()
	if err != nil {
		return nil, err
	}
	log := audit.New(nil)
	licences := domain.NewLicenceRegister()
	declarations := domain.NewRegister()
	occupancy := domain.NewOccupancyRegister()

	enforcement, err := projection.NewEnforcement(licences, declarations, occupancy, log, salt)
	if err != nil {
		return nil, err
	}
	monitor, err := equality.New(equality.DefaultK)
	if err != nil {
		return nil, err
	}

	return &App{
		Schedule:     schedule,
		Licences:     licences,
		Declarations: declarations,
		Occupancy:    occupancy,
		Parties:      domain.NewPartyRegister(),
		Premises:     map[ids.PremisesID]domain.Premises{},
		Audit:        log,
		Enforcement:  enforcement,
		Service:      projection.NewService(licences, occupancy, log),
		Equality:     monitor,
		Users:        auth.NewStore(nil),
		Ledger:       domain.NewLedger(),
		Records:      domain.NewRecords(nil),
	}, nil
}

func (a *App) SeedUsers(password string) error {
	accounts := []struct {
		username, display string
		role              audit.Role
	}{
		{"a.reyes", "Ana Reyes", audit.RoleService},
		{"j.okafor", "Joseph Okafor", audit.RoleEnforcement},
		{"m.whitlock", "Margaret Whitlock", audit.RoleAdmin},
	}
	for _, acc := range accounts {
		if _, err := a.Users.AddUser(acc.username, acc.display, acc.role, password); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) nextID(prefix string) string {
	a.seq++
	return fmt.Sprintf("%s%d", prefix, a.seq)
}

// nextLicenceNumber formats a reference as "1234 5678 90".
func (a *App) nextLicenceNumber() string {
	a.licenceSeq++
	n := 8472391055 + int64(a.licenceSeq)*37
	s := fmt.Sprintf("%010d", n%10000000000)
	return s[0:4] + " " + s[4:8] + " " + s[8:10]
}

func (a *App) AddPremises(p domain.Premises) error {
	if err := p.Validate(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Premises[p.ID] = p
	return nil
}

// IssueLicence prices and issues in one step, so the fee on the licence is
// always the fee the schedule gives for that date.
func (a *App) IssueLicence(holder ids.PartyID, premises ids.PremisesID, class domain.LicenceClass, concession domain.Concession, start timeline.Date, months int) (*domain.Licence, domain.Quote, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.issueLicenceLocked(holder, premises, class, concession, start, months)
}

func (a *App) issueLicenceLocked(holder ids.PartyID, premises ids.PremisesID, class domain.LicenceClass, concession domain.Concession, start timeline.Date, months int) (*domain.Licence, domain.Quote, error) {
	if p, ok := a.Premises[premises]; ok && !p.SeparatelyLicensable() {
		return nil, domain.Quote{}, fmt.Errorf("%w: %s", domain.ErrNotLicensable, premises)
	}

	quote, err := a.Schedule.Quote(class, concession, start, time.Now().UTC())
	if err != nil {
		return nil, domain.Quote{}, err
	}

	l, err := domain.Issue(ids.LicenceID(a.nextID("L")), holder, premises, class, concession, start, months, quote.Payable)
	if err != nil {
		return nil, domain.Quote{}, err
	}
	l.Number = a.nextLicenceNumber()
	if err := a.Licences.Add(l); err != nil {
		return nil, domain.Quote{}, err
	}
	return l, quote, nil
}

func (a *App) Declare(premises ids.PremisesID, kind domain.DeclarationKind, by ids.PartyID, on timeline.Date) (domain.Declaration, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.Premises[premises]; !ok {
		return domain.Declaration{}, fmt.Errorf("no address with reference %s", premises)
	}
	d, err := domain.NewDeclaration(a.nextID("D"), premises, kind, by, on)
	if err != nil {
		return domain.Declaration{}, err
	}
	if err := a.Declarations.Record(d); err != nil {
		return domain.Declaration{}, err
	}
	return d, nil
}

func (a *App) Plan(kind payment.PlanKind, total money.Money, start timeline.Date, months int) (payment.Plan, error) {
	return payment.Build(kind, total, start, months)
}

// Quote prices a licence without issuing one, so the console can show the fee
// before anybody commits to it.
func (a *App) Quote(class domain.LicenceClass, concession domain.Concession, on timeline.Date) (domain.Quote, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Schedule.Quote(class, concession, on, time.Now().UTC())
}

// RateCoverage reports where the recorded fee data begins and ends.
func (a *App) RateCoverage() []domain.RateCoverage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Schedule.Coverage()
}

// Notes returns the case notes filed against a subject, newest first.
func (a *App) Notes(subject string) []domain.Note {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Records.Notes(subject)
}

// Visits returns the calls recorded at a premises, newest first.
func (a *App) Visits(premises ids.PremisesID) []domain.Visit {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Records.Visits(premises)
}

// LicenceExists reports whether a licence reference is known, without
// disclosing anything about it.
func (a *App) LicenceExists(id ids.LicenceID) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.Licences.Get(id)
	return ok
}
