# Open Issues — go-goodwe @ fb652d8

All issues found by reviewing the codebase at master commit `fb652d8`.

---

## Section 1 — Code Quality / Technical Debt

### 1.1 Magic register numbers lack named constants

**File:** `et/et.go:161–166`, `et/registry.go` (`init()`)

Register addresses and counts are scattered as bare integer literals throughout
`GetSensors`, `ReadSensor`, and the `sensorLookup` init:

```go
// TODO 35100, 37000 need to be named constants
data, err := e.readOnceWithFallback(ctx, 35100, 125)
batteryData, err := e.transport.ReadRegisters(ctx, 37000, 24)
```

**Action:** Define package-level named constants, e.g.:

```go
const (
    baseRegistersOffset    uint16 = 35100
    baseRegistersCount     uint16 = 125
    batteryRegistersOffset uint16 = 37000
    batteryRegistersCount  uint16 = 24
    mpptRegistersOffset    uint16 = 35301
    mpptRegistersCount     uint16 = 61
    meterRegistersOffset   uint16 = 36000
    // meter fallback counts: 125 → 58 → 45
)
```

Replace all raw literals in `et.go` and `registry.go` (`init()`).

---

### 1.2 Modbus exception detection uses fragile string matching

**File:** `et/et.go:313–317`

```go
// TODO should use a proper typed error: ErrModbusIllegalDataAddress
func isIllegalDataAddress(err error) bool {
    return strings.Contains(err.Error(), "exception 0x02")
}
```

If the error message format ever changes this check silently breaks.

**Action:**
- Define sentinel errors in the `et` package:
  ```go
  var (
      ErrIllegalDataAddress = errors.New("modbus: illegal data address")
      ErrModbusCRC          = errors.New("modbus: CRC mismatch")
      ErrModbusException    = errors.New("modbus: exception")
  )
  ```
- Wrap them with `%w` in `parseModbusBulkResponse()`.
- Replace `isIllegalDataAddress(err)` with `errors.Is(err, ErrIllegalDataAddress)`.

---

### 1.3 AA55 pre-header assumed always present; no named offset constants

**File:** `et/dtls_transport.go:219–250` (`parseModbusBulkResponse`)

The current parser hard-codes offsets that assume AA55 is always at bytes 0–1.
Some inverters reportedly omit the header. There is also no named constant for the
2-byte header length.

**Action:**
- Detect AA55 dynamically: check `data[0] == 0xAA && data[1] == 0x55` before skipping.
- Define `const aa55HeaderLen = 2`.
- Use named offset constants for Slave ID, Function Code, Byte Count offsets within
  the response frame.

---

### 1.4 `int16Reader` does not guard against the `undef16` sentinel (0xFFFF)

**File:** `et/registry.go:465–473`

`uint16Reader` checks for `undef16 = 0xFFFF` and returns `0.0` instead.
`int16Reader` performs no such check — `0xFFFF` interpreted as `int16` is `-1`,
which will appear as a valid reading (e.g. `-0.01 Hz`) for sensors like `fgrid`,
`pgrid`, etc.

```go
func int16Reader(regIdx int, scale float64) func([]byte) any {
    return func(data []byte) any {
        // no undef16 guard here
        return float64(int16(binary.BigEndian.Uint16(data[offset:offset+2]))) * scale
    }
}
```

**Action:** Add a guard matching `uint16Reader`:
```go
raw := binary.BigEndian.Uint16(data[offset : offset+2])
if raw == undef16 {
    return float64(0)
}
return float64(int16(raw)) * scale
```

---

### 1.5 `grid_in_out` / `grid_in_out_label` use a hardcoded byte offset (80)

**File:** `et/registry.go:131–161`

Both calculators hardcode `data[80:82]` which corresponds to register index 40
(`active_power`). This overlap with `vbattery1` register numbering is confusing,
and the magic number 80 is undocumented.

**Action:** Extract as a named constant (e.g. `gridInOutRegIdx = 40`) and use
`regIdx * 2` like all other readers.

