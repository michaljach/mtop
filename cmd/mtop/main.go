package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	interval := flag.Duration("interval", 2*time.Second, "refresh interval")
	demo := flag.Bool("demo", false, "render fixed sample data instead of live metrics")
	once := flag.Bool("once", false, "print a single frame and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("mtop", version)
		return
	}

	if *once {
		fmt.Println(render(snapshotOnce(*interval, *demo)))
		return
	}

	p := tea.NewProgram(newModel(*interval, *demo), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "mtop:", err)
		os.Exit(1)
	}
}

// snapshotOnce collects one frame; rates need two samples, so it waits one interval.
func snapshotOnce(interval time.Duration, demo bool) view {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		w = 80
	}
	if demo {
		v := demoView()
		v.width, v.interval = w, interval
		return v
	}
	c := newSystemCollector()
	time.Sleep(interval)
	sys := c.collect()
	return view{width: w, interval: interval, status: "live", sys: &sys, gpu: collectGPU(), docker: collectDocker()}
}
