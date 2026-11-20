package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Error codes of the error envelope.
const (
	codeNotFound         = "not_found"
	codeBadRequest       = "bad_request"
	codeUnknownMetric    = "unknown_metric"
	codeUnauthorized     = "unauthorized"
	codeMethodNotAllowed = "method_not_allowed"
	codePayloadTooLarge  = "payload_too_large"
	codeUnavailable      = "unavailable"
	codeInternal         = "internal"
)

const gzipMinSize = 1024

// acceptsGzip reports whether the client accepts gzip (an explicit q=0 refuses it).
func acceptsGzip(r *http.Request) bool {
	for _, h := range r.Header.Values("Accept-Encoding") {
		for _, part := range strings.Split(h, ",") {
			name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
			name = strings.TrimSpace(name)
			if !strings.EqualFold(name, "gzip") && name != "*" {
				continue
			}
			switch strings.ReplaceAll(strings.ToLower(params), " ", "") {
			case "q=0", "q=0.0", "q=0.00", "q=0.000":
				continue
			}
			return true
		}
	}
	return false
}

var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}

func gzipBytes(b []byte) []byte {
	var buf bytes.Buffer
	zw := gzipPool.Get().(*gzip.Writer)
	zw.Reset(&buf)
	_, _ = zw.Write(b)
	_ = zw.Close()
	gzipPool.Put(zw)
	return buf.Bytes()
}

// writeJSON encodes v as the response body; bodies above 1 KB are gzipped when the client accepts it. HEAD gets
// the headers only (no body is built).
func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Add("Vary", "Accept-Encoding")
	if r.Method == http.MethodHead {
		w.WriteHeader(status)
		return
	}
	body, err := json.Marshal(v)
	if err != nil {
		slog.Error("encode response", "path", r.URL.Path, "err", err)
		writeError(w, r, http.StatusInternalServerError, codeInternal, "internal error")
		return
	}
	if len(body) > gzipMinSize && acceptsGzip(r) {
		body = gzipBytes(body)
		h.Set("Content-Encoding", "gzip")
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeError writes {"error":{"code","message", ...extra}}.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, msg string, extra ...any) {
	e := map[string]any{"code": code, "message": msg}
	for i := 0; i+1 < len(extra); i += 2 {
		e[extra[i].(string)] = extra[i+1]
	}
	w.Header().Del("ETag")
	w.Header().Del("Content-Encoding")
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, r, status, map[string]any{"error": e})
}