---

## Section 2 — Missing Sensor Coverage

### 2.1 MPPT block: power and current sensors missing for MPPT1–8

**File:** `et/registry.go` (mpptRegistry)

The 61-register MPPT window (35301–35361) already contains data for MPPT1–8
power and current but no registry entries exist for them:

| Sensor key      | Registers     | Reader |
|-----------------|---------------|--------|
| `pmppt1`–`pmppt8` | 35337–35344 (regs 36–43) | `uint32Reader` |
| `imppt1`–`imppt8` | 35345–35352 (regs 44–51) | `uint16Reader(_, 0.1)` |

**Action:** Add `pmppt1`..`pmppt8` and `imppt1`..`imppt8` to `mpptRegistry`.

---

### 2.2 MPPT block: reactive and apparent power per-phase missing

**File:** `et/registry.go` (mpptRegistry)

Also within the 61-register MPPT window:

| Sensor key             | Registers         | Reader |
|------------------------|-------------------|--------|
| `reactive_power1`–`3`  | 35353–35357 (regs 52–56) | `int16Reader` |
| `apparent_power1`–`3`  | 35359–35363 (regs 58–62) | `int16Reader` |

**Action:** Add these entries to `mpptRegistry`.

---

### 2.3 Battery2 block not implemented

**File:** `et/et.go`, `et/registry.go`

Dual-battery models (serial: `25KET`, `29K9ET`, or `rated_power >= 25000`)
have a second battery block at registers 39000 (22 regs) and 35262 (6 regs)
that is never read.

**Action:**
- Add `battery2Registry` and `battery2ExtendedRegistry` sensor maps.
- In `GetSensors`, check the serial/rated_power condition and conditionally read
  the second battery block.

---

## Section 3 — Architecture / Refactoring

### 3.1 Modbus RTU framing is duplicated across transports

**File:** `et/dtls_transport.go`, `et/tcp_transport.go`

CRC16, frame building, and frame parsing logic is duplicated between the DTLS and
TCP transports. This violates DRY and makes future protocol fixes error-prone.

**Action:** Extract into a dedicated `modbus` (or `et/modbus`) package:
- `RTU` struct with `SlaveID`, `FunctionCode`, `Data []byte`
- `MarshalBinary()` — builds frame + appends CRC16
- `UnmarshalBinary()` — validates CRC, returns data
- `ReadHoldingRegisters(start, quantity uint16)` helper
- Use the package from both `dtlsTransport` and `tcpTransport`

---

## Section 4 — Daemon: Incomplete Features

### 4.1 Aggregate endpoint ignores `bucket` parameter

**File:** `pkg/api/handler.go:349–352`

```go
// TODO: support bucket="hour" and bucket="day" using aggregate tables.
_ = bucket

samples, err := h.store.QueryRawSamples(...)
```

The `/api/data/{sensor}/aggregate?bucket=hour|day` parameter is parsed but
immediately discarded. All queries go to raw samples, making 7-day and 30-day
dashboard views slow and high-cardinality.

**Action:**
- Implement `pkg/db.QueryAggregated(ctx, sensor, bucket, since, until)` against
  the `sensor_hourly` / `sensor_daily` tables.
- When `bucket=hour` or `bucket=day`, call `QueryAggregated` instead of
  `QueryRawSamples`.

---

### 4.2 Hourly and daily aggregation pipelines not implemented

**File:** `pkg/db/store.go`, `pkg/daemon/daemon.go`

The `sensor_hourly` and `sensor_daily` schema tables exist in the migration but
none of the aggregation functions are implemented:

| Function | Status |
|----------|--------|
| `RunHourlyAggregation(ctx) error` | ❌ not implemented |
| `RunDailyAggregation(ctx) error` | ❌ not implemented |
| `PurgeRawSamples(ctx, before time.Time) error` | ❌ not implemented |
| `PurgeHourlySamples(ctx, before time.Time) error` | ❌ not implemented |

