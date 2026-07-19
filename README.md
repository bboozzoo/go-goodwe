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

A multi-stage Dockerfile is provided at the repository root. Pre-built images are available via GitHub Container Registry:

```sh
podman pull ghcr.io/bboozzoo/go-goodwe:latest
```

To build from source:

```sh
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
  goodwe-daemon  # or ghcr.io/bboozzoo/go-goodwe:latest
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
| `RETENTION_DAYS` | `30` | Number of days of raw data to keep (0 = disable pruning) |
| `AGGREGATE_INTERVAL` | `1h` | Background aggregation interval (0 = disabled) |

The entrypoint also supports running commands inside the container. For example, to run voltage analysis offline:

```sh
podman exec -it goodwe goodwe-daemon -offline-analyze-voltage
```

Or use the CLI tool:

```sh
podman run --rm goodwe-daemon goodwe -version
```

### Docker Compose

A `docker-compose.yaml` is provided at the repository root for easy deployment:

```sh
docker compose up -d
# or with podman:
podman-compose up -d
```

Set the inverter IP and poll interval in the environment section before starting.

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
| `-aggregate` | `false` | One-shot: aggregate pending raw data into hourly/daily tables and exit |
| `-aggregate-backfill` | `false` | One-shot: aggregate ALL raw data into hourly/daily tables and exit |
| `-prune` | `false` | One-shot: delete raw samples older than `-retention-days` and exit |
| `-retention-days` | `30` | Number of days of raw data to keep (used with `-prune` and background pruning) |
| `-aggregate-interval` | `1h` | How often to run background aggregation (`0` = disabled) |
| `-purge` | `""` | One-shot: purge all data older than this date and exit (e.g. `2026-01-01`) |
| `-debug` | `false` | Enable debug logging |
| `-version` | `false` | Display version information and exit |
| `-offline-analyze-voltage` | `false` | Run voltage analysis on the DB once and exit |

### Database Maintenance

The daemon stores raw sensor samples every poll cycle. Over time the database
grows large (roughly 1 GB per month at 30 s polling with 199 sensors). To keep
the size manageable, the daemon aggregates raw data into hourly and daily
summary tables and prunes old raw data automatically.

**Retention defaults** (configurable via `-retention-days` and `-aggregate-interval`):

| Data | Retention | Approx. size |
|------|-----------|--------------|
| Raw samples | 30 days | ~1 GB |
| Hourly aggregates | 365 days | ~100 MB |
| Daily aggregates | forever | ~4 MB/year |

If the database has grown large (e.g. from a period without aggregation), you can
recover it manually before restarting the daemon:

```sh
# 1. Backfill ALL existing raw data into hourly/daily aggregate tables.
#    This may take a while on a large database.
$ ./goodwe-daemon -dbstore sqlite://~/.goodwe/goodwe.db -aggregate-backfill

# 2. Prune old raw samples (keep the last 30 days).
$ ./goodwe-daemon -dbstore sqlite://~/.goodwe/goodwe.db -prune -retention-days 30

# 3. Start the daemon normally. Background aggregation runs every hour
#    and pruning keeps raw data within the retention window.
$ ./goodwe-daemon -inverterip 192.168.4.82 -poll 30s -dashboard
```

To run a quick incremental aggregation (only unaggregated data since the last
run) without a full backfill:

```sh
$ ./goodwe-daemon -dbstore sqlite://~/.goodwe/goodwe.db -aggregate
```

After pruning, the SQLite file does not shrink automatically. To reclaim disk
space, run a one-time VACUUM (requires free space equal to the database size):

```sh
$ sqlite3 ~/.goodwe/goodwe.db 'VACUUM;'
```

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Service health status |
| `GET` | `/api/info` | Inverter information |
| `GET` | `/api/sensors` | List of available sensors |
| `GET` | `/api/data/{sensor}` | Live sensor reading |
| `GET` | `/api/data/{sensor}/aggregate` | Historical sensor data from DB: `?since=&until=&limit=&latest=&delta=&bucket=` |
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
Supports `?since=` and `?until=` (RFC 3339), `?limit=` (default 1000),
`?latest=true` to retrieve only the most recent sample, and `?bucket=hour`
or `?bucket=day` to query pre-aggregated hourly/daily summary tables instead
of raw samples (used automatically by the dashboard for 7 d and 30 d ranges):

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

# Pre-aggregated buckets (hourly or daily)
$ curl -s "http://localhost:8080/api/data/e_day/aggregate?bucket=day&since=2026-07-01T00:00:00Z&limit=2"
{
  "aggregated": "day",
  "sensor": "e_day",
  "samples": [
    {
      "sampled_at": "2026-07-01T00:00:00Z",
      "value": 9.6,
      "value_min": 0.0,
      "value_max": 3400.0,
      "value_avg": 9.6,
      "sample_count": 288
    },
    ...
  ]
}

For bucket queries, each sample includes `value` (alias for `value_avg`, kept
for backward compatibility), `value_min`, `value_max`, `value_avg`, and
`sample_count` (number of raw readings aggregated into this bucket).

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