// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Maciej Borzecki <maciej.borzecki@gmail.com>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
// 3. Neither the name of the copyright holder nor the names of its contributors
//    may be used to endorse or promote products derived from this software
//    without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package et

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadSampleHex(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err, "failed to read testdata/%s", name)
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	require.NoError(t, err, "failed to decode hex")
	return b
}

func parseSampleData(t *testing.T, name string) []byte {
	t.Helper()
	raw := loadSampleHex(t, name)
	data, err := parseModbusBulkResponse(raw)
	require.NoError(t, err, "parseModbusBulkResponse(%s)", name)
	require.Len(t, data, 250, "expected 250 bytes (125 registers)")
	return data
}

func reg16(t *testing.T, data []byte, idx int) uint16 {
	t.Helper()
	return binary.BigEndian.Uint16(data[idx*2 : idx*2+2])
}

func TestParseModbusBulkResponse_GW10K(t *testing.T) {
	data := parseSampleData(t, "GW10K-ET_running_data.hex")
	tests := []struct {
		index int
		want  uint16
		name  string
	}{
		{0, 0x1508, "timestamp[0]"},
		{3, 0x0CFE, "vpv1 (35103)"},
		{4, 0x0033, "ipv1 (35104)"},
		{7, 0x0CFE, "vpv2 (35107)"},
		{8, 0x0035, "ipv2 (35108)"},
		{10, 0x06E1, "ppv2 high (35110)"},
		{21, 0x0959, "vgrid (35121)"},
		{25, 0x0150, "pgrid (35125)"},
		{36, 0x0001, "grid_mode (35136)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, reg16(t, data, tt.index), "index %d", tt.index)
		})
	}
}

func TestParseModbusBulkResponse_GW20K(t *testing.T) {
	data := parseSampleData(t, "GW20K-ET_running_data.hex")
	tests := []struct {
		index int
		want  uint16
		name  string
	}{
		{0, 0x1A03, "timestamp[0]"},
		{3, 0x192B, "vpv1 (35103)"},
		{4, 0x0011, "ipv1 (35104)"},
		{6, 0x044A, "ppv1.high (35106)"},
		{7, 0x192B, "vpv2 (35107)"},
		{10, 0x03E5, "ppv2 high (35110)"},
		{11, 0x163D, "vpv3 (35111)"},
		{15, 0x163D, "vpv4 (35115)"},
		{25, 0x028B, "pgrid (35125)"},
		{36, 0x0001, "grid_mode (35136)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, reg16(t, data, tt.index), "index %d", tt.index)
		})
	}
}

func sensorFloat(t *testing.T, data []byte, name string) float64 {
	t.Helper()
	sv := registry[name].Calculator(data)
	f, ok := sv.(float64)
	require.True(t, ok, "sensor %q should return float64, got %T", name, sv)
	return f
}

func sensorString(t *testing.T, data []byte, name string) string {
	t.Helper()
	sv := registry[name].Calculator(data)
	s, ok := sv.(string)
	require.True(t, ok, "sensor %q should return string, got %T", name, sv)
	return s
}

func houseConsumptionValue(t *testing.T, data []byte) float64 {
	t.Helper()
	return sensorFloat(t, data, "house_consumption")
}

func TestHouseConsumption_GW10K(t *testing.T) {
	data := parseSampleData(t, "GW10K-ET_running_data.hex")
	assert.Equal(t, 947.0, houseConsumptionValue(t, data))
}

func TestHouseConsumptionComponents_GW10K(t *testing.T) {
	data := parseSampleData(t, "GW10K-ET_running_data.hex")

	ppv1 := readUint32(data, 5)
	if ppv1 == undef32 {
		ppv1 = 0
	}
	assert.EqualValues(t, 1695, ppv1, "ppv1")

	ppv2 := readUint32(data, 9)
	if ppv2 == undef32 {
		ppv2 = 0
	}
	assert.EqualValues(t, 1761, ppv2, "ppv2")

	ppv3 := readUint32(data, 13)
	if ppv3 == undef32 {
		ppv3 = 0
	}
	assert.EqualValues(t, 0, ppv3, "ppv3 (undefined)")

	ppv4 := readUint32(data, 17)
	if ppv4 == undef32 {
		ppv4 = 0
	}
	assert.EqualValues(t, 0, ppv4, "ppv4 (undefined)")

	assert.Equal(t, int32(-2512), int32(readUint32(data, 82)), "pbattery1")
	assert.Equal(t, int16(-3), int16(binary.BigEndian.Uint16(data[80:82])), "active_power")
}

func TestHouseConsumption_GW20K(t *testing.T) {
	data := parseSampleData(t, "GW20K-ET_running_data.hex")
	assert.Equal(t, 386.0, houseConsumptionValue(t, data))
}