The daemon poll loop never triggers aggregation runs.

**Action:**
- Implement the four missing `Store` methods.
- Wire `RunHourlyAggregation` into the poll loop at each completed hour boundary.
- Wire `RunDailyAggregation` at each UTC midnight boundary.
- Use `INSERT ... ON CONFLICT DO UPDATE` to make each run idempotent.

---

### 4.3 Gap backfill for aggregation not implemented on daemon restart

**File:** `pkg/daemon/daemon.go`

When the daemon restarts after downtime, historical buckets between the last
sample and now are never aggregated. The `LastSampleTime` hook exists in the DB
but is not wired into startup.

**Action:**
1. On startup, query `LastSampleTime` from `sensor_samples`.
2. For each missed completed-hour bucket: call `RunHourlyAggregation(hour)`.
3. For each missed completed-day bucket: call `RunDailyAggregation(day)`.
4. Empty buckets (no data because daemon was down) are omitted; the dashboard
   renders gaps via `spanGaps: false`.

---

### 4.4 Poll inserts are not transactional

**File:** `pkg/daemon/daemon.go`, `pkg/db/store.go`

Each sensor sample is currently inserted as an individual auto-commit statement.
A crash or forced shutdown mid-poll leaves a partial set of samples for that tick.

**Action:**
- Implement `BeginTx(ctx) (*Tx, error)` in `pkg/db/store.go`.
- Wrap all N sensor inserts per poll tick in `BeginTx()/Commit()`.
- On error: call `Rollback()` to ensure no partial data is persisted.

---

### 4.5 `/api/status` endpoint not implemented

**File:** `pkg/api/handler.go`

The dashboard Live Status View requires a single endpoint returning all live
sensor readings. It is listed as pending and no route exists.

**Action:**
- Add `GET /api/status` route.
- Call `inverter.GetSensors(ctx)` and return the full map as JSON structured by
  category:
  ```json
  {
    "sensors": {"ppv": {"value": 3200, "unit": "W", ...}},
    "timestamp": "2026-06-14T20:00:00Z"
  }
  ```
- Dashboard auto-refreshes this endpoint every 5 s in Live mode.

---

### 4.6 Purge endpoints not implemented

**File:** `pkg/api/handler.go`

Data-management endpoints are planned but absent:
```
DELETE /api/data/{sensor}        purge samples older than ?before=<date>
POST   /api/data/{sensor}/purge  alternative JSON-body form
```

**Action:** Implement both routes, delegating to `PurgeRawSamples` in the store.

---

## Section 5 — Model Detection (Missing Capabilities)

### 5.1 Inverter capability detection from serial number not implemented

**File:** `et/et.go`, `discovery/discover.go`

The Python reference library derives capabilities from the serial number string.
None of these checks are ported:

| Function | Purpose |
|----------|---------|
| `is_single_phase()` / `is_3_phase()` | Filter L2/L3 sensors for single-phase models |
| `is_4_mppt()` | Filter PV3/PV4 sensors for 2-MPPT models |
| `is_2_battery()` | Conditionally enable battery2 block |
| `is_745_platform()` / `is_753_platform()` | Firmware-version-specific register layouts |

**Action:** Add a `Capabilities` struct derived from serial + rated_power in
`et/et.go`; use it in `GetSensors` to gate optional read blocks and filter
irrelevant sensor entries.

---

### 5.2 `SensorKind` / category not exposed on sensors

**File:** `et/registry.go`, `goodwe.go`

Sensors have no machine-readable category tag. The API and dashboard work around
this by embedding category info in the JS, which is fragile.

**Action:**
- Add `SensorKind` type (PV, AC, UPS, BAT, GRID, BMS, MPPT, METER).
- Add `Kind SensorKind` field to `sensorDefinition`.
- Expose `Kind` (as a string) in `SensorValue` and the `/api/sensors` response.
- Add `GetSensorsByKind(kind SensorKind) map[string]SensorValue` helper.

