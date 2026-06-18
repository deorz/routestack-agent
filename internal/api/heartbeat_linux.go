//go:build linux

package api

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// CollectSystemInfo returns OS, architecture, and kernel version strings.
func CollectSystemInfo() (osName, arch, kernel string) {
	osName = readOSRelease()
	if osName == "" {
		osName = "Linux"
	}

	var uts syscall.Utsname
	if err := syscall.Uname(&uts); err == nil {
		arch = charsToString(uts.Machine[:])
		kernel = charsToString(uts.Release[:])
	}
	if arch == "" {
		arch = "unknown"
	}
	if kernel == "" {
		kernel = "unknown"
	}

	return osName, arch, kernel
}

// readOSRelease reads PRETTY_NAME from /etc/os-release.
func readOSRelease() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			val := strings.TrimPrefix(line, "PRETTY_NAME=")
			val = strings.Trim(val, `"`)
			return val
		}
	}
	return ""
}

// CollectSystemMetrics gathers CPU, memory, disk, and load metrics.
func CollectSystemMetrics() SystemMetrics {
	var m SystemMetrics

	m.CPUPercent = cpuPercent()
	m.MemoryUsedMB = memoryUsedMB()
	m.DiskFreeGB = diskFreeGB("/")
	m.LoadAvg1m = loadAvg1m()

	return m
}

// cpuPercent reads /proc/stat twice with a 1-second interval and computes
// the CPU utilization percentage.
func cpuPercent() float64 {
	stat1, err := readCPUStat()
	if err != nil {
		return 0
	}
	time.Sleep(1 * time.Second)
	stat2, err := readCPUStat()
	if err != nil {
		return 0
	}

	idle := stat2.idle - stat1.idle
	total := stat2.total - stat1.total
	if total == 0 {
		return 0
	}

	usage := (1.0 - float64(idle)/float64(total)) * 100
	return math.Round(usage*10) / 10
}

type cpuStat struct {
	total, idle uint64
}

func readCPUStat() (cpuStat, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuStat{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return cpuStat{}, fmt.Errorf("empty /proc/stat")
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuStat{}, fmt.Errorf("unexpected /proc/stat format")
	}

	var s cpuStat
	for i, f := range fields[1:] {
		val, _ := strconv.ParseUint(f, 10, 64)
		s.total += val
		if i == 3 { // idle is the 4th field (index 3 after "cpu")
			s.idle = val
		}
	}
	return s, nil
}

// memoryUsedMB returns used memory in MB from /proc/meminfo.
func memoryUsedMB() int64 {
	total := procMeminfoField("MemTotal")
	avail := procMeminfoField("MemAvailable")
	if total == 0 {
		return 0
	}
	used := total - avail
	return used / 1024 // kB → MB
}

func procMeminfoField(key string) int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, key+":") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseInt(fields[1], 10, 64)
				return val
			}
		}
	}
	return 0
}

// diskFreeGB returns free disk space in GB for the given path.
func diskFreeGB(path string) float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0
	}
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	return float64(freeBytes) / (1024 * 1024 * 1024)
}

// loadAvg1m reads the 1-minute load average from /proc/loadavg.
func loadAvg1m() float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}
	val, _ := strconv.ParseFloat(fields[0], 64)
	return val
}

// charsToString converts a syscall-int8 array to a Go string,
// stopping at the first null byte.
func charsToString(ca []int8) string {
	var b strings.Builder
	for _, c := range ca {
		if c == 0 {
			break
		}
		b.WriteByte(byte(c))
	}
	return b.String()
}
