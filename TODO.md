# TODO

## Vision
Go implementation of a GoodWe inverter library with full sensor coverage matching the Python reference, clean typed API, and robust endian-safe encoding.

---

## ✅ Done

### Phase 1: Typed Struct + Units + []byte Calculator
- [x] Add `Unit string`, `Name string` to `sensorDefinition`
- [x] Switch Calculator signature from `[]uint16` to `[]byte`
- [x] Refactor all reader helpers to use `encoding/binary.BigEndian`
- [x] Introduce typed return struct `SensorValue{Value any, Unit string, Name string}`
- [x] Change `GetSensors` to return `map[string]SensorValue`
- [x] Update `goodwe.go` interface
- [x] Update CLI to handle new struct
- [x] Update tests
- [x] Endianness verification: all readers work correctly on big-endian hosts

### Phase 2: Label/Enum Sensors
- [x] Port all const dicts from Python (`const.go`)
- [x] Add `enumReader`, `enumHReader`, `enumLReader`, `enum2Reader`, `enumBitmap4Reader`, `enumBitmap22Reader` helpers
- [x] Add label sensors to registry
- [x] Update tests with expected string values

### Phase 3: Battery Block
- [x] Bulk read for register 37000, 24 registers
- [x] Battery sensor registry (SoC, SoH, temperatures, cell voltages, BMS errors/warnings)
- [x] `GetSensors` performs two bulk reads and merges
- [x] Fallback: skip battery if ILLEGAL_DATA_ADDRESS
- [x] Update tests with battery sample data

### Phase 4: Meter Block
- [x] Bulk read for register 36000 with fallback chain (125→58→45 regs)
- [x] Meter sensor registry
- [x] Merge into GetSensors

### Phase 5: MPPT Block
- [x] Bulk read for register 35301, 61 regs
- [x] MPPT sensor registry
- [x] Merge into GetSensors

### End-to-End / CLI
- [x] CLI with `-ip`, `-readsensor`, `-poll`, `-listsensors`, `-verbose`, `-debug`, `-info`
- [x] `-info` displays model, serial, firmware, DSP/ARM versions, rated power, and operating mode
- [x] `ReadSensor(ctx, name)` for minimal per-block reads
- [x] Auto-reconnect on Modbus read failures
- [x] Busy probe response handling (`@busy` detection + longer backoff)
- [x] `Connect()` retries both probe + DTLS handshake with backoff

### Device Info
- [x] Read device info block (register 35000, 33 regs) for real model/firmware/serial/rated_power
- [x] `decodeGoodweString()` helper for UTF-16BE/ASCII auto-detection
- [x] `GetInfo()` returns populated `RatedPower`, `DSPVersion`, `ARMVersion` fields

### Discovery & Transport Architecture
- [x] `Transport` interface (Connect, Close, ReadRegisters)
- [x] `dtlsTransport` — DTLS + Modbus RTU framing (extracted from old service.go)
- [x] `tcpTransport` — plain TCP + Modbus TCP framing (MBAP header, no CRC)
- [x] `discovery.Discover()` — probes UDP:48899, detects DTLS vs TCP, creates correct transport
- [x] `goodwe.ErrUnsupported` — returned when inverter model is not recognized
- [x] Model tag matching from Python reference (KEU, KET, ETT, EHB, etc.)
- [x] CLI uses `discovery.Discover()` instead of `et.New() + Connect()`

### Device Info — Firmware & Model Derivation
- [x] Model derived from rated power when register field is blank (e.g. `15000W` → `"GW15K-ET"`)
- [x] DSP version parsed from firmware string fallback (`"04062-07-S00"` → `"07"`)
- [x] ARM version parsed from arm_firmware string fallback (`"02071-13-439"` → `"13"`)
- [x] `decodeGoodweString` hex fallback for non-printable bytes (matches Python `_decode`)

### Testing — Device Info
- [x] 9 ET device info hex files copied from Python reference library (MIT License)
- [x] `TestDeviceInfoFromPythonSamples` — validates all register fields against Python expected values
- [x] `TestDeviceInfoVersionFallback` — synthetic test for zero-uint16 version parsing

