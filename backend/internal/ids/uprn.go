// Package ids holds the domain identifier types. Premises are keyed on UPRN,
// never on a free-text address.
package ids

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidUPRN = errors.New("ids: invalid UPRN")

// UPRN is a Unique Property Reference Number: up to twelve digits, no check
// digit, so validation confirms shape only.
type UPRN uint64

func ParseUPRN(s string) (UPRN, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, fmt.Errorf("%w: empty", ErrInvalidUPRN)
	}
	if len(t) > 12 {
		return 0, fmt.Errorf("%w: %q is longer than twelve digits", ErrInvalidUPRN, s)
	}
	if len(t) > 1 && t[0] == '0' {
		return 0, fmt.Errorf("%w: %q has a leading zero", ErrInvalidUPRN, s)
	}
	for _, r := range t {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("%w: %q contains a non-digit", ErrInvalidUPRN, s)
		}
	}
	n, err := strconv.ParseUint(t, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidUPRN, s)
	}
	if n == 0 {
		return 0, fmt.Errorf("%w: zero is not a UPRN", ErrInvalidUPRN)
	}
	return UPRN(n), nil
}

func MustParseUPRN(s string) UPRN {
	u, err := ParseUPRN(s)
	if err != nil {
		panic(err)
	}
	return u
}

func (u UPRN) String() string { return strconv.FormatUint(uint64(u), 10) }

// MarshalJSON emits a string: twelve digits exceed JavaScript's exact integer
// range and would round to a different property.
func (u UPRN) MarshalJSON() ([]byte, error) { return []byte(`"` + u.String() + `"`), nil }

func (u *UPRN) UnmarshalJSON(b []byte) error {
	p, err := ParseUPRN(strings.Trim(string(b), `"`))
	if err != nil {
		return err
	}
	*u = p
	return nil
}

type PartyID string
type LicenceID string

// PremisesID is a surrogate key. Sub-premises may share a parent UPRN while
// being separately licensable.
type PremisesID string
