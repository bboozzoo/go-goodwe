# TODO

## Vision
Go implementation of a GoodWe inverter library with full sensor coverage matching the Python reference, clean typed API, and robust endian-safe encoding.

## Phase 1: Typed struct + Units + []byte Calculator
- [ ] Add `Unit string`, `Name string` to `sensorDefinition`
- [ ] Switch Calculator signature from `[]uint16` to `[]byte`
- [ ] Refactor all reader helpers to use `encoding/binary.BigEndian`
- [ ] Introduce typed return struct `SensorValue{Value any, Unit string, Name string}`
- [ ] Change `GetSensors` to return `map[string]SensorValue`
- [ ] Update `goodwe.go` interface
- [ ] Update CLI to handle new struct
- [ ] Update tests
- [ ] Endianness verification: all readers work correctly on big-endian hosts

## Phase 2: Label/Enum Sensors
- [ ] Port all const dicts from Python (`const.go`): PV_MODES, GRID_MODES, BATTERY_MODES, WORK_MODES_ET, GRID_IN_OUT_MODES, SAFETY_COUNTRIES, ERROR_CODES, DIAG_STATUS_CODES, BMS_ALARM_CODES, BMS_WARNING_CODES
- [ ] Add `enumReader`, `enumHReader`, `enumLReader`, `enum2Reader`, `enumBitmap4Reader`, `enumBitmap22Reader` helpers returning `string`
- [ ] Add label sensors to registry (`grid_mode_label`, `pv*_mode_label`, `battery_mode_label`, `work_mode_label`, `safety_country_label`, `errors`, `diagnose_result_label`, `grid_in_out_label`)
- [ ] Update tests with expected string values for both datasets

## Phase 3: Battery Block
- [ ] Add bulk read for register 37000, 24 registers
- [ ] Add battery sensor registry (SoC, SoH, temperatures, cell voltages, BMS errors/warnings)
- [ ] `GetSensors` performs two bulk reads and merges
- [ ] Fallback: skip battery if ILLEGAL_DATA_ADDRESS
- [ ] Add battery2 block (39000, 22 regs + 35262, 6 regs)
- [ ] Update tests with battery sample data

## Phase 4: Meter Block
- [ ] Add bulk read for register 36000 with fallback chain (125→58→45 regs)
- [ ] Add meter sensor registry
- [ ] Merge into GetSensors

## Phase 5: MPPT Block
- [ ] Add bulk read for register 35301, 61 regs
- [ ] Add MPPT sensor registry
- [ ] Merge into GetSensors
