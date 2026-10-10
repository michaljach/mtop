package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// SystemSnapshot holds host-level metrics. Sizes are in bytes, rates in bytes/s.
type SystemSnapshot struct {
	Host    string
	Uptime  time.Duration
	At      time.Time
	CPU     float64
	CPUTemp float64 // 0 when no sensor is available

	RAMUsed, RAMTotal   uint64
	SwapUsed, SwapTotal uint64

	Disks []DiskStat
	Nets  []NetStat
}

type DiskStat struct {
	Name, Mount       string
	Used, Total       uint64
	Percent           float64 // df-style: used / (used + available)
	ReadBps, WriteBps float64
	device, ioKey     string
}

type NetStat struct {
	Name         string // "lan" or "vpn"
	RxBps, TxBps float64
}

// systemCollector keeps the previous counters so it can turn them into rates.
type systemCollector struct {
	host     string
	prevAt   time.Time
	prevNet  map[string]net.IOCountersStat
	prevDisk map[string]disk.IOCountersStat
}

func newSystemCollector() *systemCollector {
	h, _ := os.Hostname()
	h, _, _ = strings.Cut(h, ".")
	c := &systemCollector{host: h}
	cpu.Percent(0, false) // prime: the next call measures since now
	c.collect()           // prime rate counters
	return c
}

func (c *systemCollector) collect() SystemSnapshot {
	now := time.Now()
	s := SystemSnapshot{Host: c.host, At: now}

	if up, err := host.Uptime(); err == nil {
		s.Uptime = time.Duration(up) * time.Second
	}
	if p, err := cpu.Percent(0, false); err == nil && len(p) > 0 {
		s.CPU = p[0]
	}
	s.CPUTemp = cpuTemp()
	if v, err := mem.VirtualMemory(); err == nil {
		s.RAMTotal = v.Total
		s.RAMUsed = v.Total - v.Available
	}
	if v, err := mem.SwapMemory(); err == nil {
		s.SwapUsed, s.SwapTotal = v.Used, v.Total
	}

	dt := now.Sub(c.prevAt).Seconds()
	s.Disks = c.disks(dt)
	s.Nets = c.nets(dt)
	c.prevAt = now
	return s
}

// cpuTemp returns the hottest CPU package/die sensor, or 0 if none is found.
func cpuTemp() float64 {
	if t, _ := smcTemps(); t > 0 {
		return t
	}
	temps, _ := host.SensorsTemperatures() // partial results come with an error
	var best float64
	for _, t := range temps {
		k := strings.ToLower(t.SensorKey)
		if strings.Contains(k, "package") || strings.Contains(k, "tctl") || strings.Contains(k, "tdie") ||
			strings.HasPrefix(k, "coretemp") || strings.HasPrefix(k, "k10temp") || strings.HasPrefix(k, "cpu") {
			if t.Temperature > best && t.Temperature < 150 {
				best = t.Temperature
			}
		}
	}
	return best
}

var skipFS = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "overlay": true, "squashfs": true, "iso9660": true,
	"devfs": true, "autofs": true, "nullfs": true,
}

func (c *systemCollector) disks(dt float64) []DiskStat {
	parts, err := disk.Partitions(false)
	if err != nil {
		return nil
	}
	hasDataVol := false
	for _, p := range parts {
		if p.Mountpoint == "/System/Volumes/Data" {
			hasDataVol = true
		}
	}

	seen := map[string]bool{}
	var out []DiskStat
	for _, p := range parts {
		if skipFS[p.Fstype] || seen[p.Device] || keepOut(p, hasDataVol) {
			continue
		}
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total < 1<<30 {
			continue
		}
		seen[p.Device] = true
		out = append(out, DiskStat{Mount: p.Mountpoint, Used: u.Used, Total: u.Total, Percent: u.UsedPercent, device: p.Device})
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].Mount) < len(out[j].Mount) })

	io, _ := disk.IOCounters()
	for i := range out {
		out[i].Name = "disk" + strconv.Itoa(i)
		key := ioKey(out[i].device, io)
		if key == "" {
			continue
		}
		cur := io[key]
		if prev, ok := c.prevDisk[key]; ok && dt > 0 {
			out[i].ReadBps = rate(prev.ReadBytes, cur.ReadBytes, dt)
			out[i].WriteBps = rate(prev.WriteBytes, cur.WriteBytes, dt)
		}
	}
	c.prevDisk = io
	return out
}