### CLI Polish
- [x] `-version` flag — reads `vcs.revision` from `debug.ReadBuildInfo()`
- [x] Minimum 5s poll interval guardrail (prevents inverter instability)
- [x] CLI moved from `examples/goodwe/` to `cmd/goodwe/` (Go convention)

### Release Infrastructure
- [x] `.goreleaser.yaml` — builds linux/darwin amd64+arm64, draft releases
- [x] `.github/workflows/ci.yml` — test + lint + snapshot build on master push
- [x] `.github/workflows/release.yml` — goreleaser on `v*.*.*` tag push

---

### Daemon — Voltage Quality Analysis
- [x] `pkg/analysis/` — analysis engine for grid voltage events (IEC 60038: 207V–253V)
- [x] DB tables: `voltage_analysis_cursor`, `voltage_events` with PRAGMA user_version schema tracking
- [x] Incremental cursor-based processing using sensor_samples.rowid (not timestamps)
- [x] Ongoing event detection across daemon restarts
- [x] `GET /api/analysis/grid_voltage` with cursor-based pagination
- [x] Daemon integration: runs after each poll cycle
- [x] `-offline-analyze-voltage` companion mode for one-shot analysis
- [x] `?delta=<timestamp>` parameter on aggregate endpoint for cumulative register deltas
- [x] Dashboard: Grid Voltage view with infinite-scroll events table
- [x] Multi-day integration test with real SQLite Store

### Dashboard — UX Improvements
- [x] Interactive zoom/pan with chartjs-plugin-zoom
- [x] Manual drag-to-pan via Pointer Events (cross-browser)
- [x] Visible max value per dataset in chart legend, updated on zoom/pan
- [x] Consistent y-axis width for x-axis alignment across charts
- [x] Daily energy cards (PV Today, Load Today, Grid Import/Export)
- [x] Temperature card (radiator)
- [x] Formula tooltips on system grid cards
- [x] Sensor filter clear button
- [x] Auto-load house_consumption and ppv_total on startup (6h default)

### Server-Side
- [x] Server-side downsampling for large time ranges (max 2000 points)
- [x] `?delta=<timestamp>` for cumulative register delta queries
- [x] PRAGMA user_version schema versioning for migrations

## 📋 Short-term — Easy Wins

Data already within existing read windows or small isolated changes.

### Register Constants (`et/et.go:122-126`)
- [ ] Replace magic numbers (35100, 37000, 36000, 35301, 125, 24, 61, etc.) with named package-level constants

### MPPT Block: Missing Sensors (data in 61-reg window, just need registry entries)
- [ ] Add `pmppt1`..`pmppt8` (MPPT1-8 power, regs 35337-35344) — `uint32Reader(36)`..`uint32Reader(43)`
- [ ] Add `imppt1`..`imppt8` (MPPT1-8 current, regs 35345-35352) — `uint16Reader(44, 0.1)`..`uint16Reader(51, 0.1)`
- [ ] Add `reactive_power1`..`reactive_power3` (regs 35353-35357) — `int16Reader`
- [ ] Add `apparent_power1`..`apparent_power3` (regs 35359-35363) — `int16Reader`

### AA55 Pre-Header — Replace Magic Offsets with Named Constants (`et/dtls_transport.go`)
- [ ] Detect AA55 presence rather than assuming it (uncommon but some inverters omit it)
- [ ] Define `aa55HeaderLen = 2` constant and use named offset constants

### Typed Modbus Error Handling (`et/et.go:275`)
- [ ] Define sentinel errors: `ErrIllegalDataAddress`, `ErrModbusCRC`, `ErrModbusException`
- [ ] Return typed errors from `parseModbusBulkResponse()`
- [ ] Replace `strings.Contains(err.Error(), "exception 0x02")` with `errors.Is(err, ErrIllegalDataAddress)`

---

## 🎯 Medium-term — Meaningful Features

