Go package for interacting with GoodWe inverters.

## goodwe — CLI Tool

The `goodwe` CLI provides direct access to a GoodWe inverter from the command line.

### Building

```sh
$ go build ./cmd/goodwe/
```

### Commands

Display inverter information:

```sh
$ ./goodwe -ip 192.168.100.151 -info
```

List all available sensors:

```sh
$ ./goodwe -ip 192.168.100.151 -listsensors
```

Read specific sensors (comma-separated):

```sh
$ ./goodwe -ip 192.168.100.151 -readsensor battery_soc,house_consumption,ppv
```

Poll sensors at a fixed interval:

```sh
$ ./goodwe -ip 192.168.100.151 -poll 10s -readsensor ppv,work_mode_label
```

Enable verbose or debug logging:

```sh
$ ./goodwe -ip 192.168.100.151 -info -verbose
$ ./goodwe -ip 192.168.100.151 -info -debug
```

### Example Output

From a GW15K-ET inverter:

```text
$ ./goodwe -ip 192.168.4.82 -info
Inverter Information:
  Serial:     <SN>
  Model:      GW15K-ET
  Firmware:   04062-07-S0002071-13-439
  DSP:        07
  ARM:        13
  Rated:      15000 W
  Mode:       Normal (On-Grid)
```

---

## goodwe-daemon — Daemon Tool (WIP)

The `goodwe-daemon` is a persistent background service that polls the inverter,
stores sensor readings in a SQLite database, and exposes a REST API. This tool
is a work-in-progress.

### Building

```sh
$ go build ./cmd/goodwe-daemon/
```

### Docker / Podman

A multi-stage Dockerfile is provided at the repository root. Build the image:

```sh
# Build with podman (or docker):
podman build -t goodwe-daemon .
```

Run the daemon with environment variables:

```sh
podman run -d \
  --name goodwe \
  --restart unless-stopped \
  -p 8080:8080 \
  -v goodwe-data:/var/lib/goodwe \
  -e INVERTER_IP=192.168.4.82 \
  -e POLL_INTERVAL=30s \
  -e LISTEN=:8080 \
  -e DASHBOARD=true \
  goodwe-daemon
```

Available environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `INVERTER_IP` | — | IP address of the GoodWe inverter (required) |
| `POLL_INTERVAL` | — | Sensor poll interval (e.g. `30s`, `1m`) |
| `LISTEN` | `:8080` | Address and port for the HTTP API server |
| `DB_STORE` | `sqlite:///var/lib/goodwe/goodwe.db` | Database path |
| `DASHBOARD` | `false` | Set to `true` to enable the embedded dashboard |
| `DEBUG` | `false` | Set to `true` for debug logging |

The entrypoint also supports running commands inside the container. For example, to run voltage analysis offline:

```sh
podman exec -it goodwe goodwe-daemon -offline-analyze-voltage
```

Or use the CLI tool:

```sh
podman run --rm goodwe-daemon goodwe -version
```

### Starting the Daemon

Connect to an inverter and start the API server:

```sh
$ ./goodwe-daemon -listen :8080 -inverterip 192.168.4.82
```

With debug logging and a custom database path:

```sh
$ ./goodwe-daemon -listen :8080 -inverterip 192.168.4.82 \
    -dbstore sqlite:///var/lib/goodwe/history.db \
    -poll 30s -debug
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-listen` | `":8080"` | Address and port for the HTTP API server |
| `-inverterip` | `""` | IP address of the GoodWe inverter |
| `-dbstore` | `"sqlite://~/.goodwe/goodwe.db"` | Database connection string (e.g. `sqlite:///var/lib/goodwe/history.db`) |
| `-poll` | `"0"` (disabled) | Sensor poll interval (e.g. `30s`, `1m`; minimum 5s) |
| `-dashboard` | `false` | Enable the embedded JS dashboard at `/dashboard` |
| `-purge` | `""` | One-shot: purge all data older than this date and exit (e.g. `2026-01-01`) |
| `-debug` | `false` | Enable debug logging |
| `-version` | `false` | Display version information and exit |
| `-offline-analyze-voltage` | `false` | Run voltage analysis on the DB once and exit |

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Service health status |
| `GET` | `/api/info` | Inverter information |
| `GET` | `/api/sensors` | List of available sensors |
| `GET` | `/api/data/{sensor}` | Live sensor reading |
| `GET` | `/api/data/{sensor}/aggregate` | Historical sensor data from DB: `?since=&until=&limit=&latest=&delta=` |
| `GET` | `/api/analysis/grid_voltage` | Voltage events: `?before=&limit=` |
| `GET` | `/dashboard` | Embedded dashboard (WIP) |

### REST API

All endpoints return JSON. The examples below assume the daemon is listening on
`localhost:8080`.

**`GET /api/health`** — Service health and inverter connection state:

```sh
$ curl -s http://localhost:8080/api/health
{
  "status": "ok",
  "timestamp": "2026-06-14T12:00:00Z",
  "inverter": {
    "connected": true
  }
}
```

