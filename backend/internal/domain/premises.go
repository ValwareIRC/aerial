package domain

import (
	"fmt"
	"strings"

	"github.com/aerial-project/aerial/internal/ids"
)

type PremisesKind string

const (
	KindDwelling  PremisesKind = "DWELLING"
	KindHMORoom   PremisesKind = "HMO_ROOM"
	KindCareRoom  PremisesKind = "CARE_ROOM"
	KindHotelUnit PremisesKind = "HOTEL_UNIT"
	KindBusiness  PremisesKind = "BUSINESS"
	KindVehicle   PremisesKind = "VEHICLE"
)

// Label is the wording shown to staff.
func (k PremisesKind) Label() string {
	switch k {
	case KindDwelling:
		return "Home"
	case KindHMORoom:
		return "Room in shared house"
	case KindCareRoom:
		return "Care home room"
	case KindHotelUnit:
		return "Hotel room"
	case KindBusiness:
		return "Business"
	case KindVehicle:
		return "Vehicle"
	}
	return string(k)
}

type Address struct {
	Line1    string `json:"line1"`
	Line2    string `json:"line2,omitempty"`
	Town     string `json:"town"`
	Postcode string `json:"postcode"`
}

// SingleLine joins the populated parts with commas.
func (a Address) SingleLine() string {
	parts := []string{}
	for _, s := range []string{a.Line1, a.Line2, a.Town, a.Postcode} {
		if strings.TrimSpace(s) != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

// NormalisedPostcode is uppercase with spaces removed, for comparison.
func (a Address) NormalisedPostcode() string {
	return strings.ToUpper(strings.ReplaceAll(a.Postcode, " ", ""))
}

// Premises is a licensable unit. Sub-premises share a parent's UPRN.
type Premises struct {
	ID      ids.PremisesID `json:"id"`
	UPRN    ids.UPRN       `json:"uprn"`
	Parent  ids.PremisesID `json:"parent,omitempty"`
	Kind    PremisesKind   `json:"kind"`
	Address Address        `json:"address"`
	// UnitsCovered is how many units one licence here covers. A hotel covers 15.
	UnitsCovered int `json:"unitsCovered"`
}

func (p Premises) SeparatelyLicensable() bool {
	switch p.Kind {
	case KindDwelling, KindHMORoom, KindCareRoom, KindBusiness, KindVehicle:
		return true
	case KindHotelUnit:
		return false
	}
	return false
}

func (p Premises) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("domain: premises needs an id")
	}
	if p.UPRN == 0 {
		return fmt.Errorf("domain: premises %s needs a UPRN", p.ID)
	}
	if p.UnitsCovered < 1 {
		return fmt.Errorf("domain: premises %s must cover at least one unit", p.ID)
	}
	if p.Address.Line1 == "" || p.Address.Postcode == "" {
		return fmt.Errorf("domain: premises %s needs a first address line and a postcode", p.ID)
	}
	if p.Kind == KindHotelUnit && p.Parent == "" {
		return fmt.Errorf("domain: hotel room %s needs a parent premises", p.ID)
	}
	return nil
}

// Matches reports whether a search term matches this premises.
func (p Premises) Matches(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	squashed := strings.ReplaceAll(term, " ", "")
	if strings.HasPrefix(strings.ToLower(p.Address.NormalisedPostcode()), squashed) {
		return true
	}
	haystack := strings.ToLower(p.Address.SingleLine() + " " + string(p.ID) + " " + p.UPRN.String())
	return strings.Contains(haystack, term)
}

// Key is the normalised identity of an address: postcode plus first line with
// punctuation and spacing removed. Two records with the same key are the same
// front door recorded twice.
func (a Address) Key() string {
	strip := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	return a.NormalisedPostcode() + "|" + strip(a.Line1)
}

// SamePostcode reports whether two addresses share a postcode, which is the
// test for "near enough to warn about".
func (a Address) SamePostcode(o Address) bool {
	return a.NormalisedPostcode() != "" && a.NormalisedPostcode() == o.NormalisedPostcode()
}

// ProvisionalUPRNBase is where locally-assigned references start. A real
// deployment matches against AddressBase instead; anything in this range has
// not been matched yet.
const ProvisionalUPRNBase = 900000000000

func (p Premises) HasProvisionalUPRN() bool {
	return uint64(p.UPRN) >= ProvisionalUPRNBase
}
