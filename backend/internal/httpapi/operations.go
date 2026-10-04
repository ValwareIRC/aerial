package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/aerial-project/aerial/internal/app"

	"github.com/aerial-project/aerial/internal/audit"
	"github.com/aerial-project/aerial/internal/auth"
	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/payment"
	"github.com/aerial-project/aerial/internal/timeline"
)

func (s *Server) attachOperations(mux *http.ServeMux) {
	staff := []audit.Role{audit.RoleService, audit.RoleAdmin}

	mux.HandleFunc("GET /api/people", s.guard(s.searchPeople, staff...))
	mux.HandleFunc("GET /api/people/{id}", s.guard(s.personDetail, staff...))
	mux.HandleFunc("POST /api/people/{id}/name", s.guard(s.changeName, staff...))
	mux.HandleFunc("POST /api/people/{id}/contact", s.guard(s.updateContact, staff...))
	mux.HandleFunc("POST /api/people/{id}/address", s.guard(s.changeAddress, staff...))
	mux.HandleFunc("POST /api/people/{id}/deceased", s.guard(s.recordDeath, staff...))
	mux.HandleFunc("POST /api/people/{id}/evidence", s.guard(s.addEvidence, staff...))
	mux.HandleFunc("GET /api/people/{id}/export", s.guard(s.subjectAccess, staff...))

	mux.HandleFunc("POST /api/notes", s.guard(s.addNote, audit.RoleService, audit.RoleAdmin, audit.RoleEnforcement))
	mux.HandleFunc("GET /api/notes", s.guard(s.notes, staff...))

	mux.HandleFunc("GET /api/licences/{id}/account", s.guard(s.account, staff...))
	mux.HandleFunc("POST /api/licences/{id}/payments", s.guard(s.takePayment, staff...))
	mux.HandleFunc("POST /api/licences/{id}/plan", s.guard(s.setPlan, staff...))
	mux.HandleFunc("POST /api/licences/{id}/direct-debit", s.guard(s.setDirectDebit, staff...))
	mux.HandleFunc("POST /api/licences/{id}/renew", s.guard(s.renew, staff...))

	mux.HandleFunc("POST /api/visits", s.guard(s.recordVisit, audit.RoleEnforcement))
	mux.HandleFunc("GET /api/premises/{id}/visits", s.guard(s.visits, audit.RoleService, audit.RoleAdmin, audit.RoleEnforcement))

	mux.HandleFunc("GET /api/equality/{attribute}", s.guard(s.equality, audit.RoleAdmin))
	mux.HandleFunc("POST /api/renewal-reminders", s.guard(s.renewalReminders, audit.RoleAdmin))
	mux.HandleFunc("GET /api/reference", s.guard(s.reference, audit.RoleService, audit.RoleAdmin, audit.RoleEnforcement))

	mux.HandleFunc("POST /api/premises", s.guard(s.createAddress, staff...))
	mux.HandleFunc("POST /api/premises/check", s.guard(s.checkAddress, staff...))
	mux.HandleFunc("POST /api/licences/{id}/reinstate", s.guard(s.reinstateLicence, staff...))
	mux.HandleFunc("POST /api/people/{id}/deceased/reverse", s.guard(s.reverseDeath, staff...))
}

// decode reads one JSON object from the body and nothing else. Every handler
// that takes a body goes through here, so the size cap, the "one object only"
// rule and the wording of the failure are in one place.
func decode[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var body T
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) {
			writeJSON(w, http.StatusRequestEntityTooLarge,
				apiError{Error: "That was too long to accept"})
			return body, false
		}
		writeJSON(w, http.StatusBadRequest, apiError{Error: "Could not read what was sent"})
		return body, false
	}
	if dec.More() {
		writeJSON(w, http.StatusBadRequest,
			apiError{Error: "Could not read what was sent", Reason: "more than one JSON value in the body"})
		return body, false
	}
	return body, true
}

// --- people ----------------------------------------------------------------

func (s *Server) searchPeople(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.app.SearchPeople(r.URL.Query().Get("q"), on))
}

func (s *Server) personDetail(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	id := ids.PartyID(r.PathValue("id"))
	d, err := s.app.PersonDetail(id, on)
	if err != nil {
		writeJSON(w, http.StatusNotFound, apiError{Error: "No record found for that person"})
		return
	}
	s.app.Audit.Record(audit.Entry{
		Actor: sess.Actor(), Action: "person.viewed", Subject: string(id),
		Purpose: "customer service", Allowed: true,
	})
	writeJSON(w, http.StatusOK, d)
}

type nameRequest struct {
	Title    string `json:"title"`
	Forename string `json:"forename"`
	Surname  string `json:"surname"`
	Reason   string `json:"reason"`
	On       string `json:"on"`
}