func TestHouseConsumptionComponents_GW20K(t *testing.T) {
	data := parseSampleData(t, "GW20K-ET_running_data.hex")

	ppv1 := readUint32(data, 5)
	if ppv1 == undef32 {
		ppv1 = 0
	}
	assert.EqualValues(t, 1098, ppv1, "ppv1")

	ppv2 := readUint32(data, 9)
	if ppv2 == undef32 {
		ppv2 = 0
	}
	assert.EqualValues(t, 997, ppv2, "ppv2")

	ppv3 := readUint32(data, 13)
	if ppv3 == undef32 {
		ppv3 = 0
	}
	assert.EqualValues(t, 0, ppv3, "ppv3")

	ppv4 := readUint32(data, 17)
	if ppv4 == undef32 {
		ppv4 = 0
	}
	assert.EqualValues(t, 0, ppv4, "ppv4")

	assert.Equal(t, int32(-153), int32(readUint32(data, 82)), "pbattery1")
	assert.Equal(t, int16(1556), int16(binary.BigEndian.Uint16(data[80:82])), "active_power")
}

func TestSensorValues_GW10K(t *testing.T) {
	data := parseSampleData(t, "GW10K-ET_running_data.hex")

	actual := []struct {
		name string
		want float64
	}{
		{"vpv1", 332.6},
		{"ipv1", 5.1},
		{"ppv1", 1695},
		{"vpv2", 332.6},
		{"ipv2", 5.3},
		{"ppv2", 1761},
		{"ppv", 3456},
		{"vgrid", 239.3},
		{"igrid", 1.5},
		{"fgrid", 49.99},
		{"pgrid", 336},
		{"vgrid2", 241.5},
		{"igrid2", 1.3},
		{"fgrid2", 49.99},
		{"pgrid2", 287},
		{"vgrid3", 241.1},
		{"igrid3", 1.1},
		{"fgrid3", 49.99},
		{"pgrid3", 206},
		{"grid_mode", 1},
		{"total_inverter_power", 831},
		{"active_power", -3},
		{"reactive_power", 0},
		{"apparent_power", 0},
		{"backup_v1", 239.0},
		{"backup_i1", 0.6},
		{"backup_f1", 49.98},
		{"load_mode1", 1},
		{"backup_p1", 107},
		{"backup_v2", 241.3},
		{"backup_i2", 0.9},
		{"backup_f2", 50.0},
		{"load_mode2", 1},
		{"backup_p2", 189},
		{"backup_v3", 241.2},
		{"backup_i3", 0.2},
		{"backup_f3", 49.99},
		{"load_mode3", 1},
		{"backup_p3", 0},
		{"load_p1", 224},
		{"load_p2", 80},
		{"load_p3", 233},
		{"backup_ptotal", 312},
		{"load_ptotal", 522},
		{"ups_load", 4},
		{"temperature_air", 51.0},
		{"temperature_module", 0},
		{"temperature", 58.7},
		{"function_bit", 0},
		{"bus_voltage", 803.6},
		{"nbus_voltage", 401.8},
		{"vbattery1", 254.2},
		{"ibattery1", -9.8},
		{"pbattery1", -2512},
		{"battery_mode", 3},
		{"warning_code", 0},
		{"safety_country", 32},
		{"work_mode", 1},
		{"operation_mode", 0},
		{"error_codes", 0},
		{"e_total", 6085.3},
		{"e_day", 12.5},
		{"e_total_exp", 4718.6},
		{"h_total", 9246},
		{"e_day_exp", 9.8},
		{"e_total_imp", 58.0},
		{"e_day_imp", 0},
		{"e_load_total", 8820.2},
		{"e_load_day", 11.6},
		{"e_bat_charge_total", 2758.1},
		{"e_bat_charge_day", 5.3},
		{"e_bat_discharge_total", 2442.1},
		{"e_bat_discharge_day", 2.9},
		{"diagnose_result", 117442560},
	}
	for _, tc := range actual {
		t.Run(tc.name, func(t *testing.T) {
			got := sensorFloat(t, data, tc.name)
			assert.InDelta(t, tc.want, got, 0.01, "%s", tc.name)
		})
	}
}

