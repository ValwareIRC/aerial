package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/timeline"
)

// ErrDuplicateAddress means the address is already on the system.
var ErrDuplicateAddress = errors.New("app: this address is already recorded")

type DuplicateCheck struct {
	Exact []PremisesSummary `json:"exact"`
	Near  []PremisesSummary `json:"near"`
}

// CheckAddress looks for an address already on the system. Exact matches share
// a normalised postcode and first line; near matches share only the postcode.
func (a *App) CheckAddress(addr domain.Address, on timeline.Date) DuplicateCheck {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.checkAddressLocked(addr, on)
}

func (a *App) checkAddressLocked(addr domain.Address, on timeline.Date) DuplicateCheck {
	out := DuplicateCheck{Exact: []PremisesSummary{}, Near: []PremisesSummary{}}
	key := addr.Key()
	for _, p := range a.Premises {
		switch {
		case p.Address.Key() == key:
			out.Exact = append(out.Exact, a.summarise(p, on))
		case p.Address.SamePostcode(addr):
			out.Near = append(out.Near, a.summarise(p, on))
		}
	}
	return out
}

// CreateAddress records a new address, refusing an exact duplicate outright.
// Near matches are returned for the caller to show, and are only accepted once
// acknowledged.
func (a *App) CreateAddress(addr domain.Address, kind domain.PremisesKind, parent ids.PremisesID, units int, acknowledgedNear bool, on timeline.Date) (PremisesSummary, DuplicateCheck, error) {
	addr.Line1 = strings.TrimSpace(addr.Line1)
	addr.Line2 = strings.TrimSpace(addr.Line2)
	addr.Town = strings.TrimSpace(addr.Town)
	addr.Postcode = strings.ToUpper(strings.TrimSpace(addr.Postcode))

	// One lock for the whole operation: checking for a duplicate and then
	// creating the address have to be a single step, or two agents typing the
	// same new address at the same moment both pass the check.
	a.mu.Lock()
	defer a.mu.Unlock()

	check := a.checkAddressLocked(addr, on)
	if len(check.Exact) > 0 {
		return PremisesSummary{}, check, fmt.Errorf("%w: %s", ErrDuplicateAddress, check.Exact[0].AddressLine)
	}
	if len(check.Near) > 0 && !acknowledgedNear {
		return PremisesSummary{}, check, nil
	}

	if units < 1 {
		units = 1
	}
	if parent != "" {
		if _, ok := a.Premises[parent]; !ok {
			return PremisesSummary{}, check, fmt.Errorf("no address with reference %s", parent)
		}
	}

	a.seq++
	id := ids.PremisesID(fmt.Sprintf("P%d", 100+a.seq))
	a.provisionalUPRN++
	uprn := ids.UPRN(domain.ProvisionalUPRNBase + a.provisionalUPRN)

	p := domain.Premises{
		ID: id, UPRN: uprn, Parent: parent, Kind: kind,
		Address: addr, UnitsCovered: units,
	}
	if err := p.Validate(); err != nil {
		return PremisesSummary{}, check, err
	}
	a.Premises[p.ID] = p
	return a.summarise(p, on), check, nil
}
