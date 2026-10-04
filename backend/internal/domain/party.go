package domain

import (
	"fmt"
	"strings"

	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/timeline"
)

type NameChangeReason string

const (
	NameChangeMarriage   NameChangeReason = "MARRIAGE"
	NameChangeDeedPoll   NameChangeReason = "DEED_POLL"
	NameChangeCorrection NameChangeReason = "CORRECTION"
)

func (r NameChangeReason) Label() string {
	switch r {
	case NameChangeMarriage:
		return "Marriage or civil partnership"
	case NameChangeDeedPoll:
		return "Changed by deed poll"
	case NameChangeCorrection:
		return "Correcting a mistake"
	}
	return string(r)
}

type NameChange struct {
	On     timeline.Date    `json:"on"`
	From   string           `json:"from"`
	To     string           `json:"to"`
	Reason NameChangeReason `json:"reason"`
	By     string           `json:"by"`
}

type Party struct {
	ID           ids.PartyID    `json:"id"`
	Title        string         `json:"title,omitempty"`
	Forename     string         `json:"forename,omitempty"`
	Surname      string         `json:"surname,omitempty"`
	Organisation string         `json:"organisation,omitempty"`
	DateOfBirth  *timeline.Date `json:"dateOfBirth,omitempty"`
	Deceased     *timeline.Date `json:"deceased,omitempty"`

	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Paperless bool   `json:"paperless"`
	// Correspondence overrides where letters go. Nil means the licensed address.
	Correspondence *Address     `json:"correspondence,omitempty"`
	NameHistory    []NameChange `json:"nameHistory,omitempty"`
}

func (p Party) Name() string {
	if p.Organisation != "" {
		return p.Organisation
	}
	parts := []string{}
	for _, s := range []string{p.Title, p.Forename, p.Surname} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

func (p Party) SortName() string {
	if p.Organisation != "" {
		return p.Organisation
	}
	lead := strings.TrimSpace(p.Title + " " + p.Forename)
	if lead == "" {
		return p.Surname
	}
	return p.Surname + ", " + lead
}

// AgeOn returns the party's age, or -1 if the date of birth is not held.
func (p Party) AgeOn(on timeline.Date) int {
	if p.DateOfBirth == nil {
		return -1
	}
	years := on.Year() - p.DateOfBirth.Year()
	if on.Month() < p.DateOfBirth.Month() ||
		(on.Month() == p.DateOfBirth.Month() && on.Day() < p.DateOfBirth.Day()) {
		years--
	}
	return years
}

// WritableOn reports whether letters may still be sent.
func (p Party) WritableOn(on timeline.Date) bool {
	return p.Deceased == nil || on.Before(*p.Deceased)
}

func (p Party) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("domain: party needs an id")
	}
	if p.Surname == "" && p.Organisation == "" {
		return fmt.Errorf("domain: party %s needs a surname or an organisation name", p.ID)
	}
	if p.Email != "" && !strings.Contains(p.Email, "@") {
		return fmt.Errorf("domain: %q is not an email address", p.Email)
	}
	return nil
}

type PartyRegister struct {
	byID map[ids.PartyID]*Party
}

func NewPartyRegister() *PartyRegister {
	return &PartyRegister{byID: map[ids.PartyID]*Party{}}
}

func (r *PartyRegister) Add(p Party) error {
	if err := p.Validate(); err != nil {
		return err
	}
	copied := p
	r.byID[p.ID] = &copied
	return nil
}

func (r *PartyRegister) Get(id ids.PartyID) (Party, bool) {
	p, ok := r.byID[id]
	if !ok {
		return Party{}, false
	}
	return *p, true
}

func (r *PartyRegister) All() []Party {
	out := make([]Party, 0, len(r.byID))
	for _, p := range r.byID {
		out = append(out, *p)
	}
	return out
}

func (r *PartyRegister) Name(id ids.PartyID) string {
	if p, ok := r.byID[id]; ok {
		return p.Name()
	}
	return string(id)
}

// Rename records a change of name and keeps the previous one in history.
func (r *PartyRegister) Rename(id ids.PartyID, title, forename, surname string, reason NameChangeReason, on timeline.Date, by string) error {
	p, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("domain: no party %s", id)
	}
	if surname == "" {
		return fmt.Errorf("domain: a surname is required")
	}
	before := p.Name()
	p.Title, p.Forename, p.Surname = title, forename, surname
	p.NameHistory = append(p.NameHistory, NameChange{
		On: on, From: before, To: p.Name(), Reason: reason, By: by,
	})
	return nil
}

func (r *PartyRegister) UpdateContact(id ids.PartyID, email, phone string, paperless bool, correspondence *Address) error {
	p, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("domain: no party %s", id)
	}
	if email != "" && !strings.Contains(email, "@") {
		return fmt.Errorf("domain: %q is not an email address", email)
	}
	if paperless && email == "" {
		return fmt.Errorf("domain: an email address is needed for paperless")
	}
	p.Email, p.Phone, p.Paperless, p.Correspondence = email, phone, paperless, correspondence
	return nil
}

// RecordDeath marks a party deceased. Letters stop from that date.
func (r *PartyRegister) RecordDeath(id ids.PartyID, on timeline.Date) error {
	p, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("domain: no party %s", id)
	}
	p.Deceased = &on
	p.Paperless = false
	return nil
}

func (r *PartyRegister) Match(term string) []Party {
	term = strings.ToLower(strings.TrimSpace(term))
	var out []Party
	for _, p := range r.byID {
		if term == "" ||
			strings.Contains(strings.ToLower(p.Name()), term) ||
			strings.Contains(strings.ToLower(p.Email), term) ||
			strings.Contains(strings.ToLower(string(p.ID)), term) {
			out = append(out, *p)
		}
	}
	return out
}

// ClearDeath undoes a death recorded in error.
func (r *PartyRegister) ClearDeath(id ids.PartyID) error {
	p, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("domain: no party %s", id)
	}
	if p.Deceased == nil {
		return fmt.Errorf("domain: %s is not recorded as died", id)
	}
	p.Deceased = nil
	return nil
}