func TestSensorValues_GW20K(t *testing.T) {
	data := parseSampleData(t, "GW20K-ET_running_data.hex")

	actual := []struct {
		name string
		want float64
	}{
		{"vpv1", 644.3},
		{"ipv1", 1.7},
		{"ppv1", 1098},
		{"vpv2", 644.3},
		{"ipv2", 0},
		{"ppv2", 997},
		{"vpv3", 569.3},
		{"ipv3", 1.8},
		{"ppv3", 0},
		{"vpv4", 569.3},
		{"ipv4", 0},
		{"ppv4", 0},
		{"ppv", 2095},
		{"vgrid", 233.8},
		{"igrid", 2.9},
		{"fgrid", 49.99},
		{"pgrid", 651},
		{"vgrid2", 233.7},
		{"igrid2", 3.0},
		{"fgrid2", 50.0},
		{"pgrid2", 652},
		{"vgrid3", 234.7},
		{"igrid3", 3.0},
		{"fgrid3", 49.99},
		{"pgrid3", 663},
		{"grid_mode", 1},
		{"total_inverter_power", 1966},
		{"active_power", 1556},
		{"reactive_power", 827},
		{"apparent_power", 2083},
		{"backup_v1", 233.2},
		{"backup_i1", 1.7},
		{"backup_f1", 49.99},
		{"load_mode1", 0},
		{"backup_p1", 235},
		{"backup_v2", 233.7},
		{"backup_i2", 1.2},
		{"backup_f2", 50.0},
		{"load_mode2", 0},
		{"backup_p2", 140},
		{"backup_v3", 234.1},
		{"backup_i3", 0.4},
		{"backup_f3", 49.99},
		{"load_mode3", 0},
		{"backup_p3", 91},
		{"load_p1", 221},
		{"load_p2", 125},
		{"load_p3", 65},
		{"backup_ptotal", 484},
		{"load_ptotal", 0},
		{"ups_load", 4},
		{"temperature_air", 43.9},
		{"temperature_module", 0},
		{"temperature", 42.2},
		{"function_bit", 1},
		{"bus_voltage", 762.5},
		{"nbus_voltage", 381.3},
		{"vbattery1", 381.6},
		{"ibattery1", -0.4},
		{"pbattery1", -153},
		{"battery_mode", 3},
		{"warning_code", 0},
		{"safety_country", 2},
		{"work_mode", 1},
		{"operation_mode", 0},
		{"error_codes", 0},
		{"e_total", 50.1},
		{"e_day", 49.9},
		{"e_total_exp", 331.6},
		{"h_total", 493},
		{"e_day_exp", 40.8},
		{"e_total_imp", 392.0},
		{"e_day_imp", 0},
		{"e_load_total", 285.6},
		{"e_load_day", 0.2},
		{"e_bat_charge_total", 381.7},
		{"e_bat_charge_day", 10.4},
		{"e_bat_discharge_total", 303.6},
		{"e_bat_discharge_day", 2.8},
		{"diagnose_result", 318767360},
	}
	for _, tc := range actual {
		t.Run(tc.name, func(t *testing.T) {
			got := sensorFloat(t, data, tc.name)
			assert.InDelta(t, tc.want, got, 0.01, "%s", tc.name)
		})
	}
}

func TestTimestamp_GW10K(t *testing.T) {
	data := parseSampleData(t, "GW10K-ET_running_data.hex")
	v := registry["timestamp"].Calculator(data)
	ts, ok := v.(time.Time)
	require.True(t, ok, "timestamp should return time.Time, got %T", v)
	expected := time.Date(2021, time.August, 22, 11, 11, 12, 0, time.Local)
	assert.Equal(t, expected, ts)
}

func TestTimestamp_GW20K(t *testing.T) {
	data := parseSampleData(t, "GW20K-ET_running_data.hex")
	v := registry["timestamp"].Calculator(data)
	ts, ok := v.(time.Time)
	require.True(t, ok, "timestamp should return time.Time, got %T", v)
	expected := time.Date(2026, time.March, 29, 17, 26, 45, 0, time.Local)
	assert.Equal(t, expected, ts)
}

func TestPVMode_GW10K(t *testing.T) {
	data := parseSampleData(t, "GW10K-ET_running_data.hex")
	assert.Equal(t, float64(0), sensorFloat(t, data, "pv4_mode"))
	assert.Equal(t, float64(0), sensorFloat(t, data, "pv3_mode"))
	assert.Equal(t, float64(2), sensorFloat(t, data, "pv2_mode"))
	assert.Equal(t, float64(2), sensorFloat(t, data, "pv1_mode"))
}

func TestGridInOut_GW10K(t *testing.T) {
	data := parseSampleData(t, "GW10K-ET_running_data.hex")
	assert.Equal(t, float64(0), sensorFloat(t, data, "grid_in_out"))
}

func TestGridInOut_GW20K(t *testing.T) {
	data := parseSampleData(t, "GW20K-ET_running_data.hex")
	assert.Equal(t, float64(1), sensorFloat(t, data, "grid_in_out"))
}

func TestLabelSensors_GW10K(t *testing.T) {
	data := parseSampleData(t, "GW10K-ET_running_data.hex")
	tests := []struct {
		name string
		want string
	}{
		{"pv1_mode_label", "PV panels connected, producing power"},
		{"pv2_mode_label", "PV panels connected, producing power"},
		{"grid_mode_label", "Connected to grid"},
		{"grid_in_out_label", "Idle"},
		{"battery_mode_label", "Charge"},
		{"safety_country_label", "50Hz 230Vac Default"},
		{"work_mode_label", "Normal (On-Grid)"},
		{"errors", ""},
		{"diagnose_result_label", "Self-use load light, Export power limit set, PF value set, Real power limit set"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sensorString(t, data, tc.name))
		})
	}
}

func TestLabelSensors_GW20K(t *testing.T) {
	data := parseSampleData(t, "GW20K-ET_running_data.hex")
	tests := []struct {
		name string
		want string
	}{
		{"pv1_mode_label", "PV panels connected, producing power"},
		{"pv2_mode_label", "PV panels connected, producing power"},
		{"pv3_mode_label", "PV panels not connected"},
		{"pv4_mode_label", "PV panels not connected"},
		{"grid_mode_label", "Connected to grid"},
		{"grid_in_out_label", "Exporting"},
		{"battery_mode_label", "Charge"},
		{"safety_country_label", "DE LV with PV"},
		{"work_mode_label", "Normal (On-Grid)"},
		{"errors", ""},
		{"diagnose_result_label", "APP: Discharge current too low, Export power limit set, PF value set, SOC protect off"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sensorString(t, data, tc.name))
		})
	}
}

