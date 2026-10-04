package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/timeline"
)

// --- case notes ------------------------------------------------------------

// Note is a case note. Append-only: notes are never edited or deleted.
type Note struct {
	ID      string    `json:"id"`
	Subject string    `json:"subject"`
	Text    string    `json:"text"`
	By      string    `json:"by"`
	At      time.Time `json:"at"`
}

// --- correspondence --------------------------------------------------------

type LetterKind string

const (
	LetterLicenceConfirmation LetterKind = "LICENCE_CONFIRMATION"
	LetterRenewalReminder     LetterKind = "RENEWAL_REMINDER"
	LetterPaymentReminder     LetterKind = "PAYMENT_REMINDER"
	LetterDeclarationReceived LetterKind = "DECLARATION_RECEIVED"
	LetterDeclarationReview   LetterKind = "DECLARATION_REVIEW"
	LetterRefundConfirmation  LetterKind = "REFUND_CONFIRMATION"
	LetterDetailsChanged      LetterKind = "DETAILS_CHANGED"
)

func (k LetterKind) Label() string {
	switch k {
	case LetterLicenceConfirmation:
		return "Licence confirmation"
	case LetterRenewalReminder:
		return "Renewal reminder"
	case LetterPaymentReminder:
		return "Payment reminder"
	case LetterDeclarationReceived:
		return "Confirmation that no licence is needed"
	case LetterDeclarationReview:
		return "Check the declaration is still right"
	case LetterRefundConfirmation:
		return "Refund confirmation"
	case LetterDetailsChanged:
		return "Confirmation of changed details"
	}
	return string(k)
}

type Channel string

const (
	ChannelPost  Channel = "POST"
	ChannelEmail Channel = "EMAIL"
)

type Letter struct {
	ID      string        `json:"id"`
	To      ids.PartyID   `json:"to"`
	Kind    LetterKind    `json:"kind"`
	Channel Channel       `json:"channel"`
	SentOn  timeline.Date `json:"sentOn"`
	About   string        `json:"about,omitempty"`
	Address string        `json:"address,omitempty"`
}

// --- concession evidence ---------------------------------------------------

type EvidenceKind string

const (
	EvidenceBlindRegistration EvidenceKind = "BLIND_REGISTRATION"
	EvidencePensionCredit     EvidenceKind = "PENSION_CREDIT"
	EvidenceCareHome          EvidenceKind = "CARE_HOME_PLACE"
)

func (k EvidenceKind) Label() string {
	switch k {
	case EvidenceBlindRegistration:
		return "Certificate of sight impairment"
	case EvidencePensionCredit:
		return "Pension Credit award letter"
	case EvidenceCareHome:
		return "Confirmation of a care home place"
	}
	return string(k)
}

// Supports says which concession an evidence kind unlocks.
func (k EvidenceKind) Supports() Concession {
	switch k {
	case EvidenceBlindRegistration:
		return ConcessionBlind50
	case EvidencePensionCredit:
		return ConcessionOver75PensionCredit
	}
	return ConcessionNone
}

type Evidence struct {
	ID        string         `json:"id"`
	Party     ids.PartyID    `json:"party"`
	Kind      EvidenceKind   `json:"kind"`
	Reference string         `json:"reference,omitempty"`
	SeenOn    timeline.Date  `json:"seenOn"`
	ExpiresOn *timeline.Date `json:"expiresOn,omitempty"`
	SeenBy    string         `json:"seenBy"`
}

func (e Evidence) InDateOn(on timeline.Date) bool {
	if on.Before(e.SeenOn) {
		return false
	}
	return e.ExpiresOn == nil || on.Before(*e.ExpiresOn)
}

// --- visits ----------------------------------------------------------------

type VisitOutcome string

const (
	OutcomeNoAnswer         VisitOutcome = "NO_ANSWER"
	OutcomeLicenceSold      VisitOutcome = "LICENCE_SOLD"
	OutcomeDeclarationTaken VisitOutcome = "DECLARATION_TAKEN"
	OutcomeAlreadyLicensed  VisitOutcome = "ALREADY_LICENSED"
	OutcomeNotRequired      VisitOutcome = "NOT_REQUIRED"
	OutcomeRefused          VisitOutcome = "REFUSED"
)

func (o VisitOutcome) Label() string {
	switch o {
	case OutcomeNoAnswer:
		return "No answer"
	case OutcomeLicenceSold:
		return "Licence bought"
	case OutcomeDeclarationTaken:
		return "Occupier said no licence is needed"
	case OutcomeAlreadyLicensed:
		return "Already licensed"
	case OutcomeNotRequired:
		return "No licence needed"
	case OutcomeRefused:
		return "Occupier did not want to talk"
	}
	return string(o)
}

func (o VisitOutcome) Valid() bool { return o.Label() != string(o) }