---

### 5.3 Settings read/write not implemented

**File:** `et/et.go`

The Python reference library has a full `__all_settings` dictionary with
`read_setting` / `write_setting` for inverter configuration (charge power,
battery mode, etc.). Nothing is ported.

**Action:**
- Define the settings dictionary (matching Python's `__all_settings`).
- Implement `ReadSetting(ctx, name string) (any, error)`.
- Implement `WriteSetting(ctx, name string, value any) error` with Modbus FC06/FC16.
- Handle ARM FW 19 vs 22 register layout variants.

---

## Section 6 — Testing Gaps

### 6.1 `parseProbeResponse` has no edge-case tests

**File:** `discovery/discover.go:97`

`parseProbeResponse` handles `@busy` responses and edge cases in the probe
string format, but there are no unit tests covering these paths.

**Action:** Add table-driven tests for `@busy`, missing fields, malformed
response strings, and the DTLS vs TCP detection branch.

---

### 6.2 `ReadSensor` meter fallback path has no test

**File:** `et/et.go`

`ReadSensor` for meter sensors retries with progressively shorter register
windows (125 → 58 → 45). This fallback is tested implicitly by the integration
path but has no isolated unit test.

**Action:** Add a unit test using a mock transport that fails the first two
reads with `ILLEGAL_DATA_ADDRESS` to cover the fallback chain.

---

### 6.3 No integration test harness

**File:** (new)

There is no facility for replaying real inverter sessions or using a mock
inverter for end-to-end tests. Protocol regressions can only be caught manually.

**Action:** Create an integration test harness (e.g. a mock UDP/DTLS server
that replays captured hex frames) wired into `go test -tags=integration`.

---

### 6.4 Public API has no Go doc comments

**File:** `goodwe.go`, `et/et.go`, `discovery/discover.go`

Exported types and functions (`Inverter`, `GetSensors`, `GetInfo`, `Discover`,
`SensorValue`, `Info`, `ErrUnsupported`) have no Go doc comments, making the
package hard to use from godoc or IDE hover.

**Action:** Add doc comments to all exported symbols.

---

## Section 7 — Documentation

### 7.1 README missing API usage examples

**File:** `README.md`

The daemon REST API is described in `TODO.md` but `README.md` contains no `curl`
examples for any endpoint.

**Action:** Add a "REST API" section to README with `curl` examples for:
`/api/health`, `/api/info`, `/api/sensors`, `/api/data/{sensor}`,
`/api/data/{sensor}/aggregate`.

---

### 7.2 README missing database management section

**File:** `README.md`

There is no documentation on:
- How to back up the SQLite database (`sqlite3 .backup` / file copy)
- How to use the `-purge` flag
- DSN format and `~` expansion
- WAL mode implications

**Action:** Add a "Database Management" section to README.

---

## Section 8 — Long-term / New Feature

### 8.1 MQTT integration for Home Assistant

**File:** (new — `pkg/mqtt/`)

Forward sensor values to an MQTT broker for Home Assistant auto-discovery.

**Design summary:**
- New flag: `-mqtt-broker tcp://host:1883`
- Additional flags: `-mqtt-user`, `-mqtt-pass`, `-mqtt-topic-prefix`
- Publish each sensor after every poll tick as JSON to `goodwe/<serial>/sensor/<name>`
- Publish HA discovery config to `homeassistant/sensor/goodwe_<serial>_<name>/config`
- Non-blocking: MQTT unavailability must not stall the poll loop
- Dependency: `github.com/eclipse/paho.mqtt.golang`

**Implementation steps:**
1. Add `eclipse/paho.mqtt.golang` to `go.mod`
2. Create `pkg/mqtt/publisher.go` — connects, publishes samples
3. Create `pkg/mqtt/discovery.go` — HA MQTT discovery payloads
4. Wire into `daemon.pollOnce()` after `InsertSample`
5. Add CLI flags to `cmd/goodwe-daemon/main.go`
6. Update README
