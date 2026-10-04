package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPersonalDataIsNeverCached(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("GET", "/api/people/PARTY1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("person = %d: %s", w.Code, w.Body)
	}
	if store := w.Header().Get("Cache-Control"); !strings.Contains(store, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", store)
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestAnOversizedBodyIsRefused(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	huge := `{"subject":"PARTY1","text":"` + strings.Repeat("x", maxBodyBytes+1) + `"}`
	w := c.do("POST", "/api/notes", huge)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized note = %d, want 413: %s", w.Code, w.Body)
	}
}

func TestTrailingRubbishAfterTheJSONIsRefused(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	w := c.do("POST", "/api/notes", `{"subject":"PARTY1","text":"first"} {"subject":"PARTY1","text":"smuggled"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("two objects in one body = %d, want 400: %s", w.Code, w.Body)
	}
	if strings.Contains(c.body(t, "GET", "/api/notes?subject=PARTY1"), "smuggled") {
		t.Error("the second object should not have been acted on")
	}
}

func TestRepeatedBadPasswordsAreThrottled(t *testing.T) {
	c := newClient(t)
	var last int
	for i := 0; i < 6; i++ {
		w := c.do("POST", "/api/session", `{"username":"a.reyes","password":"not-the-password"}`)
		last = w.Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("sixth bad attempt = %d, want 429", last)
	}
}

func TestTheAuditLogAnswersNewestFirstAndNarrows(t *testing.T) {
	officer := newClient(t)
	officer.signIn("j.okafor")
	officer.do("GET", "/api/premises", "") // refused: wrong role

	admin := newClientSharing(t, officer)
	admin.signIn("m.whitlock")

	var page struct {
		Entries []struct {
			Action  string `json:"action"`
			Allowed bool   `json:"allowed"`
			Actor   struct {
				Role string `json:"role"`
			} `json:"actor"`
		} `json:"entries"`
		Matched  int `json:"matched"`
		Returned int `json:"returned"`
		Total    int `json:"total"`
	}

	w := admin.do("GET", "/api/audit?limit=1", "")
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Returned != 1 || len(page.Entries) != 1 {
		t.Fatalf("limit=1 returned %d entries", len(page.Entries))
	}
	if page.Entries[0].Action != "session.sign_in" {
		t.Errorf("newest entry = %q, want the administrator's own sign-in", page.Entries[0].Action)
	}
	if page.Total < 3 {
		t.Errorf("total = %d, want the whole log counted", page.Total)
	}

	w = admin.do("GET", "/api/audit?refused=true", "")
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Matched == 0 {
		t.Fatal("no refusals found, but one was provoked")
	}
	for _, e := range page.Entries {
		if e.Allowed {
			t.Errorf("%s was allowed and should not be in a refused-only view", e.Action)
		}
	}

	w = admin.do("GET", "/api/audit?role=ENFORCEMENT", "")
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	for _, e := range page.Entries {
		if e.Actor.Role != "ENFORCEMENT" {
			t.Errorf("role filter let a %s entry through", e.Actor.Role)
		}
	}

	if w := admin.do("GET", "/api/audit?limit=nonsense", ""); w.Code != http.StatusBadRequest {
		t.Errorf("limit=nonsense = %d, want 400", w.Code)
	}
}

func TestTheLicenceNoticeIsPublicAndAccurate(t *testing.T) {
	c := newClient(t)

	w := c.do("GET", "/api/licence", "")
	if w.Code != http.StatusOK {
		t.Fatalf("licence notice = %d, and it should need no session", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Aerial Public Licence", "Capita", "source-available"} {
		if !strings.Contains(body, want) {
			t.Errorf("the notice does not mention %q: %s", want, body)
		}
	}
	if strings.Contains(body, `"openSource":"yes"`) {
		t.Error("the notice must not describe this licence as open source")
	}
}

func TestLicenceDetailPricesEveryCancellationReason(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	var list []struct {
		ID      string `json:"id"`
		FeePaid int64  `json:"feePaid"`
	}
	w := c.do("GET", "/api/licences", "")
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("no licences in the seeded data")
	}

	var detail struct {
		Summary struct {
			FeePaid int64 `json:"feePaid"`
		} `json:"summary"`
		Refund  int64            `json:"refund"`
		Refunds map[string]int64 `json:"refunds"`
	}
	w = c.do("GET", "/api/licences/"+list[0].ID+"?on=2026-10-01", "")
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"NO_LONGER_WATCHING", "MOVED_ABROAD", "LICENCE_HOLDER_DIED", "NOW_ELIGIBLE_FREE", "ISSUED_IN_ERROR"} {
		if _, ok := detail.Refunds[reason]; !ok {
			t.Errorf("no refund figure for %s", reason)
		}
	}
	if detail.Refunds["ISSUED_IN_ERROR"] != detail.Summary.FeePaid {
		t.Errorf("issued in error = %d, want the whole fee %d",
			detail.Refunds["ISSUED_IN_ERROR"], detail.Summary.FeePaid)
	}
	if detail.Refund != detail.Refunds["NO_LONGER_WATCHING"] {
		t.Error("the headline refund should match the ordinary reason")
	}
}

func TestALicenceCannotBeTransferredToAnAddressThatDoesNotExist(t *testing.T) {
	c := newClient(t)
	c.signIn("a.reyes")

	var list []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(c.do("GET", "/api/licences", "").Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}

	w := c.do("POST", "/api/licences/"+list[0].ID+"/transfer", `{"premises":"P999","on":"2026-10-01"}`)
	if w.Code == http.StatusOK {
		t.Error("the transfer should have been refused")
	}
}

func (c *client) body(t *testing.T, method, path string) string {
	t.Helper()
	return c.do(method, path, "").Body.String()
}