func loadBatteryHex(t *testing.T, name string) []byte {
	t.Helper()
	raw := loadSampleHex(t, name)
	data, err := parseModbusBulkResponse(raw)
	require.NoError(t, err, "parseModbusBulkResponse(%s)", name)
	require.Len(t, data, 48, "expected 48 bytes (24 registers) for battery")
	return data
}

type batteryTest struct {
	name string
	want float64
	unit string
}

type batteryStringTest struct {
	name string
	want string
}

func testBatteryValues(t *testing.T, data []byte, floatTests []batteryTest, stringTests []batteryStringTest) {
	t.Helper()
	for _, tc := range floatTests {
		t.Run(tc.name, func(t *testing.T) {
			def, ok := batteryRegistry[tc.name]
			require.True(t, ok, "battery sensor %q not found", tc.name)
			sv := def.Calculator(data)
			f, ok := sv.(float64)
			require.True(t, ok, "battery sensor %q should return float64, got %T", tc.name, sv)
			assert.InDelta(t, tc.want, f, 0.01, "%s", tc.name)
		})
	}
	for _, tc := range stringTests {
		t.Run(tc.name, func(t *testing.T) {
			def, ok := batteryRegistry[tc.name]
			require.True(t, ok, "battery sensor %q not found", tc.name)
			sv := def.Calculator(data)
			s, ok := sv.(string)
			require.True(t, ok, "battery sensor %q should return string, got %T", tc.name, sv)
			assert.Equal(t, tc.want, s, "%s", tc.name)
		})
	}
}

func TestBatterySensors_GW10K(t *testing.T) {
	data := loadBatteryHex(t, "GW10K-ET_battery_info.hex")
	testBatteryValues(t, data,
		[]batteryTest{
			{"battery_bms", 255, ""},
			{"battery_index", 256, ""},
			{"battery_status", 1, ""},
			{"battery_temperature", 35.0, "C"},
			{"battery_charge_limit", 25, "A"},
			{"battery_discharge_limit", 25, "A"},
			{"battery_error_l", 0, ""},
			{"battery_soc", 68, "%"},
			{"battery_soh", 99, "%"},
			{"battery_modules", 5, ""},
			{"battery_warning_l", 0, ""},
			{"battery_protocol", 257, ""},
			{"battery_error_h", 0, ""},
			{"battery_warning_h", 0, ""},
			{"battery_sw_version", 0, ""},
			{"battery_hw_version", 0, ""},
			{"battery_max_cell_temp_id", 0, ""},
			{"battery_min_cell_temp_id", 0, ""},
			{"battery_max_cell_voltage_id", 0, ""},
			{"battery_min_cell_voltage_id", 0, ""},
			{"battery_max_cell_temp", 0, "C"},
			{"battery_min_cell_temp", 0, "C"},
			{"battery_max_cell_voltage", 0, "V"},
			{"battery_min_cell_voltage", 0, "V"},
		},
		[]batteryStringTest{
			{"battery_error", ""},
			{"battery_warning", ""},
		},
	)
}

func TestBatterySensors_GW20K(t *testing.T) {
	data := loadBatteryHex(t, "GW20K-ET_battery_info.hex")
	testBatteryValues(t, data,
		[]batteryTest{
			{"battery_bms", 1, ""},
			{"battery_index", 498, ""},
			{"battery_status", 1, ""},
			{"battery_temperature", 14.1, "C"},
			{"battery_charge_limit", 0, "A"},
			{"battery_discharge_limit", 30, "A"},
			{"battery_error_l", 0, ""},
			{"battery_soc", 100, "%"},
			{"battery_soh", 100, "%"},
			{"battery_modules", 7, ""},
			{"battery_warning_l", 0, ""},
			{"battery_protocol", 286, ""},
			{"battery_error_h", 0, ""},
			{"battery_warning_h", 0, ""},
			{"battery_sw_version", 2502, ""},
			{"battery_hw_version", 2752, ""},
			{"battery_max_cell_temp_id", 0, ""},
			{"battery_min_cell_temp_id", 0, ""},
			{"battery_max_cell_voltage_id", 0, ""},
			{"battery_min_cell_voltage_id", 0, ""},
			{"battery_max_cell_temp", 14.1, "C"},
			{"battery_min_cell_temp", 9.0, "C"},
			{"battery_max_cell_voltage", 4.0, "V"},
			{"battery_min_cell_voltage", 3.961, "V"},
		},
		[]batteryStringTest{
			{"battery_error", ""},
			{"battery_warning", ""},
		},
	)
}

func loadMeterHex(t *testing.T, name string) []byte {
	t.Helper()
	raw := loadSampleHex(t, name)
	data, err := parseModbusBulkResponse(raw)
	require.NoError(t, err, "parseModbusBulkResponse(%s)", name)
	return data
}

