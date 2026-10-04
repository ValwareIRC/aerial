package domain

import (
	"fmt"

	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/timeline"
)

type ukRate struct {
	class  LicenceClass
	from   string // inclusive
	to     string // exclusive; empty means open-ended
	amount string
}

// ukRates holds only rates verified against published sources. Unverified rates
// are omitted so a quote for them errors rather than guessing. See QUESTIONS.md.
var ukRates = []ukRate{
	{ClassColour, "2024-04-01", "2025-04-01", "169.50"},
	{ClassColour, "2025-04-01", "2026-04-01", "174.50"},
	{ClassColour, "2026-04-01", "", "180.00"},

	{ClassMonochrome, "2026-04-01", "", "60.50"},

	{ClassARC, "2026-04-01", "", "7.50"},
}

// ukRateRecordedOffsetMonths approximates publication time for seed data. A
// deployment must record the real entry timestamp.
const ukRateRecordedOffsetMonths = -2

func SeedUKSchedule() (*Schedule, error) {
	s := NewSchedule()
	for _, r := range ukRates {
		from, err := timeline.ParseDate(r.from)
		if err != nil {
			return nil, fmt.Errorf("seeding %s: %w", r.class, err)
		}

		var valid timeline.Range
		if r.to == "" {
			valid = timeline.From(from)
		} else {
			to, err := timeline.ParseDate(r.to)
			if err != nil {
				return nil, fmt.Errorf("seeding %s: %w", r.class, err)
			}
			valid, err = timeline.NewRange(from, &to)
			if err != nil {
				return nil, fmt.Errorf("seeding %s: %w", r.class, err)
			}
		}

		amount, err := money.FromPounds(r.amount)
		if err != nil {
			return nil, fmt.Errorf("seeding %s: %w", r.class, err)
		}

		recordedAt := from.AddMonths(ukRateRecordedOffsetMonths).Time()
		if err := s.SetRate(r.class, valid, recordedAt, amount); err != nil {
			return nil, fmt.Errorf("seeding %s: %w", r.class, err)
		}
	}
	return s, nil
}

func MustSeedUKSchedule() *Schedule {
	s, err := SeedUKSchedule()
	if err != nil {
		panic(err)
	}
	return s
}

type RateCoverage struct {
	Class         LicenceClass  `json:"class"`
	EarliestKnown timeline.Date `json:"earliestKnown"`
	HasOpenEnded  bool          `json:"hasOpenEnded"`
	RecordCount   int           `json:"recordCount"`
}

// Coverage reports where the rate data runs out, per class.
func (s *Schedule) Coverage() []RateCoverage {
	out := make([]RateCoverage, 0, len(s.rates))
	for class, store := range s.rates {
		cov := RateCoverage{Class: class, RecordCount: store.Len()}
		for i, rec := range store.All() {
			if i == 0 || rec.Valid.Start.Before(cov.EarliestKnown) {
				cov.EarliestKnown = rec.Valid.Start
			}
			if rec.Valid.IsOpen() {
				cov.HasOpenEnded = true
			}
		}
		out = append(out, cov)
	}
	return out
}