### Modbus RTU Package (`et/dtls_transport.go`)
- [ ] Extract CRC16 to a separate `modbus` package
- [ ] Define `RTU` struct with `SlaveID`, `FunctionCode`, `Data []byte`
- [ ] `MarshalBinary()` — builds frame + appends CRC16
- [ ] `UnmarshalBinary()` — validates CRC and returns data
- [ ] `func (r *RTU) ReadHoldingRegisters(start, quantity uint16)` helper
- [ ] Use the package in dtlsTransport and tcpTransport

### Battery2 Block (39000, 22 regs + 35262, 6 regs)
- [ ] Add `blockBattery2` block type
- [ ] Add `battery2Registry` — 37000-style sensors for second battery
- [ ] Add `battery2ExtendedRegistry` — `vbattery2`, `ibattery2`, `pbattery2`, `battery2_mode`, `battery2_mode_label`
- [ ] Conditionally enable via serial number match (`BAT_2_MODELS` = `"25KET"`, `"29K9ET"`) or `rated_power >= 25000`

### DTLS Read Deadline + Retry
- [x] Set `SetReadDeadline(time.Now().Add(3 * time.Second))` on each `dtlsConn.Read()`
- [x] Auto-reconnect on `isConnClosed` errors (both DTLS and TCP transports)
- [x] Read/write deadlines: 5s DTLS, 10s TCP

### Poll Loop Reconnection
- [x] State machine with backoff: 5s initial, 5min max, reset on poll success
- [x] Startup connect failure no longer exits daemon — enters backoff
- [x] Poll failure disconnects and retries from scratch via backoff

---

## 🚀 Long-term — Major Features

### Daemon Mode with REST API + Dashboard

A persistent daemon that polls the inverter at a configurable interval, stores
sensor readings in a local SQLite database with time-based aggregation, and
exposes a REST API + embedded JS dashboard.

#### CLI Flags
```
-listen <address>:<port>  — address and port for the HTTP API server (default: ":8080")
-dashboard               — enable the embedded JS dashboard at /dashboard
-dbstore <dsn>           — database connection string (default: sqlite://~/.goodwe/goodwe.db,
                           e.g. sqlite:///var/lib/goodwe/history.db)
-poll <interval>          — sensor poll interval (default: "0" (disabled), min: 5s,
                           e.g. 30s, 1m)
-inverterip <ip>          — IP address of the GoodWe inverter
-purge <date>             — one-shot: purge all data older than this date and exit (e.g. 2026-01-01)
-debug                    — enable debug logging (includes HTTP request logging)
```

#### Implementation Status