type meterFloatTest struct {
	name string
	want float64
}

type meterStringTest struct {
	name string
	want string
}

func testMeterValues(t *testing.T, data []byte, floatTests []meterFloatTest, stringTests []meterStringTest) {
	t.Helper()
	for _, tc := range floatTests {
		t.Run(tc.name, func(t *testing.T) {
			def, ok := meterRegistry[tc.name]
			require.True(t, ok, "meter sensor %q not found", tc.name)
			sv := def.Calculator(data)
			f, ok := sv.(float64)
			require.True(t, ok, "meter sensor %q should return float64, got %T", tc.name, sv)
			assert.InDelta(t, tc.want, f, 0.01, "%s", tc.name)
		})
	}
	for _, tc := range stringTests {
		t.Run(tc.name, func(t *testing.T) {
			def, ok := meterRegistry[tc.name]
			require.True(t, ok, "meter sensor %q not found", tc.name)
			sv := def.Calculator(data)
			s, ok := sv.(string)
			require.True(t, ok, "meter sensor %q should return string, got %T", tc.name, sv)
			assert.Equal(t, tc.want, s, "%s", tc.name)
		})
	}
}

func TestMeterSensors_GW10K(t *testing.T) {
	data := loadMeterHex(t, "GW10K-ET_meter_data.hex")
	testMeterValues(t, data,
		[]meterFloatTest{
			{"commode", 1},
			{"rssi", 35},
			{"meter_test_status", 0},
			{"meter_comm_status", 1},
			{"active_power1", -57},
			{"active_power2", -46},
			{"active_power3", -6},
			{"active_power_total", -110},
			{"reactive_power_total", 1336},
			{"meter_power_factor1", -0.145},
			{"meter_power_factor2", -0.124},
			{"meter_power_factor3", -0.014},
			{"meter_power_factor", -0.08},
			{"meter_freq", 50.05},
			{"meter_e_total_exp", 10.514},
			{"meter_e_total_imp", 3254.462},
			{"meter_active_power1", -57},
			{"meter_active_power2", -46},
			{"meter_active_power3", -6},
			{"meter_active_power_total", -110},
			{"meter_reactive_power1", 364},
			{"meter_reactive_power2", 357},
			{"meter_reactive_power3", 614},
			{"meter_reactive_power_total", 1336},
			{"meter_apparent_power1", -402},
			{"meter_apparent_power2", -372},
			{"meter_apparent_power3", -627},
			{"meter_apparent_power_total", -1403},
			{"meter_type", 1},
			{"meter_sw_version", 3},
		},
		nil,
	)
}

func TestMeterSensors_GW20K(t *testing.T) {
	data := loadMeterHex(t, "GW20K-ET_meter_data.hex")
	testMeterValues(t, data,
		[]meterFloatTest{
			{"commode", 7},
			{"rssi", 0},
			{"meter_test_status", 273},
			{"meter_comm_status", 1},
			{"active_power1", 430},
			{"active_power2", 527},
			{"active_power3", 598},
			{"active_power_total", 1556},
			{"reactive_power_total", 82},
			{"meter_power_factor1", 0.084},
			{"meter_power_factor2", 0.09},
			{"meter_power_factor3", 0.091},
			{"meter_power_factor", 0.089},
			{"meter_freq", 50.0},
			{"meter_active_power1", 430},
			{"meter_active_power2", 527},
			{"meter_active_power3", 598},
			{"meter_active_power_total", 1556},
			{"meter_reactive_power1", 102},
			{"meter_reactive_power2", 19},
			{"meter_reactive_power3", -39},
			{"meter_reactive_power_total", 82},
			{"meter_apparent_power1", 506},
			{"meter_apparent_power2", 581},
			{"meter_apparent_power3", 652},
			{"meter_apparent_power_total", 1740},
			{"meter_type", 2},
			{"meter_sw_version", 5},
			{"meter2_active_power", 0},
			{"meter2_e_total_exp", 0.0},
			{"meter2_e_total_imp", 0.0},
			{"meter2_comm_status", 0},
			{"meter_voltage1", 233.8},
			{"meter_voltage2", 233.9},
			{"meter_voltage3", 233.7},
			{"meter_current1", 2.1},
			{"meter_current2", 2.4},
			{"meter_current3", 2.8},
			{"meter_e_total_exp1", 10.72},
			{"meter_e_total_exp2", 12.61},
			{"meter_e_total_exp3", 13.9},
			{"meter_e_total_exp_8", 35.55},
			{"meter_e_total_imp1", 1.33},
			{"meter_e_total_imp2", 0.49},
			{"meter_e_total_imp3", 0.02},
			{"meter_e_total_imp_8", 0.15},
		},
		nil,
	)
}

func loadMpptHex(t *testing.T, name string) []byte {
	t.Helper()
	raw := loadSampleHex(t, name)
	data, err := parseModbusBulkResponse(raw)
	require.NoError(t, err, "parseModbusBulkResponse(%s)", name)
	return data
}