func (s *Server) changeName(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	req, ok := decode[nameRequest](w, r)
	if !ok {
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	err = s.app.ChangeName(ids.PartyID(r.PathValue("id")), req.Title, req.Forename, req.Surname,
		domain.NameChangeReason(req.Reason), on, sess.Username)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type contactRequest struct {
	Email          string          `json:"email"`
	Phone          string          `json:"phone"`
	Paperless      bool            `json:"paperless"`
	Correspondence *domain.Address `json:"correspondence"`
}

func (s *Server) updateContact(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	req, ok := decode[contactRequest](w, r)
	if !ok {
		return
	}
	if err := s.app.UpdateContact(ids.PartyID(r.PathValue("id")), req.Email, req.Phone, req.Paperless, req.Correspondence); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type moveRequest struct {
	Premises string `json:"premises"`
	On       string `json:"on"`
}

func (s *Server) changeAddress(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	req, ok := decode[moveRequest](w, r)
	if !ok {
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	moved, err := s.app.ChangeAddress(ids.PartyID(r.PathValue("id")), ids.PremisesID(req.Premises), on)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"licencesMoved": moved})
}

func (s *Server) recordDeath(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	req, ok := decode[dateRequest](w, r)
	if !ok {
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	refund, err := s.app.RecordDeath(ids.PartyID(r.PathValue("id")), on)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"refund": refund})
}

type evidenceRequest struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
	SeenOn    string `json:"seenOn"`
	ExpiresOn string `json:"expiresOn"`
}

func (s *Server) addEvidence(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	req, ok := decode[evidenceRequest](w, r)
	if !ok {
		return
	}
	seenOn, err := timeline.ParseDate(req.SeenOn)
	if err != nil {
		writeErr(w, err)
		return
	}
	var expires *timeline.Date
	if req.ExpiresOn != "" {
		d, err := timeline.ParseDate(req.ExpiresOn)
		if err != nil {
			writeErr(w, err)
			return
		}
		expires = &d
	}
	e, err := s.app.AddEvidence(ids.PartyID(r.PathValue("id")), domain.EvidenceKind(req.Kind),
		req.Reference, seenOn, expires, sess.Username)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) subjectAccess(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	id := ids.PartyID(r.PathValue("id"))
	export, err := s.app.SubjectAccess(id, on)
	if err != nil {
		writeJSON(w, http.StatusNotFound, apiError{Error: "No record found for that person"})
		return
	}
	s.app.Audit.Record(audit.Entry{
		Actor: sess.Actor(), Action: "person.subject_access_export", Subject: string(id),
		Purpose: "subject access request", Allowed: true,
	})
	writeJSON(w, http.StatusOK, export)
}

// --- notes -----------------------------------------------------------------

type noteRequest struct {
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

func (s *Server) addNote(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	req, ok := decode[noteRequest](w, r)
	if !ok {
		return
	}
	n, err := s.app.AddNote(req.Subject, req.Text, sess.DisplayName)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) notes(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	writeJSON(w, http.StatusOK, s.app.Notes(r.URL.Query().Get("subject")))
}

// --- money -----------------------------------------------------------------

func (s *Server) account(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	account, err := s.app.Account(ids.LicenceID(r.PathValue("id")), on)
	if err != nil {
		writeJSON(w, http.StatusNotFound, apiError{Error: "No licence found with that reference"})
		return
	}
	writeJSON(w, http.StatusOK, account)
}

type paymentRequest struct {
	Amount    string `json:"amount"`
	Method    string `json:"method"`
	On        string `json:"on"`
	Reference string `json:"reference"`
}

func (s *Server) takePayment(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	req, ok := decode[paymentRequest](w, r)
	if !ok {
		return
	}
	amount, err := money.FromPounds(req.Amount)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "Enter an amount such as 15.00"})
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.app.TakePayment(ids.LicenceID(r.PathValue("id")), amount,
		domain.PaymentMethod(req.Method), on, req.Reference, sess.DisplayName)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

type planRequest struct {
	Kind   string `json:"kind"`
	Method string `json:"method"`
	On     string `json:"on"`
}

