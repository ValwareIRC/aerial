package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/timeline"
)

type PremisesSummary struct {
	ID              ids.PremisesID      `json:"id"`
	UPRN            ids.UPRN            `json:"uprn"`
	Kind            domain.PremisesKind `json:"kind"`
	KindLabel       string              `json:"kindLabel"`
	Address         domain.Address      `json:"address"`
	AddressLine     string              `json:"addressLine"`
	Occupier        string              `json:"occupier"`
	Licensed        bool                `json:"licensed"`
	LicenceNumber   string              `json:"licenceNumber,omitempty"`
	LicenceID       ids.LicenceID       `json:"licenceId,omitempty"`
	Expiry          *timeline.Date      `json:"expiry,omitempty"`
	Declared        bool                `json:"declared"`
	DueForReview    bool                `json:"dueForReview"`
	AccessWithdrawn bool                `json:"accessWithdrawn"`
	CoveredByParent bool                `json:"coveredByParent"`
}

// Status is the single word shown in a results list.
func (s PremisesSummary) Status() string {
	switch {
	case s.CoveredByParent:
		return "Covered by main licence"
	case s.Licensed:
		return "Licensed"
	case s.Declared:
		return "No licence needed"
	default:
		return "No licence"
	}
}

type PremisesDetail struct {
	Premises     domain.Premises      `json:"premises"`
	Summary      PremisesSummary      `json:"summary"`
	Licences     []LicenceSummary     `json:"licences"`
	Declarations []domain.Declaration `json:"declarations"`
	Occupants    []domain.Party       `json:"occupants"`
	Parent       *PremisesSummary     `json:"parent,omitempty"`
}

type LicenceSummary struct {
	ID          ids.LicenceID        `json:"id"`
	Number      string               `json:"number"`
	Holder      ids.PartyID          `json:"holder"`
	HolderName  string               `json:"holderName"`
	Class       domain.LicenceClass  `json:"class"`
	ClassLabel  string               `json:"classLabel"`
	Concession  domain.Concession    `json:"concession"`
	Status      domain.LicenceStatus `json:"status"`
	Premises    ids.PremisesID       `json:"premises"`
	AddressLine string               `json:"addressLine"`
	Start       timeline.Date        `json:"start"`
	Expiry      timeline.Date        `json:"expiry"`
	FeePaid     int64                `json:"feePaid"`
}

// lessAddress orders address lines the way a person reads them: "2 Rowan
// Close" before "12 Rowan Close", not after it. Runs of digits compare as
// numbers, everything else case-insensitively as text.
func lessAddress(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ac, bc := a[ai], b[bi]
		if isDigit(ac) && isDigit(bc) {
			an, aNext := readNumber(a, ai)
			bn, bNext := readNumber(b, bi)
			if an != bn {
				return an < bn
			}
			ai, bi = aNext, bNext
			continue
		}
		al, bl := lowerByte(ac), lowerByte(bc)
		if al != bl {
			return al < bl
		}
		ai++
		bi++
	}
	return len(a)-ai < len(b)-bi
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func lowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// readNumber reads the run of digits starting at i. Runs longer than can be
// held in an int are compared by length, which is enough to order addresses.
func readNumber(s string, i int) (value int, next int) {
	for i < len(s) && s[i] == '0' {
		i++
	}
	start := i
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	digits := s[start:i]
	if len(digits) > 9 {
		return 1 << 30, i
	}
	for _, d := range []byte(digits) {
		value = value*10 + int(d-'0')
	}
	return value, i
}

func classLabel(c domain.LicenceClass) string {
	switch c {
	case domain.ClassColour:
		return "Standard"
	case domain.ClassMonochrome:
		return "Black and white"
	case domain.ClassARC:
		return "Care home resident"
	}
	return string(c)
}

// ConcessionLabel is the wording shown to staff.
func ConcessionLabel(c domain.Concession) string {
	switch c {
	case domain.ConcessionNone:
		return "None"
	case domain.ConcessionBlind50:
		return "Sight impaired — half price"
	case domain.ConcessionOver75PensionCredit:
		return "Over 75 on Pension Credit — free"
	}
	return string(c)
}

func (a *App) summarise(p domain.Premises, on timeline.Date) PremisesSummary {
	s := PremisesSummary{
		ID: p.ID, UPRN: p.UPRN, Kind: p.Kind, KindLabel: p.Kind.Label(),
		Address: p.Address, AddressLine: p.Address.SingleLine(),
		CoveredByParent: !p.SeparatelyLicensable(),
	}

	if names := a.occupantNames(p.ID, on); len(names) > 0 {
		s.Occupier = strings.Join(names, ", ")
	}
	if l, ok := a.Licences.LicensedOn(p.ID, on); ok {
		expiry := l.Expiry()
		s.Licensed = true
		s.LicenceID = l.ID
		s.LicenceNumber = l.Number
		s.Expiry = &expiry
	}
	s.Declared, s.DueForReview = a.Declarations.NoLicenceDeclared(p.ID, on)
	s.AccessWithdrawn = a.Declarations.VisitPermitted(p.ID, on) != nil
	return s
}

