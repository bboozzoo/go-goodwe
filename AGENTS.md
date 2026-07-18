# Agent Instructions

## Core Context
- This project is a Go implementation of a GoodWe inverter library.
- The Python implementation in `python/goodwe/` is the primary reference for protocol logic and implementation details.
- `cmd/goodwe/main.go` is a functional CLI client with -readsensor, -poll, -listsensors, -info, and -version flags.
- `GetSensors(ctx)` returns `map[string]SensorValue` where `SensorValue{Value any, Unit string, Name string}`.
- Four register blocks are read in `GetSensors`:
  - Main telemetry (35100, 125 regs) → `registry`
  - Battery info (37000, 24 regs) → `batteryRegistry` (skipped on ILLEGAL_DATA_ADDRESS)
  - Meter data (36000, 125→58→45 regs fallback) → `meterRegistry` (skipped on error)
  - MPPT data (35301, 61 regs) → `mpptRegistry` (skipped on ILLEGAL_DATA_ADDRESS)

## Development Workflow
- **Code Quality**: 
  - All code must be `gofmt` clean.
  - Use `golangci-lint` for verification (run `golangci-lint run ./...` before committing).
  - Test files must pass linter too — suppressed errors are acceptable for trivial cases
    (`_ = fn()` style is preferred over naked calls that errcheck would flag).
  - **Test assertions**: Use `github.com/stretchr/testify/assert` and `require` instead of raw `t.Fatalf()` calls.
    Prefer `assert.Equal(t, expected, actual)`, `require.NoError(t, err)`, `require.NotNil(t, v)`, etc.
- **Task Tracking**: Consult `TODO.md` for the current roadmap and pending items.
- **Code Map**: See `TODO.md § 📁 Code Map` for a file-level overview of the codebase.
- **Testing Strategy**: Refer to the Python test suite (`python/goodwe/tests/`) to understand expected behavior and protocol nuances.
  - All new features and bug fixes must include unit tests using `github.com/stretchr/testify/assert` and `require`.
  - Integration tests using real SQLite (`db.Open`) are preferred over pure mocks for DB-dependent logic.
- **Database Migrations**: Schema changes are versioned using `PRAGMA user_version`. Always add a migration step in the `migrate()` function gated by a version check (`if sv < N { ... }`). Never run DROP/CREATE unconditionally on startup.


## PROTOCOL

Target Port	Transport Protocol	Encryption	Modbus Payload	Header / Trailer
502	TCP	None	Modbus TCP	7-byte MBAP Header, No CRC
8899 (Legacy)	UDP	None	Modbus RTU	Raw RTU + 2-byte CRC
8899 (Modern)	UDP	DTLS	Modbus RTU	Encrypted RTU + 2-byte CRC

The GoodWe Modbus RTU Over UDP ArchitectureWhen communicating with GoodWe ET/EH
inverters over the Wi-Fi/Dongle interface (Port 8899), the protocol utilizes
Modbus RTU encapsulation inside network datagrams (UDP or DTLS) rather than
standard Modbus TCP.

### Frame Structure

Every request sent to the inverter must be structured as a valid Modbus RTU
frame. It does not use the network-facing MBAP header.

| Component | Size | Value / Range |Purpose |
| Slave ID | 1 Byte| 0xF7 (Decimal 247) | Identifies the hybrid inverter target. Requests targeting 0x01 are ignored.
| Function Code| 1 Byte | 0x03 (Read Holding) | Commands the internal processor to fetch register values.
| Register Address| 2 Bytes| e.g., 0x89 0x1C (35100)| The physical starting register address in Big-Endian format.
| Quantity | 2 Bytes| e.g., 0x00 0x7D (125 regs)| Number of 16-bit registers to read.
| CRC16 Trailer| 2 Bytes| Variable (Low, High)| Cyclic Redundancy Check calculated over all previous bytes.

### The Interaction Lifecycle

