package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const defaultFSTypes = "proc,sysfs,devtmpfs,devpts,cgroup,cgroup2,securityfs,debugfs,tracefs,pstore,configfs,fusectl,mqueue,hugetlbfs"

type Config struct {
	ListenAddress            string
	TelemetryPath            string
	EnabledCollectors        map[string]bool
	CollectorTimeout         time.Duration
	CacheTTL                 time.Duration
	ProcPath                 string
	FilesystemExcludeFSTypes map[string]bool
	FilesystemExcludeMounts  *regexp.Regexp
	DiskExcludeDevices       *regexp.Regexp
	AuthTokenFile            string
	TLSCertFile              string
	TLSKeyFile               string
	MaxConcurrentRequests    int
	ShutdownTimeout          time.Duration
}

func Parse(args []string) (Config, error) {
	fs := flag.NewFlagSet("sma", flag.ContinueOnError)
	listen := fs.String("listen-address", env("SMA_LISTEN_ADDRESS", "127.0.0.1:9108"), "HTTP listen address")
	telemetryPath := fs.String("web.telemetry-path", env("SMA_TELEMETRY_PATH", "/metrics"), "Prometheus metrics path")
	enabled := fs.String("collector.enabled", env("SMA_COLLECTOR_ENABLED", "cpu,memory,filesystem,disk,load,uptime"), "comma-separated collectors")
	collectorTimeout := fs.Duration("collector.timeout", envDuration("SMA_COLLECTOR_TIMEOUT", 2*time.Second), "timeout for a collection round")
	cacheTTL := fs.Duration("collector.cache-ttl", envDuration("SMA_CACHE_TTL", 500*time.Millisecond), "snapshot cache duration")
	procPath := fs.String("path.procfs", env("SMA_PROC_PATH", "/proc"), "procfs mount path")
	excludeFSTypes := fs.String("filesystem.exclude-fs-types", env("SMA_FILESYSTEM_EXCLUDE_FS_TYPES", defaultFSTypes), "comma-separated filesystem types")
	excludeMounts := fs.String("filesystem.exclude-mountpoints", env("SMA_FILESYSTEM_EXCLUDE_MOUNTPOINTS", "^/(dev|proc|sys|run/credentials)(/|$)"), "mountpoint exclusion regexp")
	excludeDevices := fs.String("disk.exclude-devices", env("SMA_DISK_EXCLUDE_DEVICES", "^(loop|ram|fd|sr)[0-9]*$"), "device exclusion regexp")
	authTokenFile := fs.String("web.auth-token-file", env("SMA_AUTH_TOKEN_FILE", ""), "file containing bearer token")
	tlsCertFile := fs.String("web.tls-cert-file", env("SMA_TLS_CERT_FILE", ""), "TLS certificate file")
	tlsKeyFile := fs.String("web.tls-key-file", env("SMA_TLS_KEY_FILE", ""), "TLS private key file")
	maxConcurrent := fs.Int("web.max-concurrent-requests", envInt("SMA_MAX_CONCURRENT_REQUESTS", 32), "maximum concurrent requests")
	shutdownTimeout := fs.Duration("web.shutdown-timeout", envDuration("SMA_SHUTDOWN_TIMEOUT", 10*time.Second), "graceful shutdown timeout")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	known := map[string]bool{"cpu": true, "memory": true, "filesystem": true, "disk": true, "load": true, "uptime": true}
	collectors := make(map[string]bool)
	for _, name := range splitCSV(*enabled) {
		if !known[name] {
			return Config{}, fmt.Errorf("unknown collector %q", name)
		}
		collectors[name] = true
	}
	if len(collectors) == 0 {
		return Config{}, errors.New("at least one collector must be enabled")
	}
	if !strings.HasPrefix(*telemetryPath, "/") || *telemetryPath == "/" {
		return Config{}, errors.New("web.telemetry-path must start with / and cannot be /")
	}
	for _, reserved := range []string{"/v1/snapshot", "/healthz", "/readyz", "/version"} {
		if *telemetryPath == reserved {
			return Config{}, fmt.Errorf("web.telemetry-path conflicts with reserved path %s", reserved)
		}
	}
	if *collectorTimeout <= 0 || *cacheTTL < 0 || *maxConcurrent <= 0 || *shutdownTimeout <= 0 {
		return Config{}, errors.New("timeouts and max concurrent requests must be positive")
	}
	if (*tlsCertFile == "") != (*tlsKeyFile == "") {
		return Config{}, errors.New("TLS certificate and key must be configured together")
	}
	mountRE, err := regexp.Compile(*excludeMounts)
	if err != nil {
		return Config{}, fmt.Errorf("compile filesystem exclusion regexp: %w", err)
	}
	deviceRE, err := regexp.Compile(*excludeDevices)
	if err != nil {
		return Config{}, fmt.Errorf("compile disk exclusion regexp: %w", err)
	}
	fstypes := make(map[string]bool)
	for _, item := range splitCSV(*excludeFSTypes) {
		fstypes[item] = true
	}

	return Config{
		ListenAddress: *listen, TelemetryPath: *telemetryPath, EnabledCollectors: collectors,
		CollectorTimeout: *collectorTimeout, CacheTTL: *cacheTTL, ProcPath: *procPath,
		FilesystemExcludeFSTypes: fstypes, FilesystemExcludeMounts: mountRE, DiskExcludeDevices: deviceRE,
		AuthTokenFile: *authTokenFile, TLSCertFile: *tlsCertFile, TLSKeyFile: *tlsKeyFile,
		MaxConcurrentRequests: *maxConcurrent, ShutdownTimeout: *shutdownTimeout,
	}, nil
}

func splitCSV(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(name string, fallback int) int {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
