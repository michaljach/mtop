package main

import "time"

// demoView reproduces the reference screenshot, for checking the layout on
// machines without an NVIDIA GPU or docker.
func demoView() view {
	const mib = 1 << 20
	const gib = 1 << 30
	return view{
		status: "demo",
		sys: &SystemSnapshot{
			Host:      "hub",
			At:        time.Now(),
			Uptime:    6*24*time.Hour + 7*time.Hour,
			CPU:       9,
			CPUTemp:   58,
			RAMUsed:   9552 * mib,
			RAMTotal:  15923 * mib,
			SwapUsed:  2286 * mib,
			SwapTotal: 16307 * mib,
			Disks: []DiskStat{
				{Name: "disk0", Used: 341 * gib, Total: 441 * gib, Percent: 82},
				{Name: "disk1", Used: 175 * gib, Total: 220 * gib, Percent: 84},
			},
			Nets: []NetStat{
				{Name: "lan", RxBps: 5 * 1024, TxBps: 22 * 1024},
				{Name: "vpn", RxBps: 1 * 1024, TxBps: 16 * 1024},
			},
		},
		gpu: &GPUStat{
			Name: "RTX 3060 12G", Util: 99, Temp: 67,
			MemUsed: 11703, MemTotal: 12288,
			Power: 169, PowerLimit: 170, Fan: 48,
			Procs: []GPUProc{{Name: "llama-swap", Mem: 11694}},
		},
		docker: &DockerSnapshot{
			Projects: []DockerProject{{Name: "sensity", Running: 0}, {Name: "infra/ai", Running: 7}},
			Running:  7,
			Containers: []Container{
				{Name: "llama-swap", MemPerc: 74, Mem: 11795 * mib, CPU: 104},
				{Name: "adguardhome", MemPerc: 2, Mem: 425 * mib, CPU: 0},
				{Name: "flaskblog", MemPerc: 1, Mem: 213 * mib, CPU: 0},
				{Name: "uptime-kuma", MemPerc: 1, Mem: 170 * mib, CPU: 1},
				{Name: "dozzle", MemPerc: 0, Mem: 18 * mib, CPU: 0},
				{Name: "buildx_buildkit_bench_builder0", MemPerc: 0, Mem: 10 * mib, CPU: 0},
			},
		},
	}
}
