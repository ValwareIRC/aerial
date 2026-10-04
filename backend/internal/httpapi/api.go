// Package httpapi exposes the app over JSON. The role comes from the session,
// never from the request.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aerial-project/aerial/internal/app"
	"github.com/aerial-project/aerial/internal/audit"
	"github.com/aerial-project/aerial/internal/auth"
	"github.com/aerial-project/aerial/internal/domain"
	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/payment"
	"github.com/aerial-project/aerial/internal/projection"
	"github.com/aerial-project/aerial/internal/timeline"
)

const sessionCookie = "aerial_session"

type Server struct {
	app    *app.App
	secure bool
}

// New builds the API. secure marks the session cookie Secure, which requires
// the service to be behind TLS.
func New(a *app.App, secure bool) *Server {
	return &Server{app: a, secure: secure}
}

type handlerWithSession func(http.ResponseWriter, *http.Request, *auth.Session)

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/session", s.logIn)
	mux.HandleFunc("DELETE /api/session", s.logOut)
	mux.HandleFunc("GET /api/session", s.guard(s.whoAmI))

	staff := []audit.Role{audit.RoleService, audit.RoleAdmin}
	all := []audit.Role{audit.RoleService, audit.RoleAdmin, audit.RoleEnforcement, audit.RoleSubject}

	mux.HandleFunc("GET /api/overview", s.guard(s.overview, staff...))
	mux.HandleFunc("GET /api/premises", s.guard(s.searchPremises, staff...))
	mux.HandleFunc("GET /api/premises/{id}", s.guard(s.premisesDetail, staff...))
	mux.HandleFunc("GET /api/licences", s.guard(s.listLicences, staff...))
	mux.HandleFunc("GET /api/licences/{id}", s.guard(s.licenceDetail, staff...))
	mux.HandleFunc("POST /api/licences", s.guard(s.issueLicence, staff...))
	mux.HandleFunc("POST /api/licences/{id}/cancel", s.guard(s.cancelLicence, staff...))
	mux.HandleFunc("POST /api/licences/{id}/transfer", s.guard(s.transferLicence, staff...))
	mux.HandleFunc("POST /api/declarations", s.guard(s.declare, staff...))
	mux.HandleFunc("GET /api/parties/{id}/history", s.guard(s.partyHistory, staff...))

	mux.HandleFunc("GET /api/worklist", s.guard(s.worklist, audit.RoleEnforcement))
	mux.HandleFunc("GET /api/premises/{id}/enforcement", s.guard(s.enforcementView, audit.RoleEnforcement))

	mux.HandleFunc("GET /api/fees/quote", s.guard(s.quote, all...))
	mux.HandleFunc("GET /api/fees/coverage", s.guard(s.coverage, all...))
	mux.HandleFunc("GET /api/plans", s.guard(s.plan, all...))
	mux.HandleFunc("GET /api/audit", s.guard(s.auditLog, audit.RoleAdmin))
	mux.HandleFunc("GET /api/licence", s.licenceNotice)

	s.attachOperations(mux)
	return s.withMiddleware(mux)
}

// --- session ---------------------------------------------------------------

