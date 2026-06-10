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
$ ./goodwe-daemon -daemon :8080 -inverterip 192.168.4.82
```

With debug logging and a custom database path:

```sh
$ ./goodwe-daemon -daemon :8080 -inverterip 192.168.4.82 \
    -dbstore sqlite:///var/lib/goodwe/history.db \
    -poll 30s -debug
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-daemon` | (required) | Address and port for the HTTP API server (e.g. `:8080`) |
| `-inverterip` | `""` | IP address of the GoodWe inverter |
| `-dbstore` | `sqlite://~/.goodwe/goodwe.db` | Database connection string |
| `-poll` | `0` | Sensor poll interval (minimum 5s) |
| `-dashboard` | `false` | Enable the embedded JS dashboard |
| `-purge` | `""` | One-shot: purge data older than this date and exit |
| `-debug` | `false` | Enable debug logging |
| `-version` | `false` | Display version information |

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Service health status |
| `GET` | `/api/info` | Inverter information |
| `GET` | `/api/sensors` | List of available sensors |
| `GET` | `/api/data/{sensor}` | Sensor sample data (planned) |
| `GET` | `/api/data/{sensor}/aggregate` | Aggregated sensor data (planned) |
| `GET` | `/dashboard` | Embedded dashboard (WIP) |

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