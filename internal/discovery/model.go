package discovery

import (
	"context"
	"time"
)

const SchemaVersion = "v1"

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusPartial   Status = "partial"
	StatusFailed    Status = "failed"
)

type Run struct {
	ID          string      `json:"id"`
	Status      Status      `json:"status"`
	Detectors   []string    `json:"detectors"`
	CreatedAt   time.Time   `json:"createdAt"`
	StartedAt   *time.Time  `json:"startedAt,omitempty"`
	CompletedAt *time.Time  `json:"completedAt,omitempty"`
	Result      *Result     `json:"result,omitempty"`
	Report      ReportState `json:"report"`
}

type RunSummary struct {
	ID          string     `json:"id"`
	Status      Status     `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	AssetCount  int        `json:"assetCount"`
}

type Result struct {
	SchemaVersion string          `json:"schemaVersion"`
	Host          Host            `json:"host"`
	Assets        []Asset         `json:"assets"`
	Summary       map[string]int  `json:"summary"`
	Errors        []DetectorError `json:"errors"`
}

type Host struct {
	Hostname string `json:"hostname"`
}

type Asset struct {
	ID          string     `json:"id"`
	Category    string     `json:"category"`
	Product     string     `json:"product"`
	DisplayName string     `json:"displayName"`
	Status      string     `json:"status"`
	Confidence  string     `json:"confidence"`
	Ports       []Port     `json:"ports"`
	Evidence    []Evidence `json:"evidence"`
	DetectedAt  time.Time  `json:"detectedAt"`
}

type Port struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

type Evidence struct {
	Source string `json:"source"`
	Value  string `json:"value"`
	PID    int    `json:"pid,omitempty"`
}

type DetectorError struct {
	Detector string `json:"detector"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type ReportState struct {
	Requested   bool       `json:"requested"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	LastAttempt *time.Time `json:"lastAttempt,omitempty"`
	Error       string     `json:"error,omitempty"`
}

type TriggerRequest struct {
	Detectors []string `json:"detectors,omitempty"`
	Report    *bool    `json:"report,omitempty"`
}

type TriggerResponse struct {
	ID        string    `json:"id"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	Links     RunLinks  `json:"links"`
}

type RunLinks struct {
	Self string `json:"self"`
}

type Capabilities struct {
	Enabled        bool     `json:"enabled"`
	Detectors      []string `json:"detectors"`
	ReportEnabled  bool     `json:"reportEnabled"`
	MaxConcurrency int      `json:"maxConcurrency"`
}

type Detector interface {
	Name() string
	Detect(ctx context.Context) ([]Asset, error)
}
