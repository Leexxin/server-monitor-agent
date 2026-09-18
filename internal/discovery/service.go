package discovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrBusy            = errors.New("a discovery run is already active")
	ErrRunNotFound     = errors.New("discovery run not found")
	ErrUnknownDetector = errors.New("unknown discovery detector")
)

type Options struct {
	Enabled   bool
	Timeout   time.Duration
	Retention int
	Reporter  *Reporter
}

type Service struct {
	mu        sync.Mutex
	options   Options
	detectors map[string]Detector
	order     []string
	runs      map[string]Run
	runOrder  []string
	active    bool
	hostname  string
}

func New(detectors []Detector, options Options) *Service {
	if options.Retention < 1 {
		options.Retention = 20
	}
	if options.Timeout <= 0 {
		options.Timeout = 30 * time.Second
	}
	indexed := make(map[string]Detector, len(detectors))
	var order []string
	for _, detector := range detectors {
		if detector == nil || detector.Name() == "" {
			continue
		}
		indexed[detector.Name()] = detector
		order = append(order, detector.Name())
	}
	sort.Strings(order)
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	return &Service{
		options: options, detectors: indexed, order: order, runs: make(map[string]Run), hostname: hostname,
	}
}

func (s *Service) Capabilities() Capabilities {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Capabilities{
		Enabled: s.options.Enabled, Detectors: append([]string(nil), s.order...),
		ReportEnabled: s.options.Reporter != nil, MaxConcurrency: 1,
	}
}

func (s *Service) Trigger(request TriggerRequest) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.options.Enabled {
		return Run{}, errors.New("discovery is disabled")
	}
	if s.active {
		return Run{}, ErrBusy
	}
	detectors := append([]string(nil), request.Detectors...)
	if len(detectors) == 0 {
		detectors = append(detectors, s.order...)
	}
	seen := make(map[string]bool, len(detectors))
	for _, name := range detectors {
		if _, ok := s.detectors[name]; !ok {
			return Run{}, fmt.Errorf("%w: %s", ErrUnknownDetector, name)
		}
		if seen[name] {
			return Run{}, fmt.Errorf("duplicate discovery detector: %s", name)
		}
		seen[name] = true
	}
	sort.Strings(detectors)
	reportRequested := s.options.Reporter != nil
	if request.Report != nil {
		reportRequested = *request.Report
	}
	now := time.Now().UTC()
	run := Run{
		ID: newRunID(), Status: StatusQueued, Detectors: detectors, CreatedAt: now,
		Report: ReportState{Requested: reportRequested, Status: reportInitialStatus(reportRequested, s.options.Reporter != nil)},
	}
	s.runs[run.ID] = run
	s.runOrder = append(s.runOrder, run.ID)
	s.trimLocked()
	s.active = true
	go s.execute(run.ID)
	return run, nil
}

func (s *Service) Get(id string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return Run{}, ErrRunNotFound
	}
	return cloneRun(run), nil
}

func (s *Service) List() []RunSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]RunSummary, 0, len(s.runOrder))
	for i := len(s.runOrder) - 1; i >= 0; i-- {
		run := s.runs[s.runOrder[i]]
		assetCount := 0
		if run.Result != nil {
			assetCount = len(run.Result.Assets)
		}
		result = append(result, RunSummary{
			ID: run.ID, Status: run.Status, CreatedAt: run.CreatedAt, CompletedAt: run.CompletedAt, AssetCount: assetCount,
		})
	}
	return result
}

func (s *Service) execute(id string) {
	started := time.Now().UTC()
	s.mu.Lock()
	run := s.runs[id]
	run.Status = StatusRunning
	run.StartedAt = &started
	s.runs[id] = run
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), s.options.Timeout)
	defer cancel()
	var assets []Asset
	var detectorErrors []DetectorError
	for _, name := range run.Detectors {
		if err := ctx.Err(); err != nil {
			detectorErrors = append(detectorErrors, DetectorError{Detector: name, Code: "timeout", Message: "discovery timed out"})
			continue
		}
		detected, err := detectSafely(ctx, s.detectors[name])
		assets = append(assets, detected...)
		if err != nil {
			code := "discovery_failed"
			if ctx.Err() != nil {
				code = "timeout"
			}
			detectorErrors = append(detectorErrors, DetectorError{Detector: name, Code: code, Message: safeError(err)})
		}
	}
	completed := time.Now().UTC()
	assets = mergeAssets(s.hostname, assets, completed)
	result := &Result{
		SchemaVersion: SchemaVersion, Host: Host{Hostname: s.hostname}, Assets: assets,
		Summary: summarize(assets), Errors: detectorErrors,
	}
	status := StatusSucceeded
	if len(detectorErrors) > 0 {
		status = StatusPartial
		if len(assets) == 0 && len(detectorErrors) == len(run.Detectors) {
			status = StatusFailed
		}
	}
	run.Result = result
	run.Status = status
	run.CompletedAt = &completed
	if run.Report.Requested && s.options.Reporter == nil && run.Status == StatusSucceeded {
		run.Status = StatusPartial
	}

	if run.Report.Requested && s.options.Reporter != nil {
		reportCtx, reportCancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := s.options.Reporter.Send(reportCtx, run)
		reportCancel()
		attempted := time.Now().UTC()
		run.Report.Attempts = 1
		run.Report.LastAttempt = &attempted
		if err != nil {
			run.Report.Status = "failed"
			run.Report.Error = safeError(err)
			if run.Status == StatusSucceeded {
				run.Status = StatusPartial
			}
		} else {
			run.Report.Status = "delivered"
		}
	}

	s.mu.Lock()
	s.runs[id] = run
	s.active = false
	s.mu.Unlock()
}

