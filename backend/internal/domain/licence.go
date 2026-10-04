package domain

import (
	"fmt"

	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/timeline"
)

type LicenceStatus string

const (
	LicencePending   LicenceStatus = "PENDING"
	LicenceActive    LicenceStatus = "ACTIVE"
	LicenceExpired   LicenceStatus = "EXPIRED"
	LicenceCancelled LicenceStatus = "CANCELLED"
)

// DefaultTermMonths is the standard licence term.
const DefaultTermMonths = 12

// Coverage is a premises the licence covered for a period. A licence gains a
// segment when it is transferred to a new address.
type Coverage struct {
	Premises ids.PremisesID `json:"premises"`
	Period   timeline.Range `json:"period"`
}

type Licence struct {
	ID ids.LicenceID `json:"id"`
	// Number is the reference quoted to the licence holder.
	Number       string             `json:"number"`
	Holder       ids.PartyID        `json:"holder"`
	Class        LicenceClass       `json:"class"`
	Concession   Concession         `json:"concession"`
	Term         timeline.Range     `json:"term"`
	FeePaid      money.Money        `json:"feePaid"`
	IssuedOn     timeline.Date      `json:"issuedOn"`
	CancelledOn  *timeline.Date     `json:"cancelledOn,omitempty"`
	CancelReason CancellationReason `json:"cancelReason,omitempty"`
	Coverage     []Coverage         `json:"coverage"`
}

// Issue creates a licence running months from start, covering one premises.
func Issue(id ids.LicenceID, holder ids.PartyID, premises ids.PremisesID, class LicenceClass, concession Concession, start timeline.Date, months int, fee money.Money) (*Licence, error) {
	if !class.Valid() {
		return nil, fmt.Errorf("domain: unknown licence class %q", class)
	}
	if !concession.Valid() {
		return nil, fmt.Errorf("domain: unknown concession %q", concession)
	}
	if months < 1 {
		return nil, fmt.Errorf("domain: licence term must be at least one month")
	}
	if fee.IsNegative() {
		return nil, fmt.Errorf("domain: licence fee cannot be negative")
	}
	end := start.AddMonths(months)
	term, err := timeline.NewRange(start, &end)
	if err != nil {
		return nil, err
	}
	return &Licence{
		ID: id, Holder: holder, Class: class, Concession: concession,
		Term: term, FeePaid: fee, IssuedOn: start,
		Coverage: []Coverage{{Premises: premises, Period: term}},
	}, nil
}

func (l *Licence) Expiry() timeline.Date { return *l.Term.End }

func (l *Licence) TermMonths() int {
	return l.Term.Start.CompleteMonthsUntil(l.Expiry())
}

func (l *Licence) StatusOn(on timeline.Date) LicenceStatus {
	if l.CancelledOn != nil && on.OnOrAfter(*l.CancelledOn) {
		return LicenceCancelled
	}
	if on.Before(l.Term.Start) {
		return LicencePending
	}
	if l.Term.Contains(on) {
		return LicenceActive
	}
	return LicenceExpired
}

// EffectiveCoverage truncates coverage at the cancellation date.
func (l *Licence) EffectiveCoverage() []Coverage {
	out := make([]Coverage, 0, len(l.Coverage))
	for _, c := range l.Coverage {
		if l.CancelledOn == nil {
			out = append(out, c)
			continue
		}
		cancel := *l.CancelledOn
		if c.Period.Start.OnOrAfter(cancel) {
			continue
		}
		end := cancel
		if c.Period.End != nil && c.Period.End.Before(cancel) {
			end = *c.Period.End
		}
		r, err := timeline.NewRange(c.Period.Start, &end)
		if err != nil {
			continue
		}
		out = append(out, Coverage{Premises: c.Premises, Period: r})
	}
	return out
}