type Visit struct {
	ID       string         `json:"id"`
	Premises ids.PremisesID `json:"premises"`
	On       timeline.Date  `json:"on"`
	Officer  string         `json:"officer"`
	Outcome  VisitOutcome   `json:"outcome"`
	Note     string         `json:"note,omitempty"`
}

// --- cancellation ----------------------------------------------------------

type CancellationReason string

const (
	CancelMovedAbroad   CancellationReason = "MOVED_ABROAD"
	CancelNoLongerWatch CancellationReason = "NO_LONGER_WATCHING"
	CancelDeceased      CancellationReason = "LICENCE_HOLDER_DIED"
	CancelOver75        CancellationReason = "NOW_ELIGIBLE_FREE"
	CancelIssuedInError CancellationReason = "ISSUED_IN_ERROR"
)

func (r CancellationReason) Label() string {
	switch r {
	case CancelMovedAbroad:
		return "Moved abroad"
	case CancelNoLongerWatch:
		return "No longer watches live TV or BBC iPlayer"
	case CancelDeceased:
		return "The licence holder has died"
	case CancelOver75:
		return "Now entitled to a free licence"
	case CancelIssuedInError:
		return "Issued in error"
	}
	return string(r)
}

func (r CancellationReason) Valid() bool { return r.Label() != string(r) }

// CancellationReasons lists every reason a licence may be surrendered for, in
// the order the console offers them. One list, so the options a user is shown
// and the reasons the server accepts cannot drift apart.
func CancellationReasons() []CancellationReason {
	return []CancellationReason{
		CancelNoLongerWatch, CancelMovedAbroad, CancelDeceased, CancelOver75, CancelIssuedInError,
	}
}

// RefundsWholeFee reports whether the whole fee comes back rather than only the
// complete months remaining.
func (r CancellationReason) RefundsWholeFee() bool { return r == CancelIssuedInError }

// --- store -----------------------------------------------------------------

type Records struct {
	notes    []Note
	letters  []Letter
	evidence []Evidence
	visits   []Visit
	seq      int
	now      func() time.Time
}

func NewRecords(now func() time.Time) *Records {
	if now == nil {
		now = time.Now
	}
	return &Records{now: now}
}

func (r *Records) id(prefix string) string {
	r.seq++
	return fmt.Sprintf("%s%d", prefix, r.seq)
}

func (r *Records) AddNote(subject, text, by string) (Note, error) {
	if strings.TrimSpace(text) == "" {
		return Note{}, fmt.Errorf("domain: a note cannot be empty")
	}
	n := Note{ID: r.id("N"), Subject: subject, Text: strings.TrimSpace(text), By: by, At: r.now().UTC()}
	r.notes = append(r.notes, n)
	return n, nil
}

func (r *Records) Notes(subject string) []Note {
	out := []Note{}
	for _, n := range r.notes {
		if n.Subject == subject {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

func (r *Records) Send(to ids.PartyID, kind LetterKind, channel Channel, on timeline.Date, about, address string) Letter {
	l := Letter{ID: r.id("LTR"), To: to, Kind: kind, Channel: channel, SentOn: on, About: about, Address: address}
	r.letters = append(r.letters, l)
	return l
}

func (r *Records) Letters(to ids.PartyID) []Letter {
	out := []Letter{}
	for _, l := range r.letters {
		if l.To == to {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SentOn.After(out[j].SentOn) })
	return out
}

func (r *Records) AddEvidence(e Evidence) (Evidence, error) {
	if e.Party == "" {
		return Evidence{}, fmt.Errorf("domain: evidence needs a person")
	}
	if e.Kind.Label() == string(e.Kind) {
		return Evidence{}, fmt.Errorf("domain: unknown evidence type %q", e.Kind)
	}
	e.ID = r.id("EV")
	r.evidence = append(r.evidence, e)
	return e, nil
}

func (r *Records) EvidenceFor(party ids.PartyID) []Evidence {
	out := []Evidence{}
	for _, e := range r.evidence {
		if e.Party == party {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SeenOn.After(out[j].SeenOn) })
	return out
}

// SupportsConcession reports whether in-date evidence exists for a concession.
func (r *Records) SupportsConcession(party ids.PartyID, c Concession, on timeline.Date) bool {
	if c == ConcessionNone {
		return true
	}
	for _, e := range r.evidence {
		if e.Party == party && e.Kind.Supports() == c && e.InDateOn(on) {
			return true
		}
	}
	return false
}

func (r *Records) RecordVisit(v Visit) (Visit, error) {
	if !v.Outcome.Valid() {
		return Visit{}, fmt.Errorf("domain: unknown visit outcome %q", v.Outcome)
	}
	v.ID = r.id("V")
	r.visits = append(r.visits, v)
	return v, nil
}

func (r *Records) Visits(premises ids.PremisesID) []Visit {
	out := []Visit{}
	for _, v := range r.visits {
		if v.Premises == premises {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].On.After(out[j].On) })
	return out
}

func (r *Records) AllVisits() []Visit {
	out := make([]Visit, len(r.visits))
	copy(out, r.visits)
	return out
}
