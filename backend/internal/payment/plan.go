// Package payment builds instalment schedules. Every schedule sums exactly to
// the fee; no rounding is left on the floor.
package payment

import (
	"fmt"

	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/timeline"
)

type PlanKind string

const (
	PlanAnnual          PlanKind = "ANNUAL"
	PlanQuarterlyDD     PlanKind = "QUARTERLY_DD"
	PlanMonthlyDD       PlanKind = "MONTHLY_DD"
	PlanFortnightlyCash PlanKind = "FORTNIGHTLY_CASH"
	PlanWeeklyCash      PlanKind = "WEEKLY_CASH"
)

func (k PlanKind) Valid() bool {
	switch k {
	case PlanAnnual, PlanQuarterlyDD, PlanMonthlyDD, PlanFortnightlyCash, PlanWeeklyCash:
		return true
	}
	return false
}

type Instalment struct {
	Seq    int           `json:"seq"`
	Due    timeline.Date `json:"due"`
	Amount money.Money   `json:"amount"`
}

type Plan struct {
	Kind        PlanKind      `json:"kind"`
	Total       money.Money   `json:"total"`
	Start       timeline.Date `json:"start"`
	Instalments []Instalment  `json:"instalments"`
}

// Build produces the schedule for a plan over a term in months.
// Sub-annual plans require a twelve-month term.
func Build(kind PlanKind, total money.Money, start timeline.Date, termMonths int) (Plan, error) {
	if !kind.Valid() {
		return Plan{}, fmt.Errorf("payment: unknown plan kind %q", kind)
	}
	if total.IsNegative() {
		return Plan{}, fmt.Errorf("payment: total cannot be negative")
	}
	if termMonths < 1 {
		return Plan{}, fmt.Errorf("payment: term must be at least one month")
	}

	count, step, unit, err := shape(kind, termMonths)
	if err != nil {
		return Plan{}, err
	}

	amounts, err := total.Split(count)
	if err != nil {
		return Plan{}, err
	}

	p := Plan{Kind: kind, Total: total, Start: start}
	for i, amount := range amounts {
		due := start
		switch unit {
		case unitMonth:
			due = start.AddMonths(i * step)
		case unitDay:
			due = start.AddDays(i * step)
		}
		p.Instalments = append(p.Instalments, Instalment{Seq: i + 1, Due: due, Amount: amount})
	}
	return p, nil
}

type dueUnit int

const (
	unitMonth dueUnit = iota
	unitDay
)

func shape(kind PlanKind, termMonths int) (count, step int, unit dueUnit, err error) {
	switch kind {
	case PlanAnnual:
		return 1, 0, unitMonth, nil
	case PlanMonthlyDD:
		return termMonths, 1, unitMonth, nil
	case PlanQuarterlyDD:
		if termMonths%3 != 0 {
			return 0, 0, 0, fmt.Errorf("payment: a quarterly plan needs a term divisible by three, got %d months", termMonths)
		}
		return termMonths / 3, 3, unitMonth, nil
	case PlanFortnightlyCash:
		if termMonths != 12 {
			return 0, 0, 0, fmt.Errorf("payment: a fortnightly plan needs a twelve-month term, got %d", termMonths)
		}
		return 26, 14, unitDay, nil
	case PlanWeeklyCash:
		if termMonths != 12 {
			return 0, 0, 0, fmt.Errorf("payment: a weekly plan needs a twelve-month term, got %d", termMonths)
		}
		return 52, 7, unitDay, nil
	}
	return 0, 0, 0, fmt.Errorf("payment: unknown plan kind %q", kind)
}

// Paid totals the instalments due on or before a date.
func (p Plan) Paid(on timeline.Date) money.Money {
	var due []money.Money
	for _, i := range p.Instalments {
		if i.Due.OnOrBefore(on) {
			due = append(due, i.Amount)
		}
	}
	return money.Sum(due)
}

// Outstanding is the total less what has fallen due by a date.
func (p Plan) Outstanding(on timeline.Date) money.Money {
	return p.Total.Sub(p.Paid(on))
}

// Largest returns the biggest single instalment, which is what affordability
// should be judged on rather than the annual total.
func (p Plan) Largest() money.Money {
	var max money.Money
	for _, i := range p.Instalments {
		if max.Less(i.Amount) {
			max = i.Amount
		}
	}
	return max
}
