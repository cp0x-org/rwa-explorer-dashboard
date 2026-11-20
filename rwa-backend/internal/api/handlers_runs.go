package api

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

const (
	defaultRunsLimit = 50
	maxRunsLimit     = 200
)

type changesSummary struct {
	NewTokensCount     int `json:"new_tokens_count"`
	RemovedTokensCount int `json:"removed_tokens_count"`
	NewPositionsCount  int `json:"new_positions_count"`
	GonePositionsCount int `json:"gone_positions_count"`
	EligibleDelta      int `json:"eligible_delta"`
}

type runItem struct {
	ID              int64           `json:"id"`
	RunKey          string          `json:"run_key"`
	Mode            string          `json:"mode"`
	Trigger         string          `json:"trigger"`
	Status          string          `json:"status"`
	StartedAt       time.Time       `json:"started_at"`
	FinishedAt      *time.Time      `json:"finished_at"`
	DurationSeconds *float64        `json:"duration_seconds"`
	SelectedCount   int             `json:"selected_count"`
	CompletedCount  int             `json:"completed_count"`
	FailedCount     int             `json:"failed_count"`
	CMCRequests     int             `json:"cmc_requests"`
	PortalsRequests int             `json:"portals_requests"`
	CMCCredits      int             `json:"cmc_credits"`
	WarningsCount   int             `json:"warnings_count"`
	Error           *string         `json:"error"`
	BuildID         *int64          `json:"build_id"`
	Changes         *changesSummary `json:"changes"`
}

type runUnderlying struct {
	UnderlyingID    string   `json:"underlying_id"`
	Ticker          string   `json:"ticker"`
	Status          string   `json:"status"`
	Error           *string  `json:"error"`
	DurationSeconds *float64 `json:"duration_seconds"`
}

type changesFull struct {
	NewTokens          []int32         `json:"new_tokens"`
	RemovedTokens      []int32         `json:"removed_tokens"`
	NewIssuers         []string        `json:"new_issuers"`
	NewPositionsCount  int             `json:"new_positions_count"`
	GonePositionsCount int             `json:"gone_positions_count"`
	EligibleDelta      int             `json:"eligible_delta"`
	Details            json.RawMessage `json:"details"`
}

type runDetailResponse struct {
	runItem
	Warnings    []string        `json:"warnings"`
	Underlyings []runUnderlying `json:"underlyings"`
	Changes     *changesFull    `json:"changes"`
}

func duration(start time.Time, end *time.Time) *float64 {
	if end == nil {
		return nil
	}
	d := end.Sub(start).Seconds()
	if d < 0 {
		d = 0
	}
	return &d
}

func toRunItem(r *store.Run, bc *store.RunBuildChanges) runItem {
	var warnings []string
	_ = json.Unmarshal(r.Warnings, &warnings)
	it := runItem{
		ID: r.ID, RunKey: r.RunKey, Mode: r.Mode, Trigger: r.Trigger, Status: r.Status,
		StartedAt: r.StartedAt.UTC(), FinishedAt: utcPtr(r.FinishedAt), DurationSeconds: duration(r.StartedAt, r.FinishedAt),
		SelectedCount: r.Counters.Selected, CompletedCount: r.Counters.Completed, FailedCount: r.Counters.Failed,
		CMCRequests: r.Counters.CMCRequests, PortalsRequests: r.Counters.PortalsRequests, CMCCredits: r.Counters.CMCCredits,
		WarningsCount: len(warnings), Error: r.Error,
	}
	if r.Mode == "import" {
		it.DurationSeconds = nil // an import's finished_at is the import time, not the 1.x collection time
	}
	if bc != nil {
		id := bc.BuildID
		it.BuildID = &id
		if bc.HasChanges {
			it.Changes = &changesSummary{len(bc.NewTokens), len(bc.RemovedTokens), bc.NewPositionsCount, bc.GonePositionsCount, bc.EligibleDelta}
		}
	}
	return it
}

