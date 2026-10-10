package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const barWidth = 22

type palette struct{ fill, track lipgloss.Color }

var (
	green  = palette{"#8bd3a5", "#1c2621"}
	yellow = palette{"#f0cf65", "#2b2617"}
	red    = palette{"#d9604f", "#2b1514"}
	blue   = palette{"#7fcfe8", "#1b2530"}

	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ececec"))
	styleText  = lipgloss.NewStyle().Foreground(lipgloss.Color("#d6d6d6"))
	styleDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("#6e6e6e"))
	styleRule  = lipgloss.NewStyle().Foreground(lipgloss.Color("#4a4a4a"))
	styleGreen = lipgloss.NewStyle().Foreground(green.fill)
	styleRed   = lipgloss.NewStyle().Foreground(red.fill)
	styleWarn  = lipgloss.NewStyle().Foreground(yellow.fill)
)

// level picks the bar colour for a utilisation percentage.
func level(pct float64) palette {
	switch {
	case pct >= 90:
		return red
	case pct >= 75:
		return yellow
	default:
		return green
	}
}

var eighths = []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// bar draws a fixed-width bar with sub-cell resolution on a tinted track.
func bar(pct float64, p palette, width int) string {
	pct = math.Max(0, math.Min(100, pct))
	cells := pct / 100 * float64(width)
	full := int(cells)
	frac := int((cells - float64(full)) * 8)

	fill := strings.Repeat("█", full)
	rest := width - full
	if rest > 0 && frac > 0 {
		fill += eighths[frac]
		rest--
	}
	style := lipgloss.NewStyle().Foreground(p.fill).Background(p.track)
	return style.Render(fill + strings.Repeat(" ", rest))
}

func pad(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func rpad(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return strings.Repeat(" ", w-n) + s
	}
	return s
}

func truncate(s string, w int) string {
	if r := []rune(s); len(r) > w {
		return string(r[:w]) + ".."
	}
	return s
}

func pctStr(p float64) string { return fmt.Sprintf("%.0f%%", math.Floor(p)) }

func ratio(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return used / total * 100
}

func mb(bytes uint64) uint64 { return bytes >> 20 }
func gb(bytes uint64) uint64 { return bytes >> 30 }

func rateStr(bps float64) string {
	switch {
	case bps < 1024:
		return fmt.Sprintf("%.0f B/s", bps)
	case bps < 1<<20:
		return fmt.Sprintf("%.0f KB/s", bps/1024)
	case bps < 1<<30:
		return fmt.Sprintf("%.1f MB/s", bps/(1<<20))
	default:
		return fmt.Sprintf("%.1f GB/s", bps/(1<<30))
	}
}

func uptimeStr(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	return fmt.Sprintf("%dh %dm", hours, int(d.Minutes())%60)
}

// view is everything the renderer needs for one frame.
type view struct {
	width    int
	interval time.Duration
	status   string // shown after the dot, e.g. "live"
	sys      *SystemSnapshot
	gpu      *GPUStat
	docker   *DockerSnapshot
}

func render(v view) string {
	if v.sys == nil {
		return styleDim.Render("collecting…")
	}
	var b strings.Builder
	line := func(s string) { b.WriteString(s + "\n") }

	sep := styleDim.Render(" · ")
	line(styleTitle.Render(v.sys.Host) + sep +
		styleText.Render(v.sys.At.Format("Mon 15:04:05")) + sep +
		styleDim.Render("up "+uptimeStr(v.sys.Uptime)) + sep +
		styleDim.Render(fmt.Sprintf("every %s", v.interval)) + sep +
		styleGreen.Render("●") + " " + styleDim.Render(v.status))

	renderSystem(line, v)
	renderThroughput(line, v)
	if v.gpu != nil {
		renderGPU(line, v)
	}
	if v.docker != nil {
		renderDocker(line, v)
	}

	line("")
	b.WriteString(styleDim.Render("ctrl-c to exit"))
	return b.String()
}

func section(line func(string), width int, title, sub string) {
	line("")
	head := styleTitle.Render(title)
	n := len(title) + 1
	if sub != "" {
		head += " " + styleRule.Render("──") + " " + styleText.Render(sub)
		n += 4 + lipgloss.Width(sub)
	}
	line(head + " " + styleRule.Render(strings.Repeat("─", max(3, width-n-1))))
}

// gauge renders "label  [bar]   NN%   detail".
func gauge(label string, labelW int, pct float64, p palette, detail string) string {
	return styleText.Render(pad(label, labelW)) + bar(pct, p, barWidth) + "  " +
		styleText.Render(rpad(pctStr(pct), 5)) + "   " + styleText.Render(detail)
}

