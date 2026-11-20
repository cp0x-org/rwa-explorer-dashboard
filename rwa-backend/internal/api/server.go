// Package api is the read-only HTTP API of RWA Explorer 2.0. It serves the current build from memory, history and
// run lists from PostgreSQL, logos, and a token-protected admin endpoint that queues manual runs.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// Version is the API version reported by /api/v1/status.
const Version = "v1"

// Server is the HTTP API.
type Server struct {
	cfg   *config.Config
	st    Store
	log   *slog.Logger
	cache *cache
	now   func() time.Time
}

// New returns a Server over the pool.
func New(cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) *Server {
	return NewWithStore(cfg, pgStore{Store: store.New(pool), pool: pool}, log)
}

// NewWithStore returns a Server over any Store (tests use fakes).
func NewWithStore(cfg *config.Config, st Store, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{cfg: cfg, st: st, log: log, cache: newCache(st, log), now: time.Now}
}

// Handler returns the routed handler with all middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /api/v1/openapi.yaml", s.handleOpenAPI)

	mux.HandleFunc("GET /api/v1/export", s.handleExport)
	mux.HandleFunc("GET /api/v1/status", s.handleStatus)
	mux.HandleFunc("GET /api/v1/summary", s.snap(s.handleSummary))
	mux.HandleFunc("GET /api/v1/underlyings", s.snap(s.handleUnderlyings))
	mux.HandleFunc("GET /api/v1/underlyings/{idOrTicker}", s.snap(s.handleUnderlying))
	mux.HandleFunc("GET /api/v1/tokens", s.snap(s.handleTokens))
	mux.HandleFunc("GET /api/v1/tokens/{id}", s.snapValidated(validateTokenID, s.handleToken))
	mux.HandleFunc("GET /api/v1/issuers", s.snap(s.handleIssuers))
	mux.HandleFunc("GET /api/v1/issuers/{id}", s.snap(s.handleIssuer))
	mux.HandleFunc("GET /api/v1/opportunities", s.snap(s.handleOpportunities))

	mux.HandleFunc("GET /api/v1/runs", s.handleRuns)
	mux.HandleFunc("GET /api/v1/runs/{id}", s.handleRun)
	mux.HandleFunc("GET /api/v1/runs/{id}/events", s.handleRunEvents)
	mux.HandleFunc("POST /api/v1/admin/runs", s.admin(s.handleAdminCreateRun))
	mux.HandleFunc("GET /api/v1/admin/runs/{id}", s.admin(s.handleAdminGetRun))

	mux.HandleFunc("GET /api/v1/history/metrics", s.handleHistoryMetrics)
	mux.HandleFunc("GET /api/v1/history/summary", s.handleHistory("summary"))
	for _, e := range []string{"underlyings", "tokens", "issuers", "opportunities"} {
		mux.HandleFunc("GET /api/v1/history/"+e+"/{id}", s.handleHistory(e))
	}

	mux.HandleFunc("GET /logos/{kind}/{file}", s.handleLogo)

	// The mux answers unmatched requests with plain-text 404/405; turn those into the JSON envelope (keeping Allow).
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler, pattern := mux.Handler(r)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		c := &capture{h: http.Header{}}
		handler.ServeHTTP(c, r)
		switch c.code {
		case http.StatusNotFound:
			writeError(w, r, http.StatusNotFound, codeNotFound, "no such endpoint")
		case http.StatusMethodNotAllowed:
			w.Header().Set("Allow", c.h.Get("Allow"))
			writeError(w, r, http.StatusMethodNotAllowed, codeMethodNotAllowed, "method not allowed")
		default: // e.g. a redirect to the cleaned path
			for k, v := range c.h {
				w.Header()[k] = v
			}
			w.WriteHeader(c.code)
		}
	})
	h = s.recoverer(h)
	h = s.cors(h)
	h = securityHeaders(h)
	h = s.accessLog(h)
	h = requestID(h)
	return h
}

// capture records the status and headers of a handler whose body is discarded.
type capture struct {
	h    http.Header
	code int
}

func (c *capture) Header() http.Header         { return c.h }
func (c *capture) Write(b []byte) (int, error) { return len(b), nil }
func (c *capture) WriteHeader(code int)        { c.code = code }

// Run serves until ctx is cancelled, then shuts down gracefully (10 s). It also keeps the build cache fresh.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(s.log.Handler(), slog.LevelWarn),
	}
	ln, err := net.Listen("tcp", s.cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("api: listen: %w", err)
	}
	cacheCtx, stopCache := context.WithCancel(ctx)
	defer stopCache()
	done := make(chan struct{})
	go func() { defer close(done); s.cache.run(cacheCtx) }()

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	s.log.Info("api listening", "addr", ln.Addr().String())
	select {
	case err := <-errc:
		stopCache()
		<-done
		return fmt.Errorf("api: serve: %w", err)
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(sctx)
	stopCache()
	<-done
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("api: shutdown: %w", err)
	}
	return nil
}
