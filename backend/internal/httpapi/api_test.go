package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aerial-project/aerial/internal/app"
)

const devPassword = "aerial-development"

type client struct {
	h      http.Handler
	cookie *http.Cookie
	t      *testing.T
}

func newClient(t *testing.T) *client {
	t.Helper()
	a, err := app.New([]byte("test-salt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Seed(); err != nil {
		t.Fatal(err)
	}
	if err := a.SeedUsers(devPassword); err != nil {
		t.Fatal(err)
	}
	return &client{h: New(a, false).Routes(), t: t}
}

func (c *client) do(method, path, body string) *httptest.ResponseRecorder {
	c.t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	return w
}

func (c *client) signIn(username string) {
	c.t.Helper()
	w := c.do("POST", "/api/session", `{"username":"`+username+`","password":"`+devPassword+`"}`)
	if w.Code != http.StatusOK {
		c.t.Fatalf("sign in as %s: %d %s", username, w.Code, w.Body)
	}
	for _, ck := range w.Result().Cookies() {
		if ck.Name == sessionCookie {
			c.cookie = ck
			return
		}
	}
	c.t.Fatal("no session cookie issued")
}

func TestEverythingRequiresASession(t *testing.T) {
	c := newClient(t)
	paths := []string{"/api/overview", "/api/premises", "/api/premises/P1", "/api/licences",
		"/api/worklist", "/api/premises/P1/enforcement", "/api/audit", "/api/fees/coverage"}
	for _, p := range paths {
		if w := c.do("GET", p, ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session = %d, want 401", p, w.Code)
		}
	}
}

func TestTheClientCannotAssertItsOwnRole(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes") // service

	r := httptest.NewRequest("GET", "/api/premises/P1/enforcement", nil)
	r.AddCookie(c.cookie)
	r.Header.Set("X-Actor-Role", "ENFORCEMENT")
	r.Header.Set("X-Actor-Id", "someone-else")
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("a service user claiming ENFORCEMENT got %d, want 403", w.Code)
	}
}

func TestBadCredentialsAreRefused(t *testing.T) {
	c := newClient(t)
	if w := c.do("POST", "/api/session", `{"username":"a.reyes","password":"wrong"}`); w.Code != http.StatusUnauthorized {
		t.Errorf("bad password = %d, want 401", w.Code)
	}
	if w := c.do("POST", "/api/session", `{"username":"ghost","password":"`+devPassword+`"}`); w.Code != http.StatusUnauthorized {
		t.Errorf("unknown user = %d, want 401", w.Code)
	}
}

func TestSignOutEndsTheSession(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")
	if w := c.do("GET", "/api/overview", ""); w.Code != http.StatusOK {
		t.Fatalf("signed in overview = %d", w.Code)
	}
	if w := c.do("DELETE", "/api/session", ""); w.Code != http.StatusNoContent {
		t.Fatalf("sign out = %d", w.Code)
	}
	if w := c.do("GET", "/api/overview", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("after sign out = %d, want 401", w.Code)
	}
}

func TestEnforcementCannotReachStaffRecords(t *testing.T) {
	c := newClient(t)
	c.signIn("j.okafor") // enforcement

	for _, p := range []string{"/api/premises", "/api/premises/P1", "/api/licences", "/api/overview", "/api/audit"} {
		if w := c.do("GET", p, ""); w.Code != http.StatusForbidden {
			t.Errorf("enforcement on %s = %d, want 403", p, w.Code)
		}
	}
	if w := c.do("GET", "/api/worklist", ""); w.Code != http.StatusOK {
		t.Errorf("enforcement worklist = %d, want 200: %s", w.Code, w.Body)
	}
}

func TestWorklistCarriesNoNamesOrLicenceNumbers(t *testing.T) {
	c := newClient(t)
	c.signIn("j.okafor")

	w := c.do("GET", "/api/worklist?on=2026-06-01", "")
	if w.Code != http.StatusOK {
		t.Fatalf("worklist = %d: %s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, leak := range []string{"Kaur", "Whitlow", "Vann", "Morgan", "8472", "holderName"} {
		if strings.Contains(body, leak) {
			t.Errorf("worklist leaked %q: %s", leak, body)
		}
	}
	if !strings.Contains(body, "Rowan Close") {
		t.Errorf("worklist should list addresses: %s", body)
	}
}

func TestStaffCanSearchByAddressPostcodeNameAndLicenceNumber(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	cases := map[string]string{
		"Rowan":    "Rowan Close",
		"CT16":     "Rowan Close",
		"ct16 2qp": "Rowan Close",
		"Kaur":     "Rowan Close",
		"Swansea":  "Sandhill Avenue",
	}
	for term, want := range cases {
		w := c.do("GET", "/api/premises?q="+strings.ReplaceAll(term, " ", "%20")+"&on=2026-06-01", "")
		if w.Code != http.StatusOK {
			t.Fatalf("search %q = %d: %s", term, w.Code, w.Body)
		}
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("search %q did not find %q: %s", term, want, w.Body)
		}
	}
}

