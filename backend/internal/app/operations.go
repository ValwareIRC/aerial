package app

import (
	"fmt"

	"github.com/aerial-project/aerial/internal/audit"

	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/equality"
	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/payment"
	"github.com/aerial-project/aerial/internal/timeline"
)

// channelFor picks post or email from the person's preference.
func (a *App) channelFor(party ids.PartyID) domain.Channel {
	if p, ok := a.Parties.Get(party); ok && p.Paperless && p.Email != "" {
		return domain.ChannelEmail
	}
	return domain.ChannelPost
}

// writeTo sends a letter unless the person has died.
func (a *App) writeTo(party ids.PartyID, kind domain.LetterKind, on timeline.Date, about string) {
	p, ok := a.Parties.Get(party)
	if !ok || !p.WritableOn(on) {
		return
	}
	address := ""
	if p.Correspondence != nil {
		address = p.Correspondence.SingleLine()
	}
	a.Records.Send(party, kind, a.channelFor(party), on, about, address)
}

// IssueWithPayment issues a licence, opens its account, sets a payment plan and
// writes to the holder. Concessions require in-date evidence.
func (a *App) IssueWithPayment(holder ids.PartyID, premises ids.PremisesID, class domain.LicenceClass, concession domain.Concession, start timeline.Date, months int, method domain.PaymentMethod, planKind payment.PlanKind) (*domain.Licence, domain.Quote, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.issueWithPaymentLocked(holder, premises, class, concession, start, months, method, planKind)
}

func (a *App) issueWithPaymentLocked(holder ids.PartyID, premises ids.PremisesID, class domain.LicenceClass, concession domain.Concession, start timeline.Date, months int, method domain.PaymentMethod, planKind payment.PlanKind) (*domain.Licence, domain.Quote, error) {
	if _, ok := a.Parties.Get(holder); !ok {
		return nil, domain.Quote{}, fmt.Errorf("no person %s", holder)
	}
	if concession != domain.ConcessionNone && !a.Records.SupportsConcession(holder, concession, start) {
		return nil, domain.Quote{}, fmt.Errorf("%w: %s", domain.ErrConcessionNotEvidenced, ConcessionLabel(concession))
	}

	l, quote, err := a.issueLicenceLocked(holder, premises, class, concession, start, months)
	if err != nil {
		return nil, domain.Quote{}, err
	}

	a.Ledger.Open(l.ID, l.FeePaid, method)
	if planKind != "" && !l.FeePaid.IsZero() {
		plan, err := payment.Build(planKind, l.FeePaid, start, months)
		if err != nil {
			return nil, domain.Quote{}, err
		}
		if err := a.Ledger.SetPlan(l.ID, plan, method); err != nil {
			return nil, domain.Quote{}, err
		}
	}
	a.writeTo(holder, domain.LetterLicenceConfirmation, start, l.Number)
	return l, quote, nil
}

func (a *App) TakePayment(licence ids.LicenceID, amount money.Money, method domain.PaymentMethod, on timeline.Date, reference, takenBy string) (domain.Payment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Ledger.Record(licence, amount, method, on, reference, takenBy)
}

func (a *App) SetPaymentPlan(licence ids.LicenceID, kind payment.PlanKind, method domain.PaymentMethod, on timeline.Date) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	l, ok := a.Licences.Get(licence)
	if !ok {
		return fmt.Errorf("no licence %s", licence)
	}
	account, ok := a.Ledger.Get(licence)
	if !ok {
		account = a.Ledger.Open(licence, l.FeePaid, method)
	}
	outstanding := account.Balance()
	if outstanding.IsNegative() || outstanding.IsZero() {
		return fmt.Errorf("nothing is outstanding on this licence")
	}
	months := l.Term.Start.CompleteMonthsUntil(l.Expiry())
	plan, err := payment.Build(kind, outstanding, on, months)
	if err != nil {
		return err
	}
	if err := a.Ledger.SetPlan(licence, plan, method); err != nil {
		return err
	}
	a.writeTo(l.Holder, domain.LetterDetailsChanged, on, "payment plan")
	return nil
}

func (a *App) SetDirectDebit(licence ids.LicenceID, accountName, sortCode, accountNumber string, day int, on timeline.Date) error {
	mandate, err := domain.NewDirectDebit(accountName, sortCode, accountNumber, day, on)
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.Ledger.SetMandate(licence, mandate); err != nil {
		return err
	}
	if l, ok := a.Licences.Get(licence); ok {
		a.writeTo(l.Holder, domain.LetterDetailsChanged, on, "Direct Debit")
	}
	return nil
}