| Component | Status |
|-----------|--------|
| SQLite dependency (`modernc.org/sqlite`) | Done |
| `pkg/db/store.go` — `Open`, `Close`, `GetInverterIdentity`, `SetInverterIdentity` | Done |
| `pkg/db/store.go` — `InsertSample`, `QueryRawSamples`, `LastSampleTime`, `LatestSample` | Done |
| `pkg/db/store.go` — `PurgeBadSamples` (data sanitization) | Done |
| `pkg/db/store.go` — `sensor_samples` + `inverter_identity` tables + migration | Done |
| `pkg/db/store.go` — `BeginTx`/transactional batching | Pending |
| `pkg/db/store.go` — `QueryAggregated` (hourly/daily) | Pending |
| `pkg/db/store.go` — `RunHourlyAggregation`, `RunDailyAggregation` | Pending |
| `pkg/db/store.go` — `PurgeRawSamples`, `PurgeHourlySamples` | Pending |
| `pkg/daemon/daemon.go` — Poll loop with `GetSensors` + `InsertSample` | Done |
| `pkg/daemon/daemon.go` — Identity verification (`verifyIdentity`) | Done |
| `pkg/daemon/daemon.go` — `InverterConnState` tracking | Done |
| `pkg/daemon/daemon.go` — Inverter reconnection on failure (state machine with backoff) | Done |
| `pkg/daemon/daemon.go` — Gap backfill on startup | Pending |
| `pkg/daemon/daemon.go` — Hourly/daily aggregation triggers | Pending |
| `pkg/api/handler.go` — Routes: health, sensors, info, data, aggregate | Done |
| `pkg/api/handler.go` — `?latest=true` support on aggregate endpoint | Done |
| `pkg/api/handler.go` — `/api/info` served from database (no inverter hit) | Done |
| `pkg/api/handler.go` — `last_poll_time` field in info response | Done |
| `pkg/api/handler.go` — CORS + request logging middleware | Done |
| `pkg/api/handler.go` — `DaemonStatus` interface | Done |
| `pkg/api/handler.go` — `SensorStore` interface | Done |
| `pkg/api/handler.go` — Purge endpoints (DELETE/POST) | Pending |
| `cmd/goodwe-daemon/main.go` — Flag parsing, DB, discovery, HTTP + poll | Done |
| `cmd/goodwe-daemon/main.go` — Graceful shutdown (15s timeout) | Done |
| `pkg/dashboard/` — Embedded HTML+JS single-page app | Done |
| `pkg/dashboard/` — Sensor list sidebar with search filter | Done |
| `pkg/dashboard/` — Insert null waypoints at time gaps > 10 min | Done |
| `pkg/dashboard/` — 'No data' message when time range has no samples | Done |
| `pkg/dashboard/` — Line charts with time range selector | Done |
| `pkg/dashboard/` — Live mode toggle (auto-refresh) | Done |
| `pkg/dashboard/` — Inverter status header (from DB) | Done |
| `pkg/dashboard/` — System status cards (grid, PV, battery, errors) | Done |
| README — CLI + daemon documentation | Done |
| Data sanitization at startup (`PurgeBadSamples`) | Done |

#### Gap Handling on Restart
When the daemon starts after a period of downtime (e.g., machine was off),
the aggregation logic must handle missing hours/days gracefully rather than
silently skipping them:

1. **On startup**, query the latest `sampled_at` from `sensor_samples`.
2. **Backfill aggregation** for any completed hour bucket between `latest_sample` and `now`:
   - For each missed hour: run `RunHourlyAggregation()` for that specific hour bucket.
   - After hourly backfill: run `RunDailyAggregation()` for each missed day bucket.
3. **Normal poll loop** then resumes, and subsequent aggregation triggers fire at hour/day boundaries as usual.
4. **Data integrity check**: The `RunHourlyAggregation` must be idempotent — if an hour bucket
already has an aggregate row, it should be **updated** (re-aggregated from raw samples for that hour)
rather than failing on a PRIMARY KEY conflict. Use `INSERT ... ON CONFLICT DO UPDATE`.
5. **Empty buckets**: If a missed hour has zero raw samples (because the daemon was down),
the aggregation for that hour should still produce a row with `sample_count=0` and NULL-ish
values, so the dashboard can show the gap visually. Alternatively, omit the row entirely
and let the dashboard handle gaps naturally (Chart.js `spanGaps: false`).
   - **Chosen approach**: Omit empty buckets. The dashboard uses `spanGaps: false` in Chart.js
     so missing time ranges display as line breaks rather than misleading interpolations.

#### Directory Layout
```
cmd/goodwe/               — existing CLI (unchanged)
cmd/goodwe-daemon/        — daemon binary
  main.go                 — flag parsing + daemon orchestration
pkg/daemon/
  daemon.go               — poll loop, identity verification, state tracking
  daemon_test.go          — state machine unit tests
pkg/db/
  store.go                — DB interface + SQLite implementation (includes
                           schema, queries, migrations — no separate files)
  store_test.go           — db unit tests (CRUD, sanitization, DSN parsing)
pkg/api/
  handler.go              — HTTP handler, routes, CORS + logging middleware
  handler_test.go         — API unit tests (all endpoints, CORS, gzip)
pkg/dashboard/
  dashboard.go            — embed wrapper
  index.html              — single-page dashboard app
```