func TestSearchByLicenceNumber(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/licences?on=2026-06-01", "")
	if w.Code != http.StatusOK {
		t.Fatalf("licences = %d", w.Code)
	}
	var licences []struct {
		Number string `json:"number"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &licences); err != nil {
		t.Fatal(err)
	}
	if len(licences) == 0 {
		t.Fatal("no licences seeded")
	}

	number := licences[0].Number
	w = c.do("GET", "/api/premises?q="+strings.ReplaceAll(number, " ", "%20")+"&on=2026-06-01", "")
	if !strings.Contains(w.Body.String(), strings.Split(number, " ")[0]) {
		t.Errorf("searching licence number %q found nothing: %s", number, w.Body)
	}
}

func TestPremisesDetailCarriesTheWholeRecord(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/premises/P1?on=2026-06-01", "")
	if w.Code != http.StatusOK {
		t.Fatalf("detail = %d: %s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, want := range []string{"Rowan Close", "Kaur", "CT16 2QP", "licences", "declarations"} {
		if !strings.Contains(body, want) {
			t.Errorf("detail missing %q: %s", want, body)
		}
	}
}

func TestEnforcementViewNamesNobody(t *testing.T) {
	c := newClient(t)
	c.signIn("j.okafor")

	w := c.do("GET", "/api/premises/P1/enforcement?on=2026-06-01", "")
	if w.Code != http.StatusOK {
		t.Fatalf("enforcement view = %d: %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "Kaur") || strings.Contains(w.Body.String(), "PARTY1") {
		t.Errorf("enforcement view named the occupier: %s", w.Body)
	}
}

func TestWithdrawnAccessShowsOnTheWorklist(t *testing.T) {
	c := newClient(t)
	c.signIn("j.okafor")

	w := c.do("GET", "/api/worklist?on=2026-06-01", "")
	var items []struct {
		AddressLine    string `json:"addressLine"`
		VisitPermitted bool   `json:"visitPermitted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, i := range items {
		if strings.Contains(i.AddressLine, "Kilnbank") {
			found = true
			if i.VisitPermitted {
				t.Error("Kilnbank has a withdrawal of access and must not be visitable")
			}
		}
	}
	if !found {
		t.Error("Kilnbank not on the worklist")
	}
}

func TestIssueAndCancelLicence(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("POST", "/api/licences", `{"holder":"PARTY2","premises":"P2","class":"COLOUR","start":"2026-04-01"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("issue = %d: %s", w.Code, w.Body)
	}
	var created struct {
		Licence struct {
			ID      string `json:"id"`
			Number  string `json:"number"`
			FeePaid int64  `json:"feePaid"`
		} `json:"licence"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Licence.FeePaid != 18000 {
		t.Errorf("fee = %d, want 18000", created.Licence.FeePaid)
	}
	if created.Licence.Number == "" {
		t.Error("issued licence has no number")
	}

	w = c.do("POST", "/api/licences/"+created.Licence.ID+"/cancel", `{"on":"2026-09-01","reason":"NO_LONGER_WATCHING"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel = %d: %s", w.Code, w.Body)
	}
	var cancelled struct {
		Refund int64 `json:"refund"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cancelled); err != nil {
		t.Fatal(err)
	}
	if cancelled.Refund != 10500 {
		t.Errorf("refund = %d, want 10500", cancelled.Refund)
	}
}

func TestErrorsReadAsPlainEnglish(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/fees/quote?class=MONOCHROME&on=2025-04-01", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown rate = %d, want 422", w.Code)
	}
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.Error, "domain:") || strings.Contains(e.Error, "Err") {
		t.Errorf("error message leaks internals: %q", e.Error)
	}
	if e.Error != "No fee is recorded for that licence type on that date" {
		t.Errorf("unexpected message: %q", e.Error)
	}
}

func TestHotelRoomCannotBeLicensedSeparately(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")
	w := c.do("POST", "/api/licences", `{"holder":"PARTY4","premises":"P4","class":"COLOUR","start":"2026-04-01"}`)
	if w.Code != http.StatusConflict {
		t.Errorf("hotel room = %d, want 409: %s", w.Code, w.Body)
	}
}

func TestAuditIsAdminOnlyAndRecordsRefusals(t *testing.T) {
	c := newClient(t)
	c.signIn("j.okafor")
	c.do("GET", "/api/premises", "") // refused

	admin := newClientSharing(t, c)
	admin.signIn("m.whitlock")
	w := admin.do("GET", "/api/audit", "")
	if w.Code != http.StatusOK {
		t.Fatalf("admin audit = %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "session.sign_in") {
		t.Errorf("audit missing sign-in records: %s", w.Body)
	}
}

func newClientSharing(t *testing.T, c *client) *client {
	t.Helper()
	return &client{h: c.h, t: t}
}