func (a *App) occupantNames(id ids.PremisesID, on timeline.Date) []string {
	var out []string
	for _, party := range a.Occupancy.OccupantsOn(id, on) {
		out = append(out, a.Parties.Name(party))
	}
	sort.Strings(out)
	return out
}

func (a *App) licenceSummary(l *domain.Licence, on timeline.Date) LicenceSummary {
	coverage := l.EffectiveCoverage()
	var current ids.PremisesID
	if len(coverage) > 0 {
		current = coverage[len(coverage)-1].Premises
	}
	line := ""
	if p, ok := a.Premises[current]; ok {
		line = p.Address.SingleLine()
	}
	return LicenceSummary{
		ID: l.ID, Number: l.Number, Holder: l.Holder, HolderName: a.Parties.Name(l.Holder),
		Class: l.Class, ClassLabel: classLabel(l.Class), Concession: l.Concession,
		Status: l.StatusOn(on), Premises: current, AddressLine: line,
		Start: l.Term.Start, Expiry: l.Expiry(), FeePaid: l.FeePaid.Pence(),
	}
}

// Search matches an address, a postcode, an occupier's surname, or a licence
// number. An empty term returns everything.
func (a *App) Search(term string, on timeline.Date) []PremisesSummary {
	a.mu.Lock()
	defer a.mu.Unlock()

	term = strings.TrimSpace(term)
	digits := strings.ReplaceAll(term, " ", "")

	out := make([]PremisesSummary, 0, len(a.Premises))
	for _, p := range a.Premises {
		s := a.summarise(p, on)
		match := p.Matches(term)

		if !match && digits != "" && s.LicenceNumber != "" {
			match = strings.HasPrefix(strings.ReplaceAll(s.LicenceNumber, " ", ""), digits)
		}
		if !match && term != "" {
			for _, name := range a.occupantNames(p.ID, on) {
				if strings.Contains(strings.ToLower(name), strings.ToLower(term)) {
					match = true
					break
				}
			}
		}
		if match {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return lessAddress(out[i].AddressLine, out[j].AddressLine) })
	return out
}

func (a *App) PremisesDetail(id ids.PremisesID, on timeline.Date) (PremisesDetail, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	p, ok := a.Premises[id]
	if !ok {
		return PremisesDetail{}, fmt.Errorf("no premises %s", id)
	}

	detail := PremisesDetail{
		Premises:     p,
		Summary:      a.summarise(p, on),
		Declarations: a.Declarations.All(id),
		Licences:     []LicenceSummary{},
		Occupants:    []domain.Party{},
	}
	if detail.Declarations == nil {
		detail.Declarations = []domain.Declaration{}
	}
	for _, party := range a.Occupancy.OccupantsOn(id, on) {
		if pt, ok := a.Parties.Get(party); ok {
			detail.Occupants = append(detail.Occupants, pt)
		}
	}
	for _, l := range a.Licences.AtPremises(id) {
		detail.Licences = append(detail.Licences, a.licenceSummary(l, on))
	}
	sort.Slice(detail.Licences, func(i, j int) bool {
		return detail.Licences[i].Start.After(detail.Licences[j].Start)
	})
	if p.Parent != "" {
		if parent, ok := a.Premises[p.Parent]; ok {
			s := a.summarise(parent, on)
			detail.Parent = &s
		}
	}
	return detail, nil
}

