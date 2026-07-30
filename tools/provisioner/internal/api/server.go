package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/telecom-poc/provisioner/internal/milenage"
	"github.com/telecom-poc/provisioner/internal/profile"
	"github.com/telecom-poc/provisioner/internal/provisioning"
	"github.com/telecom-poc/provisioner/internal/store"
	"github.com/telecom-poc/provisioner/internal/subscriber"
)

const (
	plmnPrefix = "99970"
	imsiBase   = "999700000000000" // MSIN increments from here
	testOP     = "cdc202d5123e20f62b6d676ac72cb318"
)

type Server struct {
	st     store.Store
	token  string
	actor  string
	keygen func() []byte
}

func NewServer(st store.Store, token, actor string, keygen func() []byte) *Server {
	return &Server{st: st, token: token, actor: actor, keygen: keygen}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("POST /subscribers/issue", s.auth(s.handleIssue))
	mux.HandleFunc("POST /subscribers", s.auth(s.handleAdd))
	mux.HandleFunc("GET /subscribers", s.auth(s.handleList))
	mux.HandleFunc("GET /subscribers/{imsi}", s.auth(s.handleGet))
	mux.HandleFunc("DELETE /subscribers/{imsi}", s.auth(s.handleRemove))
	mux.HandleFunc("POST /subscribers/{imsi}/suspend", s.auth(s.handleSuspend))
	mux.HandleFunc("POST /subscribers/{imsi}/resume", s.auth(s.handleResume))
	mux.HandleFunc("PATCH /subscribers/{imsi}/plan", s.auth(s.handlePlan))
	mux.HandleFunc("PATCH /subscribers/{imsi}/ip", s.auth(s.handleIP))
	mux.HandleFunc("GET /subscribers/{imsi}/history", s.auth(s.handleHistory))
	mux.HandleFunc("GET /subscribers/{imsi}/usage", s.auth(s.handleUsage))
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == "" || got != s.token {
			writeErr(w, http.StatusUnauthorized, "invalid or missing API token")
			return
		}
		next(w, r)
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) audit(r *http.Request, imsi string, action provisioning.Action, reason, note string) {
	_ = s.st.AppendAudit(r.Context(), provisioning.AuditRecord{
		IMSI: imsi, Action: string(action), Reason: reason, Note: note, Actor: s.actor, At: time.Now().UTC(),
	})
}

func (s *Server) nextIMSI(r *http.Request) (string, error) {
	max, err := s.st.MaxMSIN(r.Context(), plmnPrefix)
	if err != nil {
		return "", err
	}
	base, _ := strconv.ParseUint(imsiBase[len(plmnPrefix):], 10, 64)
	return plmnPrefix + fmt.Sprintf("%0*d", len(imsiBase)-len(plmnPrefix), base+max+1), nil
}

func (s *Server) handleIssue(w http.ResponseWriter, r *http.Request) {
	var req IssueRequest
	json.NewDecoder(r.Body).Decode(&req)
	if !provisioning.ValidReason(provisioning.ActionIssue, req.Reason) {
		writeErr(w, http.StatusBadRequest, "valid reason required")
		return
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}
	apn := req.APN
	if apn == "" {
		apn = "internet"
	}
	op, _ := hex.DecodeString(testOP)
	dl, ul := subscriber.DefaultAMBR()
	out := make([]IssuedSIM, 0, count)
	for i := 0; i < count; i++ {
		imsi, err := s.nextIMSI(r)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ki := s.keygen()
		opc, err := milenage.ComputeOPc(ki, op)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		sub := subscriber.Subscriber{
			IMSI: imsi, K: strings.ToUpper(hex.EncodeToString(ki)),
			OPc: strings.ToUpper(hex.EncodeToString(opc)), AMF: "8000", APN: apn, DL: dl, UL: ul,
		}
		prof := profile.FromSubscriber(sub)
		if err := prof.Validate(sub); err != nil { // fail-loud
			writeErr(w, http.StatusInternalServerError, "profile validation failed: "+err.Error())
			return
		}
		if err := s.st.Insert(r.Context(), sub); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.audit(r, imsi, provisioning.ActionIssue, req.Reason, req.Note)
		out = append(out, IssuedSIM{IMSI: imsi, K: sub.K, OPc: sub.OPc, UsimBlock: prof.UsimBlock()})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	var req AddRequest
	json.NewDecoder(r.Body).Decode(&req)
	if !provisioning.ValidReason(provisioning.ActionAdd, req.Reason) {
		writeErr(w, http.StatusBadRequest, "valid reason required")
		return
	}
	if req.IMSI == "" || req.K == "" || req.OPc == "" {
		writeErr(w, http.StatusBadRequest, "imsi, k and opc are required")
		return
	}
	apn := req.APN
	if apn == "" {
		apn = "internet"
	}
	dl, ul := subscriber.DefaultAMBR()
	sub := subscriber.Subscriber{IMSI: req.IMSI, K: strings.ToUpper(req.K), OPc: strings.ToUpper(req.OPc), AMF: "8000", APN: apn, DL: dl, UL: ul}
	prof := profile.FromSubscriber(sub)
	if err := prof.Validate(sub); err != nil {
		writeErr(w, http.StatusInternalServerError, "profile validation failed: "+err.Error())
		return
	}
	if err := s.st.Insert(r.Context(), sub); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, req.IMSI, provisioning.ActionAdd, req.Reason, req.Note)
	writeJSON(w, http.StatusCreated, IssuedSIM{IMSI: sub.IMSI, K: sub.K, OPc: sub.OPc, UsimBlock: prof.UsimBlock()})
}

func ambrStr(a subscriber.AMBR) string {
	unit := map[int]string{0: "bps", 1: "K", 2: "M", 3: "G", 4: "T"}[a.Unit]
	return strconv.Itoa(a.Value) + unit
}

func view(r store.Record) SubscriberView {
	status := "active"
	if r.Barred {
		status = "suspended"
	}
	return SubscriberView{
		IMSI: r.IMSI, Status: status, DL: ambrStr(r.DL), UL: ambrStr(r.UL),
		StaticIPv4: r.StaticIPv4, LastAction: r.LastAction, LastReason: r.LastReason,
	}
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	recs, err := s.st.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]SubscriberView, 0, len(recs))
	for _, rec := range recs {
		out = append(out, view(rec))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	rec, err := s.st.Get(r.Context(), r.PathValue("imsi"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "subscriber not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view(*rec))
}

func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	imsi := r.PathValue("imsi")
	if err := s.st.Delete(r.Context(), imsi); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, imsi, provisioning.ActionRemove, "DEPROVISION", "")
	w.WriteHeader(http.StatusNoContent)
}

// --- temporary stubs, replaced in Task 7 ---
func (s *Server) handleSuspend(w http.ResponseWriter, r *http.Request) { writeErr(w, 501, "todo") }
func (s *Server) handleResume(w http.ResponseWriter, r *http.Request)  { writeErr(w, 501, "todo") }
func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request)    { writeErr(w, 501, "todo") }
func (s *Server) handleIP(w http.ResponseWriter, r *http.Request)      { writeErr(w, 501, "todo") }
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) { writeErr(w, 501, "todo") }
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request)   { writeErr(w, 501, "todo") }
