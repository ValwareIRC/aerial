package timeline

import (
	"testing"
	"time"
)

func TestAddMonthsClampsRatherThanOverflowing(t *testing.T) {
	cases := []struct {
		from string
		n    int
		want string
	}{
		{"2026-01-31", 1, "2026-02-28"},
		{"2028-01-31", 1, "2028-02-29"}, // leap year
		{"2026-01-31", 3, "2026-04-30"},
		{"2026-03-31", -1, "2026-02-28"},
		{"2026-04-01", 12, "2027-04-01"},
		{"2026-12-31", 1, "2027-01-31"},
		{"2026-05-15", 0, "2026-05-15"},
	}
	for _, c := range cases {
		got := MustParseDate(c.from).AddMonths(c.n)
		if got.String() != c.want {
			t.Errorf("%s.AddMonths(%d) = %s, want %s", c.from, c.n, got, c.want)
		}
	}
}

func TestAddYearsFromLeapDay(t *testing.T) {
	got := MustParseDate("2028-02-29").AddYears(1)
	if got.String() != "2029-02-28" {
		t.Errorf("leap day + 1 year = %s, want 2029-02-28", got)
	}
}

func TestCompleteMonthsUntil(t *testing.T) {
	cases := []struct {
		from, to string
		want     int
	}{
		{"2026-04-01", "2027-04-01", 12},
		{"2026-04-01", "2026-04-01", 0},
		{"2026-04-15", "2026-05-14", 0}, // one day short of a month
		{"2026-04-15", "2026-05-15", 1},
		{"2026-04-15", "2026-05-16", 1},
		{"2026-01-31", "2026-02-28", 0}, // 28 Feb is not a complete month from 31 Jan
		{"2027-04-01", "2026-04-01", 0}, // backwards yields zero, never negative
		{"2026-06-10", "2027-01-09", 6},
	}
	for _, c := range cases {
		got := MustParseDate(c.from).CompleteMonthsUntil(MustParseDate(c.to))
		if got != c.want {
			t.Errorf("%s.CompleteMonthsUntil(%s) = %d, want %d", c.from, c.to, got, c.want)
		}
	}
}

func TestRangeIsHalfOpen(t *testing.T) {
	r := Until(MustParseDate("2026-04-01"), MustParseDate("2027-04-01"))

	if !r.Contains(MustParseDate("2026-04-01")) {
		t.Error("range should contain its start date")
	}
	if r.Contains(MustParseDate("2027-04-01")) {
		t.Error("range should not contain its end date")
	}
	if !r.Contains(MustParseDate("2027-03-31")) {
		t.Error("range should contain the day before its end")
	}
}

func TestAbuttingRangesDoNotOverlap(t *testing.T) {
	first := Until(MustParseDate("2026-04-01"), MustParseDate("2027-04-01"))
	second := Until(MustParseDate("2027-04-01"), MustParseDate("2028-04-01"))

	if first.Overlaps(second) {
		t.Error("abutting ranges must not overlap")
	}
	if second.Overlaps(first) {
		t.Error("overlap must be symmetric")
	}
}

func TestOverlapDetection(t *testing.T) {
	a := Until(MustParseDate("2026-01-01"), MustParseDate("2026-06-01"))
	overlapping := Until(MustParseDate("2026-05-01"), MustParseDate("2026-08-01"))
	disjoint := Until(MustParseDate("2026-06-01"), MustParseDate("2026-08-01"))
	open := From(MustParseDate("2026-05-01"))

	if !a.Overlaps(overlapping) {
		t.Error("overlapping ranges not detected")
	}
	if a.Overlaps(disjoint) {
		t.Error("disjoint ranges reported as overlapping")
	}
	if !a.Overlaps(open) {
		t.Error("open-ended range starting inside should overlap")
	}
}

func TestNewRangeRejectsInvertedRange(t *testing.T) {
	start := MustParseDate("2026-04-01")
	end := MustParseDate("2026-03-01")
	if _, err := NewRange(start, &end); err == nil {
		t.Error("a range ending before it starts should be rejected")
	}
	if _, err := NewRange(start, &start); err == nil {
		t.Error("an empty half-open range should be rejected")
	}
}

func TestStoreReplaysHistoricBeliefs(t *testing.T) {
	s := NewStore[string]()

	feb := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	nov := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)

	// In February we recorded that the value from April would be "180".
	s.Append(From(MustParseDate("2026-04-01")), feb, "180")
	// In November we corrected that: it should have been "185" all along.
	s.Append(From(MustParseDate("2026-04-01")), nov, "185")

	on := MustParseDate("2026-06-01")

	if got, ok := s.AsAt(on, time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)); !ok || got != "180" {
		t.Errorf("as known in July, June value = %q (found=%v), want 180", got, ok)
	}
	if got, ok := s.AsAt(on, time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)); !ok || got != "185" {
		t.Errorf("as known in December, June value = %q (found=%v), want 185", got, ok)
	}
}

func TestStoreRespectsValidTime(t *testing.T) {
	s := NewStore[string]()
	rec := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	s.Append(Until(MustParseDate("2025-04-01"), MustParseDate("2026-04-01")), rec, "174.50")
	s.Append(From(MustParseDate("2026-04-01")), rec, "180.00")

	if got, _ := s.Current(MustParseDate("2026-03-31")); got != "174.50" {
		t.Errorf("31 March rate = %q, want 174.50", got)
	}
	if got, _ := s.Current(MustParseDate("2026-04-01")); got != "180.00" {
		t.Errorf("1 April rate = %q, want 180.00", got)
	}
}

func TestStoreReportsMissingRatherThanGuessing(t *testing.T) {
	s := NewStore[string]()
	s.Append(From(MustParseDate("2026-04-01")), time.Now(), "180.00")

	if _, ok := s.Current(MustParseDate("2020-01-01")); ok {
		t.Error("a date before any recorded range must report not found")
	}
}

func TestDateJSONRoundTrip(t *testing.T) {
	d := MustParseDate("2026-04-01")
	b, err := d.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"2026-04-01"` {
		t.Errorf("MarshalJSON = %s", b)
	}
	var back Date
	if err := back.UnmarshalJSON(b); err != nil {
		t.Fatal(err)
	}
	if back != d {
		t.Errorf("round trip gave %s, want %s", back, d)
	}
}
