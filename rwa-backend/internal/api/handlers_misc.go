package api

import (
	"context"
	_ "embed"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

//go:embed openapi.yaml
var openAPISpec []byte

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "application/yaml; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(openAPISpec)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.st.Ping(ctx); err != nil {
		s.log.Warn("readyz: database unreachable", "err", err)
		writeError(w, r, http.StatusServiceUnavailable, codeUnavailable, "database is unreachable")
		return
	}
	if s.cache.current() == nil {
		writeError(w, r, http.StatusServiceUnavailable, codeUnavailable, "no build is loaded yet")
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleLogo(w http.ResponseWriter, r *http.Request) {
	kind, file := r.PathValue("kind"), r.PathValue("file")
	notFound := func() { writeError(w, r, http.StatusNotFound, codeNotFound, "logo not found") }
	switch kind {
	case "tokens", "issuers", "underlyings":
	default:
		notFound()
		return
	}
	dot := strings.LastIndexByte(file, '.')
	if dot <= 0 || dot == len(file)-1 {
		notFound()
		return
	}
	id, ext := file[:dot], file[dot+1:]
	l, err := s.st.GetLogo(r.Context(), kind, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound()
			return
		}
		s.internalError(w, r, "get logo", err)
		return
	}
	if !strings.EqualFold(l.Ext, ext) {
		notFound()
		return
	}
	etag := `"` + l.SHA256 + `"`
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", "public, max-age=86400")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	if etagMatches(r, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", l.ContentType)
	h.Set("Content-Length", strconv.Itoa(len(l.Bytes)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(l.Bytes)
	}
}