func TestMpptSensors_GW20K(t *testing.T) {
	data := loadMpptHex(t, "GW20K-ET_mppt_data.hex")
	tests := []struct {
		name string
		want float64
	}{
		{"ppv_total", 2095},
		{"pv_channel", 2},
		{"vpv5", 0},
		{"ipv5", 0},
		{"vpv6", 0},
		{"ipv6", 0},
		{"vpv7", 0},
		{"ipv7", 0},
		{"vpv8", 0},
		{"ipv8", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def, ok := mpptRegistry[tc.name]
			require.True(t, ok, "mppt sensor %q not found", tc.name)
			sv := def.Calculator(data)
			f, ok := sv.(float64)
			require.True(t, ok, "mppt sensor %q should return float64, got %T", tc.name, sv)
			assert.InDelta(t, tc.want, f, 0.01, "%s", tc.name)
		})
	}
}

// deviceInfoTest represents expected device info fields from the Python reference.
type deviceInfoTest struct {
	hexFile      string
	modelName    string
	serialNumber string
	ratedPower   int
	modbusVer    int
	acOutputType int
	dsp1         int
	dsp2         int
	arm          int
	firmware     string
	armFirmware  string
}

func parseDeviceInfoData(t *testing.T, name string) []byte {
	t.Helper()
	raw := loadSampleHex(t, name)
	data, err := parseModbusBulkResponse(raw)
	require.NoError(t, err, "parseModbusBulkResponse(%s)", name)
	require.Len(t, data, 66, "expected 66 bytes (33 registers)")
	return data
}

func TestDeviceInfoFromPythonSamples(t *testing.T) {
	// Test data and expected values from the Python reference library
	// https://github.com/marcelblijleven/goodwe (MIT License)
	tests := []deviceInfoTest{
		{
			hexFile:      "GW10K-ET_device_info_fw617.hex",
			modelName:    "GW10K-ET",
			serialNumber: "9010KETU000W0000",
			ratedPower:   10000,
			modbusVer:    1,
			acOutputType: 254,
			dsp1:         6, dsp2: 6, arm: 17,
			firmware:    "04029-06-S11",
			armFirmware: "02041-17-S00",
		},
		{
			hexFile:      "GW10K-ET_device_info_fw819.hex",
			modelName:    "0GW10K-ET",
			serialNumber: "9010KETU00000000",
			ratedPower:   10000,
			modbusVer:    1,
			acOutputType: 254,
			dsp1:         8, dsp2: 8, arm: 19,
			firmware:    "04029-08-S11",
			armFirmware: "02041-19-S00",
		},
		{
			hexFile:      "GW10K-ET_device_info_fw1023.hex",
			modelName:    "GW10K-ET",
			serialNumber: "9010KETU000W0000",
			ratedPower:   10000,
			modbusVer:    2,
			acOutputType: 254,
			dsp1:         10, dsp2: 10, arm: 23,
			firmware:    "04029-10-S11",
			armFirmware: "02041-23-S00",
		},
		{
			hexFile:      "GW20K-ET_device_info.hex",
			modelName:    "",
			serialNumber: "9020KETT232W0000",
			ratedPower:   20000,
			modbusVer:    0,
			acOutputType: 1,
			dsp1:         6, dsp2: 6, arm: 8,
			firmware:    "04062-08-S00",
			armFirmware: "02020-10-S01",
		},
		{
			hexFile:      "GW25K-ET_device_info.hex",
			modelName:    "",
			serialNumber: "9025KETT00000000",
			ratedPower:   25000,
			modbusVer:    0,
			acOutputType: 1,
			dsp1:         6, dsp2: 6, arm: 8,
			firmware:    "04062-",
			armFirmware: "02020-08-S01",
		},
		{
			hexFile:      "GW29K9-ET_device_info.hex",
			modelName:    "",
			serialNumber: "929K9ETT00CW0000",
			ratedPower:   29900,
			modbusVer:    0,
			acOutputType: 1,
			dsp1:         2, dsp2: 2, arm: 3,
			firmware:    "04062-",
			armFirmware: "02020-03-S01",
		},
		{
			hexFile:      "GW6000_EH_device_info.hex",
			modelName:    "GW6000-EH",
			serialNumber: "00000EHU00000000",
			ratedPower:   6000,
			modbusVer:    0,
			acOutputType: 254,
			dsp1:         3, dsp2: 3, arm: 16,
			firmware:    "04034-03-S10",
			armFirmware: "02041-16-S00",
		},
		{
			hexFile:      "GW6000-ES-20_device_info.hex",
			modelName:    "GW6000ES20",
			serialNumber: "56000ESN00AW0000",
			ratedPower:   6050,
			modbusVer:    121,
			acOutputType: 0,
			dsp1:         2, dsp2: 2, arm: 5,
			firmware:    "ffffffffffffffffffffffff",
			armFirmware: "02020-05-S01",
		},
		{
			hexFile:      "GW5K-BT_device_info.hex",
			modelName:    "GW5K-BT",
			serialNumber: "95000BTU203W0000",
			ratedPower:   5000,
			modbusVer:    0,
			acOutputType: 254,
			dsp1:         3, dsp2: 3, arm: 11,
			firmware:    "04029-03-S10",
			armFirmware: "02041-11-S00",
		},
	}

	for _, tc := range tests {
		t.Run(tc.hexFile, func(t *testing.T) {
			data := parseDeviceInfoData(t, tc.hexFile)

			// Register-level fields
			assert.Equal(t, tc.modbusVer, int(binary.BigEndian.Uint16(data[0:2])), "modbus_version")
			assert.Equal(t, tc.ratedPower, int(binary.BigEndian.Uint16(data[2:4])), "rated_power")
			assert.Equal(t, tc.acOutputType, int(binary.BigEndian.Uint16(data[4:6])), "ac_output_type")

			// String fields
			assert.Equal(t, tc.serialNumber, decodeGoodweString(data[6:22]), "serial_number")
			assert.Equal(t, tc.modelName, decodeGoodweString(data[22:32]), "model_name")
			assert.Equal(t, tc.firmware, decodeGoodweString(data[42:54]), "firmware")
			assert.Equal(t, tc.armFirmware, decodeGoodweString(data[54:66]), "arm_firmware")

			// Version fields (uint16)
			assert.Equal(t, tc.dsp1, int(binary.BigEndian.Uint16(data[32:34])), "dsp1_version")
			assert.Equal(t, tc.dsp2, int(binary.BigEndian.Uint16(data[34:36])), "dsp2_version")
			assert.Equal(t, tc.arm, int(binary.BigEndian.Uint16(data[38:40])), "arm_version")
		})
	}
}