// guard requires a session, and optionally one of a set of roles.
func (s *Server) guard(next handlerWithSession, roles ...audit.Role) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, apiError{Error: "Sign in to continue"})
			return
		}
		sess, err := s.app.Users.Lookup(cookie.Value)
		if err != nil {
			s.clearCookie(w)
			writeJSON(w, http.StatusUnauthorized, apiError{Error: "Your session has ended. Sign in again."})
			return
		}
		if len(roles) > 0 {
			permitted := false
			for _, role := range roles {
				if sess.Role == role {
					permitted = true
					break
				}
			}
			if !permitted {
				s.app.Audit.Record(audit.Entry{
					Actor: sess.Actor(), Action: "http." + r.Method + " " + r.URL.Path,
					Subject: r.URL.Path, Purpose: "access control", Allowed: false,
					Detail: "role not permitted",
				})
				writeJSON(w, http.StatusForbidden, apiError{Error: "You do not have access to this"})
				return
			}
		}
		next(w, r, sess)
	}
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) logIn(w http.ResponseWriter, r *http.Request) {
	c, ok := decode[credentials](w, r)
	if !ok {
		return
	}
	sess, err := s.app.Users.Authenticate(c.Username, c.Password)
	if err != nil {
		detail := ""
		status, message := http.StatusUnauthorized, "Username or password not recognised"
		if errors.Is(err, auth.ErrTooManyAttempts) {
			status, message = http.StatusTooManyRequests,
				"Too many failed attempts. Wait fifteen minutes and try again."
			detail = "locked out"
		}
		s.app.Audit.Record(audit.Entry{
			Actor: audit.Actor{ID: c.Username, Role: "UNKNOWN"}, Action: "session.sign_in",
			Subject: c.Username, Purpose: "access control", Allowed: false, Detail: detail,
		})
		writeJSON(w, status, apiError{Error: message})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: sess.Token, Path: "/",
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
		Expires: sess.Expires,
	})
	s.app.Audit.Record(audit.Entry{
		Actor: sess.Actor(), Action: "session.sign_in", Subject: sess.Username,
		Purpose: "access control", Allowed: true,
	})
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) logOut(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		s.app.Users.Revoke(cookie.Value)
	}
	s.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) whoAmI(w http.ResponseWriter, _ *http.Request, sess *auth.Session) {
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/",
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// --- helpers ---------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type apiError struct {
	Error  string `json:"error"`
	Reason string `json:"reason,omitempty"`
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrRateNotKnown):
		writeJSON(w, http.StatusUnprocessableEntity, apiError{"No fee is recorded for that licence type on that date", err.Error()})
	case errors.Is(err, domain.ErrConcessionPolicyUnknown):
		writeJSON(w, http.StatusUnprocessableEntity, apiError{"That discount cannot be applied to that licence type", err.Error()})
	case errors.Is(err, domain.ErrVisitProhibited):
		writeJSON(w, http.StatusForbidden, apiError{"Visits are not permitted at this address", err.Error()})
	case errors.Is(err, projection.ErrWrongRole):
		writeJSON(w, http.StatusForbidden, apiError{"You do not have access to this", err.Error()})
	case errors.Is(err, domain.ErrOverlappingLicence):
		writeJSON(w, http.StatusConflict, apiError{"This address is already licensed for part of that period", err.Error()})
	case errors.Is(err, domain.ErrConcessionNotEvidenced):
		writeJSON(w, http.StatusUnprocessableEntity, apiError{"Record the evidence for this discount before applying it", err.Error()})
	case errors.Is(err, domain.ErrNotLicensable):
		writeJSON(w, http.StatusConflict, apiError{"This address is covered by another licence and cannot have its own", err.Error()})
	case errors.Is(err, domain.ErrLicenceState):
		writeJSON(w, http.StatusConflict, apiError{"That cannot be done to this licence", err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, apiError{"The request could not be completed", err.Error()})
	}
}

func dateParam(r *http.Request, name string) (timeline.Date, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return today(), nil
	}
	return timeline.ParseDate(raw)
}

func today() timeline.Date {
	n := time.Now().UTC()
	return timeline.NewDate(n.Year(), n.Month(), n.Day())
}

// --- handlers --------------------------------------------------------------

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": app.Name,
		"version": app.Version,
	})
}

// licenceNotice states the terms the running software is licensed under. It
// needs no session: a notice nobody can read is not a notice.
func (s *Server) licenceNotice(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service":   app.Name,
		"version":   app.Version,
		"copyright": app.Copyright,
		"licence":   app.LicenceName,
		"summary":   app.LicenceNotice,
		"openSource": "no — this licence discriminates by person and by field of endeavour, " +
			"so it is source-available rather than open source",
	})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.app.Overview(on))
}

func (s *Server) searchPremises(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.app.Search(r.URL.Query().Get("q"), on))
}

func (s *Server) premisesDetail(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	detail, err := s.app.PremisesDetail(ids.PremisesID(r.PathValue("id")), on)
	if err != nil {
		writeJSON(w, http.StatusNotFound, apiError{Error: "No record found for that address"})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) worklist(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.app.Worklist(r.URL.Query().Get("q"), on))
}

func (s *Server) enforcementView(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	snap, err := s.app.Enforcement.Snapshot(sess.Actor(), ids.PremisesID(r.PathValue("id")), on)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) listLicences(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.app.ListLicences(on))
}

func (s *Server) licenceDetail(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	// The detail carries a refund figure for every cancellation reason, so the
	// console can show what each one is worth without working out money of
	// its own.
	detail, err := s.app.LicenceDetail(ids.LicenceID(r.PathValue("id")), on)
	if err != nil {
		writeJSON(w, http.StatusNotFound, apiError{Error: "No licence found with that reference"})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) partyHistory(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	h, err := s.app.Service.History(sess.Actor(), ids.PartyID(r.PathValue("id")))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h)
}

type issueRequest struct {
	Holder     string `json:"holder"`
	Premises   string `json:"premises"`
	Class      string `json:"class"`
	Concession string `json:"concession"`
	Start      string `json:"start"`
	Months     int    `json:"months"`
	Method     string `json:"method"`
	Plan       string `json:"plan"`
}

