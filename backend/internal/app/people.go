package app

import (
	"fmt"
	"sort"

	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/timeline"
)

type PersonSummary struct {
	ID          ids.PartyID    `json:"id"`
	Name        string         `json:"name"`
	SortName    string         `json:"sortName"`
	Address     string         `json:"address"`
	Email       string         `json:"email,omitempty"`
	Phone       string         `json:"phone,omitempty"`
	Deceased    *timeline.Date `json:"deceased,omitempty"`
	LicenceHeld bool           `json:"licenceHeld"`
}

type AddressHistoryItem struct {
	Premises    ids.PremisesID `json:"premises"`
	AddressLine string         `json:"addressLine"`
	From        timeline.Date  `json:"from"`
	To          *timeline.Date `json:"to,omitempty"`
	Current     bool           `json:"current"`
}

type AccountView struct {
	Licence        ids.LicenceID           `json:"licence"`
	LicenceNumber  string                  `json:"licenceNumber"`
	Method         domain.PaymentMethod    `json:"method"`
	MethodLabel    string                  `json:"methodLabel"`
	Charged        money.Money             `json:"charged"`
	Paid           money.Money             `json:"paid"`
	Balance        money.Money             `json:"balance"`
	Arrears        money.Money             `json:"arrears"`
	Mandate        *domain.DirectDebit     `json:"mandate,omitempty"`
	MandateMasked  string                  `json:"mandateMasked,omitempty"`
	NextCollection *domain.InstalmentView  `json:"nextCollection,omitempty"`
	Instalments    []domain.InstalmentView `json:"instalments,omitempty"`
	Payments       []domain.Payment        `json:"payments"`
}

type PersonDetail struct {
	Party       domain.Party         `json:"party"`
	Name        string               `json:"name"`
	Addresses   []AddressHistoryItem `json:"addresses"`
	Licences    []LicenceSummary     `json:"licences"`
	Accounts    []AccountView        `json:"accounts"`
	Letters     []domain.Letter      `json:"letters"`
	Notes       []domain.Note        `json:"notes"`
	Evidence    []domain.Evidence    `json:"evidence"`
	Concessions []string             `json:"concessionsAvailable"`
}

func (a *App) accountView(l *domain.Licence, on timeline.Date) AccountView {
	view := AccountView{Licence: l.ID, LicenceNumber: l.Number, Charged: l.FeePaid, Payments: []domain.Payment{}}
	account, ok := a.Ledger.Get(l.ID)
	if !ok {
		view.Balance = l.FeePaid
		return view
	}
	view.Method = account.Method
	view.MethodLabel = account.Method.Label()
	view.Charged = account.Charged
	view.Paid = account.Paid()
	view.Balance = account.Balance()
	view.Arrears = account.Arrears(on)
	view.Payments = account.Payments
	view.Instalments = account.Instalments(on)
	view.NextCollection = account.NextCollection(on)
	if account.Mandate != nil {
		view.Mandate = account.Mandate
		view.MandateMasked = account.Mandate.Masked()
	}
	return view
}

func (a *App) SearchPeople(term string, on timeline.Date) []PersonSummary {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := []PersonSummary{}
	for _, p := range a.Parties.Match(term) {
		s := PersonSummary{
			ID: p.ID, Name: p.Name(), SortName: p.SortName(),
			Email: p.Email, Phone: p.Phone, Deceased: p.Deceased,
		}
		for _, id := range a.Occupancy.PremisesOf(p.ID, on) {
			if prem, ok := a.Premises[id]; ok {
				s.Address = prem.Address.SingleLine()
			}
			break
		}
		for _, l := range a.Licences.All() {
			if l.Holder == p.ID && l.StatusOn(on) == domain.LicenceActive {
				s.LicenceHeld = true
				break
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SortName < out[j].SortName })
	return out
}

func (a *App) PersonDetail(id ids.PartyID, on timeline.Date) (PersonDetail, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.personDetailLocked(id, on)
}

func (a *App) personDetailLocked(id ids.PartyID, on timeline.Date) (PersonDetail, error) {
	p, ok := a.Parties.Get(id)
	if !ok {
		return PersonDetail{}, fmt.Errorf("no person %s", id)
	}

	d := PersonDetail{
		Party: p, Name: p.Name(),
		Addresses: []AddressHistoryItem{}, Licences: []LicenceSummary{},
		Accounts: []AccountView{}, Concessions: []string{},
	}

	for _, o := range a.Occupancy.HistoryOf(id) {
		item := AddressHistoryItem{
			Premises: o.Premises, From: o.Period.Start,
			To: o.Period.End, Current: o.Period.Contains(on),
		}
		if prem, ok := a.Premises[o.Premises]; ok {
			item.AddressLine = prem.Address.SingleLine()
		}
		d.Addresses = append(d.Addresses, item)
	}

	for _, l := range a.Licences.All() {
		if l.Holder != id {
			continue
		}
		d.Licences = append(d.Licences, a.licenceSummary(l, on))
		d.Accounts = append(d.Accounts, a.accountView(l, on))
	}
	sort.Slice(d.Licences, func(i, j int) bool { return d.Licences[i].Start.After(d.Licences[j].Start) })

	d.Letters = a.Records.Letters(id)
	d.Notes = a.Records.Notes(string(id))
	d.Evidence = a.Records.EvidenceFor(id)

	for _, c := range []domain.Concession{domain.ConcessionBlind50, domain.ConcessionOver75PensionCredit} {
		if a.Records.SupportsConcession(id, c, on) {
			d.Concessions = append(d.Concessions, string(c))
		}
	}
	return d, nil
}

// SubjectAccess is everything held about a person, for a subject access request.
type SubjectAccess struct {
	GeneratedOn  timeline.Date        `json:"generatedOn"`
	Person       PersonDetail         `json:"person"`
	Declarations []domain.Declaration `json:"declarations"`
	Visits       []domain.Visit       `json:"visits"`
	AccessLog    []string             `json:"accessLog"`
}

func (a *App) SubjectAccess(id ids.PartyID, on timeline.Date) (SubjectAccess, error) {
	a.mu.Lock()
	person, err := a.personDetailLocked(id, on)
	if err != nil {
		a.mu.Unlock()
		return SubjectAccess{}, err
	}

	out := SubjectAccess{
		GeneratedOn: on, Person: person,
		Declarations: []domain.Declaration{}, Visits: []domain.Visit{}, AccessLog: []string{},
	}

	for _, addr := range person.Addresses {
		for _, d := range a.Declarations.All(addr.Premises) {
			if d.MadeBy == id {
				out.Declarations = append(out.Declarations, d)
			}
		}
		out.Visits = append(out.Visits, a.Records.Visits(addr.Premises)...)
	}
	a.mu.Unlock()

	for _, e := range a.Audit.For(string(id)) {
		out.AccessLog = append(out.AccessLog,
			fmt.Sprintf("%s — %s by %s (%s)", e.At.Format("2006-01-02 15:04"), e.Action, e.Actor.ID, e.Actor.Role))
	}
	return out, nil
}

// Account is the money side of one licence: what was charged, what has been
// paid, the plan, and where the plan has got to.
func (a *App) Account(licence ids.LicenceID, on timeline.Date) (AccountView, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	l, ok := a.Licences.Get(licence)
	if !ok {
		return AccountView{}, fmt.Errorf("no licence %s", licence)
	}
	return a.accountView(l, on), nil
}
