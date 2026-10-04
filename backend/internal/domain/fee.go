package domain

import (
	"fmt"
	"time"

	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/timeline"
)

// LicenceClass determines the base rate. Hotels, mobile units and company
// licences are coverage rules on the premises, not classes.
type LicenceClass string

const (
	ClassColour     LicenceClass = "COLOUR"
	ClassMonochrome LicenceClass = "MONOCHROME"
	ClassARC        LicenceClass = "ARC" // Accommodation for Residential Care
)

func (c LicenceClass) Valid() bool {
	switch c {
	case ClassColour, ClassMonochrome, ClassARC:
		return true
	}
	return false
}

type Concession string

const (
	ConcessionNone Concession = "NONE"
	// ConcessionBlind50 attaches to the premises, evidenced by a party.
	ConcessionBlind50 Concession = "BLIND_50"
	// ConcessionOver75PensionCredit attaches to the party, so it survives a
	// change of address.
	ConcessionOver75PensionCredit Concession = "OVER_75_PENSION_CREDIT"
)

func (c Concession) Valid() bool {
	switch c {
	case ConcessionNone, ConcessionBlind50, ConcessionOver75PensionCredit:
		return true
	}
	return false
}

// concessionRule is the proportion payable: 1/1 full, 1/2 half, 0/1 free.
type concessionRule struct {
	num, den int64
}

type policyKey struct {
	class      LicenceClass
	concession Concession
}

// Quote carries the working as well as the result.
type Quote struct {
	Class      LicenceClass  `json:"class"`
	Concession Concession    `json:"concession"`
	On         timeline.Date `json:"on"`
	Base       money.Money   `json:"base"`
	Payable    money.Money   `json:"payable"`
}

func (q Quote) Discount() money.Money { return q.Base.Sub(q.Payable) }

// Schedule resolves class, concession and date to an amount payable. Rates are
// bitemporal, so a refund uses the rate in force then as it was recorded then.
type Schedule struct {
	rates  map[LicenceClass]*timeline.Store[money.Money]
	policy map[policyKey]concessionRule
}

func NewSchedule() *Schedule {
	return &Schedule{
		rates: map[LicenceClass]*timeline.Store[money.Money]{},
		policy: map[policyKey]concessionRule{
			{ClassColour, ConcessionNone}:                    {1, 1},
			{ClassMonochrome, ConcessionNone}:                {1, 1},
			{ClassARC, ConcessionNone}:                       {1, 1},
			{ClassColour, ConcessionBlind50}:                 {1, 2},
			{ClassMonochrome, ConcessionBlind50}:             {1, 2},
			{ClassColour, ConcessionOver75PensionCredit}:     {0, 1},
			{ClassMonochrome, ConcessionOver75PensionCredit}: {0, 1},

			// ARC with a concession is absent on purpose: whether concessions
			// stack on the reduced ARC rate is unconfirmed. See QUESTIONS.md.
		},
	}
}

func (s *Schedule) SetRate(class LicenceClass, valid timeline.Range, recordedAt time.Time, amount money.Money) error {
	if !class.Valid() {
		return fmt.Errorf("domain: unknown licence class %q", class)
	}
	if amount.IsNegative() {
		return fmt.Errorf("domain: rate for %s cannot be negative", class)
	}
	store, ok := s.rates[class]
	if !ok {
		store = timeline.NewStore[money.Money]()
		s.rates[class] = store
	}
	store.Append(valid, recordedAt, amount)
	return nil
}

func (s *Schedule) BaseRate(class LicenceClass, on timeline.Date, knownAt time.Time) (money.Money, error) {
	store, ok := s.rates[class]
	if !ok {
		return money.Zero, fmt.Errorf("%w: class %s", ErrRateNotKnown, class)
	}
	amount, ok := store.AsAt(on, knownAt)
	if !ok {
		return money.Zero, fmt.Errorf("%w: class %s on %s", ErrRateNotKnown, class, on)
	}
	return amount, nil
}

func (s *Schedule) Quote(class LicenceClass, concession Concession, on timeline.Date, knownAt time.Time) (Quote, error) {
	if !concession.Valid() {
		return Quote{}, fmt.Errorf("domain: unknown concession %q", concession)
	}
	base, err := s.BaseRate(class, on, knownAt)
	if err != nil {
		return Quote{}, err
	}
	rule, ok := s.policy[policyKey{class, concession}]
	if !ok {
		return Quote{}, fmt.Errorf("%w: %s with %s", ErrConcessionPolicyUnknown, class, concession)
	}
	return Quote{
		Class:      class,
		Concession: concession,
		On:         on,
		Base:       base,
		Payable:    base.Proportion(rule.num, rule.den),
	}, nil
}

func (s *Schedule) QuoteNow(class LicenceClass, concession Concession, on timeline.Date) (Quote, error) {
	return s.Quote(class, concession, on, time.Now().UTC().Add(time.Hour))
}