func TestDeviceInfoVersionFallback(t *testing.T) {
	// Synthetic test: simulate an inverter where the uint16 version fields are
	// zero but the firmware strings contain version information.
	// Pattern observed on Solar-5015KET (TCP inverter).
	data := make([]byte, 66)

	binary.BigEndian.PutUint16(data[2:4], 15000) // rated_power
	copy(data[6:22], []byte("5015KETT246L0592")) // serial
	copy(data[42:54], []byte("04062-07-S00"))    // firmware
	copy(data[54:66], []byte("02071-13-439"))    // arm_firmware

	// dsp1/dsp2/arm at offsets 32,34,38 are zero → triggers fallback

	model := decodeGoodweString(data[22:32])
	assert.Equal(t, "", model, "model_name should be empty")

	firmware := decodeGoodweString(data[42:54])
	assert.Equal(t, "04062-07-S00", firmware)

	armFirmware := decodeGoodweString(data[54:66])
	assert.Equal(t, "02071-13-439", armFirmware)

	// Verify fallback DSP parsing
	dsp1 := binary.BigEndian.Uint16(data[32:34])
	dsp2 := binary.BigEndian.Uint16(data[34:36])
	assert.Equal(t, uint16(0), dsp1)
	assert.Equal(t, uint16(0), dsp2)
	if dsp1 == 0 && dsp2 == 0 {
		parts := strings.SplitN(firmware, "-", 3)
		if len(parts) >= 2 {
			assert.Equal(t, "07", parts[1], "DSP from firmware fallback")
		}
	}

	// Verify fallback ARM parsing
	arm := binary.BigEndian.Uint16(data[38:40])
	assert.Equal(t, uint16(0), arm)
	if arm == 0 {
		parts := strings.SplitN(armFirmware, "-", 3)
		if len(parts) >= 2 {
			assert.Equal(t, "13", parts[1], "ARM from arm_firmware fallback")
		}
	}

	// Verify model derived from rated power
	ratedPower := int(binary.BigEndian.Uint16(data[2:4]))
	if model == "" && ratedPower > 0 {
		kw := ratedPower / 1000
		if kw > 0 {
			assert.Equal(t, "GW15K-ET", fmt.Sprintf("GW%dK-ET", kw), "model from rated_power")
		}
	}
}

func TestIsConnClosed(t *testing.T) {
	tests := []struct {
		err    error
		expect bool
	}{
		{nil, false},
		{fmt.Errorf("dtls fatal: conn is closed"), true},
		{fmt.Errorf("write: connection refused"), true},
		{fmt.Errorf("broken pipe"), true},
		{fmt.Errorf("read: connection reset by peer"), true},
		{fmt.Errorf("i/o timeout"), false},
		{fmt.Errorf("handshake error: read udp ...: read: connection refused"), true},
		{fmt.Errorf("not a connection error"), false},
	}

	for _, tc := range tests {
		got := isConnClosed(tc.err)
		assert.Equal(t, tc.expect, got, "isConnClosed(%v)", tc.err)
	}
}

