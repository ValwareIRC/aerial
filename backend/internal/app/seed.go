package app

import (
	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/payment"
	"github.com/aerial-project/aerial/internal/timeline"
)

func date(s string) timeline.Date { return timeline.MustParseDate(s) }

func dateP(s string) *timeline.Date {
	d := date(s)
	return &d
}

// Seed loads a demonstration caseload.
func (a *App) Seed() error {
	premises := []domain.Premises{
		{ID: "P1", UPRN: ids.MustParseUPRN("100023336956"), Kind: domain.KindDwelling, UnitsCovered: 1,
			Address: domain.Address{Line1: "12 Rowan Close", Town: "Dover", Postcode: "CT16 2QP"}},
		{ID: "P2", UPRN: ids.MustParseUPRN("100023336957"), Kind: domain.KindDwelling, UnitsCovered: 1,
			Address: domain.Address{Line1: "4 Ashgrove Terrace", Town: "Gateshead", Postcode: "NE8 4XR"}},
		{ID: "P3", UPRN: ids.MustParseUPRN("100023336958"), Kind: domain.KindCareRoom, UnitsCovered: 1,
			Address: domain.Address{Line1: "Room 14, Beechwood Care Home", Line2: "220 Hollow Lane", Town: "Chelmsford", Postcode: "CM1 2LN"}},
		{ID: "P4", UPRN: ids.MustParseUPRN("100023336959"), Kind: domain.KindHotelUnit, Parent: "P5", UnitsCovered: 1,
			Address: domain.Address{Line1: "Room 203, The Corn Exchange Hotel", Line2: "8 Market Place", Town: "Ripon", Postcode: "HG4 1BZ"}},
		{ID: "P5", UPRN: ids.MustParseUPRN("100023336960"), Kind: domain.KindBusiness, UnitsCovered: 15,
			Address: domain.Address{Line1: "The Corn Exchange Hotel", Line2: "8 Market Place", Town: "Ripon", Postcode: "HG4 1BZ"}},
		{ID: "P6", UPRN: ids.MustParseUPRN("100023336961"), Kind: domain.KindHMORoom, UnitsCovered: 1,
			Address: domain.Address{Line1: "Room 3, 88a Fairview Road", Town: "Leicester", Postcode: "LE5 3TQ"}},
		{ID: "P7", UPRN: ids.MustParseUPRN("100023336962"), Kind: domain.KindDwelling, UnitsCovered: 1,
			Address: domain.Address{Line1: "31 Sandhill Avenue", Town: "Swansea", Postcode: "SA1 6HN"}},
		{ID: "P8", UPRN: ids.MustParseUPRN("100023336963"), Kind: domain.KindDwelling, UnitsCovered: 1,
			Address: domain.Address{Line1: "7 Kilnbank Cottages", Town: "Perth", Postcode: "PH2 8DL"}},
		{ID: "P9", UPRN: ids.MustParseUPRN("100023336964"), Kind: domain.KindDwelling, UnitsCovered: 1,
			Address: domain.Address{Line1: "58 Marlborough Road", Town: "Bristol", Postcode: "BS2 8LT"}},
	}
	for _, p := range premises {
		if err := a.AddPremises(p); err != nil {
			return err
		}
	}

	people := []struct {
		party domain.Party
		at    ids.PremisesID
		from  string
	}{
		{domain.Party{ID: "PARTY1", Title: "Ms", Forename: "Nadia", Surname: "Kaur",
			DateOfBirth: dateP("1979-06-14"), Email: "n.kaur@example.com", Phone: "07700 900412", Paperless: true}, "P1", "2024-01-08"},
		{domain.Party{ID: "PARTY2", Title: "Mr", Forename: "Terence", Surname: "Whitlow",
			DateOfBirth: dateP("1962-11-02"), Phone: "0191 496 0118"}, "P2", "2025-06-14"},
		{domain.Party{ID: "PARTY3", Title: "Mrs", Forename: "Edith", Surname: "Vann",
			DateOfBirth: dateP("1941-03-27")}, "P3", "2026-02-02"},
		{domain.Party{ID: "PARTY4", Organisation: "Corn Exchange Hotels Limited",
			Email: "accounts@cornexchangehotels.example", Paperless: true}, "P5", "2019-03-01"},
		{domain.Party{ID: "PARTY5", Title: "Mr", Forename: "Samuel", Surname: "Adeyemi",
			DateOfBirth: dateP("1998-09-30"), Phone: "07700 900733"}, "P6", "2026-05-01"},
		{domain.Party{ID: "PARTY6", Title: "Mrs", Forename: "Delyth", Surname: "Morgan",
			DateOfBirth: dateP("1955-01-19"), Phone: "01792 460255"}, "P7", "2018-09-20"},
		{domain.Party{ID: "PARTY7", Title: "Mr", Forename: "Alasdair", Surname: "Frayne",
			DateOfBirth: dateP("1971-07-05")}, "P8", "2023-11-11"},
		{domain.Party{ID: "PARTY8", Title: "Mr", Forename: "Idris", Surname: "Bello",
			DateOfBirth: dateP("1948-04-11"), Phone: "0117 496 0210"}, "P9", "2011-05-30"},
	}
	for _, p := range people {
		if err := a.Parties.Add(p.party); err != nil {
			return err
		}
		if err := a.Occupancy.Record(domain.Occupancy{
			Party: p.party.ID, Premises: p.at, Period: timeline.From(date(p.from)),
		}); err != nil {
			return err
		}
	}

	evidence := []struct {
		party   ids.PartyID
		kind    domain.EvidenceKind
		ref     string
		seen    string
		expires *timeline.Date
	}{
		{"PARTY6", domain.EvidenceBlindRegistration, "SSI/2019/44821", "2019-02-11", nil},
		{"PARTY3", domain.EvidenceCareHome, "BEECH/14", "2026-02-02", nil},
		{"PARTY8", domain.EvidencePensionCredit, "PC/2026/77310", "2026-03-02", dateP("2027-03-02")},
	}
	for _, e := range evidence {
		if _, err := a.AddEvidence(e.party, e.kind, e.ref, date(e.seen), e.expires, "a.reyes"); err != nil {
			return err
		}
	}

	licences := []struct {
		holder     ids.PartyID
		at         ids.PremisesID
		class      domain.LicenceClass
		concession domain.Concession
		start      string
		method     domain.PaymentMethod
		plan       payment.PlanKind
	}{
		{"PARTY1", "P1", domain.ClassColour, domain.ConcessionNone, "2026-04-01", domain.MethodDirectDebit, payment.PlanMonthlyDD},
		{"PARTY3", "P3", domain.ClassARC, domain.ConcessionNone, "2026-04-01", domain.MethodCard, payment.PlanAnnual},
		{"PARTY4", "P5", domain.ClassColour, domain.ConcessionNone, "2026-04-01", domain.MethodDirectDebit, payment.PlanQuarterlyDD},
		{"PARTY6", "P7", domain.ClassColour, domain.ConcessionBlind50, "2026-04-01", domain.MethodPaymentCard, payment.PlanWeeklyCash},
		{"PARTY8", "P9", domain.ClassColour, domain.ConcessionOver75PensionCredit, "2026-04-01", domain.MethodCard, payment.PlanAnnual},
	}
	issued := map[ids.PartyID]ids.LicenceID{}
	for _, l := range licences {
		lic, _, err := a.IssueWithPayment(l.holder, l.at, l.class, l.concession, date(l.start), domain.DefaultTermMonths, l.method, l.plan)
		if err != nil {
			return err
		}
		issued[l.holder] = lic.ID
	}

	if err := a.SetDirectDebit(issued["PARTY1"], "N Kaur", "60-16-13", "31926044", 6, date("2026-03-18")); err != nil {
		return err
	}
	if err := a.SetDirectDebit(issued["PARTY4"], "Corn Exchange Hotels Ltd", "20-45-77", "80114226", 12, date("2026-03-20")); err != nil {
		return err
	}

	payments := []struct {
		licence ids.LicenceID
		amount  string
		method  domain.PaymentMethod
		on      string
		ref     string
	}{
		{issued["PARTY1"], "15.00", domain.MethodDirectDebit, "2026-04-06", "DD collection 1"},
		{issued["PARTY1"], "15.00", domain.MethodDirectDebit, "2026-05-06", "DD collection 2"},
		{issued["PARTY1"], "15.00", domain.MethodDirectDebit, "2026-06-08", "DD collection 3"},
		{issued["PARTY3"], "7.50", domain.MethodCard, "2026-04-01", "card ending 4417"},
		{issued["PARTY4"], "45.00", domain.MethodDirectDebit, "2026-04-12", "DD collection 1"},
		{issued["PARTY4"], "45.00", domain.MethodDirectDebit, "2026-07-13", "DD collection 2"},
		{issued["PARTY6"], "1.74", domain.MethodPaymentCard, "2026-04-01", "PayPoint"},
		{issued["PARTY6"], "1.74", domain.MethodPaymentCard, "2026-04-08", "PayPoint"},
	}
	for _, p := range payments {
		amount, err := money.FromPounds(p.amount)
		if err != nil {
			return err
		}
		if _, err := a.TakePayment(p.licence, amount, p.method, date(p.on), p.ref, "a.reyes"); err != nil {
			return err
		}
	}

	declarations := []struct {
		at   ids.PremisesID
		kind domain.DeclarationKind
		by   ids.PartyID
		on   string
	}{
		{"P2", domain.DeclarationNLN, "PARTY2", "2026-01-15"},
		{"P8", domain.DeclarationWOIRA, "PARTY7", "2026-03-04"},
	}
	for _, d := range declarations {
		if _, err := a.Declare(d.at, d.kind, d.by, date(d.on)); err != nil {
			return err
		}
	}

	visits := []struct {
		at      ids.PremisesID
		on      string
		outcome domain.VisitOutcome
		note    string
	}{
		{"P6", "2026-06-11", domain.OutcomeNoAnswer, ""},
		{"P6", "2026-07-23", domain.OutcomeNoAnswer, "Called early evening."},
		{"P2", "2026-02-04", domain.OutcomeDeclarationTaken, "Occupier confirmed no live TV."},
	}
	for _, v := range visits {
		if _, err := a.RecordVisit(v.at, date(v.on), "Joseph Okafor", v.outcome, v.note); err != nil {
			return err
		}
	}

	notes := []struct{ subject, text, by string }{
		{"PARTY1", "Called to say the Direct Debit date does not suit. Moved collection to the 6th.", "Ana Reyes"},
		{"PARTY2", "Confirmed by phone that the household does not watch live TV. Declaration recorded.", "Ana Reyes"},
		{"PARTY5", "Room in a shared house. Landlord says tenant is responsible for their own licence.", "Ana Reyes"},
		{"PARTY6", "Certificate of sight impairment seen. Half price licence applied.", "Ana Reyes"},
	}
	for _, n := range notes {
		if _, err := a.AddNote(n.subject, n.text, n.by); err != nil {
			return err
		}
	}

	if err := a.Parties.UpdateContact("PARTY8", "", "0117 496 0210", false, &domain.Address{
		Line1: "c/o Mrs J Bello", Line2: "4 Cotham Vale", Town: "Bristol", Postcode: "BS6 6HR",
	}); err != nil {
		return err
	}
	return nil
}