// ChangeAddress moves a person and takes their licence with them.
func (a *App) ChangeAddress(party ids.PartyID, to ids.PremisesID, on timeline.Date) (moved []ids.LicenceID, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	target, ok := a.Premises[to]
	if !ok {
		return nil, fmt.Errorf("no address with reference %s", to)
	}
	if !target.SeparatelyLicensable() {
		return nil, fmt.Errorf("%w: %s", domain.ErrNotLicensable, to)
	}
	if _, ok := a.Parties.Get(party); !ok {
		return nil, fmt.Errorf("no person %s", party)
	}

	current := a.Occupancy.PremisesOf(party, on)
	if err := a.Occupancy.End(party, on); err != nil {
		return nil, err
	}
	if err := a.Occupancy.Record(domain.Occupancy{Party: party, Premises: to, Period: timeline.From(on)}); err != nil {
		return nil, err
	}

	for _, from := range current {
		if from == to {
			continue
		}
		if l, ok := a.Licences.LicensedOn(from, on); ok && l.Holder == party {
			if err := a.Licences.Transfer(l.ID, to, on); err != nil {
				return nil, err
			}
			moved = append(moved, l.ID)
		}
	}
	a.writeTo(party, domain.LetterDetailsChanged, on, target.Address.SingleLine())
	return moved, nil
}

// TransferLicence moves one licence to another address without moving the
// person: the target is checked, the holder is written to, and the correction
// is audited. The HTTP layer calls this rather than the register, so a transfer
// cannot land on an address that does not exist or cannot be licensed.
func (a *App) TransferLicence(licence ids.LicenceID, to ids.PremisesID, on timeline.Date, by string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	l, ok := a.Licences.Get(licence)
	if !ok {
		return fmt.Errorf("no licence %s", licence)
	}
	target, ok := a.Premises[to]
	if !ok {
		return fmt.Errorf("no address with reference %s", to)
	}
	if !target.SeparatelyLicensable() {
		return fmt.Errorf("%w: %s", domain.ErrNotLicensable, to)
	}
	if err := a.Licences.Transfer(licence, to, on); err != nil {
		return err
	}
	a.writeTo(l.Holder, domain.LetterDetailsChanged, on, target.Address.SingleLine())
	a.Audit.Record(audit.Entry{
		Actor: audit.Actor{ID: by, Role: audit.RoleService}, Action: "licence.transferred",
		Subject: string(licence), Purpose: "customer service", Allowed: true,
		Detail: "to " + string(to),
	})
	return nil
}

func (a *App) ChangeName(party ids.PartyID, title, forename, surname string, reason domain.NameChangeReason, on timeline.Date, by string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.Parties.Rename(party, title, forename, surname, reason, on, by); err != nil {
		return err
	}
	a.writeTo(party, domain.LetterDetailsChanged, on, "name")
	return nil
}

func (a *App) UpdateContact(party ids.PartyID, email, phone string, paperless bool, correspondence *domain.Address) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Parties.UpdateContact(party, email, phone, paperless, correspondence)
}

// RecordDeath marks a person deceased, stops letters, and cancels any licence
// they hold with a refund.
func (a *App) RecordDeath(party ids.PartyID, on timeline.Date) (money.Money, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.Parties.RecordDeath(party, on); err != nil {
		return money.Zero, err
	}
	refund := money.Zero
	for _, l := range a.Licences.All() {
		if l.Holder != party || l.StatusOn(on) != domain.LicenceActive {
			continue
		}
		amount, err := l.Cancel(on, domain.CancelDeceased)
		if err != nil {
			continue
		}
		refund = refund.Add(amount)
		if !amount.IsZero() {
			_, _ = a.Ledger.Record(l.ID, amount.Neg(), domain.MethodRefund, on, "bereavement", "system")
		}
	}
	return refund, nil
}

func (a *App) AddEvidence(party ids.PartyID, kind domain.EvidenceKind, reference string, seenOn timeline.Date, expires *timeline.Date, seenBy string) (domain.Evidence, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.Parties.Get(party); !ok {
		return domain.Evidence{}, fmt.Errorf("no person %s", party)
	}
	return a.Records.AddEvidence(domain.Evidence{
		Party: party, Kind: kind, Reference: reference,
		SeenOn: seenOn, ExpiresOn: expires, SeenBy: seenBy,
	})
}

// RecordVisit stores an officer's visit outcome and feeds equality monitoring.
func (a *App) RecordVisit(premises ids.PremisesID, on timeline.Date, officer string, outcome domain.VisitOutcome, note string) (domain.Visit, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.Premises[premises]; !ok {
		return domain.Visit{}, fmt.Errorf("no address with reference %s", premises)
	}
	if err := a.Declarations.VisitPermitted(premises, on); err != nil {
		return domain.Visit{}, err
	}
	v, err := a.Records.RecordVisit(domain.Visit{
		Premises: premises, On: on, Officer: officer, Outcome: outcome, Note: note,
	})
	if err != nil {
		return domain.Visit{}, err
	}

	// Equality monitoring is fed from the first occupant with a date of birth
	// on file. The observation carries no identifier, only the band.
	attrs := map[string]string{}
	for _, party := range a.Occupancy.OccupantsOn(premises, on) {
		p, ok := a.Parties.Get(party)
		if !ok {
			continue
		}
		if age := p.AgeOn(on); age >= 0 {
			attrs["ageBand"] = ageBand(age)
			break
		}
	}
	_ = a.Equality.Observe(equality.Observation{Outcome: string(outcome), Attributes: attrs})
	return v, nil
}

