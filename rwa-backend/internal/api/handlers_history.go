package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

const (
	defaultHistoryDays = 365
	historyCache       = "public, max-age=60"
)

type historyPoint struct {
	RunID      int64          `json:"run_id"`
	ObservedAt time.Time      `json:"observed_at"`
	Values     map[string]any `json:"values"`
}

type historyResponse struct {
	Entity    string         `json:"entity"`
	ID        *string        `json:"id"`
	Metrics   []string       `json:"metrics"`
	Step      string         `json:"step"`
	Points    []historyPoint `json:"points"`
	Truncated bool           `json:"truncated"`
}

// parseTime accepts RFC 3339 or a plain date (UTC midnight; with endOfDay the last microsecond of that day).
func parseTime(v string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Microsecond)
	}
	return t.UTC(), nil
}

var entityNames = map[string]string{"summary": "summary", "underlyings": "underlying", "tokens": "token", "issuers": "issuer", "opportunities": "opportunity"}

func (s *Server) handleHistory(entity string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var metrics []string
		seen := map[string]bool{}
		for _, m := range strings.Split(q.Get("metrics"), ",") {
			if m = strings.TrimSpace(m); m != "" && !seen[m] {
				seen[m] = true
				metrics = append(metrics, m)
			}
		}
		if len(metrics) == 0 {
			writeError(w, r, http.StatusBadRequest, codeBadRequest, "metrics is required: a comma-separated list of metric names")
			return
		}
		step := q.Get("step")
		if step == "" {
			step = store.StepRun
		}
		limit := store.DefaultHistoryLimit
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > store.MaxHistoryLimit {
				writeError(w, r, http.StatusBadRequest, codeBadRequest, fmt.Sprintf("limit must be an integer between 1 and %d", store.MaxHistoryLimit))
				return
			}
			limit = n
		}
		from := s.now().UTC().AddDate(0, 0, -defaultHistoryDays)
		if v := q.Get("from"); v != "" {
			t, err := parseTime(v, false)
			if err != nil {
				writeError(w, r, http.StatusBadRequest, codeBadRequest, "from must be an RFC 3339 timestamp or a YYYY-MM-DD date")
				return
			}
			from = t
		}
		var to *time.Time
		if v := q.Get("to"); v != "" {
			t, err := parseTime(v, true)
			if err != nil {
				writeError(w, r, http.StatusBadRequest, codeBadRequest, "to must be an RFC 3339 timestamp or a YYYY-MM-DD date")
				return
			}
			to = &t
			if to.Before(from) {
				writeError(w, r, http.StatusBadRequest, codeBadRequest, "from must not be after to")
				return
			}
		}

		id := r.PathValue("id")
		var respID *string
		sn := s.cache.current()
		known := entity == "summary" // whether the entity is in the current build
		switch entity {
		case "summary":
		case "underlyings":
			if sn != nil {
				if u := sn.underlying(id); u != nil {
					id, known = u.ID, true
				}
			}
		case "tokens":
			if n, err := strconv.ParseInt(id, 10, 64); err == nil && sn != nil {
				_, known = sn.tokenByID[n]
			}
		case "issuers":
			if sn != nil {
				_, known = sn.issuerByID[id]
			}
		case "opportunities":
			if sn != nil {
				_, known = sn.oppByKey[id]
			}
		}
		if entity != "summary" {
			respID = &id
		}

		res, err := s.st.QueryHistory(r.Context(), entity, id, metrics, &from, to, step, limit)
		if err != nil {
			var um *store.UnknownMetricError
			switch {
			case errors.As(err, &um):
				writeError(w, r, http.StatusBadRequest, codeUnknownMetric, fmt.Sprintf("unknown metric %q for %s", um.Metric, entity),
					"metric", um.Metric, "allowed", store.HistoryMetrics(entity))
			case errors.Is(err, store.ErrUnknownStep):
				writeError(w, r, http.StatusBadRequest, codeBadRequest, "step must be run, day or week")
			case errors.Is(err, store.ErrBadID):
				writeError(w, r, http.StatusBadRequest, codeBadRequest, fmt.Sprintf("%q is not a valid %s id", id, entity))
			case errors.Is(err, store.ErrUnknownEntity):
				writeError(w, r, http.StatusNotFound, codeNotFound, "unknown history entity "+entity)
			default:
				s.internalError(w, r, "query history", err)
			}
			return
		}
		if len(res.Points) == 0 && !known {
			writeError(w, r, http.StatusNotFound, codeNotFound, fmt.Sprintf("%s %q is unknown", entityNames[entity], r.PathValue("id")))
			return
		}
		// Echo the metrics deduplicated, in request order.
		points := make([]historyPoint, len(res.Points))
		for i, p := range res.Points {
			vals := make(map[string]any, len(p.Values))
			for k, v := range p.Values {
				if t, ok := v.(time.Time); ok {
					v = t.UTC()
				}
				vals[k] = v
			}
			points[i] = historyPoint{p.RunID, p.ObservedAt.UTC(), vals}
		}
		w.Header().Set("Cache-Control", historyCache)
		writeJSON(w, r, http.StatusOK, historyResponse{Entity: entity, ID: respID, Metrics: metrics, Step: step, Points: points, Truncated: res.Truncated})
	}
}

type metricInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// nonChartMetrics are queryable history columns that are not numeric series (text, arrays, timestamps, JSON); the
// catalog is for chart selects, so they are not listed (they are still accepted by the history endpoints).
var nonChartMetrics = map[string]bool{"status": true, "eligibility": true, "flags": true, "source_updated_at": true, "breakdowns": true}

func metricType(name string) string {
	switch {
	case strings.HasSuffix(name, "_usd"), strings.Contains(name, "_usd_"):
		return "money"
	case strings.HasSuffix(name, "_percent"):
		return "percent"
	case strings.HasSuffix(name, "_score"):
		return "score"
	}
	return "count"
}

func (s *Server) handleHistoryMetrics(w http.ResponseWriter, r *http.Request) {
	out := map[string][]metricInfo{}
	for _, e := range []string{"summary", "underlyings", "tokens", "issuers", "opportunities"} {
		list := []metricInfo{}
		for _, n := range store.HistoryMetrics(e) {
			if !nonChartMetrics[n] {
				list = append(list, metricInfo{n, metricType(n)})
			}
		}
		out[e] = list
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, r, http.StatusOK, map[string]any{"entities": out})
}
