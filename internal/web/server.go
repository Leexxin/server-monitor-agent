package web

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"sma/internal/buildinfo"
	"sma/internal/config"
	"sma/internal/discovery"
	"sma/internal/metric"
	"sma/internal/snapshot"
)

type Server struct {
	cfg       config.Config
	snapshot  *snapshot.Service
	discovery *discovery.Service
	build     buildinfo.Info
	token     string
	sem       chan struct{}
	scrapes   atomic.Uint64
	errors    atomic.Uint64
}

func New(cfg config.Config, service *snapshot.Service, discoveryService *discovery.Service, build buildinfo.Info) (*Server, error) {
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
	return &Server{
		cfg: cfg, snapshot: service, discovery: discoveryService, build: build, token: token,
		sem: make(chan struct{}, cfg.MaxConcurrentRequests),
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.TelemetryPath, s.protected(s.metrics))
	mux.HandleFunc("/v1/snapshot", s.protected(s.snapshotJSON))
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/readyz", s.ready)
	mux.HandleFunc("/version", s.version)
	if s.discovery != nil {
		mux.HandleFunc("/v1/discovery/capabilities", s.protected(s.discoveryCapabilities))
		mux.HandleFunc("/v1/discovery/runs", s.protected(s.discoveryRuns))
		mux.HandleFunc("/v1/discovery/runs/", s.protected(s.discoveryRun))
	}
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

func (s *Server) discoveryCapabilities(w http.ResponseWriter, r *http.Request) {
	if !allowRead(w, r) {
		return
	}
	writeJSON(w, r, http.StatusOK, s.discovery.Capabilities())
}

func (s *Server) discoveryRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		writeJSON(w, r, http.StatusOK, map[string]any{"runs": s.discovery.List()})
	case http.MethodPost:
		if s.token == "" && !remoteIsLoopback(r.RemoteAddr) {
			writeError(w, http.StatusForbidden, "discovery_auth_required", "remote discovery triggers require bearer token authentication")
			return
		}
		var request discovery.TriggerRequest
		decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid_request", "request body must be valid discovery JSON")
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid_request", "request body must contain exactly one JSON object")
			return
		}
		run, err := s.discovery.Trigger(request)
		if err != nil {
			switch {
			case errors.Is(err, discovery.ErrBusy):
				writeError(w, http.StatusConflict, "discovery_busy", "a discovery run is already active")
			case errors.Is(err, discovery.ErrUnknownDetector):
				writeError(w, http.StatusBadRequest, "unknown_detector", err.Error())
			default:
				writeError(w, http.StatusServiceUnavailable, "discovery_unavailable", err.Error())
			}
			return
		}
		location := "/v1/discovery/runs/" + run.ID
		w.Header().Set("Location", location)
		w.Header().Set("Retry-After", "1")
		writeJSON(w, r, http.StatusAccepted, discovery.TriggerResponse{
			ID: run.ID, Status: run.Status, CreatedAt: run.CreatedAt, Links: discovery.RunLinks{Self: location},
		})
	default:
		w.Header().Set("Allow", "GET, HEAD, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "supported methods are GET, HEAD and POST")
	}
}

func (s *Server) discoveryRun(w http.ResponseWriter, r *http.Request) {
	if !allowRead(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/discovery/runs/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not_found", "discovery run not found")
		return
	}
	run, err := s.discovery.Get(id)
	if errors.Is(err, discovery.ErrRunNotFound) {
		writeError(w, http.StatusNotFound, "discovery_run_not_found", "discovery run not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "discovery_read_failed", "unable to read discovery run")
		return
	}
	writeJSON(w, r, http.StatusOK, run)
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

func remoteIsLoopback(remoteAddress string) bool {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		host = remoteAddress
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
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
