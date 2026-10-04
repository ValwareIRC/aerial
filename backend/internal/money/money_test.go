package money

import (
	"math"
	"testing"
)

func TestFromPounds(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"180", 18000},
		{"180.00", 18000},
		{"£180.00", 18000},
		{"60.50", 6050},
		{"7.50", 750},
		{"0", 0},
		{"0.01", 1},
		{"180.5", 18050},
		{"1,234.56", 123456},
		{"-7.50", -750},
		{"£1,000,000.00", 100000000},
	}
	for _, c := range cases {
		got, err := FromPounds(c.in)
		if err != nil {
			t.Fatalf("FromPounds(%q) returned error: %v", c.in, err)
		}
		if got.Pence() != c.want {
			t.Errorf("FromPounds(%q) = %d pence, want %d", c.in, got.Pence(), c.want)
		}
	}
}

func TestFromPoundsRejectsJunk(t *testing.T) {
	for _, in := range []string{"", "abc", "1.234", "£", "12.3.4", "1..2"} {
		if _, err := FromPounds(in); err == nil {
			t.Errorf("FromPounds(%q) accepted a bad value", in)
		}
	}
}

func TestPercentMatchesPublishedConcessionRates(t *testing.T) {
	colour := MustFromPounds("180.00")
	mono := MustFromPounds("60.50")

	if got := colour.Percent(50); got.Pence() != 9000 {
		t.Errorf("50%% of colour = %s, want £90.00", got)
	}
	if got := mono.Percent(50); got.Pence() != 3025 {
		t.Errorf("50%% of monochrome = %s, want £30.25", got)
	}
}

func TestPercentRoundsHalfUp(t *testing.T) {
	// 5p at 50% is 2.5p, which must round to 3p, not 2p.
	if got := FromPence(5).Percent(50); got.Pence() != 3 {
		t.Errorf("50%% of 5p = %d, want 3", got.Pence())
	}
	// Negative amounts round away from zero symmetrically.
	if got := FromPence(-5).Percent(50); got.Pence() != -3 {
		t.Errorf("50%% of -5p = %d, want -3", got.Pence())
	}
}

func TestSplitSumsExactly(t *testing.T) {
	for _, parts := range []int{1, 2, 3, 4, 7, 12, 52} {
		for _, amount := range []string{"180.00", "174.50", "7.50", "60.50", "0.01"} {
			m := MustFromPounds(amount)
			got, err := m.Split(parts)
			if err != nil {
				t.Fatalf("Split(%d) of %s: %v", parts, amount, err)
			}
			if len(got) != parts {
				t.Fatalf("Split(%d) returned %d parts", parts, len(got))
			}
			if total := Sum(got); total != m {
				t.Errorf("Split(%d) of %s sums to %s, want %s", parts, amount, total, m)
			}
		}
	}
}

func TestSplitDistributesRemainderToEarliestParts(t *testing.T) {
	// £180.00 over 52 weeks is 346.15...p, so 48 parts of 347p and 4 of 346p.
	got, err := MustFromPounds("180.00").Split(52)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Pence() != 347 {
		t.Errorf("first weekly instalment = %d, want 347", got[0].Pence())
	}
	if got[51].Pence() != 346 {
		t.Errorf("last weekly instalment = %d, want 346", got[51].Pence())
	}
}

func TestSplitRejectsBadInput(t *testing.T) {
	if _, err := MustFromPounds("10.00").Split(0); err == nil {
		t.Error("Split(0) should fail")
	}
	if _, err := FromPence(-100).Split(2); err == nil {
		t.Error("Split of a negative amount should fail")
	}
}

func TestString(t *testing.T) {
	cases := map[int64]string{
		0:         "£0.00",
		1:         "£0.01",
		750:       "£7.50",
		18000:     "£180.00",
		123456:    "£1,234.56",
		100000000: "£1,000,000.00",
		-750:      "-£7.50",
	}
	for pence, want := range cases {
		if got := FromPence(pence).String(); got != want {
			t.Errorf("FromPence(%d).String() = %q, want %q", pence, got, want)
		}
	}
}

func TestJSONRoundTripsAsPence(t *testing.T) {
	m := MustFromPounds("180.00")
	b, err := m.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "18000" {
		t.Errorf("MarshalJSON = %s, want 18000", b)
	}
	var back Money
	if err := back.UnmarshalJSON(b); err != nil {
		t.Fatal(err)
	}
	if back != m {
		t.Errorf("round trip gave %s, want %s", back, m)
	}
}

func TestProportionOfALargeAmountDoesNotWrap(t *testing.T) {
	// A whole-country total, taken as a fraction. The intermediate product
	// exceeds int64, which used to wrap silently into a small or negative
	// answer; the only acceptable outcomes are the exact figure, or a refusal
	// to answer at all.
	huge := FromPence(4_000_000_000_000) // £40bn
	half := huge.Proportion(1, 2)
	if half.Pence() != 2_000_000_000_000 {
		t.Errorf("half of %s = %s, want £20,000,000,000.00", huge, half)
	}

	// 999999/1000000 of £40bn: the product is about 4e18, inside int64, and
	// the doubled numerator is not.
	almostAll := huge.Proportion(999_999, 1_000_000)
	if almostAll.Pence() != 3_999_996_000_000 {
		t.Errorf("999999/1000000 of %s = %d pence, want 3999996000000", huge, almostAll.Pence())
	}
	if almostAll.IsNegative() {
		t.Error("a proportion of a positive amount must never come out negative")
	}
}

func TestProportionStillRoundsHalfUpAtScale(t *testing.T) {
	cases := []struct {
		pence    int64
		num, den int64
		want     int64
	}{
		{6050, 1, 2, 3025},       // the published monochrome half rate
		{18000, 1, 2, 9000},      // colour
		{3, 1, 2, 2},             // 1.5 rounds up
		{-3, 1, 2, -2},           // and away from zero on the way down
		{1 << 62, 2, 2, 1 << 62}, // exact at the edge of int64
		// The product overflows int64 while the answer does not, and the
		// exact quotient is a half, so it rounds up.
		{(1 << 62) + 1, 2, 4, (1 << 61) + 1},
	}
	for _, c := range cases {
		got := FromPence(c.pence).Proportion(c.num, c.den).Pence()
		if got != c.want {
			t.Errorf("%d x %d/%d = %d, want %d", c.pence, c.num, c.den, got, c.want)
		}
	}
}

func TestMultiplicationRefusesToWrap(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("multiplying past the end of the type should panic, not wrap")
		}
	}()
	FromPence(1 << 62).MulInt(8)
}

func TestAdditionRefusesToWrap(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("adding past the end of the type should panic, not wrap")
		}
	}()
	FromPence(math.MaxInt64 - 10).Add(FromPence(100))
}
