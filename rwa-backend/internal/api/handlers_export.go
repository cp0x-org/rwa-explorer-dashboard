package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

const (
	cacheCurrent   = "public, max-age=60, stale-while-revalidate=600"
	cacheImmutable = "public, max-age=31536000, immutable"
)

// snap wraps a handler that reads the current build: 503 without a build, otherwise the shared cache headers,
// a weak ETag derived from the build and 304 for a matching If-None-Match.
func (s *Server) snap(h func(http.ResponseWriter, *http.Request, *snapshot)) http.HandlerFunc {
	return s.snapValidated(nil, h)
}

// snapValidated is snap with a parameter check that runs before the ETag comparison, so an invalid request is a 400
// and never a 304. validate returns false after writing the error itself.
func (s *Server) snapValidated(validate func(http.ResponseWriter, *http.Request) bool, h func(http.ResponseWriter, *http.Request, *snapshot)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if validate != nil && !validate(w, r) {
			return
		}
		sn := s.cache.current()
		if sn == nil {
			writeError(w, r, http.StatusServiceUnavailable, codeUnavailable, "no build is loaded yet")
			return
		}
		etag := "W/" + sn.etag
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", cacheCurrent)
		w.Header().Add("Vary", "Accept-Encoding")
		if etagMatches(r, etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		h(w, r, sn)
	}
}

// handleExport serves the stored export bytes verbatim, gzipped from a per-build precomputed copy.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	var sn *blob
	if cur := s.cache.current(); cur != nil {
		sn = &cur.blob
	}
	cc := cacheCurrent
	if b := r.URL.Query().Get("build"); b != "" {
		id, err := strconv.ParseInt(b, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, r, http.StatusBadRequest, codeBadRequest, "build must be a positive integer")
			return
		}
		sn, err = s.cache.byID(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, r, http.StatusNotFound, codeNotFound, "build "+b+" does not exist")
				return
			}
			s.internalError(w, r, "load build", err)
			return
		}
		cc = cacheImmutable
	}
	if sn == nil {
		writeError(w, r, http.StatusServiceUnavailable, codeUnavailable, "no build is loaded yet")
		return
	}
	h := w.Header()
	h.Set("ETag", sn.etag)
	h.Set("Cache-Control", cc)
	h.Add("Vary", "Accept-Encoding")
	if etagMatches(r, sn.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body := sn.raw
	if acceptsGzip(r) {
		body = sn.gz
		h.Set("Content-Encoding", "gzip")
	}
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, what string, err error) {
	s.log.Error(what, "err", err, "path", r.URL.Path, "request_id", RequestID(r.Context()))
	writeError(w, r, http.StatusInternalServerError, codeInternal, "internal error")
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request, sn *snapshot) {
	e := sn.export
	writeJSON(w, r, http.StatusOK, map[string]any{
		"generated_at": e.GeneratedAt, "build_id": sn.meta.ID, "run_id": sn.meta.RunID,
		"settings": e.Settings, "summary": e.Summary,
	})
}

type statusBuild struct {
	ID            int64     `json:"id"`
	RunID         int64     `json:"run_id"`
	GeneratedAt   time.Time `json:"generated_at"`
	ScoreVersion  string    `json:"score_version"`
	ExportSHA256  string    `json:"export_sha256"`
	SchemaVersion int       `json:"schema_version"`
}

type statusRun struct {
	ID             int64      `json:"id"`
	RunKey         string     `json:"run_key"`
	Mode           string     `json:"mode"`
	Trigger        string     `json:"trigger"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	CompletedCount int        `json:"completed_count"`
	FailedCount    int        `json:"failed_count"`
	Error          *string    `json:"error"`
}

type statusSchedule struct {
	Cron     *string `json:"cron"`
	Interval *string `json:"interval"`
}

type statusResponse struct {
	CurrentBuild     *statusBuild   `json:"current_build"`
	LastRun          *statusRun     `json:"last_run"`
	NextRunAt        *time.Time     `json:"next_run_at"`
	Schedule         statusSchedule `json:"schedule"`
	CollectorVersion *string        `json:"collector_version"`
	APIVersion       string         `json:"api_version"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	resp := statusResponse{APIVersion: Version}
	if m, err := s.st.CurrentBuildMeta(ctx); err == nil {
		resp.CurrentBuild = &statusBuild{m.ID, m.RunID, m.GeneratedAt.UTC(), m.ScoreVersion, m.ExportSHA256, m.SchemaVersion}
	} else if !errors.Is(err, store.ErrNotFound) {
		s.internalError(w, r, "status: current build", err)
		return
	}
	if run, err := s.st.LastRun(ctx); err == nil {
		resp.LastRun = &statusRun{run.ID, run.RunKey, run.Mode, run.Trigger, run.Status, run.StartedAt.UTC(), utcPtr(run.FinishedAt),
			run.Counters.Completed, run.Counters.Failed, run.Error}
		if run.CollectorVersion != "" {
			v := run.CollectorVersion
			resp.CollectorVersion = &v
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		s.internalError(w, r, "status: last run", err)
		return
	}
	resp.Schedule, resp.NextRunAt = s.schedule(r)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, r, http.StatusOK, resp)
}

// schedule reports the collector schedule from the same env the collector reads. COLLECT_INTERVAL wins over
// COLLECT_CRON (as in the scheduler); the cron is evaluated in UTC; an interval counts from the last successful run.
func (s *Server) schedule(r *http.Request) (statusSchedule, *time.Time) {
	now := s.now().UTC()
	if iv := s.cfg.CollectInterval; iv > 0 {
		str := iv.String()
		sc := statusSchedule{Interval: &str}
		last, err := s.st.LastSuccessfulRunStart(r.Context())
		if err != nil {
			return sc, nil // no successful run yet: the next start is not predictable
		}
		next := last.UTC().Add(iv)
		if next.Before(now) {
			next = now
		}
		return sc, &next
	}
	if s.cfg.CollectCron == "" {
		return statusSchedule{}, nil
	}
	spec := s.cfg.CollectCron
	sc := statusSchedule{Cron: &spec}
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		return sc, nil
	}
	if ss, ok := sched.(*cron.SpecSchedule); ok {
		ss.Location = time.UTC
	}
	next := sched.Next(now)
	if next.IsZero() {
		return sc, nil
	}
	return sc, &next
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