func keepOut(p disk.PartitionStat, hasDataVol bool) bool {
	m := p.Mountpoint
	if runtime.GOOS == "darwin" {
		// The sealed system volume and its siblings share one APFS container;
		// the Data volume is the one whose usage matters.
		if m == "/" && hasDataVol {
			return true
		}
		if strings.HasPrefix(m, "/System/Volumes/") && m != "/System/Volumes/Data" {
			return true
		}
		if strings.Contains(m, "CoreSimulator") {
			return true
		}
		for _, o := range p.Opts {
			if o == "nobrowse" && m != "/System/Volumes/Data" {
				return true
			}
		}
	}
	for _, pre := range []string{"/boot", "/snap", "/run", "/var/lib/docker", "/proc", "/sys", "/dev"} {
		if m == pre || strings.HasPrefix(m, pre+"/") {
			return true
		}
	}
	for _, o := range p.Opts {
		if o == "ro" && m != "/" {
			return true
		}
	}
	return false
}

var partSuffix = regexp.MustCompile(`(p?\d+|s\d+(s\d+)?)$`)

// ioKey maps a mounted device to its entry in the IO counter table.
func ioKey(device string, io map[string]disk.IOCountersStat) string {
	base := filepath.Base(device)
	if _, ok := io[base]; ok {
		return base
	}
	for k, v := range io {
		if v.Label != "" && v.Label == base {
			return k
		}
	}
	if whole := partSuffix.ReplaceAllString(base, ""); whole != base {
		if _, ok := io[whole]; ok {
			return whole
		}
	}
	if len(io) == 1 {
		for k := range io {
			return k
		}
	}
	return ""
}

var (
	virtualIf = []string{"lo", "docker", "br-", "veth", "virbr", "vnet", "cni", "flannel", "cali", "kube",
		"awdl", "llw", "anpi", "ap", "bridge", "gif", "stf", "vmnet", "dummy", "ifb", "XHC"}
	vpnIf = []string{"tun", "tap", "wg", "tailscale", "utun", "ppp", "ipsec", "zt", "nordlynx"}
)

func ifClass(name string) string {
	for _, p := range vpnIf {
		if strings.HasPrefix(name, p) {
			return "vpn"
		}
	}
	for _, p := range virtualIf {
		if strings.HasPrefix(name, p) {
			return ""
		}
	}
	return "lan"
}

func (c *systemCollector) nets(dt float64) []NetStat {
	io, err := net.IOCounters(true)
	if err != nil {
		return nil
	}
	cur := map[string]net.IOCountersStat{}
	sums := map[string]*NetStat{}
	for _, n := range io {
		cur[n.Name] = n
		class := ifClass(n.Name)
		if class == "" {
			continue
		}
		if sums[class] == nil {
			sums[class] = &NetStat{Name: class}
		}
		if prev, ok := c.prevNet[n.Name]; ok && dt > 0 {
			sums[class].RxBps += rate(prev.BytesRecv, n.BytesRecv, dt)
			sums[class].TxBps += rate(prev.BytesSent, n.BytesSent, dt)
		}
	}
	c.prevNet = cur

	var out []NetStat
	for _, class := range []string{"lan", "vpn"} {
		if s := sums[class]; s != nil {
			out = append(out, *s)
		}
	}
	return out
}

func rate(prev, cur uint64, dt float64) float64 {
	if cur < prev { // counter reset
		return 0
	}
	return float64(cur-prev) / dt
}
