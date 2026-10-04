// Package domain holds the licensing model: premises, parties, occupancies,
// entitlements, declarations and licences. A licence attaches to a premises; a
// party attaches to a premises only for a period. See docs/DOMAIN.md.
package domain

import "errors"

var (
	// ErrRateNotKnown means no rate is recorded for that class and date. An
	// error rather than a fallback, so a data gap never becomes a wrong charge.
	ErrRateNotKnown = errors.New("domain: no fee rate recorded for that class and date")

	// ErrConcessionPolicyUnknown means the class/concession combination is not
	// in the published policy table.
	ErrConcessionPolicyUnknown = errors.New("domain: no published policy for that class and concession")

	ErrConcessionNotEvidenced = errors.New("domain: concession claimed without in-date evidence")

	// ErrNotLicensable means the premises cannot hold a licence in its own
	// right, e.g. a room covered by its parent property's licence.
	ErrNotLicensable = errors.New("domain: premises is not separately licensable")

	ErrLicenceState = errors.New("domain: operation not valid for licence in this state")

	ErrOverlappingLicence = errors.New("domain: premises already licensed for part of that period")

	// ErrVisitProhibited means the implied right of access has been withdrawn.
	ErrVisitProhibited = errors.New("domain: implied right of access has been withdrawn for this premises")
)