[ Go Client ]                                         [ GoodWe Dongle ]
      │                                                      │
      ├─────── Cleartext UDP Probe (Port 8899) ─────────────>│
      │                                                      │
      <─────── Probe Response ("dtls_port:8899") ────────────┤
      │                                                      │
      ├─────── DTLS Handshake ──────────────────────────────>│
      │                                                      │
      ├─────── Encrypted Modbus RTU Bulk Request ───────────>│ (e.g., Read 125 regs from 35100)
      │                                                      │
      <─────── Encrypted Modbus RTU Bulk Response ───────────┤ (Returns AA55 + Data or Error Exception)

### Critical Protocol Quirks

Strict Boundary Constraints (Error 0x83 0x02): The inverter firmware will throw
an ILLEGAL DATA ADDRESS exception if you attempt to read single registers or
fractional blocks. You must request predefined bulk blocks (such as the
telemetry block starting at register 35100).

The AA55 Pre-Header Response: When the inverter responds successfully, the
decrypted UDP payload always contains a proprietary 2-byte framing signature
(0xAA 0x55) prepended to the standard Modbus RTU response block. The response
layout is:

| Offset | Size | Field              | Example  |
|--------|------|--------------------|----------|
| 0      | 2    | AA55 pre-header    | AA 55    |
| 2      | 1    | Slave ID           | F7       |
| 3      | 1    | Function Code      | 03       |
| 4      | 1    | Byte Count         | FA (250) |
| 5      | N    | Register Data      | ...      |
| 5+N    | 2    | CRC16 (Modbus RTU) | ...      |

The CRC16 covers only the Modbus RTU portion (offsets 2 through 5+N-1). The
AA55 pre-header is NOT included in CRC calculation.

On error, Function Code has bit 7 set (e.g., 0x83) and offset 4 contains the
Modbus exception code (e.g., 0x02 = ILLEGAL DATA ADDRESS).

Packet Dropping: The low-power embedded Wi-Fi chips on these dongles frequently
drop packets or hit transient timeouts under load. Your Go network layer should
use aggressive timeout deadlines (e.g., 2–3 seconds) and a retry mechanism
rather than failing immediately on an EOF.

## Daemon Architecture

The daemon (`cmd/goodwe-daemon/`) is a persistent background service that polls
the inverter periodically, stores sensor readings in a local SQLite database,
and exposes a REST API + embedded JS dashboard.

### State Machine

The daemon poll loop implements an explicit state machine with backoff:

    disconnected -> doConnect() -> Connecting
      |-- success -> Connected -> doPoll() (immediately after reconnect)
      |                |-- success -> wait(pollInterval) -> Connected
      |                |-- error   -> close connection -> Disconnected
      |-- error   -> wait(backoff, doubling 5s->10s->...->5min) -> Disconnected

States: Disabled (no inverter), Disconnected (will retry), Connecting (in progress),
Connected (verified), Failed (serial mismatch, permanent stop).
State is exposed via `DaemonStatus` interface for the API handler.

On a successful reconnect, the daemon polls immediately rather than waiting
for the next poll interval tick — the fresh connection has a high probability
of a successful read.

When a DTLS dial fails, the daemon sends a cleartext UDP probe to port 48899
(`discovery.Ping`) to distinguish a broken dongle DTLS server from a completely
unreachable inverter. The probe result is logged at WARN level with a
specific message.

### Background Aggregation and Retention

The daemon runs a background goroutine (`aggregationLoop`) that periodically
aggregates raw samples into hourly and daily summary tables and prunes old data.
Configurable via `-aggregate-interval` (default 1h) and `-retention-days`
(default 30). Set either to 0 to disable.

**Retention defaults:**

| Data | Retention | Approx. size |
|------|-----------|--------------|
| Raw samples | 30 days | ~1 GB |
| Hourly aggregates | 365 days | ~100 MB |
| Daily aggregates | forever | ~4 MB/year |

Aggregation runs before pruning to guarantee no data loss. One-shot CLI
commands (`-aggregate`, `-aggregate-backfill`, `-prune`) allow manual recovery
of a bloated database before restarting the daemon.

### Connection Transport

