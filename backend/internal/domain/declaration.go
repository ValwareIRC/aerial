package domain

import (
	"fmt"

	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/timeline"
)

type DeclarationKind string

const (
	// DeclarationNLN is an occupier's declaration that no licence is needed.
	DeclarationNLN DeclarationKind = "NO_LICENCE_NEEDED"
	// DeclarationLLF is the authority's own determination that none is needed.
	DeclarationLLF DeclarationKind = "LEGALLY_LICENCE_FREE"
	// DeclarationWOIRA withdraws the implied right of access to the premises.
	DeclarationWOIRA DeclarationKind = "ACCESS_WITHDRAWN"
)

func (k DeclarationKind) Valid() bool {
	switch k {
	case DeclarationNLN, DeclarationLLF, DeclarationWOIRA:
		return true
	}
	return false
}

// NLNReviewMonths is how long before an NLN declaration is offered for
// re-confirmation. Passing this date does not revoke it.
const NLNReviewMonths = 24

type DeclarationState string

const (
	StateActive       DeclarationState = "ACTIVE"
	StateDueForReview DeclarationState = "DUE_FOR_REVIEW"
	StateRevoked      DeclarationState = "REVOKED"
)

type Declaration struct {
	ID        string          `json:"id"`
	Premises  ids.PremisesID  `json:"premises"`
	Kind      DeclarationKind `json:"kind"`
	MadeBy    ids.PartyID     `json:"madeBy"`
	MadeOn    timeline.Date   `json:"madeOn"`
	ReviewDue *timeline.Date  `json:"reviewDue,omitempty"`
	RevokedOn *timeline.Date  `json:"revokedOn,omitempty"`
	RevokedBy ids.PartyID     `json:"revokedBy,omitempty"`
}

// NewDeclaration builds a declaration, setting the NLN review date and
// rejecting a review date on a withdrawal of access.
func NewDeclaration(id string, premises ids.PremisesID, kind DeclarationKind, by ids.PartyID, on timeline.Date) (Declaration, error) {
	if !kind.Valid() {
		return Declaration{}, fmt.Errorf("domain: unknown declaration kind %q", kind)
	}
	d := Declaration{ID: id, Premises: premises, Kind: kind, MadeBy: by, MadeOn: on}
	if kind == DeclarationNLN {
		due := on.AddMonths(NLNReviewMonths)
		d.ReviewDue = &due
	}
	return d, nil
}

// StateOn reports the declaration's state. An NLN past its review date is
// DueForReview, which is still active.
func (d Declaration) StateOn(on timeline.Date) DeclarationState {
	if d.RevokedOn != nil && on.OnOrAfter(*d.RevokedOn) {
		return StateRevoked
	}
	if d.ReviewDue != nil && on.OnOrAfter(*d.ReviewDue) {
		return StateDueForReview
	}
	return StateActive
}

// InForceOn reports whether the declaration still stands.
func (d Declaration) InForceOn(on timeline.Date) bool {
	return d.StateOn(on) != StateRevoked
}

// Register holds declarations per premises.
type Register struct {
	byPremises map[ids.PremisesID][]Declaration
}

func NewRegister() *Register {
	return &Register{byPremises: map[ids.PremisesID][]Declaration{}}
}

func (r *Register) Record(d Declaration) error {
	if !d.Kind.Valid() {
		return fmt.Errorf("domain: unknown declaration kind %q", d.Kind)
	}
	if d.Kind == DeclarationWOIRA && d.ReviewDue != nil {
		return fmt.Errorf("domain: a withdrawal of access cannot carry a review date")
	}
	r.byPremises[d.Premises] = append(r.byPremises[d.Premises], d)
	return nil
}

// Revoke withdraws a declaration. A withdrawal of access can only be revoked by
// the party that made it.
func (r *Register) Revoke(premises ids.PremisesID, id string, on timeline.Date, by ids.PartyID) error {
	list := r.byPremises[premises]
	for i := range list {
		if list[i].ID != id {
			continue
		}
		if list[i].RevokedOn != nil {
			return fmt.Errorf("domain: declaration %s is already revoked", id)
		}
		if list[i].Kind == DeclarationWOIRA && list[i].MadeBy != by {
			return fmt.Errorf("domain: a withdrawal of access may only be revoked by the party that made it")
		}
		list[i].RevokedOn = &on
		list[i].RevokedBy = by
		return nil
	}
	return fmt.Errorf("domain: no declaration %s at premises %s", id, premises)
}

func (r *Register) InForce(premises ids.PremisesID, kind DeclarationKind, on timeline.Date) (Declaration, bool) {
	var latest Declaration
	found := false
	for _, d := range r.byPremises[premises] {
		if d.Kind != kind || d.MadeOn.After(on) || !d.InForceOn(on) {
			continue
		}
		if !found || d.MadeOn.OnOrAfter(latest.MadeOn) {
			latest, found = d, true
		}
	}
	return latest, found
}

// VisitPermitted returns ErrVisitProhibited where access has been withdrawn.
func (r *Register) VisitPermitted(premises ids.PremisesID, on timeline.Date) error {
	if d, ok := r.InForce(premises, DeclarationWOIRA, on); ok {
		return fmt.Errorf("%w: declaration %s made %s", ErrVisitProhibited, d.ID, d.MadeOn)
	}
	return nil
}

// NoLicenceDeclared reports whether an NLN or LLF stands, and whether it is due
// for review. Being due for review is not grounds for enforcement.
func (r *Register) NoLicenceDeclared(premises ids.PremisesID, on timeline.Date) (declared bool, dueForReview bool) {
	for _, kind := range []DeclarationKind{DeclarationLLF, DeclarationNLN} {
		if d, ok := r.InForce(premises, kind, on); ok {
			return true, d.StateOn(on) == StateDueForReview
		}
	}
	return false, false
}

func (r *Register) All(premises ids.PremisesID) []Declaration {
	out := make([]Declaration, len(r.byPremises[premises]))
	copy(out, r.byPremises[premises])
	return out
}