func detectSafely(ctx context.Context, detector Detector) (assets []Asset, err error) {
	defer func() {
		if recover() != nil {
			assets = nil
			err = errors.New("detector panic")
		}
	}()
	return detector.Detect(ctx)
}

func (s *Service) trimLocked() {
	for len(s.runOrder) > s.options.Retention {
		oldest := s.runOrder[0]
		delete(s.runs, oldest)
		s.runOrder = s.runOrder[1:]
	}
}

func reportInitialStatus(requested, configured bool) string {
	if !requested {
		return "not_requested"
	}
	if !configured {
		return "not_configured"
	}
	return "pending"
}

func newRunID() string {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err != nil {
		hash := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		data = hash[:12]
	}
	return hex.EncodeToString(data)
}

func mergeAssets(hostname string, input []Asset, detectedAt time.Time) []Asset {
	merged := make(map[string]*Asset)
	for _, candidate := range input {
		if !validCategory(candidate.Category) || !productPattern.MatchString(candidate.Product) {
			continue
		}
		key := candidate.Category + "/" + candidate.Product
		asset := merged[key]
		if asset == nil {
			copy := candidate
			copy.ID = stableAssetID(hostname, key)
			copy.DetectedAt = detectedAt
			if copy.Ports == nil {
				copy.Ports = []Port{}
			}
			if copy.Evidence == nil {
				copy.Evidence = []Evidence{}
			}
			merged[key] = &copy
			continue
		}
		if confidenceRank(candidate.Confidence) > confidenceRank(asset.Confidence) {
			asset.Confidence = candidate.Confidence
		}
		if candidate.Status == "running" {
			asset.Status = "running"
		}
		asset.Ports = appendUniquePorts(asset.Ports, candidate.Ports)
		asset.Evidence = appendUniqueEvidence(asset.Evidence, candidate.Evidence)
	}
	result := make([]Asset, 0, len(merged))
	for _, asset := range merged {
		sort.Slice(asset.Ports, func(i, j int) bool { return asset.Ports[i].Port < asset.Ports[j].Port })
		sort.Slice(asset.Evidence, func(i, j int) bool {
			if asset.Evidence[i].Source == asset.Evidence[j].Source {
				if asset.Evidence[i].Value == asset.Evidence[j].Value {
					return asset.Evidence[i].PID < asset.Evidence[j].PID
				}
				return asset.Evidence[i].Value < asset.Evidence[j].Value
			}
			return asset.Evidence[i].Source < asset.Evidence[j].Source
		})
		result = append(result, *asset)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Category == result[j].Category {
			return result[i].Product < result[j].Product
		}
		return result[i].Category < result[j].Category
	})
	return result
}

func stableAssetID(hostname, key string) string {
	hash := sha256.Sum256([]byte(hostname + "\x00" + key))
	return hex.EncodeToString(hash[:12])
}

func appendUniquePorts(existing, additions []Port) []Port {
	seen := make(map[string]bool, len(existing)+len(additions))
	for _, item := range existing {
		seen[item.Protocol+"/"+strconvItoa(item.Port)] = true
	}
	for _, item := range additions {
		key := item.Protocol + "/" + strconvItoa(item.Port)
		if !seen[key] {
			existing = append(existing, item)
			seen[key] = true
		}
	}
	return existing
}

func appendUniqueEvidence(existing, additions []Evidence) []Evidence {
	seen := make(map[string]bool, len(existing)+len(additions))
	for _, item := range existing {
		seen[fmt.Sprintf("%s\x00%s\x00%d", item.Source, item.Value, item.PID)] = true
	}
	for _, item := range additions {
		key := fmt.Sprintf("%s\x00%s\x00%d", item.Source, item.Value, item.PID)
		if !seen[key] {
			existing = append(existing, item)
			seen[key] = true
		}
	}
	return existing
}

func confidenceRank(value string) int {
	switch value {
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}

func summarize(assets []Asset) map[string]int {
	result := map[string]int{"total": len(assets), "middleware": 0, "database": 0, "file_transfer": 0}
	for _, asset := range assets {
		result[asset.Category]++
	}
	return result
}

func safeError(err error) string {
	message := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(message) > 256 {
		return message[:256]
	}
	return message
}

func cloneRun(run Run) Run {
	data, _ := json.Marshal(run)
	var cloned Run
	_ = json.Unmarshal(data, &cloned)
	return cloned
}

func strconvItoa(value int) string { return fmt.Sprintf("%d", value) }
