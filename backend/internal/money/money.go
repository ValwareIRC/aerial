// Package money represents amounts as integer pence. Never floats.
// Rounding is always half-up. Division distributes the remainder so parts sum
// back to the whole.
package money

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"strconv"
	"strings"
)

var ErrParse = errors.New("money: cannot parse amount")

// Money is an amount in pence sterling. The zero value is £0.00.
type Money struct {
	pence int64
}

var Zero = Money{}

func FromPence(p int64) Money { return Money{pence: p} }

// FromPounds parses "180", "180.00", "£180.00", "1,234.56", "-7.50".
// One decimal place is read as tenths: "180.5" is £180.50.
func FromPounds(s string) (Money, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return Zero, fmt.Errorf("%w: empty string", ErrParse)
	}

	neg := false
	if strings.HasPrefix(t, "-") {
		neg = true
		t = t[1:]
	}
	t = strings.TrimPrefix(t, "£")
	t = strings.ReplaceAll(t, ",", "")
	if t == "" {
		return Zero, fmt.Errorf("%w: %q", ErrParse, s)
	}

	whole, frac := t, "00"
	if i := strings.IndexByte(t, '.'); i >= 0 {
		whole, frac = t[:i], t[i+1:]
		switch len(frac) {
		case 1:
			frac += "0"
		case 2:
		default:
			return Zero, fmt.Errorf("%w: %q has more than two decimal places", ErrParse, s)
		}
	}
	if whole == "" {
		whole = "0"
	}

	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return Zero, fmt.Errorf("%w: %q", ErrParse, s)
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return Zero, fmt.Errorf("%w: %q", ErrParse, s)
	}

	total := w*100 + f
	if neg {
		total = -total
	}
	return Money{pence: total}, nil
}

func MustFromPounds(s string) Money {
	m, err := FromPounds(s)
	if err != nil {
		panic(err)
	}
	return m
}

func (m Money) Pence() int64      { return m.pence }
func (m Money) Neg() Money        { return Money{pence: -m.pence} }
func (m Money) IsZero() bool      { return m.pence == 0 }
func (m Money) IsNegative() bool  { return m.pence < 0 }
func (m Money) Less(o Money) bool { return m.pence < o.pence }

// Add and Sub panic rather than wrap. A silently wrapped total is a wrong
// charge, which is the one failure this type exists to make impossible.
func (m Money) Add(o Money) Money {
	sum := m.pence + o.pence
	if (m.pence > 0 && o.pence > 0 && sum < 0) || (m.pence < 0 && o.pence < 0 && sum >= 0) {
		panic(fmt.Sprintf("money: addition overflows: %d + %d", m.pence, o.pence))
	}
	return Money{pence: sum}
}

func (m Money) Sub(o Money) Money { return m.Add(o.Neg()) }

func (m Money) MulInt(n int64) Money {
	hi, lo := bits.Mul64(magnitude(m.pence), magnitude(n))
	if hi != 0 || lo > math.MaxInt64 {
		panic(fmt.Sprintf("money: multiplication overflows: %d x %d", m.pence, n))
	}
	product := int64(lo)
	if (m.pence < 0) != (n < 0) {
		product = -product
	}
	return Money{pence: product}
}

// Proportion returns num/den of the amount, rounded half-up. Proportion(1, 2)
// is half. The intermediate product is computed exactly, so a large amount
// times a large numerator cannot wrap into a small answer.
func (m Money) Proportion(num, den int64) Money {
	if den == 0 {
		panic("money: Proportion with zero denominator")
	}
	return Money{pence: mulDivRoundHalfUp(m.pence, num, den)}
}

// Percent returns pct percent of the amount. Percent(50) is half.
func (m Money) Percent(pct int64) Money { return m.Proportion(pct, 100) }

// Split divides into n parts summing exactly to m, remainder on the earliest parts.
func (m Money) Split(n int) ([]Money, error) {
	if n <= 0 {
		return nil, fmt.Errorf("money: Split requires a positive part count, got %d", n)
	}
	if m.pence < 0 {
		return nil, errors.New("money: Split of a negative amount is not defined")
	}
	base := m.pence / int64(n)
	rem := m.pence % int64(n)
	out := make([]Money, n)
	for i := 0; i < n; i++ {
		p := base
		if int64(i) < rem {
			p++
		}
		out[i] = Money{pence: p}
	}
	return out, nil
}

func Sum(ms []Money) Money {
	var total int64
	for _, m := range ms {
		total += m.pence
	}
	return Money{pence: total}
}

func (m Money) String() string {
	sign := ""
	p := m.pence
	if p < 0 {
		sign = "-"
		p = -p
	}
	whole, frac := p/100, p%100
	return fmt.Sprintf("%s£%s.%02d", sign, group(whole), frac)
}

// MarshalJSON emits pence as an integer, so clients cannot reintroduce floats.
func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(m.pence, 10)), nil
}

func (m *Money) UnmarshalJSON(b []byte) error {
	p, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return fmt.Errorf("money: expected an integer number of pence: %w", err)
	}
	m.pence = p
	return nil
}

// mulDivRoundHalfUp returns a*num/den rounded half-up, with the sign taken from
// the three operands and the multiplication carried out exactly. The int64 path
// is used whenever 2*a*num + den cannot overflow, which covers every amount a
// licence fee will ever reach; anything larger falls back to arbitrary
// precision rather than wrapping.
func mulDivRoundHalfUp(a, num, den int64) int64 {
	neg := (a < 0) != (num < 0) != (den < 0)
	ua, unum, uden := magnitude(a), magnitude(num), magnitude(den)

	hi, lo := bits.Mul64(ua, unum)
	if hi == 0 && lo <= (math.MaxUint64-uden)/2 {
		q := int64((2*lo + uden) / (2 * uden))
		if neg {
			return -q
		}
		return q
	}

	product := new(big.Int).SetUint64(hi)
	product.Lsh(product, 64)
	product.Add(product, new(big.Int).SetUint64(lo))

	numerator := new(big.Int).Lsh(product, 1)
	numerator.Add(numerator, new(big.Int).SetUint64(uden))
	denominator := new(big.Int).Lsh(new(big.Int).SetUint64(uden), 1)
	q := numerator.Quo(numerator, denominator)
	if neg {
		q.Neg(q)
	}
	if !q.IsInt64() {
		panic(fmt.Sprintf("money: %d x %d / %d does not fit in an amount", a, num, den))
	}
	return q.Int64()
}

func magnitude(n int64) uint64 {
	if n < 0 {
		// Negating math.MinInt64 overflows; its magnitude is representable
		// only as an unsigned value.
		return uint64(-(n + 1)) + 1
	}
	return uint64(n)
}

func group(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