func (s *Server) setPlan(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	req, ok := decode[planRequest](w, r)
	if !ok {
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.SetPaymentPlan(ids.LicenceID(r.PathValue("id")), payment.PlanKind(req.Kind),
		domain.PaymentMethod(req.Method), on); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type mandateRequest struct {
	AccountName   string `json:"accountName"`
	SortCode      string `json:"sortCode"`
	AccountNumber string `json:"accountNumber"`
	CollectionDay int    `json:"collectionDay"`
	On            string `json:"on"`
}

func (s *Server) setDirectDebit(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	req, ok := decode[mandateRequest](w, r)
	if !ok {
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.SetDirectDebit(ids.LicenceID(r.PathValue("id")), req.AccountName,
		req.SortCode, req.AccountNumber, req.CollectionDay, on); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type renewRequest struct {
	Method string `json:"method"`
	Plan   string `json:"plan"`
}

func (s *Server) renew(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	req, ok := decode[renewRequest](w, r)
	if !ok {
		return
	}
	method := domain.PaymentMethod(req.Method)
	if method == "" {
		method = domain.MethodCard
	}
	l, err := s.app.Renew(ids.LicenceID(r.PathValue("id")), method, payment.PlanKind(req.Plan))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

// --- visits ----------------------------------------------------------------

type visitRequest struct {
	Premises string `json:"premises"`
	On       string `json:"on"`
	Outcome  string `json:"outcome"`
	Note     string `json:"note"`
}

func (s *Server) recordVisit(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	req, ok := decode[visitRequest](w, r)
	if !ok {
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	v, err := s.app.RecordVisit(ids.PremisesID(req.Premises), on, sess.DisplayName,
		domain.VisitOutcome(req.Outcome), req.Note)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) visits(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	writeJSON(w, http.StatusOK, s.app.Visits(ids.PremisesID(r.PathValue("id"))))
}

// --- admin -----------------------------------------------------------------

func (s *Server) equality(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	attribute := r.PathValue("attribute")
	body := map[string]any{"report": s.app.Equality.Report(attribute)}
	if outcome := r.URL.Query().Get("outcome"); outcome != "" {
		body["rates"] = s.app.Equality.Rates(attribute, outcome)
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) renewalReminders(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"sent": s.app.SendRenewalReminders(on)})
}

// reference supplies the option lists the interface needs, so wording lives in
// one place rather than being duplicated in the client.
func (s *Server) reference(w http.ResponseWriter, _ *http.Request, _ *auth.Session) {
	type option struct {
		Value string `json:"value"`
		Label string `json:"label"`
	}
	opts := func(vals ...interface{ Label() string }) []option {
		out := make([]option, 0, len(vals))
		for _, v := range vals {
			out = append(out, option{Value: valueOf(v), Label: v.Label()})
		}
		return out
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"paymentMethods": opts(domain.MethodCard, domain.MethodDirectDebit, domain.MethodPaymentCard, domain.MethodCash, domain.MethodCheque),
		"cancellationReasons": opts(domain.CancelNoLongerWatch, domain.CancelMovedAbroad,
			domain.CancelDeceased, domain.CancelOver75, domain.CancelIssuedInError),
		"visitOutcomes": opts(domain.OutcomeNoAnswer, domain.OutcomeLicenceSold, domain.OutcomeDeclarationTaken,
			domain.OutcomeAlreadyLicensed, domain.OutcomeNotRequired, domain.OutcomeRefused),
		"evidenceKinds":     opts(domain.EvidenceBlindRegistration, domain.EvidencePensionCredit, domain.EvidenceCareHome),
		"nameChangeReasons": opts(domain.NameChangeMarriage, domain.NameChangeDeedPoll, domain.NameChangeCorrection),
		"letterKinds": opts(domain.LetterLicenceConfirmation, domain.LetterRenewalReminder, domain.LetterPaymentReminder,
			domain.LetterDeclarationReceived, domain.LetterDeclarationReview, domain.LetterRefundConfirmation, domain.LetterDetailsChanged),
	})
}

func valueOf(v any) string {
	switch t := v.(type) {
	case domain.PaymentMethod:
		return string(t)
	case domain.CancellationReason:
		return string(t)
	case domain.VisitOutcome:
		return string(t)
	case domain.EvidenceKind:
		return string(t)
	case domain.NameChangeReason:
		return string(t)
	case domain.LetterKind:
		return string(t)
	}
	return ""
}

// --- addresses -------------------------------------------------------------

type addressRequest struct {
	Line1            string `json:"line1"`
	Line2            string `json:"line2"`
	Town             string `json:"town"`
	Postcode         string `json:"postcode"`
	Kind             string `json:"kind"`
	Parent           string `json:"parent"`
	UnitsCovered     int    `json:"unitsCovered"`
	AcknowledgedNear bool   `json:"acknowledgedNear"`
}

func (r addressRequest) address() domain.Address {
	return domain.Address{Line1: r.Line1, Line2: r.Line2, Town: r.Town, Postcode: r.Postcode}
}

func (s *Server) checkAddress(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	req, ok := decode[addressRequest](w, r)
	if !ok {
		return
	}
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.app.CheckAddress(req.address(), on))
}

func (s *Server) createAddress(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	req, ok := decode[addressRequest](w, r)
	if !ok {
		return
	}
	if req.Line1 == "" || req.Postcode == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "Enter at least the first line and the postcode"})
		return
	}
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}

	kind := domain.PremisesKind(req.Kind)
	if kind == "" {
		kind = domain.KindDwelling
	}
	created, check, err := s.app.CreateAddress(req.address(), kind, ids.PremisesID(req.Parent),
		req.UnitsCovered, req.AcknowledgedNear, on)
	if errors.Is(err, app.ErrDuplicateAddress) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":     "This address is already on the system",
			"duplicate": check,
		})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if created.ID == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"needsConfirmation": true,
			"duplicate":         check,
		})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// --- corrections -----------------------------------------------------------

func (s *Server) reinstateLicence(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	req, ok := decode[dateRequest](w, r)
	if !ok {
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.ReinstateLicence(ids.LicenceID(r.PathValue("id")), sess.DisplayName, on); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) reverseDeath(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	req, ok := decode[dateRequest](w, r)
	if !ok {
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.ReverseDeath(ids.PartyID(r.PathValue("id")), sess.DisplayName, on); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
