package main

import (
	"testing"

	"github.com/shirou/gopsutil/v3/disk"
)

func TestParseSize(t *testing.T) {
	cases := map[string]uint64{
		"1.5GiB":  3 << 29,
		"425MiB":  425 << 20,
		"512KiB":  512 << 10,
		"1.5kB":   1500,
		"0B":      0,
		"garbage": 0,
	}
	for in, want := range cases {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestLabel(t *testing.T) {
	l := "com.docker.compose.service=web,com.docker.compose.project=infra,foo=bar"
	if got := label(l, "com.docker.compose.project"); got != "infra" {
		t.Errorf("got %q", got)
	}
	if got := label(l, "missing"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestIOKey(t *testing.T) {
	io := map[string]disk.IOCountersStat{"nvme0n1": {}, "sda": {}, "dm-0": {Label: "vg-root"}}
	cases := map[string]string{
		"/dev/nvme0n1p2":      "nvme0n1",
		"/dev/sda1":           "sda",
		"/dev/mapper/vg-root": "dm-0",
		"/dev/unknown":        "",
	}
	for dev, want := range cases {
		if got := ioKey(dev, io); got != want {
			t.Errorf("ioKey(%q) = %q, want %q", dev, got, want)
		}
	}
}

func TestShortGPUName(t *testing.T) {
	if got := shortGPUName("NVIDIA GeForce RTX 3060", 12288); got != "RTX 3060 12G" {
		t.Errorf("got %q", got)
	}
}

func TestBarWidth(t *testing.T) {
	for _, p := range []float64{-5, 0, 9, 50, 99.9, 100, 140} {
		if w := len([]rune(stripANSI(bar(p, green, barWidth)))); w != barWidth {
			t.Errorf("bar(%v) width %d, want %d", p, w, barWidth)
		}
	}
}

func stripANSI(s string) string {
	var out []rune
	esc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			esc = true
		case esc && r == 'm':
			esc = false
		case !esc:
			out = append(out, r)
		}
	}
	return string(out)
}

func TestSizeFormat(t *testing.T) {
	cases := map[string]string{
		sizeStr(10 * mib):              "10 MB",
		sizeStr(11795 * mib):           "11.5 GB",
		sizeStr(341 * gib):             "341 GB",
		sizeStr(2 * tib):               "2.0 TB",
		usageStr(9552*mib, 15923*mib):  "9.3 / 15.5 GB",
		usageStr(341*gib, 441*gib):     "341 / 441 GB",
		usageStr(250*mib, 1024*mib):    "0.2 / 1.0 GB",
		usageStr(11703*mib, 12288*mib): "11.4 / 12.0 GB",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
