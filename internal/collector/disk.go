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

const diskSectorBytes = uint64(512)

type DiskCollector struct {
	ProcPath       string
	ExcludeDevices *regexp.Regexp
}

func (DiskCollector) Name() string { return "disk" }

func (c DiskCollector) Collect(_ context.Context) (model.Partial, error) {
	f, err := os.Open(filepath.Join(c.ProcPath, "diskstats"))
	if err != nil {
		return model.Partial{}, fmt.Errorf("open diskstats: %w", err)
	}
	defer f.Close()
	disks, err := parseDiskstats(f, c.ExcludeDevices)
	if err != nil {
		return model.Partial{}, err
	}
	return model.Partial{Disks: disks}, nil
}

func parseDiskstats(r io.Reader, exclude *regexp.Regexp) ([]model.Disk, error) {
	var disks []model.Disk
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}
		device := fields[2]
		if match(exclude, device) {
			continue
		}
		value := func(index int) (uint64, error) {
			v, err := strconv.ParseUint(fields[index], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse diskstats device %s field %d: %w", device, index, err)
			}
			return v, nil
		}
		reads, err := value(3)
		if err != nil {
			return nil, err
		}
		readSectors, err := value(5)
		if err != nil {
			return nil, err
		}
		readMS, err := value(6)
		if err != nil {
			return nil, err
		}
		writes, err := value(7)
		if err != nil {
			return nil, err
		}
		writeSectors, err := value(9)
		if err != nil {
			return nil, err
		}
		writeMS, err := value(10)
		if err != nil {
			return nil, err
		}
		inProgress, err := value(11)
		if err != nil {
			return nil, err
		}
		ioMS, err := value(12)
		if err != nil {
			return nil, err
		}
		weightedMS, err := value(13)
		if err != nil {
			return nil, err
		}
		disks = append(disks, model.Disk{
			Device: device, ReadsCompleted: reads, ReadBytes: saturatingMul(readSectors, diskSectorBytes),
			ReadSeconds: float64(readMS) / 1000, WritesCompleted: writes,
			WrittenBytes: saturatingMul(writeSectors, diskSectorBytes), WriteSeconds: float64(writeMS) / 1000,
			IOInProgress: inProgress, IOSeconds: float64(ioMS) / 1000,
			WeightedIOSeconds: float64(weightedMS) / 1000,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan diskstats: %w", err)
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].Device < disks[j].Device })
	return disks, nil
}

func saturatingMul(a, b uint64) uint64 {
	const maxUint64 = ^uint64(0)
	if a != 0 && b > maxUint64/a {
		return maxUint64
	}
	return a * b
}
