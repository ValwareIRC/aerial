package projection

import (
	"fmt"

	"github.com/aerial-project/aerial/internal/audit"
	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/ids"
)

// PartyHistory is the cross-premises view: what a person needs for a transfer,
// a refund, or a concession that follows them. Service purposes only.
type PartyHistory struct {
	Party       ids.PartyID        `json:"party"`
	Occupancies []domain.Occupancy `json:"occupancies"`
	Licences    []ids.LicenceID    `json:"licences"`
}

// Service answers person-scoped questions.
type Service struct {
	licences  *domain.LicenceRegister
	occupancy *domain.OccupancyRegister
	log       *audit.Log
}

func NewService(licences *domain.LicenceRegister, occupancy *domain.OccupancyRegister, log *audit.Log) *Service {
	return &Service{licences: licences, occupancy: occupancy, log: log}
}

// History returns a party's addresses and licences. Enforcement actors are
// refused; this is the join the enforcement projection exists to prevent.
func (s *Service) History(actor audit.Actor, party ids.PartyID) (PartyHistory, error) {
	if actor.Role == audit.RoleEnforcement {
		s.log.Record(audit.Entry{
			Actor: actor, Action: "service.party_history", Subject: string(party),
			Purpose: "service", Allowed: false, Detail: "enforcement role refused",
		})
		return PartyHistory{}, fmt.Errorf("%w: %s may not read party history", ErrWrongRole, actor.Role)
	}

	h := PartyHistory{Party: party, Occupancies: s.occupancy.HistoryOf(party)}
	seen := map[ids.LicenceID]bool{}
	for _, o := range h.Occupancies {
		// Every licence this party held at this address while they lived
		// there, not only the one in force on the day they moved in: a
		// licence bought mid-tenancy, or renewed during it, is still theirs.
		for _, l := range s.licences.AtPremises(o.Premises) {
			if l.Holder != party || seen[l.ID] {
				continue
			}
			for _, c := range l.EffectiveCoverage() {
				if c.Premises == o.Premises && c.Period.Overlaps(o.Period) {
					seen[l.ID] = true
					h.Licences = append(h.Licences, l.ID)
					break
				}
			}
		}
	}

	s.log.Record(audit.Entry{
		Actor: actor, Action: "service.party_history", Subject: string(party),
		Purpose: "service", Allowed: true,
	})
	return h, nil
}