#### Database Schema (SQLite)
```sql
CREATE TABLE sensor_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sensor_name TEXT NOT NULL,
    value REAL,              -- numeric value (NULL for label/string sensors)
    value_text TEXT,         -- string value (NULL for numeric sensors), e.g. "Normal (On-Grid)"
    unit TEXT NOT NULL DEFAULT '',
    sampled_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_samples_name_time ON sensor_samples(sensor_name, sampled_at);

-- Hourly aggregates (retain for 3 months)
CREATE TABLE sensor_hourly (
    sensor_name TEXT NOT NULL,
    hour_bucket TIMESTAMP NOT NULL,
    min_val REAL NOT NULL,
    max_val REAL NOT NULL,
    avg_val REAL NOT NULL,
    sample_count INTEGER NOT NULL,
    PRIMARY KEY (sensor_name, hour_bucket)
);

-- Daily aggregates (retain indefinitely)
CREATE TABLE sensor_daily (
    sensor_name TEXT NOT NULL,
    day_bucket DATE NOT NULL,
    min_val REAL NOT NULL,
    max_val REAL NOT NULL,
    avg_val REAL NOT NULL,
    sample_count INTEGER NOT NULL,
    PRIMARY KEY (sensor_name, day_bucket)
);
```

#### Data Retention & Aggregation Schedule
| Granularity | Retention | Aggregation Trigger |
|-------------|-----------|--------------------|
| Raw samples | 7 days    | Deleted on each aggregation run |
| Hourly      | 3 months  | Runs every hour (next_hour_bucket reached) |
| Daily       | Forever   | Runs at midnight (UTC) |

#### REST API Endpoints

Implemented:
```
GET  /api/health                        → {"status":"ok"|"degraded", inverter: {connected, error}}
GET  /api/sensors                       → [{name, category}, ...]
GET  /api/info                          → {serial, model, firmware, rated_power, dsp_version,
                                            arm_version, last_poll_time, error}
GET  /api/data/{sensor}                 → live Modbus read: {name, value, unit, timestamp}
GET  /api/data/{sensor}/aggregate       → raw samples from DB (?since=&until=&limit=&latest=)
```

Pending:
```
DELETE /api/data/{sensor}               → purge samples older than ?before=<date>
POST  /api/data/{sensor}/purge          → alternative: JSON body
```

#### Dashboard (Single-page App, Chart.js)
The dashboard is split into two views:

**Live Status View** (default)
- Shows all sensors in a key-value table, grouped by category (PV, Grid, Battery, Backup, Meter, etc.)
- Numeric values shown with unit badges (e.g., `352 V`, `3.2 kW`)
- Label values shown as human-readable text (e.g., `Normal (On-Grid)`, `Discharge`)
- Error/bitmap sensors shown as lists of active flags

**Chart View** (sensor selector)
- Dropdown populated from `/api/sensors`, **filtered to numeric sensors only**
  (sensors with `type: "numeric"`). Label/code/bitmap sensors are excluded from charting
  since they produce string values that don't plot on a line chart.
- Displays label/code sensors as a separate "Status Indicators" section below the chart
- Time range selector (1h, 6h, 24h, 7d, 30d)
- Line chart of selected sensor values, auto-scaled
- `spanGaps: false` — gaps in data show as line breaks, not interpolated
- Auto-refresh toggle (polls `/api/data/{sensor}` every N seconds)
- Shows current live value at the top
- Responsive, works on mobile
The dashboard is bundled via `//go:embed` and served as static files.
No build step required — raw HTML + JS with Chart.js loaded from CDN or vendored.

#### Implementation Steps

Steps 1, 3, 4, 6, 7, 8 are largely complete; remaining sub-items are listed below each.

