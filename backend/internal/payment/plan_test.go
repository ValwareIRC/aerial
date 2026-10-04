package payment

import (
	"testing"

	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/timeline"
)

func d(s string) timeline.Date { return timeline.MustParseDate(s) }

func TestPlanShapes(t *testing.T) {
	total := money.MustFromPounds("180.00")
	start := d("2026-04-01")

	cases := []struct {
		kind      PlanKind
		wantCount int
		firstDue  string
		lastDue   string
	}{
		{PlanAnnual, 1, "2026-04-01", "2026-04-01"},
		{PlanQuarterlyDD, 4, "2026-04-01", "2027-01-01"},
		{PlanMonthlyDD, 12, "2026-04-01", "2027-03-01"},
		{PlanFortnightlyCash, 26, "2026-04-01", "2027-03-17"},
		{PlanWeeklyCash, 52, "2026-04-01", "2027-03-24"},
	}
	for _, c := range cases {
		p, err := Build(c.kind, total, start, 12)
		if err != nil {
			t.Fatalf("Build(%s): %v", c.kind, err)
		}
		if len(p.Instalments) != c.wantCount {
			t.Errorf("%s has %d instalments, want %d", c.kind, len(p.Instalments), c.wantCount)
		}
		if got := p.Instalments[0].Due.String(); got != c.firstDue {
			t.Errorf("%s first due %s, want %s", c.kind, got, c.firstDue)
		}
		if got := p.Instalments[len(p.Instalments)-1].Due.String(); got != c.lastDue {
			t.Errorf("%s last due %s, want %s", c.kind, got, c.lastDue)
		}
	}
}

func TestEveryScheduleSumsToTheFee(t *testing.T) {
	start := d("2026-04-01")
	for _, amount := range []string{"180.00", "174.50", "90.00", "60.50", "30.25", "7.50", "0.00"} {
		total := money.MustFromPounds(amount)
		for _, kind := range []PlanKind{PlanAnnual, PlanQuarterlyDD, PlanMonthlyDD, PlanFortnightlyCash, PlanWeeklyCash} {
			p, err := Build(kind, total, start, 12)
			if err != nil {
				t.Fatalf("Build(%s, %s): %v", kind, amount, err)
			}
			var sum money.Money
			for _, i := range p.Instalments {
				sum = sum.Add(i.Amount)
			}
			if sum != total {
				t.Errorf("%s of %s sums to %s", kind, total, sum)
			}
		}
	}
}

func TestWeeklyPlanPutsTheOddPenceFirst(t *testing.T) {
	p, err := Build(PlanWeeklyCash, money.MustFromPounds("180.00"), d("2026-04-01"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Instalments[0].Amount.String(); got != "£3.47" {
		t.Errorf("first weekly instalment = %s, want £3.47", got)
	}
	if got := p.Instalments[51].Amount.String(); got != "£3.46" {
		t.Errorf("last weekly instalment = %s, want £3.46", got)
	}
}

func TestLargestInstalmentIsTheAffordabilityFigure(t *testing.T) {
	weekly, err := Build(PlanWeeklyCash, money.MustFromPounds("180.00"), d("2026-04-01"), 12)
	if err != nil {
		t.Fatal(err)
	}
	annual, err := Build(PlanAnnual, money.MustFromPounds("180.00"), d("2026-04-01"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if weekly.Largest().String() != "£3.47" {
		t.Errorf("largest weekly = %s, want £3.47", weekly.Largest())
	}
	if annual.Largest().String() != "£180.00" {
		t.Errorf("largest annual = %s, want £180.00", annual.Largest())
	}
}

func TestPaidAndOutstandingTrackTheSchedule(t *testing.T) {
	p, err := Build(PlanMonthlyDD, money.MustFromPounds("180.00"), d("2026-04-01"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Paid(d("2026-04-01")).String(); got != "£15.00" {
		t.Errorf("paid after the first instalment = %s, want £15.00", got)
	}
	if got := p.Paid(d("2026-06-15")).String(); got != "£45.00" {
		t.Errorf("paid by mid June = %s, want £45.00", got)
	}
	if got := p.Outstanding(d("2026-06-15")).String(); got != "£135.00" {
		t.Errorf("outstanding by mid June = %s, want £135.00", got)
	}
	if got := p.Outstanding(d("2027-04-01")).String(); got != "£0.00" {
		t.Errorf("outstanding at term end = %s, want £0.00", got)
	}
}

func TestBuildRejectsImpossibleShapes(t *testing.T) {
	total := money.MustFromPounds("180.00")
	start := d("2026-04-01")

	if _, err := Build(PlanQuarterlyDD, total, start, 7); err == nil {
		t.Error("a quarterly plan over seven months should be rejected")
	}
	if _, err := Build(PlanWeeklyCash, total, start, 6); err == nil {
		t.Error("a weekly plan over six months should be rejected")
	}
	if _, err := Build("BARTER", total, start, 12); err == nil {
		t.Error("an unknown plan kind should be rejected")
	}
	if _, err := Build(PlanAnnual, money.FromPence(-1), start, 12); err == nil {
		t.Error("a negative total should be rejected")
	}
}
