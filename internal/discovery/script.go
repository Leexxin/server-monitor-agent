package discovery

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

const maxScriptOutput = 1 << 20

var scriptNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
var productPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type ScriptDetector struct {
	name string
	path string
}

func (d ScriptDetector) Name() string { return "script:" + d.name }

func (d ScriptDetector) Detect(ctx context.Context) ([]Asset, error) {
	stdout := &limitedBuffer{limit: maxScriptOutput}
	stderr := &limitedBuffer{limit: 16 << 10}
	cmd := exec.CommandContext(ctx, d.path)
	cmd.Dir = "/"
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(stdout.err, errOutputLimit) {
			return nil, fmt.Errorf("script output exceeds %d bytes", maxScriptOutput)
		}
		message := strings.TrimSpace(stderr.String())
		if len(message) > 256 {
			message = message[:256]
		}
		if message != "" {
			return nil, fmt.Errorf("script failed: %s", message)
		}
		return nil, fmt.Errorf("script failed: %w", err)
	}
	return parseScriptOutput(bytes.NewReader(stdout.Bytes()), d.Name())
}

func LoadScriptDetectors(directory string) ([]Detector, error) {
	if directory == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read discovery script directory: %w", err)
	}
	var detectors []Detector
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if !scriptNamePattern.MatchString(name) {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 {
			continue
		}
		if runtime.GOOS == "linux" {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Uid != 0 {
				continue
			}
		}
		detectors = append(detectors, ScriptDetector{name: name, path: path})
	}
	sort.Slice(detectors, func(i, j int) bool { return detectors[i].Name() < detectors[j].Name() })
	return detectors, nil
}

func parseScriptOutput(reader *bytes.Reader, detector string) ([]Asset, error) {
	var assets []Asset
	scanner := bufio.NewScanner(reader)
	now := time.Now().UTC()
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 5 {
			return nil, fmt.Errorf("line %d: expected 5 tab-separated fields", lineNumber)
		}
		category, product := fields[0], fields[1]
		if !validCategory(category) || !productPattern.MatchString(product) {
			return nil, fmt.Errorf("line %d: invalid category or product", lineNumber)
		}
		confidence := fields[3]
		if confidence != "high" && confidence != "medium" && confidence != "low" {
			return nil, fmt.Errorf("line %d: invalid confidence", lineNumber)
		}
		if len(fields[2]) == 0 || len(fields[2]) > 128 || len(fields[4]) == 0 || len(fields[4]) > 256 {
			return nil, fmt.Errorf("line %d: display name or evidence length is invalid", lineNumber)
		}
		assets = append(assets, Asset{
			Category: category, Product: product, DisplayName: fields[2], Status: "running", Confidence: confidence,
			Ports: []Port{}, Evidence: []Evidence{{Source: detector, Value: fields[4]}}, DetectedAt: now,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read script output: %w", err)
	}
	return assets, nil
}

func validCategory(value string) bool {
	return value == "middleware" || value == "database" || value == "file_transfer"
}

var errOutputLimit = errors.New("output limit exceeded")

type limitedBuffer struct {
	bytes.Buffer
	limit int
	err   error
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.err = errOutputLimit
		return 0, b.err
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.err = errOutputLimit
		return remaining, b.err
	}
	return b.Buffer.Write(p)
}
