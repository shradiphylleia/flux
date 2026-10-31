// Package api exposes the HTTP interface: webhook registration, event
// ingestion, and delivery inspection.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/shradiphylleia/rival/internal/config"
	"github.com/shradiphylleia/rival/internal/queue"
)

type Server struct {
	cfg config.Config
	db  *pgxpool.Pool
	rdb *redis.Client
	q   *queue.Queue
	log *slog.Logger
}

func NewServer(cfg config.Config, db *pgxpool.Pool, rdb *redis.Client, log *slog.Logger) *Server {
	return &Server{cfg: cfg, db: db, rdb: rdb, q: queue.New(rdb), log: log}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.requestLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no route for "+r.URL.Path)
	})
	// A custom 405 handler loses chi's Allow header, so rebuild it by probing
	// the router for each method.
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			if r.Match(chi.NewRouteContext(), m, req.URL.Path) {
				w.Header().Add("Allow", m)
			}
		}
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", req.Method+" not allowed on "+req.URL.Path)
	})

	r.Get("/healthz", s.healthz)

	r.Route("/webhooks", func(r chi.Router) {
		r.Post("/", s.createWebhook)
		r.Get("/", s.listWebhooks)
		r.Get("/{id}", s.getWebhook)
		r.Patch("/{id}", s.updateWebhook) // enable/disable, change url/events
	})

	r.Post("/events", s.publishEvent)

	r.Get("/deliveries/{eventID}", s.getEventDeliveries)

	return r
}

// healthCheckTimeout bounds each dependency ping so a dead dependency yields a
// fast 503 instead of hanging until the router's request timeout.
const healthCheckTimeout = 2 * time.Second

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "postgres: "+err.Error())
		return
	}
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "redis: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.log.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}

// errorBody is the uniform error envelope:
//
//	{"error": {"code": "validation_failed", "message": "...", "fields": {...}}}
type errorBody struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeValidationError(w, status, code, msg, nil)
}

func writeValidationError(w http.ResponseWriter, status int, code, msg string, fields map[string]string) {
	var b errorBody
	b.Error.Code, b.Error.Message, b.Error.Fields = code, msg, fields
	writeJSON(w, status, b)
}