1. **Add SQLite dependency** — ✅ `modernc.org/sqlite` in `go.mod`
2. **Create `pkg/db/store.go`**
   - `Open(path string) (*Store, error)` — ✅ opens/creates DB, runs migrations
   - `BeginTx(ctx) (*Tx, error)` — ❌ not yet implemented
   - `InsertSample(ctx, name, value, unit, t time.Time) error` — ✅ implemented (auto-commit only)
   - `QuerySamples(ctx, name string, since, until time.Time, limit int) ([]Sample, error)` — ✅ (`QueryRawSamples`)
   - `QueryAggregated(ctx, name, bucket string, since, until time.Time) ([]Aggregate, error)` — ❌ pending
   - `RunHourlyAggregation(ctx) error` — ❌ pending
   - `RunDailyAggregation(ctx) error` — ❌ pending
   - `PurgeRawSamples(ctx, before time.Time) error` — ❌ pending
   - `PurgeHourlySamples(ctx, before time.Time) error` — ❌ pending
3. **Create `pkg/daemon/daemon.go`** — ✅ done (including state machine with
   backoff and reconnection), missing sub-items:
   - Wrap poll inserts in `BeginTx()/Commit()` — ❌ currently single inserts
   - Gap backfill on startup (`LastSampleTime`) — ❌ not wired
   - Hourly/daily aggregation triggers — ❌ pending
4. **Create `pkg/api/handler.go`** — ✅ routes done, missing:
   - Purge endpoints — ❌ pending
5. **Create `pkg/dashboard/`** — ✅ done with full single-page app
6. **Create `cmd/goodwe-daemon/main.go`** — ✅ done
7. **Integration with existing code** — ✅ done
8. **Documentation** — ✅ README updated

#### DSN Format
The `-dbstore` flag uses a URI scheme to specify the database backend and location:

```
# Local SQLite file (default)
sqlite://~/.goodwe/goodwe.db
sqlite:///data/goodwe/history.db

# Future: could support PostgreSQL, etc.
postgres://user:pass@host:5432/goodwe?sslmode=disable
```

The `sqlite://` scheme is parsed to extract the file path. `~` is expanded to `$HOME`.
The `pkg/db/store.go` `Open(dsn string)` function parses the URI, selects the appropriate
driver, and returns the `Store` interface. This makes the architecture database-agnostic
for future backends (e.g., InfluxDB, TimescaleDB) while keeping the first implementation
simple with SQLite.

#### Concurrency & Locking

The daemon has two concurrent goroutines accessing the database:
1. **Poll loop** (producer) — writes sensor samples periodically
2. **HTTP server** (consumer) — reads samples/aggregates on demand

SQLite's concurrency model (via `database/sql` connection pool):
- Multiple concurrent **readers** are allowed (SHARED lock)
- Only one **writer** at a time (RESERVED/EXCLUSIVE lock)
- Writers block readers briefly, and vice versa

For our workload this is perfectly fine — one short write burst every 30s, reads are sporadic.
The `database/sql` pool handles concurrency transparently.

**No Go-level mutex needed.** The `*sql.DB` pool manages connections; `modernc.org/sqlite`
respects SQLite's locking protocol. All synchronization happens at the SQLite file level.

**Transactional integrity:**
- Poll loop: all N sensor inserts per tick are wrapped in `BeginTx()/Commit()`.
  If Commit fails, Rollback ensures no partial data. Duration: <50ms for ~200 inserts.
- Aggregation: each aggregation step (read raw → compute → write aggregates → delete old)
  runs in a single transaction. If the daemon crashes mid-aggregation, the incomplete
  transaction is rolled back on next open (SQLite auto-rollback on crash).
- API reads: no transaction needed for simple point queries; they use auto-commit reads.
  The `/api/data/{sensor}/aggregate` query may use a read-only transaction for consistency
  if reading multiple rows, but this is optional.

**WAL mode:** The SQLite database should be opened with `PRAGMA journal_mode=WAL`.
WAL (Write-Ahead Logging) allows concurrent reads during a write — the reader sees the
pre-write snapshot while the writer appends to the WAL. This is ideal for our producer/
consumer pattern. Enabled in `Open()` as part of migrations.

#### Database Format & Robustness

A single SQLite database file is used throughout — no yearly rotation.

