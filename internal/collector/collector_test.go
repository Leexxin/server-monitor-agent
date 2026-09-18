package collector

import (
	"regexp"
	"strings"
	"testing"
)

func TestParseCPUStat(t *testing.T) {
	input := "cpu  100 0 20 500 5 1 2 3\ncpu0 50 0 10 250 2 1 1 0\ncpu1 50 0 10 250 3 0 1 3\nintr 1\n"
	cpu, err := parseCPUStat(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if cpu.LogicalCount != 2 || cpu.Seconds["cpu0"]["user"] != 0.5 || cpu.Seconds["cpu1"]["idle"] != 2.5 {
		t.Fatalf("unexpected CPU result: %#v", cpu)
	}
}

func TestParseMeminfoAndFallback(t *testing.T) {
	input := `MemTotal:       1000 kB
MemFree:         100 kB
Buffers:          20 kB
Cached:          200 kB
SReclaimable:     30 kB
Shmem:            10 kB
SwapTotal:       500 kB
SwapFree:        400 kB
`
	memory, err := parseMeminfo(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if memory.TotalBytes != 1000*1024 || memory.CachedBytes != 220*1024 || memory.AvailableBytes != 340*1024 {
		t.Fatalf("unexpected memory result: %#v", memory)
	}
}

func TestParseDiskstats(t *testing.T) {
	input := "8 0 sda 10 0 20 300 40 0 50 600 2 700 800\n7 0 loop0 1 0 1 1 1 0 1 1 0 1 1\n"
	disks, err := parseDiskstats(strings.NewReader(input), regexp.MustCompile(`^loop`))
	if err != nil {
		t.Fatal(err)
	}
	if len(disks) != 1 || disks[0].Device != "sda" || disks[0].ReadBytes != 20*512 || disks[0].WriteSeconds != 0.6 {
		t.Fatalf("unexpected disk result: %#v", disks)
	}
}

func TestParseMountinfoEscapes(t *testing.T) {
	input := "36 25 0:32 / /data\\040disk rw,nosuid - ext4 /dev/sda1 rw\n"
	mounts, err := parseMountinfo(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || mounts[0].mountpoint != "/data disk" || mounts[0].device != "/dev/sda1" || mounts[0].readOnly {
		t.Fatalf("unexpected mount result: %#v", mounts)
	}
}

func TestSaturatingMath(t *testing.T) {
	if got := saturatingSub(1, 2); got != 0 {
		t.Fatalf("saturatingSub = %d", got)
	}
	if got := saturatingMul(^uint64(0), 2); got != ^uint64(0) {
		t.Fatalf("saturatingMul = %d", got)
	}
}
