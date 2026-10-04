package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAnExactDuplicateAddressIsRefused(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	// Same front door, typed differently.
	body := `{"line1":"12  rowan   close","town":"Dover","postcode":"ct162qp","kind":"DWELLING"}`
	w := c.do("POST", "/api/premises", body)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate address = %d, want 409: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "Rowan Close") {
		t.Errorf("the response should name the existing record: %s", w.Body)
	}
}

func TestASamePostcodeAddressNeedsConfirming(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	body := `{"line1":"14 Rowan Close","town":"Dover","postcode":"CT16 2QP","kind":"DWELLING"}`
	w := c.do("POST", "/api/premises", body)
	if w.Code != http.StatusOK {
		t.Fatalf("near match = %d, want 200 asking for confirmation: %s", w.Code, w.Body)
	}
	var res struct {
		NeedsConfirmation bool `json:"needsConfirmation"`
		Duplicate         struct {
			Near []struct {
				AddressLine string `json:"addressLine"`
			} `json:"near"`
		} `json:"duplicate"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.NeedsConfirmation || len(res.Duplicate.Near) == 0 {
		t.Fatalf("expected a near match to be offered: %s", w.Body)
	}

	confirmed := `{"line1":"14 Rowan Close","town":"Dover","postcode":"CT16 2QP","kind":"DWELLING","acknowledgedNear":true}`
	w = c.do("POST", "/api/premises", confirmed)
	if w.Code != http.StatusCreated {
		t.Fatalf("confirmed address = %d, want 201: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "14 Rowan Close") {
		t.Errorf("created address not returned: %s", w.Body)
	}
}

func TestNewAddressesGetAProvisionalReference(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("POST", "/api/premises",
		`{"line1":"2 Hollybush Lane","town":"Norwich","postcode":"NR3 1AB","kind":"DWELLING"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", w.Code, w.Body)
	}
	var created struct {
		UPRN string `json:"uprn"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.UPRN, "9") || len(created.UPRN) != 12 {
		t.Errorf("provisional reference = %q, want a 12-digit number starting 9", created.UPRN)
	}
}

func TestAddressRequiresLineOneAndPostcode(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")
	for _, body := range []string{
		`{"line1":"","town":"Dover","postcode":"CT16 9ZZ"}`,
		`{"line1":"1 Somewhere","town":"Dover","postcode":""}`,
	} {
		if w := c.do("POST", "/api/premises", body); w.Code != http.StatusBadRequest {
			t.Errorf("incomplete address = %d, want 400: %s", w.Code, w.Body)
		}
	}
}

func TestCancellationCanBeReversed(t *testing.T) {
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

	if w := c.do("POST", "/api/licences/"+id+"/cancel",
		`{"on":"2026-09-01","reason":"NO_LONGER_WATCHING"}`); w.Code != http.StatusOK {
		t.Fatalf("cancel = %d: %s", w.Code, w.Body)
	}

	w = c.do("GET", "/api/licences/"+id+"?on=2026-09-02", "")
	if !strings.Contains(w.Body.String(), "CANCELLED") {
		t.Fatalf("licence should be cancelled: %s", w.Body)
	}

	if w := c.do("POST", "/api/licences/"+id+"/reinstate", `{"on":"2026-09-02"}`); w.Code != http.StatusOK {
		t.Fatalf("reinstate = %d: %s", w.Code, w.Body)
	}

	w = c.do("GET", "/api/licences/"+id+"?on=2026-09-02", "")
	if !strings.Contains(w.Body.String(), "ACTIVE") {
		t.Errorf("licence should be back in force: %s", w.Body)
	}
}

func TestReversalIsWrittenToTheRecordRatherThanErasingIt(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/licences?on=2026-06-01", "")
	var licences []struct {
		ID     string `json:"id"`
		Holder string `json:"holder"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &licences); err != nil {
		t.Fatal(err)
	}

	c.do("POST", "/api/licences/"+licences[0].ID+"/cancel", `{"on":"2026-09-01","reason":"NO_LONGER_WATCHING"}`)
	c.do("POST", "/api/licences/"+licences[0].ID+"/reinstate", `{"on":"2026-09-02"}`)

	w = c.do("GET", "/api/people/"+licences[0].Holder+"?on=2026-09-02", "")
	body := w.Body.String()
	if !strings.Contains(body, "Recorded in error") {
		t.Errorf("the correction should appear as a note: %s", body)
	}
	if !strings.Contains(body, "cancellation reversed") {
		t.Errorf("the refund reversal should appear on the account: %s", body)
	}
}

func TestDeathCanBeReversedAndRestoresTheLicence(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	if w := c.do("POST", "/api/people/PARTY1/deceased", `{"on":"2026-08-01"}`); w.Code != http.StatusOK {
		t.Fatalf("record death = %d: %s", w.Code, w.Body)
	}

	w := c.do("GET", "/api/people/PARTY1?on=2026-08-02", "")
	if !strings.Contains(w.Body.String(), `"deceased"`) {
		t.Fatal("person should be recorded as died")
	}

	if w := c.do("POST", "/api/people/PARTY1/deceased/reverse", `{"on":"2026-08-02"}`); w.Code != http.StatusOK {
		t.Fatalf("reverse = %d: %s", w.Code, w.Body)
	}

	w = c.do("GET", "/api/people/PARTY1?on=2026-08-02", "")
	body := w.Body.String()
	if strings.Contains(body, `"deceased"`) {
		t.Error("the death record should be cleared")
	}
	if !strings.Contains(body, `"status":"ACTIVE"`) {
		t.Errorf("the licence should be back in force: %s", body)
	}
	if !strings.Contains(body, "Death record reversed") {
		t.Errorf("the correction should be on the record: %s", body)
	}
}

func TestReversingSomethingThatDidNotHappenIsRefused(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	if w := c.do("POST", "/api/people/PARTY2/deceased/reverse", `{"on":"2026-08-02"}`); w.Code == http.StatusOK {
		t.Error("reversing a death that was never recorded should fail")
	}

	w := c.do("GET", "/api/licences?on=2026-06-01", "")
	var licences []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &licences); err != nil {
		t.Fatal(err)
	}
	if w := c.do("POST", "/api/licences/"+licences[0].ID+"/reinstate", `{"on":"2026-09-02"}`); w.Code == http.StatusOK {
		t.Error("reinstating a licence that was never cancelled should fail")
	}
}

func TestReinstatementIsRefusedIfTheGapWasFilled(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/premises/P1?on=2026-06-01", "")
	var detail struct {
		Licences []struct {
			ID string `json:"id"`
		} `json:"licences"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	id := detail.Licences[0].ID

	c.do("POST", "/api/licences/"+id+"/cancel", `{"on":"2026-09-01","reason":"NO_LONGER_WATCHING"}`)

	if w := c.do("POST", "/api/licences",
		`{"holder":"PARTY1","premises":"P1","class":"COLOUR","start":"2026-09-01"}`); w.Code != http.StatusCreated {
		t.Fatalf("issuing into the gap = %d: %s", w.Code, w.Body)
	}

	if w := c.do("POST", "/api/licences/"+id+"/reinstate", `{"on":"2026-09-02"}`); w.Code != http.StatusConflict {
		t.Errorf("reinstating over another licence = %d, want 409: %s", w.Code, w.Body)
	}
}
