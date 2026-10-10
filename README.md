# mtop

A compact terminal dashboard for a home server: system load, throughput, NVIDIA GPU and Docker at a glance.

```
hub · Fri 22:23:39 · up 6d 7h · every 2s · ● live

SYSTEM ──────────────────────────────────────────────
cpu    █▉                         9%   58°C
ram    █████████████▏            59%   9.3 / 15.5 GB
...
```

## Sections

- **System**: CPU usage and temperature, RAM, swap, and usage for each mounted disk
- **Throughput**: LAN and VPN (tun/wg/tailscale/utun…) rates, plus read/write rates per disk
- **GPU**: utilisation, VRAM, power, fan and per-process VRAM via `nvidia-smi`. On Apple Silicon it shows utilisation and GPU memory via `ioreg`, and CPU/GPU temperatures from the SMC (no root needed). Hidden when neither is available.
- **Docker**: running counts per compose project, health checks, and memory/CPU per container. Uses the `docker` CLI and is hidden when it isn't installed.

Bars are green below 75%, yellow from 75% and red from 90%. Per-process and per-container bars are blue.

## Usage

```sh
go install github.com/michaljach/mtop/cmd/mtop@latest

mtop                 # live dashboard (q / ctrl-c to exit)
mtop -interval 5s    # slower refresh
mtop -once           # print one frame and exit
mtop -demo           # sample data, handy for checking the layout
```
