package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPersonRecordCarriesEverythingAnAgentNeeds(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/people/PARTY1?on=2026-06-01", "")
	if w.Code != http.StatusOK {
		t.Fatalf("person = %d: %s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, want := range []string{"Nadia", "Rowan Close", "licences", "accounts", "letters", "notes"} {
		if !strings.Contains(body, want) {
			t.Errorf("person record missing %q", want)
		}
	}
}

func TestBankDetailsAreNeverReturnedInFull(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/people/PARTY1?on=2026-06-01", "")
	body := w.Body.String()
	if strings.Contains(body, "31926044") {
		t.Error("the full account number was returned")
	}
	if strings.Contains(body, "60-16-13") {
		t.Error("the full sort code was returned")
	}
	if !strings.Contains(body, "6044") {
		t.Errorf("the masked account should show the last four digits: %s", body)
	}
}

func TestAConcessionNeedsEvidence(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	// PARTY5 has no evidence on file.
	w := c.do("POST", "/api/licences",
		`{"holder":"PARTY5","premises":"P6","class":"COLOUR","concession":"BLIND_50","start":"2026-06-01"}`)
	if w.Code == http.StatusCreated {
		t.Error("a concession was applied without evidence")
	}

	w = c.do("POST", "/api/people/PARTY5/evidence",
		`{"kind":"BLIND_REGISTRATION","reference":"SSI/2026/1","seenOn":"2026-05-01"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("record evidence = %d: %s", w.Code, w.Body)
	}

	w = c.do("POST", "/api/licences",
		`{"holder":"PARTY5","premises":"P6","class":"COLOUR","concession":"BLIND_50","start":"2026-06-01"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("with evidence the concession should be allowed: %d %s", w.Code, w.Body)
	}
}

func TestChangeOfAddressMovesTheLicence(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("POST", "/api/people/PARTY1/address", `{"premises":"P2","on":"2026-10-01"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("change address = %d: %s", w.Code, w.Body)
	}
	var moved struct {
		LicencesMoved []string `json:"licencesMoved"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &moved); err != nil {
		t.Fatal(err)
	}
	if len(moved.LicencesMoved) != 1 {
		t.Fatalf("expected one licence to move, got %d", len(moved.LicencesMoved))
	}

	w = c.do("GET", "/api/premises/P2?on=2026-11-01", "")
	if !strings.Contains(w.Body.String(), "Kaur") {
		t.Errorf("the new address should show the occupier: %s", w.Body)
	}
	w = c.do("GET", "/api/premises/P1?on=2026-11-01", "")
	if strings.Contains(w.Body.String(), `"occupier":"Ms Nadia Kaur"`) {
		t.Errorf("the old address should no longer show them as occupier: %s", w.Body)
	}
}

func TestPaymentsAndArrears(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/people/PARTY1?on=2026-08-01", "")
	var person struct {
		Accounts []struct {
			Licence string `json:"licence"`
			Paid    int64  `json:"paid"`
			Balance int64  `json:"balance"`
			Arrears int64  `json:"arrears"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &person); err != nil {
		t.Fatal(err)
	}
	if len(person.Accounts) == 0 {
		t.Fatal("no account on the person record")
	}
	a := person.Accounts[0]
	if a.Paid != 4500 {
		t.Errorf("paid = %d, want 4500", a.Paid)
	}
	if a.Balance != 13500 {
		t.Errorf("balance = %d, want 13500", a.Balance)
	}
	if a.Arrears != 3000 {
		t.Errorf("arrears by August = %d, want 3000 (two collections behind)", a.Arrears)
	}

	w = c.do("POST", "/api/licences/"+a.Licence+"/payments",
		`{"amount":"30.00","method":"DIRECT_DEBIT","on":"2026-08-06","reference":"catch up"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("take payment = %d: %s", w.Code, w.Body)
	}

	w = c.do("GET", "/api/licences/"+a.Licence+"/account?on=2026-08-10", "")
	var account struct {
		Paid    int64 `json:"paid"`
		Arrears int64 `json:"arrears"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &account); err != nil {
		t.Fatal(err)
	}
	if account.Paid != 7500 {
		t.Errorf("paid after catch-up = %d, want 7500", account.Paid)
	}
	if account.Arrears != 0 {
		t.Errorf("arrears after catch-up = %d, want 0", account.Arrears)
	}
}

func TestChangeOfNameKeepsHistory(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("POST", "/api/people/PARTY1/name",
		`{"title":"Mrs","forename":"Nadia","surname":"Kaur-Hollis","reason":"MARRIAGE","on":"2026-07-04"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("change name = %d: %s", w.Code, w.Body)
	}

	w = c.do("GET", "/api/people/PARTY1?on=2026-08-01", "")
	body := w.Body.String()
	if !strings.Contains(body, "Kaur-Hollis") {
		t.Error("the new name is not shown")
	}
	if !strings.Contains(body, "MARRIAGE") || !strings.Contains(body, "Ms Nadia Kaur") {
		t.Errorf("the previous name and reason should be kept: %s", body)
	}
}

func TestRecordingADeathStopsLettersAndRefunds(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	before := c.do("GET", "/api/people/PARTY1?on=2026-08-01", "").Body.String()
	countBefore := strings.Count(before, `"kind":"`)

	w := c.do("POST", "/api/people/PARTY1/deceased", `{"on":"2026-08-01"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("record death = %d: %s", w.Code, w.Body)
	}
	var res struct {
		Refund int64 `json:"refund"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Refund <= 0 {
		t.Errorf("a refund should be due, got %d", res.Refund)
	}

	after := c.do("GET", "/api/people/PARTY1?on=2026-09-01", "").Body.String()
	if strings.Count(after, `"kind":"`) != countBefore {
		t.Error("a letter was sent after the person had died")
	}
	if !strings.Contains(after, `"deceased"`) {
		t.Error("the record should show the person as deceased")
	}
}

func TestVisitsCannotBeRecordedWherePermissionIsWithdrawn(t *testing.T) {
	c := newClient(t)
	c.signIn("j.okafor")

	w := c.do("POST", "/api/visits", `{"premises":"P8","on":"2026-07-01","outcome":"NO_ANSWER"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("visit to a withdrawn address = %d, want 403: %s", w.Code, w.Body)
	}

	w = c.do("POST", "/api/visits", `{"premises":"P6","on":"2026-07-01","outcome":"NO_ANSWER"}`)
	if w.Code != http.StatusCreated {
		t.Errorf("ordinary visit = %d, want 201: %s", w.Code, w.Body)
	}
}

func TestRenewIssuesAFollowOnLicence(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/licences?on=2026-06-01", "")
	var licences []struct {
		ID     string `json:"id"`
		Expiry string `json:"expiry"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &licences); err != nil {
		t.Fatal(err)
	}

	w = c.do("POST", "/api/licences/"+licences[0].ID+"/renew", `{"method":"CARD","plan":"ANNUAL"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("renew = %d: %s", w.Code, w.Body)
	}
	var renewed struct {
		Number string `json:"number"`
		Term   struct {
			Start string `json:"start"`
		} `json:"term"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &renewed); err != nil {
		t.Fatal(err)
	}
	if renewed.Term.Start != licences[0].Expiry {
		t.Errorf("renewal starts %s, want %s", renewed.Term.Start, licences[0].Expiry)
	}
}

func TestSubjectAccessExportGathersEverything(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	c.do("GET", "/api/people/PARTY1?on=2026-08-01", "") // an agent opens the record first

	w := c.do("GET", "/api/people/PARTY1/export?on=2026-08-01", "")
	if w.Code != http.StatusOK {
		t.Fatalf("export = %d: %s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, want := range []string{"generatedOn", "person", "accessLog", "Rowan Close"} {
		if !strings.Contains(body, want) {
			t.Errorf("export missing %q", want)
		}
	}
	if !strings.Contains(body, "person.viewed") {
		t.Errorf("the export should list who has looked at the record: %s", body)
	}
}

func TestNotesAreAttributedAndListed(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("POST", "/api/notes", `{"subject":"PARTY2","text":"Rang about a reminder letter."}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("add note = %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "Ana Reyes") {
		t.Errorf("the note should record who wrote it: %s", w.Body)
	}
	if w := c.do("POST", "/api/notes", `{"subject":"PARTY2","text":"   "}`); w.Code == http.StatusCreated {
		t.Error("an empty note should be rejected")
	}
}

func TestReferenceListsSupplyWordingToTheClient(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/reference", "")
	if w.Code != http.StatusOK {
		t.Fatalf("reference = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"paymentMethods", "cancellationReasons", "visitOutcomes",
		"No longer watches live TV or BBC iPlayer", "TV Licensing payment card"} {
		if !strings.Contains(body, want) {
			t.Errorf("reference missing %q", want)
		}
	}
}

func TestEqualityReportIsAdminOnly(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")
	if w := c.do("GET", "/api/equality/ageBand", ""); w.Code != http.StatusForbidden {
		t.Errorf("service role on equality = %d, want 403", w.Code)
	}

	admin := &client{h: c.h, t: t}
	admin.signIn("m.whitlock")
	if w := admin.do("GET", "/api/equality/ageBand?outcome=NO_ANSWER", ""); w.Code != http.StatusOK {
		t.Errorf("admin on equality = %d, want 200: %s", w.Code, w.Body)
	}
}

func TestDirectDebitValidation(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/licences?on=2026-06-01", "")
	var licences []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &licences); err != nil {
		t.Fatal(err)
	}
	id := licences[0].ID

	bad := []string{
		`{"accountName":"","sortCode":"601613","accountNumber":"31926044","collectionDay":6,"on":"2026-06-01"}`,
		`{"accountName":"N Kaur","sortCode":"6016","accountNumber":"31926044","collectionDay":6,"on":"2026-06-01"}`,
		`{"accountName":"N Kaur","sortCode":"601613","accountNumber":"319","collectionDay":6,"on":"2026-06-01"}`,
		`{"accountName":"N Kaur","sortCode":"601613","accountNumber":"31926044","collectionDay":31,"on":"2026-06-01"}`,
	}
	for _, body := range bad {
		if w := c.do("POST", "/api/licences/"+id+"/direct-debit", body); w.Code == http.StatusOK {
			t.Errorf("bad mandate accepted: %s", body)
		}
	}
	good := `{"accountName":"N Kaur","sortCode":"60-16-13","accountNumber":"31926044","collectionDay":6,"on":"2026-06-01"}`
	if w := c.do("POST", "/api/licences/"+id+"/direct-debit", good); w.Code != http.StatusOK {
		t.Errorf("valid mandate = %d: %s", w.Code, w.Body)
	}
}
