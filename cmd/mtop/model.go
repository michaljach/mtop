package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type (
	sysMsg    SystemSnapshot
	gpuMsg    struct{ gpu *GPUStat }
	dockerMsg struct{ docker *DockerSnapshot }
)

type model struct {
	interval time.Duration
	demo     bool
	width    int

	collector *systemCollector
	sys       *SystemSnapshot
	gpu       *GPUStat
	docker    *DockerSnapshot
}

func newModel(interval time.Duration, demo bool) model {
	m := model{interval: interval, demo: demo, width: 80}
	if demo {
		v := demoView()
		m.sys, m.gpu, m.docker = v.sys, v.gpu, v.docker
	} else {
		m.collector = newSystemCollector()
	}
	return m
}

// Each source polls on its own loop so a slow `docker stats` never stalls the rest.
func (m model) Init() tea.Cmd {
	if m.demo {
		return nil
	}
	return tea.Batch(m.pollSystem(0), m.pollGPU(0), m.pollDocker(0))
}

func after(d time.Duration, f func() tea.Msg) tea.Cmd {
	if d == 0 {
		return f
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return f() })
}

func (m model) pollSystem(d time.Duration) tea.Cmd {
	c := m.collector
	return after(d, func() tea.Msg { return sysMsg(c.collect()) })
}

func (m model) pollGPU(d time.Duration) tea.Cmd {
	return after(d, func() tea.Msg { return gpuMsg{collectGPU()} })
}

func (m model) pollDocker(d time.Duration) tea.Cmd {
	return after(d, func() tea.Msg { return dockerMsg{collectDocker()} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case sysMsg:
		s := SystemSnapshot(msg)
		m.sys = &s
		return m, m.pollSystem(m.interval)
	case gpuMsg:
		m.gpu = msg.gpu
		if msg.gpu == nil {
			return m, nil // no NVIDIA GPU; stop polling
		}
		return m, m.pollGPU(m.interval)
	case dockerMsg:
		m.docker = msg.docker
		if msg.docker == nil {
			return m, nil // docker not installed; stop polling
		}
		return m, m.pollDocker(m.interval)
	}
	return m, nil
}

func (m model) View() string {
	status := "live"
	if m.demo {
		status = "demo"
	}
	return render(view{width: m.width, interval: m.interval, status: status, sys: m.sys, gpu: m.gpu, docker: m.docker})
}
