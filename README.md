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

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Service health status |
| `GET` | `/api/info` | Inverter information |
| `GET` | `/api/sensors` | List of available sensors |
| `GET` | `/api/data/{sensor}` | Live sensor reading |
| `GET` | `/api/data/{sensor}/aggregate` | Historical sensor data from DB |
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