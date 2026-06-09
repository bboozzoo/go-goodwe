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
- [ ] Set `SetReadDeadline(time.Now().Add(3 * time.Second))` on each `dtlsConn.Read()`
- [ ] On `os.ErrDeadlineExceeded`: retry the read (the inverter occasionally drops packets)
- [ ] Make timeout configurable on the service

### Poll Loop Reconnection
- [ ] CLI `-poll` loop: on `ReadSensor` error, attempt `reconnect()` and retry
- [ ] Add max consecutive failures before giving up entirely

---

## 🚀 Long-term — Major Features

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

### TCP Port 502 Support
- [x] Basic Modbus TCP transport via `tcpTransport` (MBAP framing, no CRC)
- [ ] Auto-detect DTLS vs TCP in discovery (done) and handle fallback between transports

---

## 🛠 Robustness & Polish

- [ ] Add tests for `parseProbeResponse` with `@busy` and edge cases
- [ ] Add tests for `ReadSensor` with meter fallback (45/58/125)
- [ ] Add integration test harness (mock inverter or recorded sessions)
- [ ] Document public API in Go doc comments
- [ ] `int16Reader` does not check for `undef16` (0xFFFF) — consider handling it
- [ ] `grid_in_out` / `grid_in_out_label` read hardcoded offset 80 which overlaps with `vbattery1` index — extract as named constant

---

## 📁 Code Map

| File | Purpose |
|---|---|---|
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
| `.goreleaser.yaml` | GoReleaser build config (linux/darwin, amd64/arm64) |
| `.github/workflows/ci.yml` | CI: test + lint + snapshot build on master push |
| `.github/workflows/release.yml` | Release: goreleaser draft on `v*.*.*` tag |