// CoversPremisesOn reports whether the licence covers that premises that day.
func (l *Licence) CoversPremisesOn(premises ids.PremisesID, on timeline.Date) bool {
	if l.StatusOn(on) != LicenceActive {
		return false
	}
	for _, c := range l.EffectiveCoverage() {
		if c.Premises == premises && c.Period.Contains(on) {
			return true
		}
	}
	return false
}

// TransferTo moves the licence to a new premises from `on`, keeping the same
// expiry. The licence follows the holder; it is not surrendered and reissued.
func (l *Licence) TransferTo(premises ids.PremisesID, on timeline.Date) error {
	if l.StatusOn(on) != LicenceActive {
		return fmt.Errorf("%w: cannot transfer a licence that is %s on %s", ErrLicenceState, l.StatusOn(on), on)
	}
	last := &l.Coverage[len(l.Coverage)-1]
	if last.Premises == premises {
		return fmt.Errorf("%w: licence already covers %s", ErrLicenceState, premises)
	}
	if !on.After(last.Period.Start) {
		return fmt.Errorf("%w: transfer date %s is not after the current coverage start", ErrLicenceState, on)
	}
	closed, err := timeline.NewRange(last.Period.Start, &on)
	if err != nil {
		return err
	}
	last.Period = closed

	expiry := l.Expiry()
	opened, err := timeline.NewRange(on, &expiry)
	if err != nil {
		return err
	}
	l.Coverage = append(l.Coverage, Coverage{Premises: premises, Period: opened})
	return nil
}

// RefundDue is the refund for surrendering on that date: complete months
// remaining, pro rata on the fee paid. A part month is not refunded, unless the
// reason returns the whole fee.
func (l *Licence) RefundDue(on timeline.Date, reason CancellationReason) money.Money {
	if l.FeePaid.IsZero() {
		return money.Zero
	}
	if reason.RefundsWholeFee() {
		return l.FeePaid
	}
	remaining := on.CompleteMonthsUntil(l.Expiry())
	if remaining <= 0 {
		return money.Zero
	}
	total := l.TermMonths()
	if total <= 0 {
		return money.Zero
	}
	if remaining > total {
		remaining = total
	}
	return l.FeePaid.Proportion(int64(remaining), int64(total))
}

// RefundsByReason prices a surrender on that date against every cancellation
// reason. The figure quoted to someone before they decide is then the same
// figure the decision produces, because both come from RefundDue.
func (l *Licence) RefundsByReason(on timeline.Date) map[CancellationReason]money.Money {
	out := make(map[CancellationReason]money.Money, len(CancellationReasons()))
	for _, reason := range CancellationReasons() {
		out[reason] = l.RefundDue(on, reason)
	}
	return out
}

// Cancel surrenders the licence and returns the refund due.
func (l *Licence) Cancel(on timeline.Date, reason CancellationReason) (money.Money, error) {
	if !reason.Valid() {
		return money.Zero, fmt.Errorf("%w: unknown cancellation reason %q", ErrLicenceState, reason)
	}
	switch l.StatusOn(on) {
	case LicenceCancelled:
		return money.Zero, fmt.Errorf("%w: already cancelled", ErrLicenceState)
	case LicenceExpired:
		return money.Zero, fmt.Errorf("%w: already expired", ErrLicenceState)
	}
	refund := l.RefundDue(on, reason)
	l.CancelledOn = &on
	l.CancelReason = reason
	return refund, nil
}

// LicenceRegister indexes licences and prevents double cover.
type LicenceRegister struct {
	byID       map[ids.LicenceID]*Licence
	byPremises map[ids.PremisesID][]*Licence
}

func NewLicenceRegister() *LicenceRegister {
	return &LicenceRegister{
		byID:       map[ids.LicenceID]*Licence{},
		byPremises: map[ids.PremisesID][]*Licence{},
	}
}