func renderSystem(line func(string), v view) {
	s := v.sys
	section(line, v.width, "SYSTEM", "")
	const lw = 7

	temp := ""
	if s.CPUTemp > 0 {
		temp = fmt.Sprintf("%.0f°C", s.CPUTemp)
	}
	line(gauge("cpu", lw, s.CPU, level(s.CPU), temp))

	ram := ratio(float64(s.RAMUsed), float64(s.RAMTotal))
	line(gauge("ram", lw, ram, level(ram), fmt.Sprintf("%d/%d MB", mb(s.RAMUsed), mb(s.RAMTotal))))

	if s.SwapTotal > 0 {
		swap := ratio(float64(s.SwapUsed), float64(s.SwapTotal))
		line(gauge("swap", lw, swap, level(swap), fmt.Sprintf("%d/%d MB", mb(s.SwapUsed), mb(s.SwapTotal))))
	}
	for _, d := range s.Disks {
		line(gauge(d.Name, lw, d.Percent, level(d.Percent), fmt.Sprintf("%dG/%dG", gb(d.Used), gb(d.Total))))
	}
}

func renderThroughput(line func(string), v view) {
	section(line, v.width, "THROUGHPUT", "")
	row := func(label, a, av, b, bv string) {
		line(styleText.Render(pad(label, 7)) + styleDim.Render(a) + styleText.Render(rpad(av, 11)) +
			"    " + styleDim.Render(b) + styleText.Render(rpad(bv, 11)))
	}
	for _, n := range v.sys.Nets {
		row(n.Name, "↓", rateStr(n.RxBps), "↑", rateStr(n.TxBps))
	}
	for _, d := range v.sys.Disks {
		row(d.Name, "r", rateStr(d.ReadBps), "w", rateStr(d.WriteBps))
	}
}

func renderGPU(line func(string), v view) {
	g := v.gpu
	section(line, v.width, "GPU", g.Name)
	const lw = 7

	temp := ""
	if g.Temp >= 0 {
		temp = fmt.Sprintf("%.0f°C", g.Temp)
	}
	line(gauge("util", lw, g.Util, level(g.Util), temp))

	vram := ratio(g.MemUsed, g.MemTotal)
	memLabel := "vram"
	if g.Unified {
		memLabel = "mem"
	}
	line(gauge(memLabel, lw, vram, level(vram), fmt.Sprintf("%.0f/%.0f MB", g.MemUsed, g.MemTotal)))

	if g.Power >= 0 && g.PowerLimit > 0 {
		pw := ratio(g.Power, g.PowerLimit)
		line(gauge("power", lw, pw, level(pw), fmt.Sprintf("%.0fW / %.0fW", g.Power, g.PowerLimit)))
	}
	if g.Fan >= 0 {
		line(gauge("fan", lw, g.Fan, level(g.Fan), ""))
	}

	plw := lw
	for _, p := range g.Procs {
		plw = max(plw, len(truncate(p.Name, 20))+2)
	}
	for _, p := range g.Procs {
		line(gauge(truncate(p.Name, 20), plw, ratio(p.Mem, g.MemTotal), blue, fmt.Sprintf("%.0f MB", p.Mem)))
	}
}

func renderDocker(line func(string), v view) {
	d := v.docker
	section(line, v.width, "DOCKER", "")
	if d.Err != "" {
		line(styleDim.Render(d.Err))
		return
	}

	var parts []string
	for _, p := range d.Projects {
		parts = append(parts, fmt.Sprintf("%s %d↑", p.Name, p.Running))
	}
	parts = append(parts, fmt.Sprintf("total %d↑", d.Running))
	line(styleText.Render(strings.Join(parts, "   ")))

	switch {
	case len(d.Unhealthy) > 0:
		line(styleRed.Render("✗ unhealthy: " + strings.Join(d.Unhealthy, ", ")))
	case len(d.Starting) > 0:
		line(styleWarn.Render("… starting: " + strings.Join(d.Starting, ", ")))
	case d.Running > 0:
		line(styleGreen.Render("✓") + styleText.Render(fmt.Sprintf(" all %d healthy", d.Running)))
	}

	lw := 7
	for _, c := range d.Containers {
		lw = max(lw, len(truncate(c.Name, 20))+2)
	}
	for _, c := range d.Containers {
		line(styleText.Render(pad(truncate(c.Name, 20), lw)) + bar(c.MemPerc, blue, barWidth) + "  " +
			styleText.Render(rpad(pctStr(c.MemPerc), 5)) + "   " +
			styleText.Render(rpad(fmt.Sprintf("%d MB", mb(c.Mem)), 8)) + "   " +
			styleDim.Render("cpu "+pctStr(c.CPU)))
	}
}
