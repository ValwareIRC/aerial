package domain

import (
	"fmt"
	"sort"

	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/timeline"
)

// Occupancy is a party's association with a premises for a period. It is a
// separate fact from the licence, with its own lifetime.
type Occupancy struct {
	Party    ids.PartyID    `json:"party"`
	Premises ids.PremisesID `json:"premises"`
	Period   timeline.Range `json:"period"`
}

type OccupancyRegister struct {
	all []*Occupancy
}

func NewOccupancyRegister() *OccupancyRegister { return &OccupancyRegister{} }

func (r *OccupancyRegister) Record(o Occupancy) error {
	if o.Party == "" || o.Premises == "" {
		return fmt.Errorf("domain: occupancy needs a party and a premises")
	}
	copied := o
	r.all = append(r.all, &copied)
	return nil
}

// End closes every open occupancy for a party on a date.
func (r *OccupancyRegister) End(party ids.PartyID, on timeline.Date) error {
	for _, o := range r.all {
		if o.Party != party || !o.Period.IsOpen() || !o.Period.Contains(on) {
			continue
		}
		closed, err := timeline.NewRange(o.Period.Start, &on)
		if err != nil {
			return err
		}
		o.Period = closed
	}
	return nil
}

func (r *OccupancyRegister) OccupantsOn(premises ids.PremisesID, on timeline.Date) []ids.PartyID {
	var out []ids.PartyID
	for _, o := range r.all {
		if o.Premises == premises && o.Period.Contains(on) {
			out = append(out, o.Party)
		}
	}
	return out
}

// HistoryOf returns every premises a party has occupied, most recent first.
func (r *OccupancyRegister) HistoryOf(party ids.PartyID) []Occupancy {
	out := []Occupancy{}
	for _, o := range r.all {
		if o.Party == party {
			out = append(out, *o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Period.Start.After(out[j].Period.Start) })
	return out
}

func (r *OccupancyRegister) PremisesOf(party ids.PartyID, on timeline.Date) []ids.PremisesID {
	var out []ids.PremisesID
	for _, o := range r.all {
		if o.Party == party && o.Period.Contains(on) {
			out = append(out, o.Premises)
		}
	}
	return out
}