func ageBand(age int) string {
	switch {
	case age < 25:
		return "Under 25"
	case age < 45:
		return "25 to 44"
	case age < 65:
		return "45 to 64"
	case age < 75:
		return "65 to 74"
	default:
		return "75 and over"
	}
}

// Renew issues a fresh licence starting the day the current one expires.
func (a *App) Renew(licence ids.LicenceID, method domain.PaymentMethod, planKind payment.PlanKind) (*domain.Licence, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	l, ok := a.Licences.Get(licence)
	if !ok {
		return nil, fmt.Errorf("no licence %s", licence)
	}
	if l.CancelledOn != nil {
		return nil, fmt.Errorf("%w: a cancelled licence cannot be renewed", domain.ErrLicenceState)
	}
	coverage := l.EffectiveCoverage()
	if len(coverage) == 0 {
		return nil, fmt.Errorf("licence %s covers no address", licence)
	}
	premises := coverage[len(coverage)-1].Premises

	renewed, _, err := a.issueWithPaymentLocked(l.Holder, premises, l.Class, l.Concession, l.Expiry(), domain.DefaultTermMonths, method, planKind)
	if err != nil {
		return nil, err
	}
	return renewed, nil
}

func (a *App) CancelLicence(licence ids.LicenceID, reason domain.CancellationReason, on timeline.Date) (money.Money, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	l, ok := a.Licences.Get(licence)
	if !ok {
		return money.Zero, fmt.Errorf("no licence %s", licence)
	}
	refund, err := l.Cancel(on, reason)
	if err != nil {
		return money.Zero, err
	}
	if !refund.IsZero() {
		_, _ = a.Ledger.Record(licence, refund.Neg(), domain.MethodRefund, on, reason.Label(), "system")
		a.writeTo(l.Holder, domain.LetterRefundConfirmation, on, l.Number)
	}
	return refund, nil
}

func (a *App) AddNote(subject, text, by string) (domain.Note, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Records.AddNote(subject, text, by)
}

// SendRenewalReminders writes to everyone whose licence expires within a month
// and returns how many were sent.
func (a *App) SendRenewalReminders(on timeline.Date) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := on.AddMonths(1)
	sent := 0
	for _, l := range a.Licences.All() {
		if l.StatusOn(on) != domain.LicenceActive {
			continue
		}
		if l.Expiry().After(cutoff) {
			continue
		}
		a.writeTo(l.Holder, domain.LetterRenewalReminder, on, l.Number)
		sent++
	}
	return sent
}

// ReinstateLicence undoes a cancellation recorded in error: the licence is
// restored, any refund is reversed, and a correction is written to the record.
func (a *App) ReinstateLicence(licence ids.LicenceID, by string, on timeline.Date) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.reinstateLicenceLocked(licence, by, on)
}

func (a *App) reinstateLicenceLocked(licence ids.LicenceID, by string, on timeline.Date) error {
	l, ok := a.Licences.Get(licence)
	if !ok {
		return fmt.Errorf("no licence %s", licence)
	}
	if err := a.Licences.Reinstate(licence); err != nil {
		return err
	}

	if account, ok := a.Ledger.Get(licence); ok {
		reversed := money.Zero
		for _, p := range account.Payments {
			if p.Method == domain.MethodRefund {
				reversed = reversed.Add(p.Amount)
			}
		}
		if !reversed.IsZero() {
			_, _ = a.Ledger.Record(licence, reversed.Neg(), domain.MethodRefund, on, "cancellation reversed", by)
		}
	}

	_, _ = a.Records.AddNote(string(l.Holder),
		fmt.Sprintf("Cancellation of licence %s reversed. Recorded in error.", l.Number), by)
	a.Audit.Record(audit.Entry{
		Actor: audit.Actor{ID: by, Role: audit.RoleService}, Action: "licence.reinstated",
		Subject: string(licence), Purpose: "correction", Allowed: true,
	})
	return nil
}

// ReverseDeath undoes a death recorded in error and restores anything cancelled
// because of it.
func (a *App) ReverseDeath(party ids.PartyID, by string, on timeline.Date) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	p, ok := a.Parties.Get(party)
	if !ok {
		return fmt.Errorf("no person %s", party)
	}
	if p.Deceased == nil {
		return fmt.Errorf("%s is not recorded as having died", p.Name())
	}
	diedOn := *p.Deceased

	if err := a.Parties.ClearDeath(party); err != nil {
		return err
	}
	for _, l := range a.Licences.All() {
		if l.Holder != party || l.CancelReason != domain.CancelDeceased {
			continue
		}
		if l.CancelledOn == nil || !l.CancelledOn.Equal(diedOn) {
			continue
		}
		if err := a.reinstateLicenceLocked(l.ID, by, on); err != nil {
			return err
		}
	}

	_, _ = a.Records.AddNote(string(party), "Death record reversed. Recorded in error.", by)
	a.Audit.Record(audit.Entry{
		Actor: audit.Actor{ID: by, Role: audit.RoleService}, Action: "person.death_reversed",
		Subject: string(party), Purpose: "correction", Allowed: true,
	})
	return nil
}