func (r *LicenceRegister) Add(l *Licence) error {
	if _, exists := r.byID[l.ID]; exists {
		return fmt.Errorf("domain: licence %s already registered", l.ID)
	}
	for _, c := range l.EffectiveCoverage() {
		for _, existing := range r.byPremises[c.Premises] {
			for _, ec := range existing.EffectiveCoverage() {
				if ec.Premises == c.Premises && ec.Period.Overlaps(c.Period) {
					return fmt.Errorf("%w: %s overlaps %s at %s", ErrOverlappingLicence, l.ID, existing.ID, c.Premises)
				}
			}
		}
	}
	r.byID[l.ID] = l
	for _, c := range l.Coverage {
		r.byPremises[c.Premises] = append(r.byPremises[c.Premises], l)
	}
	return nil
}

// Register re-indexes a licence after a transfer added a new coverage segment.
func (r *LicenceRegister) Register(l *Licence, premises ids.PremisesID) {
	for _, existing := range r.byPremises[premises] {
		if existing.ID == l.ID {
			return
		}
	}
	r.byPremises[premises] = append(r.byPremises[premises], l)
}

// Transfer moves a licence to a new premises, checking for double cover there.
func (r *LicenceRegister) Transfer(id ids.LicenceID, premises ids.PremisesID, on timeline.Date) error {
	l, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("domain: no licence %s", id)
	}
	expiry := l.Expiry()
	proposed, err := timeline.NewRange(on, &expiry)
	if err != nil {
		return err
	}
	for _, existing := range r.byPremises[premises] {
		if existing.ID == id {
			continue
		}
		for _, ec := range existing.EffectiveCoverage() {
			if ec.Premises == premises && ec.Period.Overlaps(proposed) {
				return fmt.Errorf("%w: %s overlaps %s at %s", ErrOverlappingLicence, id, existing.ID, premises)
			}
		}
	}
	if err := l.TransferTo(premises, on); err != nil {
		return err
	}
	r.Register(l, premises)
	return nil
}

func (r *LicenceRegister) Get(id ids.LicenceID) (*Licence, bool) {
	l, ok := r.byID[id]
	return l, ok
}

// All returns every registered licence.
func (r *LicenceRegister) All() []*Licence {
	out := make([]*Licence, 0, len(r.byID))
	for _, l := range r.byID {
		out = append(out, l)
	}
	return out
}

// AtPremises returns every licence that has ever covered a premises.
func (r *LicenceRegister) AtPremises(premises ids.PremisesID) []*Licence {
	out := make([]*Licence, len(r.byPremises[premises]))
	copy(out, r.byPremises[premises])
	return out
}

// LicensedOn returns the licence covering that premises that day, if any.
func (r *LicenceRegister) LicensedOn(premises ids.PremisesID, on timeline.Date) (*Licence, bool) {
	for _, l := range r.byPremises[premises] {
		if l.CoversPremisesOn(premises, on) {
			return l, true
		}
	}
	return nil, false
}

// Reinstate undoes a cancellation recorded in error, refusing if the period is
// now covered by another licence.
func (r *LicenceRegister) Reinstate(id ids.LicenceID) error {
	l, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("domain: no licence %s", id)
	}
	if l.CancelledOn == nil {
		return fmt.Errorf("%w: licence %s is not cancelled", ErrLicenceState, id)
	}

	was, wasReason := l.CancelledOn, l.CancelReason
	l.CancelledOn, l.CancelReason = nil, ""

	for _, c := range l.EffectiveCoverage() {
		for _, existing := range r.byPremises[c.Premises] {
			if existing.ID == id {
				continue
			}
			for _, ec := range existing.EffectiveCoverage() {
				if ec.Premises == c.Premises && ec.Period.Overlaps(c.Period) {
					l.CancelledOn, l.CancelReason = was, wasReason
					return fmt.Errorf("%w: %s now overlaps %s at %s", ErrOverlappingLicence, id, existing.ID, c.Premises)
				}
			}
		}
	}
	return nil
}