func encodeRunCursor(r store.Run) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(r.StartedAt.UnixMicro(), 10) + ":" + strconv.FormatInt(r.ID, 10)))
}

func decodeRunCursor(c string) (*store.RunCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return nil, errBadCursor
	}
	us, id, ok := strings.Cut(string(b), ":")
	if !ok {
		return nil, errBadCursor
	}
	micro, err1 := strconv.ParseInt(us, 10, 64)
	rid, err2 := strconv.ParseInt(id, 10, 64)
	if err1 != nil || err2 != nil {
		return nil, errBadCursor
	}
	return &store.RunCursor{StartedAt: time.UnixMicro(micro).UTC(), ID: rid}, nil
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	limit, ok := parseLimit(w, r, defaultRunsLimit, maxRunsLimit)
	if !ok {
		return
	}
	var cur *store.RunCursor
	if c := r.URL.Query().Get("cursor"); c != "" {
		var err error
		if cur, err = decodeRunCursor(c); err != nil {
			writeError(w, r, http.StatusBadRequest, codeBadRequest, "cursor is invalid")
			return
		}
	}
	runs, err := s.st.ListRuns(r.Context(), limit+1, cur)
	if err != nil {
		s.internalError(w, r, "list runs", err)
		return
	}
	var next *string
	if len(runs) > limit {
		runs = runs[:limit]
		c := encodeRunCursor(runs[limit-1])
		next = &c
	}
	ids := make([]int64, len(runs))
	for i := range runs {
		ids[i] = runs[i].ID
	}
	changes, err := s.st.BuildChangesForRuns(r.Context(), ids)
	if err != nil {
		s.internalError(w, r, "list runs: changes", err)
		return
	}
	items := make([]runItem, len(runs))
	for i := range runs {
		var bc *store.RunBuildChanges
		if c, ok := changes[runs[i].ID]; ok {
			bc = &c
		}
		items[i] = toRunItem(&runs[i], bc)
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, r, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, r, http.StatusBadRequest, codeBadRequest, "run id must be a positive integer")
		return
	}
	d, err := s.st.GetRun(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, codeNotFound, fmt.Sprintf("run %d does not exist", id))
			return
		}
		s.internalError(w, r, "get run", err)
		return
	}
	changes, err := s.st.BuildChangesForRuns(r.Context(), []int64{id})
	if err != nil {
		s.internalError(w, r, "get run: changes", err)
		return
	}
	var bc *store.RunBuildChanges
	if c, ok := changes[id]; ok {
		bc = &c
	}
	resp := runDetailResponse{runItem: toRunItem(&d.Run, bc), Warnings: []string{}, Underlyings: []runUnderlying{}}
	_ = json.Unmarshal(d.Warnings, &resp.Warnings)
	if resp.Warnings == nil {
		resp.Warnings = []string{}
	}
	if bc != nil && bc.HasChanges {
		resp.Changes = &changesFull{
			NewTokens: nonNilSlice(bc.NewTokens), RemovedTokens: nonNilSlice(bc.RemovedTokens), NewIssuers: nonNilSlice(bc.NewIssuers),
			NewPositionsCount: bc.NewPositionsCount, GonePositionsCount: bc.GonePositionsCount, EligibleDelta: bc.EligibleDelta,
			Details: jsonOrObject(bc.Details),
		}
	}
	tickers := map[string]string{}
	if sn := s.cache.current(); sn != nil {
		for k, u := range sn.underlyingByID {
			tickers[k] = u.Ticker
		}
	}
	for _, u := range d.Underlyings {
		t, ok := tickers[u.UnderlyingID]
		if !ok { // not in the current build (removed): fall back to the id, e.g. stock:ticker:XXX
			t = u.UnderlyingID[strings.LastIndex(u.UnderlyingID, ":")+1:]
		}
		resp.Underlyings = append(resp.Underlyings, runUnderlying{u.UnderlyingID, t, u.Status, u.Error, duration2(u.StartedAt, u.FinishedAt)})
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, r, http.StatusOK, resp)
}