func TestParseModbusTCPResponse(t *testing.T) {
	// Sample Modbus TCP response for reading 33 holding registers (0x35000, qty=33).
	// MBAP(7): 0x0002 0x0000 0x0045 0xF7
	// Followed by: Func(1) 0x03, ByteCount(1) 0x42, Data(66 bytes)
	hexResp := "000200000045f7034200003a980001353031354b4554543234364c3035393200000000000000000000000000001b9c000001b730343036322d30372d53303030323037312d31332d343339"
	data, err := hex.DecodeString(hexResp)
	require.NoError(t, err)

	result, err := parseModbusTCPResponse(data)
	require.NoError(t, err)
	require.Len(t, result, 66)

	// Check a few known bytes: rated_power low byte should be 0x3a98 = 15000
	assert.Equal(t, uint16(0x3a98), binary.BigEndian.Uint16(result[2:4]))
	// Check serial prefix
	assert.Equal(t, "5015KETT246L0592", strings.TrimRight(string(result[6:22]), "\x00"))
}

func TestParseModbusTCPResponseShort(t *testing.T) {
	_, err := parseModbusTCPResponse([]byte{0x00, 0x02})
	assert.ErrorContains(t, err, "too short")
}

func TestParseModbusTCPResponseException(t *testing.T) {
	// Modbus exception: MBAP(7) + Func(0x83) + ExceptionCode(0x02)
	hexResp := "000100000003f78302"
	data, err := hex.DecodeString(hexResp)
	require.NoError(t, err)

	_, err = parseModbusTCPResponse(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exception 0x02")
}

// mockTransport is a simple Transport implementation used in unit tests.
// Each call to ReadRegisters consumes the next entry from the responses slice.
type mockTransport struct {
	responses []mockResponse
	calls     []mockCall
}

type mockResponse struct {
	data []byte
	err  error
}

type mockCall struct {
	startReg uint16
	quantity uint16
}

func (m *mockTransport) Connect(_ context.Context) error { return nil }
func (m *mockTransport) Close() error                    { return nil }

func (m *mockTransport) ReadRegisters(_ context.Context, startReg, quantity uint16) ([]byte, error) {
	m.calls = append(m.calls, mockCall{startReg: startReg, quantity: quantity})
	if len(m.responses) == 0 {
		return nil, fmt.Errorf("mockTransport: no more responses")
	}
	r := m.responses[0]
	m.responses = m.responses[1:]
	return r.data, r.err
}

var errIllegalDataAddress = fmt.Errorf("modbus exception 0x02: ILLEGAL DATA ADDRESS")

// makeMeterData returns a zero-filled byte slice representing n registers of
// meter data, which satisfies the Calculator bounds check.
func makeMeterData(n int) []byte {
	return make([]byte, n*2)
}

// TestReadSensor_MeterFallbackChain verifies the 125→58→45 fallback path in
// ReadSensor for meter-block sensors.
func TestReadSensor_MeterFallbackChain(t *testing.T) {
	// Pick any sensor that lives in the meter block (readQty == 125).
	const sensorName = "commode"

	t.Run("fallback125to58to45", func(t *testing.T) {
		// First call (qty=125) → ILLEGAL_DATA_ADDRESS
		// Second call (qty=58) → ILLEGAL_DATA_ADDRESS
		// Third call (qty=45) → success
		mt := &mockTransport{
			responses: []mockResponse{
				{nil, errIllegalDataAddress}, // qty=125
				{nil, errIllegalDataAddress}, // qty=58
				{makeMeterData(45), nil},     // qty=45
			},
		}
		inv := NewWithTransport("SN001", mt)
		sv, err := inv.ReadSensor(context.Background(), sensorName)
		require.NoError(t, err)
		_ = sv

		require.Len(t, mt.calls, 3)
		assert.EqualValues(t, 125, mt.calls[0].quantity, "first attempt: qty=125")
		assert.EqualValues(t, 58, mt.calls[1].quantity, "second attempt: qty=58")
		assert.EqualValues(t, 45, mt.calls[2].quantity, "third attempt: qty=45")
		for _, c := range mt.calls {
			assert.EqualValues(t, 36000, c.startReg, "start register must be 36000")
		}
	})

	t.Run("fallback125to58success", func(t *testing.T) {
		// First call (qty=125) → ILLEGAL_DATA_ADDRESS
		// Second call (qty=58) → success
		mt := &mockTransport{
			responses: []mockResponse{
				{nil, errIllegalDataAddress}, // qty=125
				{makeMeterData(58), nil},     // qty=58
			},
		}
		inv := NewWithTransport("SN001", mt)
		sv, err := inv.ReadSensor(context.Background(), sensorName)
		require.NoError(t, err)
		_ = sv

		require.Len(t, mt.calls, 2)
		assert.EqualValues(t, 125, mt.calls[0].quantity)
		assert.EqualValues(t, 58, mt.calls[1].quantity)
	})

	t.Run("allFallbacksFail", func(t *testing.T) {
		// All three sizes fail with ILLEGAL_DATA_ADDRESS → ReadSensor returns error.
		mt := &mockTransport{
			responses: []mockResponse{
				{nil, errIllegalDataAddress}, // qty=125
				{nil, errIllegalDataAddress}, // qty=58
				{nil, errIllegalDataAddress}, // qty=45
			},
		}
		inv := NewWithTransport("SN001", mt)
		_, err := inv.ReadSensor(context.Background(), sensorName)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read sensor")

		require.Len(t, mt.calls, 3)
	})
}
