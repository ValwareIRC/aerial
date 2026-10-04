package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/timeline"
)

func now() time.Time { return time.Date(2026, time.August, 27, 0, 0, 0, 0, time.UTC) }

func TestSeededRatesMatchPublishedFigures(t *testing.T) {
	s := MustSeedUKSchedule()

	cases := []struct {
		class LicenceClass
		on    string
		want  string
	}{
		{ClassColour, "2026-04-01", "£180.00"},
		{ClassColour, "2026-12-25", "£180.00"},
		{ClassColour, "2026-03-31", "£174.50"},
		{ClassColour, "2025-04-01", "£174.50"},
		{ClassColour, "2025-03-31", "£169.50"},
		{ClassMonochrome, "2026-04-01", "£60.50"},
		{ClassARC, "2026-04-01", "£7.50"},
	}
	for _, c := range cases {
		got, err := s.BaseRate(c.class, timeline.MustParseDate(c.on), now())
		if err != nil {
			t.Fatalf("BaseRate(%s, %s): %v", c.class, c.on, err)
		}
		if got.String() != c.want {
			t.Errorf("BaseRate(%s, %s) = %s, want %s", c.class, c.on, got, c.want)
		}
	}
}

func TestRateChangesOnTheBoundary(t *testing.T) {
	s := MustSeedUKSchedule()

	before, err := s.BaseRate(ClassColour, timeline.MustParseDate("2026-03-31"), now())
	if err != nil {
		t.Fatal(err)
	}
	on, err := s.BaseRate(ClassColour, timeline.MustParseDate("2026-04-01"), now())
	if err != nil {
		t.Fatal(err)
	}
	if before.String() != "£174.50" || on.String() != "£180.00" {
		t.Errorf("boundary wrong: 31 March = %s, 1 April = %s", before, on)
	}
}

func TestConcessionQuotesMatchPublishedFigures(t *testing.T) {
	s := MustSeedUKSchedule()
	apr := timeline.MustParseDate("2026-04-01")

	cases := []struct {
		class      LicenceClass
		concession Concession
		want       string
	}{
		{ClassColour, ConcessionNone, "£180.00"},
		{ClassColour, ConcessionBlind50, "£90.00"},
		{ClassMonochrome, ConcessionNone, "£60.50"},
		{ClassMonochrome, ConcessionBlind50, "£30.25"},
		{ClassColour, ConcessionOver75PensionCredit, "£0.00"},
		{ClassARC, ConcessionNone, "£7.50"},
	}
	for _, c := range cases {
		q, err := s.Quote(c.class, c.concession, apr, now())
		if err != nil {
			t.Fatalf("Quote(%s, %s): %v", c.class, c.concession, err)
		}
		if q.Payable.String() != c.want {
			t.Errorf("Quote(%s, %s) payable = %s, want %s", c.class, c.concession, q.Payable, c.want)
		}
	}
}

func TestQuoteShowsItsWorking(t *testing.T) {
	s := MustSeedUKSchedule()
	q, err := s.Quote(ClassColour, ConcessionBlind50, timeline.MustParseDate("2026-04-01"), now())
	if err != nil {
		t.Fatal(err)
	}
	if q.Base.String() != "£180.00" {
		t.Errorf("base = %s, want £180.00", q.Base)
	}
	if q.Discount().String() != "£90.00" {
		t.Errorf("discount = %s, want £90.00", q.Discount())
	}
}

func TestUnknownRateIsAnErrorNotAGuess(t *testing.T) {
	s := MustSeedUKSchedule()

	if _, err := s.BaseRate(ClassMonochrome, timeline.MustParseDate("2025-04-01"), now()); !errors.Is(err, ErrRateNotKnown) {
		t.Errorf("monochrome in 2025 should be ErrRateNotKnown, got %v", err)
	}
	if _, err := s.BaseRate(ClassColour, timeline.MustParseDate("2001-04-01"), now()); !errors.Is(err, ErrRateNotKnown) {
		t.Errorf("colour in 2001 should be ErrRateNotKnown, got %v", err)
	}
}

func TestUnknownConcessionPolicyIsAnError(t *testing.T) {
	s := MustSeedUKSchedule()
	_, err := s.Quote(ClassARC, ConcessionBlind50, timeline.MustParseDate("2026-04-01"), now())
	if !errors.Is(err, ErrConcessionPolicyUnknown) {
		t.Errorf("ARC with blind concession should be ErrConcessionPolicyUnknown, got %v", err)
	}
}

func TestRejectsUnknownClassAndConcession(t *testing.T) {
	s := MustSeedUKSchedule()
	apr := timeline.MustParseDate("2026-04-01")

	if _, err := s.Quote("HOLOGRAM", ConcessionNone, apr, now()); err == nil {
		t.Error("unknown class should be rejected")
	}
	if _, err := s.Quote(ClassColour, "MATES_RATES", apr, now()); err == nil {
		t.Error("unknown concession should be rejected")
	}
}

// A rate corrected after the fact must not rewrite a decision already made.
func TestHistoricQuotesSurviveLaterCorrections(t *testing.T) {
	s := MustSeedUKSchedule()

	apr := timeline.MustParseDate("2026-04-01")
	correction := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := s.SetRate(ClassColour, timeline.From(apr), correction, money.MustFromPounds("185.00")); err != nil {
		t.Fatal(err)
	}

	asKnownInMay, err := s.BaseRate(ClassColour, apr, time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if asKnownInMay.String() != "£180.00" {
		t.Errorf("a decision made in May should still price at £180.00, got %s", asKnownInMay)
	}

	asKnownNow, err := s.BaseRate(ClassColour, apr, time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if asKnownNow.String() != "£185.00" {
		t.Errorf("after the correction the current rate should be £185.00, got %s", asKnownNow)
	}
}

func TestCoverageReportsWhereDataRunsOut(t *testing.T) {
	s := MustSeedUKSchedule()
	byClass := map[LicenceClass]RateCoverage{}
	for _, c := range s.Coverage() {
		byClass[c.Class] = c
	}

	colour, ok := byClass[ClassColour]
	if !ok {
		t.Fatal("no coverage reported for colour")
	}
	if colour.EarliestKnown.String() != "2024-04-01" {
		t.Errorf("colour earliest known = %s, want 2024-04-01", colour.EarliestKnown)
	}
	if !colour.HasOpenEnded {
		t.Error("colour should have a current open-ended rate")
	}

	mono, ok := byClass[ClassMonochrome]
	if !ok {
		t.Fatal("no coverage reported for monochrome")
	}
	if mono.EarliestKnown.String() != "2026-04-01" {
		t.Errorf("monochrome earliest known = %s, want 2026-04-01", mono.EarliestKnown)
	}
}
