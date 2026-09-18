package web

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"sma/internal/buildinfo"
	"sma/internal/config"
	"sma/internal/metric"
	"sma/internal/snapshot"
)

type Server struct {
	cfg      config.Config
	snapshot *snapshot.Service
	build    buildinfo.Info
	token    string
	sem      chan struct{}
	scrapes  atomic.Uint64
	errors   atomic.Uint64
}

func New(cfg config.Config, service *snapshot.Service, build buildinfo.Info) (*Server, error) {
	token := ""
	if cfg.AuthTokenFile != "" {
		f, err := os.Open(cfg.AuthTokenFile)
		if err != nil {
			return nil, fmt.Errorf("open auth token file: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(f, 4097))
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("read auth token file: %w", err)
		}
		if len(data) > 4096 {
			return nil, errors.New("auth token exceeds 4096 bytes")
		}
		info, err := os.Stat(cfg.AuthTokenFile)
		if err != nil {
			return nil, fmt.Errorf("stat auth token file: %w", err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("auth token file must not be accessible by group or others")
		}
		token = strings.TrimSpace(string(data))
		if token == "" {
			return nil, errors.New("auth token file is empty")
		}
	}
	return &Server{cfg: cfg, snapshot: service, build: build, token: token, sem: make(chan struct{}, cfg.MaxConcurrentRequests)}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.TelemetryPath, s.protected(s.metrics))
	mux.HandleFunc("/v1/snapshot", s.protected(s.snapshotJSON))
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/readyz", s.ready)
	mux.HandleFunc("/version", s.version)
	mux.HandleFunc("/", s.notFound)
	return s.secure(s.limit(mux))
}

func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr: s.cfg.ListenAddress, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 16 << 10,
	}
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	if !allowRead(w, r) {
		return
	}
	value, err := s.snapshot.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "snapshot_unavailable", "metric collection unavailable")
		return
	}
	scrapes := s.scrapes.Add(1)
	errorsCount := s.errors.Load()
	if len(value.Errors) > 0 {
		errorsCount = s.errors.Add(1)
	}
	body := metric.Prometheus(value, s.build, metric.AgentStats{Scrapes: scrapes, Errors: errorsCount})
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func (s *Server) snapshotJSON(w http.ResponseWriter, r *http.Request) {
	if !allowRead(w, r) {
		return
	}
	value, err := s.snapshot.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "snapshot_unavailable", "metric collection unavailable")
		return
	}
	writeJSON(w, r, http.StatusOK, value)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if !allowRead(w, r) {
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if !allowRead(w, r) {
		return
	}
	value, err := s.snapshot.Get(r.Context())
	if err != nil || !s.snapshot.Ready(value) {
		writeJSON(w, r, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	if !allowRead(w, r) {
		return
	}
	writeJSON(w, r, http.StatusOK, s.build)
}

func (s *Server) protected(next http.HandlerFunc) http.HandlerFunc {
	if s.token == "" {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid bearer token required")
			return
		}
		provided := strings.TrimPrefix(header, "Bearer ")
		if len(provided) != len(s.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid bearer token required")
			return
		}
		next(w, r)
	}
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
}

func (s *Server) limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
			next.ServeHTTP(w, r)
		default:
			writeError(w, http.StatusServiceUnavailable, "too_many_requests", "request concurrency limit reached")
		}
	})
}

func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func allowRead(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and HEAD are supported")
	return false
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
