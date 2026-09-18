package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Reporter struct {
	endpoint string
	token    string
	client   *http.Client
}

type ReportPayload struct {
	SchemaVersion string     `json:"schemaVersion"`
	RunID         string     `json:"runId"`
	Status        Status     `json:"status"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	Result        *Result    `json:"result"`
}

func NewReporter(endpoint, token string) (*Reporter, error) {
	if endpoint == "" {
		return nil, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("discovery report URL must be an absolute http or https URL")
	}
	return &Reporter{
		endpoint: endpoint, token: token,
		client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}, nil
}

func (r *Reporter) Send(ctx context.Context, run Run) error {
	payload := ReportPayload{
		SchemaVersion: SchemaVersion, RunID: run.ID, Status: run.Status,
		StartedAt: run.StartedAt, CompletedAt: run.CompletedAt, Result: run.Result,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode discovery report: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create discovery report request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "sma-discovery/1")
	if r.token != "" {
		request.Header.Set("Authorization", "Bearer "+r.token)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return fmt.Errorf("send discovery report: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("report receiver returned %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	return nil
}
