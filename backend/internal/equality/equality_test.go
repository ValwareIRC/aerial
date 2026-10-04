package equality

import (
	"testing"
)

func observe(t *testing.T, m *Monitor, n int, outcome, sex string) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := m.Observe(Observation{Outcome: outcome, Attributes: map[string]string{"sex": sex}}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSmallCellsAreWithheldAndCounted(t *testing.T) {
	m := MustNew(10)
	observe(t, m, 50, "PROSECUTED", "female")
	observe(t, m, 3, "PROSECUTED", "male")

	r := m.Report("sex")
	if len(r.Cells) != 1 {
		t.Fatalf("expected one publishable cell, got %d", len(r.Cells))
	}
	if r.Cells[0].Value != "female" || r.Cells[0].Count != 50 {
		t.Errorf("unexpected cell %+v", r.Cells[0])
	}
	if r.SuppressedCells != 1 || r.SuppressedCount != 3 {
		t.Errorf("suppression not reported: %d cells, %d records", r.SuppressedCells, r.SuppressedCount)
	}
}

func TestRatesRequireBothCellAndTotalToClearK(t *testing.T) {
	m := MustNew(10)
	observe(t, m, 70, "PROSECUTED", "female")
	observe(t, m, 30, "NOT_PROSECUTED", "female")
	observe(t, m, 5, "PROSECUTED", "male")
	observe(t, m, 2, "NOT_PROSECUTED", "male")

	rates := m.Rates("sex", "PROSECUTED")
	if len(rates) != 1 {
		t.Fatalf("expected one publishable rate, got %d", len(rates))
	}
	if rates[0].Value != "female" {
		t.Errorf("published rate for %q, expected female only", rates[0].Value)
	}
	if rates[0].Proportion != 0.7 {
		t.Errorf("proportion = %v, want 0.7", rates[0].Proportion)
	}
}

func TestDisparityIsVisibleWhenBothGroupsClearK(t *testing.T) {
	m := MustNew(10)
	observe(t, m, 70, "PROSECUTED", "female")
	observe(t, m, 30, "NOT_PROSECUTED", "female")
	observe(t, m, 30, "PROSECUTED", "male")
	observe(t, m, 70, "NOT_PROSECUTED", "male")

	rates := m.Rates("sex", "PROSECUTED")
	if len(rates) != 2 {
		t.Fatalf("expected both groups published, got %d", len(rates))
	}
	byValue := map[string]float64{}
	for _, r := range rates {
		byValue[r.Value] = r.Proportion
	}
	if byValue["female"] <= byValue["male"] {
		t.Errorf("expected the disparity to be visible: female %v, male %v", byValue["female"], byValue["male"])
	}
}

func TestBlankAttributesAreNotCounted(t *testing.T) {
	m := MustNew(2)
	for i := 0; i < 5; i++ {
		if err := m.Observe(Observation{Outcome: "PROSECUTED", Attributes: map[string]string{"sex": ""}}); err != nil {
			t.Fatal(err)
		}
	}
	if got := m.Report("sex"); len(got.Cells) != 0 || got.SuppressedCells != 0 {
		t.Errorf("blank declarations should not create cells: %+v", got)
	}
	if got := m.Report("sex").TotalCount; got != 5 {
		t.Errorf("total observations = %d, want 5", got)
	}
}

func TestObservationRequiresAnOutcome(t *testing.T) {
	m := MustNew(10)
	if err := m.Observe(Observation{Attributes: map[string]string{"sex": "female"}}); err == nil {
		t.Error("an observation without an outcome should be rejected")
	}
}

func TestKMustBeMeaningful(t *testing.T) {
	if _, err := New(1); err == nil {
		t.Error("k of 1 publishes individuals and should be rejected")
	}
	if _, err := New(0); err == nil {
		t.Error("k of 0 should be rejected")
	}
}

func TestUnknownAttributeReportsEmpty(t *testing.T) {
	m := MustNew(10)
	observe(t, m, 20, "PROSECUTED", "female")
	if r := m.Report("height"); len(r.Cells) != 0 {
		t.Errorf("unknown attribute should report no cells, got %d", len(r.Cells))
	}
}