func duration2(start, end *time.Time) *float64 {
	if start == nil {
		return nil
	}
	return duration(*start, end)
}

func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func jsonOrObject(b []byte) json.RawMessage {
	if len(b) == 0 || !json.Valid(b) {
		return json.RawMessage("{}")
	}
	return b
}

// ---------- admin ----------

// admin protects a handler with ADMIN_TOKEN: 404 when it is not configured, 401 for a missing or wrong bearer token.
func (s *Server) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := s.cfg.AdminToken
		if token == "" {
			writeError(w, r, http.StatusNotFound, codeNotFound, "no such endpoint")
			return
		}
		scheme, got, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got)), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="rwa-admin"`)
			writeError(w, r, http.StatusUnauthorized, codeUnauthorized, "missing or invalid admin token")
			return
		}
		h(w, r)
	}
}

type adminRunBody struct {
	Mode    string   `json:"mode"`
	Symbols []string `json:"symbols"`
}

const maxAdminBody = 16 << 10

func (s *Server) handleAdminCreateRun(w http.ResponseWriter, r *http.Request) {
	var body adminRunBody
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAdminBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, r, http.StatusRequestEntityTooLarge, codePayloadTooLarge, "request body is too large")
		} else {
			writeError(w, r, http.StatusBadRequest, codeBadRequest, "request body is unreadable")
		}
		return
	}
	if len(strings.TrimSpace(string(raw))) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		err := dec.Decode(&body)
		if err == nil {
			if _, extra := dec.Token(); extra != io.EOF {
				err = errors.New("trailing data")
			}
		}
		if err != nil {
			writeError(w, r, http.StatusBadRequest, codeBadRequest, "request body must be JSON {\"mode\": ..., \"symbols\": [...]}")
			return
		}
	}
	if body.Mode == "" {
		body.Mode = "refresh"
	}
	if !slices.Contains([]string{"refresh", "add", "rebuild"}, body.Mode) {
		writeError(w, r, http.StatusBadRequest, codeBadRequest, "mode must be refresh, add or rebuild")
		return
	}
	symbols := make([]string, 0, len(body.Symbols))
	for _, sym := range body.Symbols {
		sym = strings.ToUpper(strings.TrimSpace(sym))
		if sym == "" || len(sym) > 32 || strings.ContainsAny(sym, " \t\r\n,") {
			writeError(w, r, http.StatusBadRequest, codeBadRequest, "symbols must be non-empty tickers")
			return
		}
		symbols = append(symbols, sym)
	}
	if body.Mode == "add" && len(symbols) == 0 {
		writeError(w, r, http.StatusBadRequest, codeBadRequest, "mode add requires symbols")
		return
	}
	id, err := s.st.CreateRunRequest(r.Context(), "admin-api", body.Mode, symbols)
	if err != nil {
		s.internalError(w, r, "create run request", err)
		return
	}
	s.log.Info("run requested", "request_id_db", id, "mode", body.Mode, "symbols", len(symbols), "request_id", RequestID(r.Context()))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, r, http.StatusAccepted, map[string]any{"request_id": id})
}

func (s *Server) handleAdminGetRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, r, http.StatusBadRequest, codeBadRequest, "request id must be a positive integer")
		return
	}
	rr, err := s.st.GetRunRequest(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, codeNotFound, fmt.Sprintf("run request %d does not exist", id))
			return
		}
		s.internalError(w, r, "get run request", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, r, http.StatusOK, map[string]any{
		"request_id": rr.ID, "requested_at": rr.RequestedAt.UTC(), "requested_by": rr.RequestedBy, "mode": rr.Mode,
		"symbols": rr.Symbols, "taken_at": utcPtr(rr.TakenAt), "finished_at": utcPtr(rr.FinishedAt),
		"outcome": rr.Outcome, "error": rr.Error, "run_id": rr.RunID,
	})
}
