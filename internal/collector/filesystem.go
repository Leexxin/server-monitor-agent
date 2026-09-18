package collector

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"sma/internal/model"
)

type FilesystemCollector struct {
	ProcPath           string
	ExcludeFSTypes     map[string]bool
	ExcludeMountpoints *regexp.Regexp
}

type mountInfo struct {
	device, mountpoint, fsType string
	readOnly                   bool
}

func (FilesystemCollector) Name() string { return "filesystem" }

func (c FilesystemCollector) Collect(ctx context.Context) (model.Partial, error) {
	f, err := os.Open(filepath.Join(c.ProcPath, "self", "mountinfo"))
	if err != nil {
		return model.Partial{}, fmt.Errorf("open mountinfo: %w", err)
	}
	mounts, err := parseMountinfo(f)
	f.Close()
	if err != nil {
		return model.Partial{}, err
	}
	filesystems := make([]model.Filesystem, 0, len(mounts))
	var failed int
	for _, mount := range mounts {
		if err := ctx.Err(); err != nil {
			return model.Partial{}, err
		}
		if c.ExcludeFSTypes[mount.fsType] || match(c.ExcludeMountpoints, mount.mountpoint) {
			continue
		}
		stats, err := filesystemStats(mount.mountpoint)
		if err != nil {
			failed++
			continue
		}
		filesystems = append(filesystems, model.Filesystem{
			Device: mount.device, Mountpoint: mount.mountpoint, FilesystemType: mount.fsType,
			SizeBytes: stats.size, AvailableBytes: stats.available, FreeBytes: stats.free,
			Files: stats.files, FilesFree: stats.filesFree, ReadOnly: mount.readOnly,
		})
	}
	if len(filesystems) == 0 && failed > 0 {
		return model.Partial{}, fmt.Errorf("statfs failed for all %d eligible mountpoints", failed)
	}
	sort.Slice(filesystems, func(i, j int) bool { return filesystems[i].Mountpoint < filesystems[j].Mountpoint })
	if failed > 0 {
		return model.Partial{Filesystems: filesystems}, fmt.Errorf("statfs failed for %d mountpoints", failed)
	}
	return model.Partial{Filesystems: filesystems}, nil
}

func parseMountinfo(r io.Reader) ([]mountInfo, error) {
	var mounts []mountInfo
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), " - ", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("parse mountinfo: missing separator")
		}
		pre, post := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(pre) < 6 || len(post) < 2 {
			return nil, fmt.Errorf("parse mountinfo: incomplete row")
		}
		mountpoint, err := unescapeMountField(pre[4])
		if err != nil {
			return nil, err
		}
		device, err := unescapeMountField(post[1])
		if err != nil {
			return nil, err
		}
		readOnly := false
		for _, option := range strings.Split(pre[5], ",") {
			if option == "ro" {
				readOnly = true
			}
		}
		mounts = append(mounts, mountInfo{device: device, mountpoint: mountpoint, fsType: post[0], readOnly: readOnly})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan mountinfo: %w", err)
	}
	return mounts, nil
}

func unescapeMountField(value string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			i++
			continue
		}
		if i+3 >= len(value) {
			return "", fmt.Errorf("parse mountinfo escape in %q", value)
		}
		v, err := strconv.ParseUint(value[i+1:i+4], 8, 8)
		if err != nil {
			return "", fmt.Errorf("parse mountinfo escape in %q: %w", value, err)
		}
		out.WriteByte(byte(v))
		i += 4
	}
	return out.String(), nil
}

type fsStats struct{ size, available, free, files, filesFree uint64 }