func (s *Server) issueLicence(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	req, ok := decode[issueRequest](w, r)
	if !ok {
		return
	}
	start, err := timeline.ParseDate(req.Start)
	if err != nil {
		writeErr(w, err)
		return
	}
	if req.Months == 0 {
		req.Months = domain.DefaultTermMonths
	}
	concession := domain.Concession(req.Concession)
	if concession == "" {
		concession = domain.ConcessionNone
	}
	method := domain.PaymentMethod(req.Method)
	if method == "" {
		method = domain.MethodCard
	}
	planKind := payment.PlanKind(req.Plan)
	if planKind == "" {
		planKind = payment.PlanAnnual
	}
	l, quote, err := s.app.IssueWithPayment(ids.PartyID(req.Holder), ids.PremisesID(req.Premises),
		domain.LicenceClass(req.Class), concession, start, req.Months, method, planKind)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"licence": l, "quote": quote})
}

type dateRequest struct {
	On string `json:"on"`
}

type cancelRequest struct {
	On     string `json:"on"`
	Reason string `json:"reason"`
}

func (s *Server) cancelLicence(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	req, ok := decode[cancelRequest](w, r)
	if !ok {
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	id := ids.LicenceID(r.PathValue("id"))
	if !s.app.LicenceExists(id) {
		writeJSON(w, http.StatusNotFound, apiError{Error: "No licence found with that reference"})
		return
	}
	refund, err := s.app.CancelLicence(id, domain.CancellationReason(req.Reason), on)
	if err != nil {
		writeErr(w, err)
		return
	}
	summary, err := s.app.LicenceSummaryOf(id, on)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"licence": summary, "refund": refund})
}

type transferRequest struct {
	Premises string `json:"premises"`
	On       string `json:"on"`
}

func (s *Server) transferLicence(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	req, ok := decode[transferRequest](w, r)
	if !ok {
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	id := ids.LicenceID(r.PathValue("id"))
	if err := s.app.TransferLicence(id, ids.PremisesID(req.Premises), on, sess.DisplayName); err != nil {
		writeErr(w, err)
		return
	}
	summary, err := s.app.LicenceSummaryOf(id, on)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

type declareRequest struct {
	Premises string `json:"premises"`
	Kind     string `json:"kind"`
	By       string `json:"by"`
	On       string `json:"on"`
}

func (s *Server) declare(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	req, ok := decode[declareRequest](w, r)
	if !ok {
		return
	}
	on, err := timeline.ParseDate(req.On)
	if err != nil {
		writeErr(w, err)
		return
	}
	d, err := s.app.Declare(ids.PremisesID(req.Premises), domain.DeclarationKind(req.Kind), ids.PartyID(req.By), on)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) quote(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	q := r.URL.Query()
	concession := domain.Concession(q.Get("concession"))
	if concession == "" {
		concession = domain.ConcessionNone
	}
	on, err := dateParam(r, "on")
	if err != nil {
		writeErr(w, err)
		return
	}
	quote, err := s.app.Quote(domain.LicenceClass(q.Get("class")), concession, on)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, quote)
}

func (s *Server) coverage(w http.ResponseWriter, _ *http.Request, _ *auth.Session) {
	writeJSON(w, http.StatusOK, s.app.RateCoverage())
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	q := r.URL.Query()
	total, err := money.FromPounds(q.Get("total"))
	if err != nil {
		writeErr(w, err)
		return
	}
	start, err := dateParam(r, "start")
	if err != nil {
		writeErr(w, err)
		return
	}
	months := domain.DefaultTermMonths
	if raw := q.Get("months"); raw != "" {
		if months, err = strconv.Atoi(raw); err != nil {
			writeErr(w, err)
			return
		}
	}
	p, err := s.app.Plan(payment.PlanKind(q.Get("kind")), total, start, months)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// auditLog answers newest first, which is the order anybody reads a log in,
// and narrows by subject, role and outcome so that looking for refusals does
// not mean downloading everything that ever happened. The totals describe the
// whole log, not the page, so a filtered view still says what it is hiding.
func (s *Server) auditLog(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
	q := r.URL.Query()

	entries := s.app.Audit.Entries()
	if subject := q.Get("subject"); subject != "" {
		entries = s.app.Audit.For(subject)
	}

	role := audit.Role(strings.ToUpper(q.Get("role")))
	refusedOnly := q.Get("refused") == "true"

	matched := make([]audit.Entry, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if role != "" && e.Actor.Role != role {
			continue
		}
		if refusedOnly && e.Allowed {
			continue
		}
		matched = append(matched, e)
	}

	limit := 500
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, apiError{Error: "limit must be a positive whole number"})
			return
		}
		limit = n
	}

	page := matched
	if len(page) > limit {
		page = page[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entries":  page,
		"matched":  len(matched),
		"returned": len(page),
		"total":    s.app.Audit.Len(),
	})
}
