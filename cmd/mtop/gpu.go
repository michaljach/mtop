package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/mem"
)

// GPUStat describes one NVIDIA GPU. Memory is in MiB; Power and Fan are -1 when unsupported.
type GPUStat struct {
	Name              string
	Util, Temp        float64
	MemUsed, MemTotal float64
	Power, PowerLimit float64
	Fan               float64
	Unified           bool // memory is shared with the CPU (Apple Silicon)
	Procs             []GPUProc
}

type GPUProc struct {
	Name string
	Mem  float64 // MiB
}

// collectGPU returns nil when no supported GPU is present.
func collectGPU() *GPUStat {
	if g := collectNvidia(); g != nil {
		return g
	}
	if runtime.GOOS == "darwin" {
		return collectAppleGPU()
	}
	return nil
}

func collectNvidia() *GPUStat {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil
	}
	out, err := runCmd("nvidia-smi",
		"--query-gpu=name,utilization.gpu,temperature.gpu,memory.used,memory.total,power.draw,power.limit,fan.speed",
		"--format=csv,noheader,nounits", "--id=0")
	if err != nil {
		return nil
	}
	f := splitCSV(out)
	if len(f) < 8 {
		return nil
	}
	g := &GPUStat{
		Util:       num(f[1]),
		Temp:       num(f[2]),
		MemUsed:    num(f[3]),
		MemTotal:   num(f[4]),
		Power:      num(f[5]),
		PowerLimit: num(f[6]),
		Fan:        num(f[7]),
	}
	g.Name = shortGPUName(f[0], g.MemTotal)

	if out, err := runCmd("nvidia-smi", "--query-compute-apps=process_name,used_memory",
		"--format=csv,noheader,nounits", "--id=0"); err == nil {
		byName := map[string]float64{}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			p := splitCSV(line)
			if len(p) < 2 || p[0] == "" {
				continue
			}
			byName[filepath.Base(p[0])] += num(p[1])
		}
		for n, m := range byName {
			g.Procs = append(g.Procs, GPUProc{Name: n, Mem: m})
		}
		sort.Slice(g.Procs, func(i, j int) bool { return g.Procs[i].Mem > g.Procs[j].Mem })
	}
	return g
}

var (
	reIoregModel = regexp.MustCompile(`"model" = "([^"]+)"`)
	reIoregCores = regexp.MustCompile(`"gpu-core-count" = (\d+)`)
	reIoregUtil  = regexp.MustCompile(`"Device Utilization %"=(\d+)`)
	reIoregMem   = regexp.MustCompile(`"In use system memory"=(\d+)`)
)

// collectAppleGPU reads Apple Silicon GPU stats from the IOAccelerator
// registry entry, which needs no root. Temperature, power and fan are not exposed there.
func collectAppleGPU() *GPUStat {
	out, err := runCmd("ioreg", "-r", "-d", "1", "-w", "0", "-c", "IOAccelerator")
	if err != nil {
		return nil
	}
	util := reIoregUtil.FindStringSubmatch(out)
	if util == nil {
		return nil
	}
	g := &GPUStat{Util: num(util[1]), Temp: -1, Power: -1, Fan: -1, Unified: true}
	if m := reIoregModel.FindStringSubmatch(out); m != nil {
		g.Name = strings.TrimPrefix(m[1], "Apple ")
	}
	if m := reIoregCores.FindStringSubmatch(out); m != nil {
		g.Name += " " + m[1] + "-core"
	}
	if m := reIoregMem.FindStringSubmatch(out); m != nil {
		g.MemUsed = num(m[1]) / (1 << 20)
	}
	if v, err := mem.VirtualMemory(); err == nil {
		g.MemTotal = float64(v.Total >> 20)
	}
	return g
}

// shortGPUName turns "NVIDIA GeForce RTX 3060" + 12288 MiB into "RTX 3060 12G".
func shortGPUName(name string, memMiB float64) string {
	for _, p := range []string{"NVIDIA ", "GeForce "} {
		name = strings.TrimPrefix(name, p)
	}
	if memMiB > 0 {
		name += fmt.Sprintf(" %.0fG", memMiB/1024)
	}
	return name
}

func splitCSV(line string) []string {
	f := strings.Split(strings.TrimSpace(line), ",")
	for i := range f {
		f[i] = strings.TrimSpace(f[i])
	}
	return f
}

// num parses an nvidia-smi field, returning -1 for "[N/A]" and similar.
func num(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return v
}

func runCmd(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}