- `et/tcp_transport.go`: Plain TCP + Modbus TCP framing (MBAP header, port 502)
- `et/dtls_transport.go`: DTLS + Modbus RTU framing (UDP, port 8899)
- Both auto-reconnect on closed connections with read/write deadlines (5s DTLS, 10s TCP)
- `et/transport.go`: `Transport` interface (`Connect`, `Close`, `ReadRegisters`)
- `discovery/discover.go`: `Ping()` for connectivity diagnostics (UDP probe to port 48899)

### Packages

| Package | Purpose |
|---------|---------|
| `cmd/goodwe-daemon/` | Entrypoint: flag parsing, DB init, HTTP + poll loop orchestration |
| `pkg/daemon/` | Poll loop with state machine, identity verification, aggregation scheduler |
| `pkg/db/` | SQLite store: schema, migrations, samples, identity, aggregation, pruning |
| `pkg/api/` | HTTP handler, routes (health/info/sensors/data/analysis), CORS, gzip, logging |
| `pkg/analysis/` | Grid voltage quality analysis engine (IEC 60038: 207V–253V) |
| `pkg/dashboard/` | Embedded single-page HTML+JS dashboard (Chart.js, light/dark theme) |
| `discovery/` | UDP probe-based inverter discovery and `Ping()` connectivity check |
| `et/` | Modbus transport (TCP, DTLS), sensor registries, protocol framing |

### REST API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Service health: `{status, inverter: {connected, error}}` |
| `GET` | `/api/info` | Inverter identity from DB: `{serial, model, firmware, rated_power, last_poll_time, daemon_version, error}` |
| `GET` | `/api/sensors` | List all 199 sensors: `[{name, category}, ...]` |
| `GET` | `/api/data/{sensor}` | Live Modbus read: `{name, value, unit, timestamp}` |
| `GET` | `/api/data/{sensor}/aggregate` | Historical from DB: `?since=&until=&limit=&latest=&delta=&bucket=hour/day` |
| `GET` | `/api/analysis/grid_voltage` | Voltage quality events: `?before=&limit=` |

All responses include a `Goodwe-Daemon-Version` header with the build version.
| `GET` | `/dashboard` | Single-page dashboard |
| `GET` | `/` | Redirect to `/dashboard` |

### Database

- SQLite via `modernc.org/sqlite` (pure Go, no CGO, WAL mode), `PRAGMA user_version` schema tracking (currently v4)
- Default location: `~/.goodwe/goodwe.db`
- Schema versioning: all schema changes gated by `if sv < N` version checks in `migrate()`
- Tables:
  - `inverter_identity` (serial, model, firmware, dsp/arm versions, rated_power)
  - `sensor_samples` (raw readings with dual value/value_text columns)
  - `sensor_samples_hourly` (WITHOUT ROWID: per-sensor, per-hour min/max/avg/count)
  - `sensor_samples_daily` (WITHOUT ROWID: per-sensor, per-day min/max/avg/count)
  - `voltage_events` / `voltage_analysis_cursor` (grid voltage quality analysis)
- Data sanitization at startup: purges physically impossible values based on unit
  (e.g., W > rated_power*2, % > 100 or < 0, V > 600, A > 50)
- Aggregate tables use `WITHOUT ROWID` so the primary key IS the clustered index —
  no separate index needed
- Readable concurrently during writes via WAL mode

### Dashboard

- Single HTML file at `/dashboard`
- Chart.js from CDN, light/dark theme toggle (CSS custom properties, saved to localStorage)
- Sensor list sidebar with search filter, grouped by category
- Line charts with time range selector (1h, 6h, 24h, 7d, 30d)
  - 1h/6h/24h: raw samples from `sensor_samples`
  - 7d: hourly aggregates (`bucket=hour`)
  - 30d: daily aggregates (`bucket=day`)
- Inserted null waypoints break lines at time gaps > 10 minutes
- X-axis pinned to exact time window boundaries (no padding on sides)
- Live mode toggle (auto-refresh every 5s)
- System status cards (grid mode, PV power, battery SoC, errors, temperature)
- Daily energy cards (PV production, grid import/export, load today)
- Grid voltage events table with infinite scroll
- Header shows daemon version, inverter identity, last poll time, connected state
