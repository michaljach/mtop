package main

import (
	"encoding/json"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

type DockerSnapshot struct {
	Err        string // non-empty when the daemon could not be queried
	Projects   []DockerProject
	Running    int
	Unhealthy  []string
	Starting   []string
	Containers []Container
}

type DockerProject struct {
	Name    string
	Running int
}

type Container struct {
	Name    string
	MemPerc float64
	Mem     uint64 // bytes
	CPU     float64
}

// collectDocker returns nil when the docker CLI is not installed.
func collectDocker() *DockerSnapshot {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil
	}
	d := &DockerSnapshot{}

	out, err := runCmd("docker", "ps", "-a", "--format", "{{json .}}")
	if err != nil {
		d.Err = "daemon not reachable"
		return d
	}
	projects := map[string]*DockerProject{}
	var order []string
	for _, line := range jsonLines(out) {
		var c struct{ Names, Labels, State, Status string }
		if json.Unmarshal([]byte(line), &c) != nil {
			continue
		}
		running := c.State == "running"
		if running {
			d.Running++
			switch {
			case strings.Contains(c.Status, "(unhealthy)"):
				d.Unhealthy = append(d.Unhealthy, c.Names)
			case strings.Contains(c.Status, "(health: starting)"):
				d.Starting = append(d.Starting, c.Names)
			}
		}
		if p := label(c.Labels, "com.docker.compose.project"); p != "" {
			if projects[p] == nil {
				projects[p] = &DockerProject{Name: p}
				order = append(order, p)
			}
			if running {
				projects[p].Running++
			}
		}
	}
	sort.Strings(order)
	for _, p := range order {
		d.Projects = append(d.Projects, *projects[p])
	}

	if d.Running == 0 {
		return d
	}
	out, err = runCmd("docker", "stats", "--no-stream", "--format", "{{json .}}")
	if err != nil {
		return d
	}
	for _, line := range jsonLines(out) {
		var s struct{ Name, CPUPerc, MemPerc, MemUsage string }
		if json.Unmarshal([]byte(line), &s) != nil {
			continue
		}
		used, _, _ := strings.Cut(s.MemUsage, "/")
		d.Containers = append(d.Containers, Container{
			Name:    s.Name,
			CPU:     percent(s.CPUPerc),
			MemPerc: percent(s.MemPerc),
			Mem:     parseSize(used),
		})
	}
	sort.SliceStable(d.Containers, func(i, j int) bool { return d.Containers[i].Mem > d.Containers[j].Mem })
	return d
}

func jsonLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// label extracts one key from docker's "k=v,k=v" label string.
func label(labels, key string) string {
	for _, kv := range strings.Split(labels, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

func percent(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"), 64)
	return v
}

var sizeUnits = []struct {
	suffix string
	mult   float64
}{
	{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"TiB", 1 << 40},
	{"kB", 1e3}, {"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12},
	{"B", 1},
}

// parseSize parses docker sizes like "11.52GiB" or "425MiB" into bytes.
func parseSize(s string) uint64 {
	s = strings.TrimSpace(s)
	for _, u := range sizeUnits {
		if strings.HasSuffix(s, u.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 64)
			if err != nil {
				return 0
			}
			return uint64(v * u.mult)
		}
	}
	return 0
}
