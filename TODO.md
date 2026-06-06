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

### Device Info
- [x] Read device info block (register 35000, 33 regs) for real model/firmware/serial/rated_power
- [x] `decodeGoodweString()` helper for UTF-16BE/ASCII auto-detection
- [x] `GetInfo()` returns populated `RatedPower`, `DSPVersion`, `ARMVersion` fields

---

## 📋 Short-term — Easy Wins

Data already within existing read windows or small isolated changes.

### MPPT Block: Missing Sensors (data in 61-reg window, just need registry entries)
- [ ] Add `pmppt1`..`pmppt8` (MPPT1-8 power, regs 35337-35344) — `uint32Reader(36)`..`uint32Reader(43)`
- [ ] Add `imppt1`..`imppt8` (MPPT1-8 current, regs 35345-35352) — `uint16Reader(44, 0.1)`..`uint16Reader(51, 0.1)`
- [ ] Add `reactive_power1`..`reactive_power3` (regs 35353-35357) — `int16Reader`
- [ ] Add `apparent_power1`..`apparent_power3` (regs 35359-35363) — `int16Reader`

### AA55 Pre-Header — Replace Magic Offsets with Named Constants
- [ ] Define `aa55HeaderLen = 2` constant
- [ ] Detect AA55 presence rather than assuming it (uncommon but some inverters omit it)
- [ ] Remove `// TODO better handle fixed 0xaa 0x55 header` comment
- [ ] Replace `responseBytes[2:n-2]` with named offset constant

### Typed Modbus Error Handling
- [ ] Define sentinel errors: `ErrIllegalDataAddress`, `ErrModbusCRC`, `ErrModbusException`
- [ ] Return typed errors from `parseModbusBulkResponse()`
- [ ] Replace `strings.Contains(err.Error(), "exception 0x02")` with `errors.Is(err, ErrIllegalDataAddress)`

---

## 🎯 Medium-term — Meaningful Features

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
- [ ] Add cleartext Modbus TCP transport (alternative to UDP+DTLS)
- [ ] Auto-detect: try DTLS discovery first, fall back to port 502

---

## 🛠 Robustness & Polish

- [ ] Add tests for `parseProbeResponse` with `@busy` and edge cases
- [ ] Add tests for `ReadSensor` with meter fallback (45/58/125)
- [ ] Add integration test harness (mock inverter or recorded sessions)
- [ ] Document public API in Go doc comments
- [ ] `int16Reader` does not check for `undef16` (0xFFFF) — consider handling it
- [ ] `grid_in_out` / `grid_in_out_label` read hardcoded offset 80 which overlaps with `vbattery1` index — extract as named constant
