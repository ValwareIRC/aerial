// Package timeline provides civil dates, half-open date ranges, and a
// bitemporal record store separating when a fact was true from when it was
// recorded. See docs/DECISIONS.md.
package timeline

import (
	"errors"
	"fmt"
	"time"
)

// Date is a civil date with no time zone.
type Date struct {
	y int
	m time.Month
	d int
}

// NewDate normalises overflow the way time.Date does.
func NewDate(y int, m time.Month, d int) Date {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return Date{y: t.Year(), m: t.Month(), d: t.Day()}
}

// ParseDate reads an ISO 8601 calendar date, "2026-04-01".
func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, fmt.Errorf("timeline: %q is not an ISO date: %w", s, err)
	}
	return Date{y: t.Year(), m: t.Month(), d: t.Day()}, nil
}

func MustParseDate(s string) Date {
	d, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

func (d Date) Year() int         { return d.y }
func (d Date) Month() time.Month { return d.m }
func (d Date) Day() int          { return d.d }
func (d Date) IsZero() bool      { return d == Date{} }

// Time converts to midnight UTC, for arithmetic only.
func (d Date) Time() time.Time { return time.Date(d.y, d.m, d.d, 0, 0, 0, 0, time.UTC) }

func (d Date) String() string { return d.Time().Format("2006-01-02") }

func (d Date) MarshalJSON() ([]byte, error) { return []byte(`"` + d.String() + `"`), nil }

func (d *Date) UnmarshalJSON(b []byte) error {
	s := string(b)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("timeline: expected a quoted ISO date, got %s", s)
	}
	p, err := ParseDate(s[1 : len(s)-1])
	if err != nil {
		return err
	}
	*d = p
	return nil
}

func (d Date) Before(o Date) bool     { return d.Time().Before(o.Time()) }
func (d Date) After(o Date) bool      { return d.Time().After(o.Time()) }
func (d Date) Equal(o Date) bool      { return d == o }
func (d Date) OnOrBefore(o Date) bool { return !d.After(o) }
func (d Date) OnOrAfter(o Date) bool  { return !d.Before(o) }

func (d Date) AddDays(n int) Date {
	t := d.Time().AddDate(0, 0, n)
	return Date{y: t.Year(), m: t.Month(), d: t.Day()}
}

// AddMonths clamps to the end of the target month: 31 Jan + 1 month is 28 Feb.
func (d Date) AddMonths(n int) Date {
	y, m := d.y, int(d.m)+n
	y += (m - 1) / 12
	m = (m-1)%12 + 1
	if m < 1 {
		m += 12
		y--
	}
	last := daysInMonth(y, time.Month(m))
	day := d.d
	if day > last {
		day = last
	}
	return Date{y: y, m: time.Month(m), d: day}
}

func (d Date) AddYears(n int) Date { return d.AddMonths(12 * n) }

// DaysUntil counts days from d up to but not including o.
func (d Date) DaysUntil(o Date) int {
	return int(o.Time().Sub(d.Time()).Hours() / 24)
}

// CompleteMonthsUntil counts whole calendar months from d to o. Refunds are
// paid in this unit.
func (d Date) CompleteMonthsUntil(o Date) int {
	if o.OnOrBefore(d) {
		return 0
	}
	months := (o.y-d.y)*12 + int(o.m) - int(d.m)
	if o.d < d.d {
		months--
	}
	if months < 0 {
		return 0
	}
	return months
}

func daysInMonth(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

var ErrBadRange = errors.New("timeline: range ends before it starts")

// Range is a half-open interval [Start, End). A nil End means no end recorded.
type Range struct {
	Start Date
	End   *Date
}

func NewRange(start Date, end *Date) (Range, error) {
	if end != nil && !end.After(start) {
		return Range{}, fmt.Errorf("%w: %s to %s", ErrBadRange, start, *end)
	}
	return Range{Start: start, End: end}, nil
}

func MustRange(start Date, end *Date) Range {
	r, err := NewRange(start, end)
	if err != nil {
		panic(err)
	}
	return r
}

func Until(start, end Date) Range { return MustRange(start, &end) }
func From(start Date) Range       { return Range{Start: start} }

func (r Range) IsOpen() bool { return r.End == nil }

func (r Range) Contains(d Date) bool {
	if d.Before(r.Start) {
		return false
	}
	if r.End == nil {
		return true
	}
	return d.Before(*r.End)
}

func (r Range) Overlaps(o Range) bool {
	if r.End != nil && o.Start.OnOrAfter(*r.End) {
		return false
	}
	if o.End != nil && r.Start.OnOrAfter(*o.End) {
		return false
	}
	return true
}

func (r Range) String() string {
	if r.End == nil {
		return r.Start.String() + ".."
	}
	return r.Start.String() + ".." + r.End.String()
}

// Record is a value true over a range of valid time, as recorded at an instant.
type Record[T any] struct {
	Valid      Range
	RecordedAt time.Time
	Value      T
}

// Store is append-only. Corrections append a later record; nothing is mutated.
type Store[T any] struct {
	records []Record[T]
}

func NewStore[T any]() *Store[T] { return &Store[T]{} }

func (s *Store[T]) Append(valid Range, recordedAt time.Time, value T) {
	s.records = append(s.records, Record[T]{Valid: valid, RecordedAt: recordedAt.UTC(), Value: value})
}

// AsAt returns the value in force on validOn per what was recorded by knownOn.
// Latest recording wins.
func (s *Store[T]) AsAt(validOn Date, knownOn time.Time) (T, bool) {
	var best Record[T]
	found := false
	knownOn = knownOn.UTC()
	for _, r := range s.records {
		if !r.Valid.Contains(validOn) {
			continue
		}
		if r.RecordedAt.After(knownOn) {
			continue
		}
		if !found || r.RecordedAt.After(best.RecordedAt) {
			best, found = r, true
		}
	}
	return best.Value, found
}

// Current returns the value in force on validOn per everything known.
func (s *Store[T]) Current(validOn Date) (T, bool) {
	return s.AsAt(validOn, time.Now().UTC().Add(time.Hour))
}

func (s *Store[T]) All() []Record[T] {
	out := make([]Record[T], len(s.records))
	copy(out, s.records)
	return out
}

func (s *Store[T]) Len() int { return len(s.records) }