**Why a single file is sufficient:**
- The aggregation scheme keeps the database compact:
  - Raw samples: purged after 7 days (bounded at ~20K rows per sensor, ~2M total for 100 sensors)
  - Hourly aggregates: purged after 3 months (~2,160 rows per sensor per year)
  - Daily aggregates: kept forever (~365 rows per sensor per year)
- With 100 sensors and 10 years of daily aggregates: ~365K rows total. SQLite handles millions
trivially.
- The daemon is low-frequency (poll every 30s), so write volume is tiny.

**What about corruption or backup?**
- The `-purge` flag can be used for manual cleanup.
- For backups, copy the single `.db` file — `sqlite3 goodwe.db ".backup backup.db"` works online.
- If yearly archival is desired later, a separate `-archive` command can export daily aggregates
  to a read-only archive file, but this is out of scope for v1.

**Is `modernc.org/sqlite` robust enough?**
Yes. `modernc.org/sqlite` is a pure-Go translation (via the `ccgo` compiler) of the official
SQLite C source. It passes the full SQLite test suite. It's used in production by projects like
`go-kratos` and various Kubernetes tools. The performance characteristics:
- Read throughput: ~80-90% of CGO sqlite3 — irrelevant for our low-volume workload
- Write throughput: more than adequate for 1 write every 30s
- Memory: negligible
The only real downside vs CGO is larger binary size (~5MB extra from the pure Go translation),
which is fine for a daemon binary. The benefit (no CGO, trivial cross-compilation) is worth it.

#### Dependencies
- `modernc.org/sqlite` — pure Go SQLite driver (no CGO)
- No JS dependencies at build time (Chart.js loaded from CDN via <script> tag)

### Model Detection
Python detects inverter capabilities from serial number:
- [ ] `is_single_phase()` / `is_3_phase()`
- [ ] `is_4_mppt()` — filter PV3/PV4, L2/L3 for 2-MPPT / single-phase
- [ ] `is_2_battery()` — conditionally enable battery2 block
- [ ] `is_745_platform()` / `is_753_platform()` — firmware-version-specific settings

### Sensor Kind / Categorization
- [ ] Add `SensorKind` type (PV, AC, UPS, BAT, GRID, BMS, MPPT, METER)
- [ ] Add `Kind` field to `sensorDefinition`
- [ ] Add `GetSensorsByKind()` helper
- [ ] Expose kind in CLI or API

### Settings Read/Write
- [ ] Add `__all_settings` dictionary (matching Python)
- [ ] `read_setting(ctx, name)` — single-setting read
- [ ] `write_setting(ctx, name, value)` — Modbus write with encode
- [ ] Support ARM FW 19 vs 22 variant settings

### MQTT Integration

Forward sensor values to an MQTT broker for integration with Home Assistant
(or other smart-home systems). Home Assistant supports auto-discovery via
MQTT, which would automatically create sensor entities for each inverter
sensor.

#### Design

- New flag: `-mqtt-broker tcp://192.168.1.100:1883` (enable MQTT publishing)
- Additional flags: `-mqtt-user`, `-mqtt-pass`, `-mqtt-topic-prefix` (default:
  `goodwe/<serial>/sensor/<name>`)
- On each poll tick, after storing samples in the DB, publish each sensor
  value to its MQTT topic as a JSON payload:
  ```json
  {"value": 60, "unit": "%", "name": "Battery State of Charge",
   "sampled_at": "2026-06-10T12:00:00Z"}
  ```
- Home Assistant MQTT Discovery: publish discovery config to
  `homeassistant/sensor/goodwe_<serial>_<name>/config` with the correct
  `state_topic`, `unit_of_measurement`, `device_class`, etc.
- Reconnection: if the MQTT broker is unavailable, log and retry with
  exponential backoff (do not block the poll loop).
- Use `eclipse/paho.mqtt.golang` (the de-facto Go MQTT client library).

#### Implementation Steps
1. Add `eclipse/paho.mqtt.golang` dependency
2. Create `pkg/mqtt/publisher.go` — connects to broker, publishes samples
3. Create `pkg/mqtt/discovery.go` — Home Assistant MQTT discovery config
4. Wire publisher into daemon's `pollOnce()`: after `InsertSample`, publish
   to MQTT