**`GET /api/info`** — Inverter identity and last poll time:

```sh
$ curl -s http://localhost:8080/api/info
{
  "serial": "90000000000001",
  "model": "GW15K-ET",
  "firmware": "04062-07-S0002071-13-439",
  "rated_power": 15000,
  "dsp_version": "07",
  "arm_version": "13",
  "inverter_ip": "192.168.4.82",
  "daemon_version": "v0.1.0",
  "last_poll_time": "2026-06-14T11:59:30Z"
}
```

**`GET /api/sensors`** — List all available sensor names and categories:

```sh
$ curl -s http://localhost:8080/api/sensors
[
  {"name": "ppv", "category": "Main Telemetry"},
  {"name": "battery_soc", "category": "Battery"},
  {"name": "house_consumption", "category": "Meter"},
  ...
]
```

**`GET /api/data/{sensor}`** — Live Modbus read of a single sensor:

```sh
$ curl -s http://localhost:8080/api/data/battery_soc
{
  "name": "battery_soc",
  "value": 87,
  "unit": "%",
  "timestamp": "2026-06-14T12:00:01Z"
}
```

**`GET /api/data/{sensor}/aggregate`** — Historical samples from the database.
Supports `?since=` and `?until=` (RFC 3339), `?limit=` (default 1000), and
`?latest=true` to retrieve only the most recent sample:

```sh
# Last 24 hours (default)
$ curl -s http://localhost:8080/api/data/ppv/aggregate
{
  "sensor": "ppv",
  "samples": [
    {"timestamp": "2026-06-14T06:00:00Z", "value": 1200, "value_text": ""},
    ...
  ]
}

# Custom time range
$ curl -s "http://localhost:8080/api/data/ppv/aggregate?since=2026-06-14T00:00:00Z&until=2026-06-14T12:00:00Z&limit=500"

# Most recent sample only
$ curl -s "http://localhost:8080/api/data/ppv/aggregate?latest=true"

**`GET /api/data/{sensor}/aggregate?delta=`** — Cumulative delta query. Returns sample values adjusted by subtracting the value at the given timestamp. Useful for computing daily energy from cumulative registers:

```bash
$ curl -s "http://localhost:8080/api/data/e_total_exp/aggregate?delta=2026-06-26T00:00:00Z&limit=1"
```

**`GET /api/analysis/grid_voltage`** — List detected voltage quality events (outside 207V–253V range per IEC 60038). Supports cursor-based pagination:

```bash
$ curl -s "http://localhost:8080/api/analysis/grid_voltage?limit=5"
{
  "events": [
    {
      "id": 42,
      "phase": "meter_voltage1",
      "start_time": "2026-06-26T14:23:00Z",
      "end_time": "2026-06-26T14:28:12Z",
      "duration_seconds": 312,
      "min_voltage": 203.2,
      "max_voltage": 206.8,
      "avg_voltage": 205.4
    }
  ],
  "total_events": 42,
  "cursor": { "next": 37, "has_more": true },
  "analysis": { "last_run_at": "...", "poll_interval_seconds": 30 }
}
```
```

### Troubleshooting

Identify open ports. Example from a router which only supports RTU over UDP:

```sh
$ sudo nmap -sS -sU -p T:502,U:8899,U:48899 192.168.100.151  # replace with your inverter's IP
Starting Nmap 7.95 ( https://nmap.org ) at 2026-06-07 11:53 CEST
Nmap scan report for GW_WIFILAN_2 (192.168.100.151)
Host is up (0.093s latency).

PORT      STATE         SERVICE
502/tcp   closed        mbap
8899/udp  open|filtered ospf-lite
48899/udp open|filtered tc_ads_discovery
MAC Address: 28:56:2F:A8:EA:EC (Unknown)

Nmap done: 1 IP address (1 host up) scanned in 2.28 seconds
```

To query the inverter:

```sh
$ echo -n "WIFIKIT-214028-READ" | nc -u -w 2 192.168.100.151 48899
dongle@sn,dtls_port:8899,<inverter's SN>
```

And another inverter which uses WIFI+LAN Kit:

```sh
$ sudo nmap -sS -sU -p T:502,U:8899,U:48899 192.168.4.82
[sudo] password for maciek:
Starting Nmap 7.95 ( https://nmap.org ) at 2026-06-08 19:40 CEST
Nmap scan report for GW_WIFILAN_2 (192.168.4.82)
Host is up (0.074s latency).

PORT      STATE         SERVICE
502/tcp   open          mbap
8899/udp  closed        ospf-lite
48899/udp open|filtered tc_ads_discovery

Nmap done: 1 IP address (1 host up) scanned in 3.36 seconds

```

And the probe:

```sh
$ echo -n "WIFIKIT-214028-READ" | nc -u -w 2 192.168.4.82 48899
ccm@sn,ccm@sn,Solar-<sn>
```

### Links

Based on a super useful Python library: https://github.com/marcelblijleven/goodwe