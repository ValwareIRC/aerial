// Package projection splits reads by purpose. Service sees people across
// addresses; Enforcement sees one premises at a time and cannot join across
// them. See docs/DOMAIN.md.
package projection

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/aerial-project/aerial/internal/audit"
	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/timeline"
)

var (
	// ErrNoSalt means the deployment has no pseudonymisation secret configured.
	ErrNoSalt = errors.New("projection: enforcement projection requires a salt")
	// ErrWrongRole means the actor's role does not match the projection.
	ErrWrongRole = errors.New("projection: actor role is not permitted on this projection")
)

// PremisesSnapshot is everything enforcement may see about one premises.
// Occupants appear only as per-premises tokens.
type PremisesSnapshot struct {
	Premises          ids.PremisesID `json:"premises"`
	On                timeline.Date  `json:"on"`
	Licensed          bool           `json:"licensed"`
	LicenceExpiry     *timeline.Date `json:"licenceExpiry,omitempty"`
	NoLicenceDeclared bool           `json:"noLicenceDeclared"`
	DueForReview      bool           `json:"dueForReview"`
	VisitPermitted    bool           `json:"visitPermitted"`
	OccupantTokens    []string       `json:"occupantTokens"`
}

// Enforcement answers premises-scoped questions. It holds no method that takes
// a party identifier; see TestEnforcementCannotBeAskedAboutAParty.
type Enforcement struct {
	licences     *domain.LicenceRegister
	declarations *domain.Register
	occupancy    *domain.OccupancyRegister
	log          *audit.Log
	salt         []byte
}

func NewEnforcement(
	licences *domain.LicenceRegister,
	declarations *domain.Register,
	occupancy *domain.OccupancyRegister,
	log *audit.Log,
	salt []byte,
) (*Enforcement, error) {
	if len(salt) == 0 {
		return nil, ErrNoSalt
	}
	return &Enforcement{licences: licences, declarations: declarations, occupancy: occupancy, log: log, salt: salt}, nil
}

// Snapshot returns the premises-scoped view, recording the read.
func (e *Enforcement) Snapshot(actor audit.Actor, premises ids.PremisesID, on timeline.Date) (PremisesSnapshot, error) {
	if actor.Role != audit.RoleEnforcement {
		e.record(actor, "enforcement.snapshot", string(premises), false, "wrong role")
		return PremisesSnapshot{}, fmt.Errorf("%w: %s", ErrWrongRole, actor.Role)
	}

	snap := PremisesSnapshot{Premises: premises, On: on}

	if l, ok := e.licences.LicensedOn(premises, on); ok {
		expiry := l.Expiry()
		snap.Licensed = true
		snap.LicenceExpiry = &expiry
	}

	declared, due := e.declarations.NoLicenceDeclared(premises, on)
	snap.NoLicenceDeclared = declared
	snap.DueForReview = due
	snap.VisitPermitted = e.declarations.VisitPermitted(premises, on) == nil

	for _, p := range e.occupancy.OccupantsOn(premises, on) {
		snap.OccupantTokens = append(snap.OccupantTokens, e.token(premises, p))
	}

	e.record(actor, "enforcement.snapshot", string(premises), true, "")
	return snap, nil
}

// VisitPermitted reports whether a visit may be made, recording the check.
// Like Snapshot, it is refused to any actor outside the enforcement role: the
// projection answers enforcement questions only, whichever method is called.
func (e *Enforcement) VisitPermitted(actor audit.Actor, premises ids.PremisesID, on timeline.Date) error {
	if actor.Role != audit.RoleEnforcement {
		e.record(actor, "enforcement.visit_check", string(premises), false, "wrong role")
		return fmt.Errorf("%w: %s", ErrWrongRole, actor.Role)
	}
	err := e.declarations.VisitPermitted(premises, on)
	e.record(actor, "enforcement.visit_check", string(premises), err == nil, errText(err))
	return err
}

// token is a per-premises pseudonym. The same party at two premises yields two
// unrelated tokens, so enforcement reads cannot be joined into a person.
func (e *Enforcement) token(premises ids.PremisesID, party ids.PartyID) string {
	mac := hmac.New(sha256.New, e.salt)
	mac.Write([]byte(premises))
	mac.Write([]byte{0})
	mac.Write([]byte(party))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))[:16]
}

func (e *Enforcement) record(actor audit.Actor, action, subject string, allowed bool, detail string) {
	e.log.Record(audit.Entry{
		Actor: actor, Action: action, Subject: subject,
		Purpose: "enforcement", Allowed: allowed, Detail: detail,
	})
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
