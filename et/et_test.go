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
	"encoding/binary"
	"encoding/hex"
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

// TODO: Remaining sensors from Python reference:
// - label/enum sensors (pv*_mode_label, grid_mode_label, etc.) — Phase 2
// - battery info sensors (register 37000, separate bulk read) — Phase 3
// - meter sensors (register 36000, separate bulk read) — Phase 4
// - MPPT sensors (register 35301, separate bulk read) — Phase 5