func (a *App) ListLicences(on timeline.Date) []LicenceSummary {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := []LicenceSummary{}
	for _, l := range a.Licences.All() {
		out = append(out, a.licenceSummary(l, on))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// LicenceDetail is everything the licence page shows. It is assembled here,
// under the lock, so no caller holds a pointer into the register and no caller
// has to work out money for itself.
type LicenceDetail struct {
	Summary  LicenceSummary    `json:"summary"`
	Coverage []domain.Coverage `json:"coverage"`
	Account  AccountView       `json:"account"`
	// Refund is what a surrender today pays back for the ordinary reason, and
	// Refunds is the same figure for every reason, keyed by reason.
	Refund  int64            `json:"refund"`
	Refunds map[string]int64 `json:"refunds"`
}

func (a *App) LicenceDetail(id ids.LicenceID, on timeline.Date) (LicenceDetail, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	l, ok := a.Licences.Get(id)
	if !ok {
		return LicenceDetail{}, fmt.Errorf("no licence %s", id)
	}

	coverage := make([]domain.Coverage, len(l.Coverage))
	copy(coverage, l.Coverage)

	refunds := map[string]int64{}
	for reason, amount := range l.RefundsByReason(on) {
		refunds[string(reason)] = amount.Pence()
	}

	return LicenceDetail{
		Summary:  a.licenceSummary(l, on),
		Coverage: coverage,
		Account:  a.accountView(l, on),
		Refund:   l.RefundDue(on, domain.CancelNoLongerWatch).Pence(),
		Refunds:  refunds,
	}, nil
}

// LicenceSummaryOf is the one-line view of a licence, for the response to an
// operation that has just changed it.
func (a *App) LicenceSummaryOf(id ids.LicenceID, on timeline.Date) (LicenceSummary, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	l, ok := a.Licences.Get(id)
	if !ok {
		return LicenceSummary{}, fmt.Errorf("no licence %s", id)
	}
	return a.licenceSummary(l, on), nil
}

// WorklistItem is what an enforcement officer sees in a list: an address and
// whether it is licensed. No occupier names, no licence numbers.
type WorklistItem struct {
	ID             ids.PremisesID `json:"id"`
	AddressLine    string         `json:"addressLine"`
	Postcode       string         `json:"postcode"`
	Status         string         `json:"status"`
	VisitPermitted bool           `json:"visitPermitted"`
	DueForReview   bool           `json:"dueForReview"`
}

func (a *App) Worklist(term string, on timeline.Date) []WorklistItem {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := []WorklistItem{}
	for _, p := range a.Premises {
		if !p.SeparatelyLicensable() || !p.Matches(term) {
			continue
		}
		s := a.summarise(p, on)
		out = append(out, WorklistItem{
			ID: p.ID, AddressLine: p.Address.SingleLine(), Postcode: p.Address.Postcode,
			Status: s.Status(), VisitPermitted: !s.AccessWithdrawn, DueForReview: s.DueForReview,
		})
	}
	sort.Slice(out, func(i, j int) bool { return lessAddress(out[i].AddressLine, out[j].AddressLine) })
	return out
}

// Overview is the landing page: the counts, and the three lists the counts
// refer to. The lists are built here rather than by filtering a download of
// the whole register in the client, so the page asks one question and the
// definition of "outstanding" lives in one place.
type Overview struct {
	Premises        int `json:"premises"`
	Licensed        int `json:"licensed"`
	NoLicenceNeeded int `json:"noLicenceNeeded"`
	Unlicensed      int `json:"unlicensed"`
	DueForReview    int `json:"dueForReview"`
	AccessWithdrawn int `json:"accessWithdrawn"`
	ExpiringSoon    int `json:"expiringSoon"`

	// NoLicenceList, ReviewList and DoNotVisitList are the addresses behind
	// Unlicensed, DueForReview and AccessWithdrawn respectively.
	NoLicenceList  []PremisesSummary `json:"noLicenceList"`
	ReviewList     []PremisesSummary `json:"reviewList"`
	DoNotVisitList []PremisesSummary `json:"doNotVisitList"`
}

func (a *App) Overview(on timeline.Date) Overview {
	a.mu.Lock()
	defer a.mu.Unlock()

	o := Overview{
		NoLicenceList:  []PremisesSummary{},
		ReviewList:     []PremisesSummary{},
		DoNotVisitList: []PremisesSummary{},
	}
	soon := on.AddMonths(2)
	for _, p := range a.Premises {
		s := a.summarise(p, on)
		if s.CoveredByParent {
			continue
		}
		o.Premises++
		switch {
		case s.Licensed:
			o.Licensed++
			if s.Expiry != nil && s.Expiry.Before(soon) {
				o.ExpiringSoon++
			}
		case s.Declared:
			o.NoLicenceNeeded++
		default:
			o.Unlicensed++
			o.NoLicenceList = append(o.NoLicenceList, s)
		}
		if s.DueForReview {
			o.DueForReview++
			o.ReviewList = append(o.ReviewList, s)
		}
		if s.AccessWithdrawn {
			o.AccessWithdrawn++
			o.DoNotVisitList = append(o.DoNotVisitList, s)
		}
	}
	for _, list := range [][]PremisesSummary{o.NoLicenceList, o.ReviewList, o.DoNotVisitList} {
		sort.Slice(list, func(i, j int) bool { return lessAddress(list[i].AddressLine, list[j].AddressLine) })
	}
	return o
}