5. Add CLI flags to `cmd/goodwe-daemon/main.go`
6. Update README with MQTT configuration examples

#### Dependencies
- `github.com/eclipse/paho.mqtt.golang` — MQTT 3.1.1 client

### TCP Port 502 Support
- [x] Basic Modbus TCP transport via `tcpTransport` (MBAP framing, no CRC)
- [x] Auto-detect DTLS vs TCP in discovery (done) — transport is determined from probe response format

---

## 🛠 Robustness & Polish

### Daemon
- [x] Document the REST API in Go doc comments on handler methods
- [ ] Add API usage examples to README (curl commands for each endpoint)
- [ ] Add database management section to README (backup, DSN, purge)
- [x] Add unit tests for:
  - `/api/health` response format and status codes
  - `/api/info` with and without inverter configured
  - `/api/sensors` response format and sensor count
  - Serial mismatch detection and error propagation
  - DSN parsing (`sqlite://path`, `~` expansion, error cases)
  - `parseDSN` edge cases (empty path, missing scheme)
  - Graceful shutdown sequence (signal → context cancellation → resource cleanup)
  - Min poll interval enforcement

### General

- [ ] Add tests for `parseProbeResponse` with `@busy` and edge cases
- [ ] Add tests for `ReadSensor` with meter fallback (45/58/125)
- [ ] Add integration test harness (mock inverter or recorded sessions)
- [ ] Document public API in Go doc comments
- [ ] `int16Reader` does not check for `undef16` (0xFFFF) — consider handling it
- [ ] `grid_in_out` / `grid_in_out_label` read hardcoded offset 80 which overlaps with `vbattery1` index — extract as named constant

---

## 📁 Code Map

| File | Purpose |
|---|---|
| `goodwe.go` | Root interface (`Inverter`, `SensorValue`, `Info`), `ErrUnsupported` |
| `discovery/discover.go` | `Discover()` — probes inverter, detects transport, returns `Inverter` |
| `et/et.go` | `ETInverter` — Connect, Close, GetInfo, GetSensors, ReadSensor |
| `et/transport.go` | `Transport` interface (Connect, Close, ReadRegisters) |
| `et/dtls_transport.go` | DTLS + Modbus RTU framing, CRC16 |
| `et/tcp_transport.go` | Plain TCP + Modbus TCP framing (MBAP) |
| `et/registry.go` | Sensor definitions, reader helpers, decode bitmap |
| `et/const.go` | Label dictionaries (PV modes, grid modes, errors, etc.) |
| `et/resilience.go` | Exponential backoff helper |
| `et/et_test.go` | Unit tests with sample hex data |
| `cmd/goodwe/main.go` | CLI with flags: `-ip`, `-readsensor`, `-poll`, `-listsensors`, `-info`, `-version` |
| `cmd/goodwe-daemon/main.go` | Daemon entrypoint — flag parsing, DB init, HTTP + poll loop orchestration |
| `pkg/db/store.go` | SQLite store: schema, migrations, identity, samples, queries |
| `pkg/db/store_test.go` | DB unit tests: CRUD, sanitization, DSN parsing |
| `pkg/daemon/daemon.go` | Poll loop, identity verification, state tracking |
| `pkg/daemon/daemon_test.go` | State machine unit tests |
| `pkg/api/handler.go` | HTTP routes, CORS + logging middleware, daemon + store interfaces |
| `pkg/api/handler_test.go` | API unit tests: all endpoints, CORS, gzip |
| `pkg/dashboard/dashboard.go` | Embed wrapper serving the dashboard HTML |
| `pkg/dashboard/index.html` | Single-page dashboard app (Chart.js, dark theme) |
| `.goreleaser.yaml` | GoReleaser build config (linux/darwin, amd64/arm64) |
| `.github/workflows/ci.yml` | CI: test + lint + snapshot build on master push |
| `.github/workflows/release.yml` | Release: goreleaser draft on `v*.*.*` tag |
