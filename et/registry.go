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
	"math"
	"time"
)

const (
	undef16 uint16 = 0xFFFF
	undef32 uint32 = 0xFFFFFFFF
	undef64 uint64 = 0xFFFFFFFFFFFFFFFF
)

type sensorDefinition struct {
	Name       string
	Unit       string
	Calculator func(data []byte) any
}

var registry = map[string]sensorDefinition{
	// PV strings
	"vpv1": {Name: "PV1 Voltage", Unit: "V", Calculator: uint16Reader(3, 0.1)},
	"ipv1": {Name: "PV1 Current", Unit: "A", Calculator: uint16Reader(4, 0.1)},
	"ppv1": {Name: "PV1 Power", Unit: "W", Calculator: uint32Reader(5)},
	"vpv2": {Name: "PV2 Voltage", Unit: "V", Calculator: uint16Reader(7, 0.1)},
	"ipv2": {Name: "PV2 Current", Unit: "A", Calculator: uint16Reader(8, 0.1)},
	"ppv2": {Name: "PV2 Power", Unit: "W", Calculator: uint32Reader(9)},
	"vpv3": {Name: "PV3 Voltage", Unit: "V", Calculator: uint16Reader(11, 0.1)},
	"ipv3": {Name: "PV3 Current", Unit: "A", Calculator: uint16Reader(12, 0.1)},
	"ppv3": {Name: "PV3 Power", Unit: "W", Calculator: uint32Reader(13)},
	"vpv4": {Name: "PV4 Voltage", Unit: "V", Calculator: uint16Reader(15, 0.1)},
	"ipv4": {Name: "PV4 Current", Unit: "A", Calculator: uint16Reader(16, 0.1)},
	"ppv4": {Name: "PV4 Power", Unit: "W", Calculator: uint32Reader(17)},

	// PV mode codes (byte-level encoding)
	"pv4_mode": {Name: "PV4 Mode Code", Unit: "", Calculator: highByteReader(19)},
	"pv3_mode": {Name: "PV3 Mode Code", Unit: "", Calculator: lowByteReader(19)},
	"pv2_mode": {Name: "PV2 Mode Code", Unit: "", Calculator: highByteReader(20)},
	"pv1_mode": {Name: "PV1 Mode Code", Unit: "", Calculator: lowByteReader(20)},

	// PV mode labels
	"pv4_mode_label": {Name: "PV4 Mode", Unit: "", Calculator: enumHReader(19, pvModes)},
	"pv3_mode_label": {Name: "PV3 Mode", Unit: "", Calculator: enumLReader(19, pvModes)},
	"pv2_mode_label": {Name: "PV2 Mode", Unit: "", Calculator: enumHReader(20, pvModes)},
	"pv1_mode_label": {Name: "PV1 Mode", Unit: "", Calculator: enumLReader(20, pvModes)},

	// Grid (L1-L3)
	"vgrid":                {Name: "Grid L1 Voltage", Unit: "V", Calculator: uint16Reader(21, 0.1)},
	"igrid":                {Name: "Grid L1 Current", Unit: "A", Calculator: uint16Reader(22, 0.1)},
	"fgrid":                {Name: "Grid L1 Frequency", Unit: "Hz", Calculator: int16Reader(23, 0.01)},
	"pgrid":                {Name: "Grid L1 Power", Unit: "W", Calculator: int16Reader(25, 1)},
	"vgrid2":               {Name: "Grid L2 Voltage", Unit: "V", Calculator: uint16Reader(26, 0.1)},
	"igrid2":               {Name: "Grid L2 Current", Unit: "A", Calculator: uint16Reader(27, 0.1)},
	"fgrid2":               {Name: "Grid L2 Frequency", Unit: "Hz", Calculator: int16Reader(28, 0.01)},
	"pgrid2":               {Name: "Grid L2 Power", Unit: "W", Calculator: int16Reader(30, 1)},
	"vgrid3":               {Name: "Grid L3 Voltage", Unit: "V", Calculator: uint16Reader(31, 0.1)},
	"igrid3":               {Name: "Grid L3 Current", Unit: "A", Calculator: uint16Reader(32, 0.1)},
	"fgrid3":               {Name: "Grid L3 Frequency", Unit: "Hz", Calculator: int16Reader(33, 0.01)},
	"pgrid3":               {Name: "Grid L3 Power", Unit: "W", Calculator: int16Reader(35, 1)},
	"grid_mode":            {Name: "Grid Mode Code", Unit: "", Calculator: uint16Reader(36, 1)},
	"grid_mode_label":      {Name: "Grid Mode", Unit: "", Calculator: enum2Reader(36, gridModes)},
	"total_inverter_power": {Name: "Total Inverter Power", Unit: "W", Calculator: int16Reader(38, 1)},
	"active_power":         {Name: "Active Power", Unit: "W", Calculator: int16Reader(40, 1)},
	"reactive_power":       {Name: "Reactive Power", Unit: "var", Calculator: int16Reader(42, 1)},
	"apparent_power":       {Name: "Apparent Power", Unit: "VA", Calculator: int16Reader(44, 1)},

	// Grid in/out (calculated from active_power)
	"grid_in_out": {
		Name: "Grid In/Out Code",
		Unit: "",
		Calculator: func(data []byte) any {
			v := int16(binary.BigEndian.Uint16(data[80:82]))
			if v < -90 {
				return float64(2)
			}
			if v >= 90 {
				return float64(1)
			}
			return float64(0)
		},
	},
	"grid_in_out_label": {
		Name: "Grid In/Out",
		Unit: "",
		Calculator: func(data []byte) any {
			v := int16(binary.BigEndian.Uint16(data[80:82]))
			var mode int
			if v < -90 {
				mode = 2
			} else if v >= 90 {
				mode = 1
			}
			s, ok := gridInOutModes[mode]
			if !ok {
				return ""
			}
			return s
		},
	},

	// Backup/UPS
	"backup_v1":     {Name: "Backup L1 Voltage", Unit: "V", Calculator: uint16Reader(45, 0.1)},
	"backup_i1":     {Name: "Backup L1 Current", Unit: "A", Calculator: uint16Reader(46, 0.1)},
	"backup_f1":     {Name: "Backup L1 Frequency", Unit: "Hz", Calculator: int16Reader(47, 0.01)},
	"load_mode1":    {Name: "Load Mode L1", Unit: "", Calculator: uint16Reader(48, 1)},
	"backup_p1":     {Name: "Backup L1 Power", Unit: "W", Calculator: int16Reader(50, 1)},
	"backup_v2":     {Name: "Backup L2 Voltage", Unit: "V", Calculator: uint16Reader(51, 0.1)},
	"backup_i2":     {Name: "Backup L2 Current", Unit: "A", Calculator: uint16Reader(52, 0.1)},
	"backup_f2":     {Name: "Backup L2 Frequency", Unit: "Hz", Calculator: int16Reader(53, 0.01)},
	"load_mode2":    {Name: "Load Mode L2", Unit: "", Calculator: uint16Reader(54, 1)},
	"backup_p2":     {Name: "Backup L2 Power", Unit: "W", Calculator: int16Reader(56, 1)},
	"backup_v3":     {Name: "Backup L3 Voltage", Unit: "V", Calculator: uint16Reader(57, 0.1)},
	"backup_i3":     {Name: "Backup L3 Current", Unit: "A", Calculator: uint16Reader(58, 0.1)},
	"backup_f3":     {Name: "Backup L3 Frequency", Unit: "Hz", Calculator: int16Reader(59, 0.01)},
	"load_mode3":    {Name: "Load Mode L3", Unit: "", Calculator: uint16Reader(60, 1)},
	"backup_p3":     {Name: "Backup L3 Power", Unit: "W", Calculator: int16Reader(62, 1)},
	"load_p1":       {Name: "Load L1 Power", Unit: "W", Calculator: int16Reader(64, 1)},
	"load_p2":       {Name: "Load L2 Power", Unit: "W", Calculator: int16Reader(66, 1)},
	"load_p3":       {Name: "Load L3 Power", Unit: "W", Calculator: int16Reader(68, 1)},
	"backup_ptotal": {Name: "Load PTotal", Unit: "W", Calculator: int16Reader(70, 1)},
	"load_ptotal":   {Name: "Load PTotal Backup", Unit: "W", Calculator: int16Reader(72, 1)},
	"ups_load":      {Name: "UPS Load", Unit: "W", Calculator: uint16Reader(73, 1)},

	// Temperature
	"temperature_air":    {Name: "Air Temperature", Unit: "C", Calculator: int16Reader(74, 0.1)},
	"temperature_module": {Name: "Module Temperature", Unit: "C", Calculator: int16Reader(75, 0.1)},
	"temperature":        {Name: "Temperature", Unit: "C", Calculator: int16Reader(76, 0.1)},

	// Internal
	"function_bit": {Name: "Function Bit", Unit: "", Calculator: uint16Reader(77, 1)},
	"bus_voltage":  {Name: "Bus Voltage", Unit: "V", Calculator: uint16Reader(78, 0.1)},
	"nbus_voltage": {Name: "Negative Bus Voltage", Unit: "V", Calculator: uint16Reader(79, 0.1)},

	// Battery (primary telemetry block)
	"vbattery1":            {Name: "Battery Voltage", Unit: "V", Calculator: uint16Reader(80, 0.1)},
	"ibattery1":            {Name: "Battery Current", Unit: "A", Calculator: int16Reader(81, 0.1)},
	"pbattery1":            {Name: "Battery Power", Unit: "W", Calculator: int32Reader(82)},
	"battery_mode":         {Name: "Battery Mode Code", Unit: "", Calculator: uint16Reader(84, 1)},
	"battery_mode_label":   {Name: "Battery Mode", Unit: "", Calculator: enum2Reader(84, batteryModes)},
	"warning_code":         {Name: "Warning Code", Unit: "", Calculator: uint16Reader(85, 1)},
	"safety_country":       {Name: "Safety Country Code", Unit: "", Calculator: uint16Reader(86, 1)},
	"safety_country_label": {Name: "Safety Country", Unit: "", Calculator: enum2Reader(86, safetyCountries)},
	"work_mode":            {Name: "Work Mode Code", Unit: "", Calculator: uint16Reader(87, 1)},
	"work_mode_label":      {Name: "Work Mode", Unit: "", Calculator: enum2Reader(87, workModesET)},
	"operation_mode":       {Name: "Operation Mode", Unit: "", Calculator: uint16Reader(88, 1)},
	"error_codes":          {Name: "Error Codes", Unit: "", Calculator: uint32Reader(89)},
	"errors":               {Name: "Errors", Unit: "", Calculator: enumBitmap4Reader(89, errorCodes)},

	// Energy totals
	"e_total":               {Name: "Total Energy Produced", Unit: "kWh", Calculator: energy4Reader(91)},
	"e_day":                 {Name: "Daily Energy", Unit: "kWh", Calculator: energy4Reader(93)},
	"e_total_exp":           {Name: "Total Export Energy", Unit: "kWh", Calculator: energy4Reader(95)},
	"h_total":               {Name: "Total Hours", Unit: "", Calculator: uint32Reader(97)},
	"e_day_exp":             {Name: "Daily Export Energy", Unit: "kWh", Calculator: energy2Reader(99)},
	"e_total_imp":           {Name: "Total Import Energy", Unit: "kWh", Calculator: energy4Reader(100)},
	"e_day_imp":             {Name: "Daily Import Energy", Unit: "kWh", Calculator: energy2Reader(102)},
	"e_load_total":          {Name: "Total Load Energy", Unit: "kWh", Calculator: energy4Reader(103)},
	"e_load_day":            {Name: "Daily Load Energy", Unit: "kWh", Calculator: energy2Reader(105)},
	"e_bat_charge_total":    {Name: "Total Battery Charge Energy", Unit: "kWh", Calculator: energy4Reader(106)},
	"e_bat_charge_day":      {Name: "Daily Battery Charge Energy", Unit: "kWh", Calculator: energy2Reader(108)},
	"e_bat_discharge_total": {Name: "Total Battery Discharge Energy", Unit: "kWh", Calculator: energy4Reader(109)},
	"e_bat_discharge_day":   {Name: "Daily Battery Discharge Energy", Unit: "kWh", Calculator: energy2Reader(111)},
	"diagnose_result":       {Name: "Diagnose Result", Unit: "", Calculator: uint32Reader(120)},
	"diagnose_result_label": {Name: "Diagnose Result", Unit: "", Calculator: enumBitmap4Reader(120, diagStatusCodes)},

	// Calculated
	"ppv": {
		Name: "Total PV Power",
		Unit: "W",
		Calculator: func(data []byte) any {
			v1 := readUint32(data, 5)
			if v1 == undef32 {
				v1 = 0
			}
			v2 := readUint32(data, 9)
			if v2 == undef32 {
				v2 = 0
			}
			v3 := readUint32(data, 13)
			if v3 == undef32 {
				v3 = 0
			}
			v4 := readUint32(data, 17)
			if v4 == undef32 {
				v4 = 0
			}
			return float64(v1 + v2 + v3 + v4)
		},
	},
	"house_consumption": {
		Name: "House Consumption",
		Unit: "W",
		Calculator: func(data []byte) any {
			v1 := readUint32(data, 5)
			if v1 == undef32 {
				v1 = 0
			}
			v2 := readUint32(data, 9)
			if v2 == undef32 {
				v2 = 0
			}
			v3 := readUint32(data, 13)
			if v3 == undef32 {
				v3 = 0
			}
			v4 := readUint32(data, 17)
			if v4 == undef32 {
				v4 = 0
			}
			v5 := int32(readUint32(data, 82))
			v6 := int16(binary.BigEndian.Uint16(data[80:82]))
			return float64(int64(v1) + int64(v2) + int64(v3) + int64(v4) + int64(v5) - int64(v6))
		},
	},

	// Timestamp (6 bytes across 3 registers at index 0-2)
	"timestamp": {Name: "Timestamp", Unit: "", Calculator: timestampReader(0)},
}

// batteryRegistry contains sensors from the battery info block (register 37000, 24 registers).
// All register indices are relative to 37000.
var batteryRegistry = map[string]sensorDefinition{
	"battery_bms":                 {Name: "Battery BMS", Unit: "", Calculator: uint16Reader(0, 1)},
	"battery_index":               {Name: "Battery Index", Unit: "", Calculator: uint16Reader(1, 1)},
	"battery_status":              {Name: "Battery Status", Unit: "", Calculator: uint16Reader(2, 1)},
	"battery_temperature":         {Name: "Battery Temperature", Unit: "C", Calculator: tempReader(3)},
	"battery_charge_limit":        {Name: "Battery Charge Limit", Unit: "A", Calculator: uint16Reader(4, 1)},
	"battery_discharge_limit":     {Name: "Battery Discharge Limit", Unit: "A", Calculator: uint16Reader(5, 1)},
	"battery_error_l":             {Name: "Battery Error Low", Unit: "", Calculator: uint16Reader(6, 1)},
	"battery_soc":                 {Name: "Battery State of Charge", Unit: "%", Calculator: uint16Reader(7, 1)},
	"battery_soh":                 {Name: "Battery State of Health", Unit: "%", Calculator: uint16Reader(8, 1)},
	"battery_modules":             {Name: "Battery Modules", Unit: "", Calculator: uint16Reader(9, 1)},
	"battery_warning_l":           {Name: "Battery Warning Low", Unit: "", Calculator: uint16Reader(10, 1)},
	"battery_protocol":            {Name: "Battery Protocol", Unit: "", Calculator: uint16Reader(11, 1)},
	"battery_error_h":             {Name: "Battery Error High", Unit: "", Calculator: uint16Reader(12, 1)},
	"battery_error":               {Name: "Battery Error", Unit: "", Calculator: enumBitmap22Reader(12, 6, bmsAlarmCodes)},
	"battery_warning_h":           {Name: "Battery Warning High", Unit: "", Calculator: uint16Reader(13, 1)},
	"battery_warning":             {Name: "Battery Warning", Unit: "", Calculator: enumBitmap22Reader(13, 10, bmsWarningCodes)},
	"battery_sw_version":          {Name: "Battery Software Version", Unit: "", Calculator: uint16Reader(14, 1)},
	"battery_hw_version":          {Name: "Battery Hardware Version", Unit: "", Calculator: uint16Reader(15, 1)},
	"battery_max_cell_temp_id":    {Name: "Battery Max Cell Temperature ID", Unit: "", Calculator: uint16Reader(16, 1)},
	"battery_min_cell_temp_id":    {Name: "Battery Min Cell Temperature ID", Unit: "", Calculator: uint16Reader(17, 1)},
	"battery_max_cell_voltage_id": {Name: "Battery Max Cell Voltage ID", Unit: "", Calculator: uint16Reader(18, 1)},
	"battery_min_cell_voltage_id": {Name: "Battery Min Cell Voltage ID", Unit: "", Calculator: uint16Reader(19, 1)},
	"battery_max_cell_temp":       {Name: "Battery Max Cell Temperature", Unit: "C", Calculator: tempReader(20)},
	"battery_min_cell_temp":       {Name: "Battery Min Cell Temperature", Unit: "C", Calculator: tempReader(21)},
	"battery_max_cell_voltage":    {Name: "Battery Max Cell Voltage", Unit: "V", Calculator: cellVoltageReader(22)},
	"battery_min_cell_voltage":    {Name: "Battery Min Cell Voltage", Unit: "V", Calculator: cellVoltageReader(23)},
}

// meterRegistry contains sensors from the meter block (register 36000).
// Register indices are relative to 36000. Sensor availability depends on
// how many registers were successfully read (45, 58, or 125).
var meterRegistry = map[string]sensorDefinition{
	"commode":                    {Name: "Commode", Unit: "", Calculator: uint16Reader(0, 1)},
	"rssi":                       {Name: "RSSI", Unit: "", Calculator: uint16Reader(1, 1)},
	"manufacture_code":           {Name: "Manufacture Code", Unit: "", Calculator: uint16Reader(2, 1)},
	"meter_test_status":          {Name: "Meter Test Status", Unit: "", Calculator: uint16Reader(3, 1)},
	"meter_comm_status":          {Name: "Meter Communication Status", Unit: "", Calculator: uint16Reader(4, 1)},
	"active_power1":              {Name: "Active Power L1", Unit: "W", Calculator: int16Reader(5, 1)},
	"active_power2":              {Name: "Active Power L2", Unit: "W", Calculator: int16Reader(6, 1)},
	"active_power3":              {Name: "Active Power L3", Unit: "W", Calculator: int16Reader(7, 1)},
	"active_power_total":         {Name: "Active Power Total", Unit: "W", Calculator: int16Reader(8, 1)},
	"reactive_power_total":       {Name: "Reactive Power Total", Unit: "var", Calculator: int16Reader(9, 1)},
	"meter_power_factor1":        {Name: "Meter Power Factor L1", Unit: "", Calculator: decimalReader(10, 1000)},
	"meter_power_factor2":        {Name: "Meter Power Factor L2", Unit: "", Calculator: decimalReader(11, 1000)},
	"meter_power_factor3":        {Name: "Meter Power Factor L3", Unit: "", Calculator: decimalReader(12, 1000)},
	"meter_power_factor":         {Name: "Meter Power Factor", Unit: "", Calculator: decimalReader(13, 1000)},
	"meter_freq":                 {Name: "Meter Frequency", Unit: "Hz", Calculator: int16Reader(14, 0.01)},
	"meter_e_total_exp":          {Name: "Meter Total Energy (export)", Unit: "kWh", Calculator: floatReader(15, 1000)},
	"meter_e_total_imp":          {Name: "Meter Total Energy (import)", Unit: "kWh", Calculator: floatReader(17, 1000)},
	"meter_active_power1":        {Name: "Meter Active Power L1", Unit: "W", Calculator: int32Reader(19)},
	"meter_active_power2":        {Name: "Meter Active Power L2", Unit: "W", Calculator: int32Reader(21)},
	"meter_active_power3":        {Name: "Meter Active Power L3", Unit: "W", Calculator: int32Reader(23)},
	"meter_active_power_total":   {Name: "Meter Active Power Total", Unit: "W", Calculator: int32Reader(25)},
	"meter_reactive_power1":      {Name: "Meter Reactive Power L1", Unit: "var", Calculator: int32Reader(27)},
	"meter_reactive_power2":      {Name: "Meter Reactive Power L2", Unit: "var", Calculator: int32Reader(29)},
	"meter_reactive_power3":      {Name: "Meter Reactive Power L3", Unit: "var", Calculator: int32Reader(31)},
	"meter_reactive_power_total": {Name: "Meter Reactive Power Total", Unit: "var", Calculator: int32Reader(33)},
	"meter_apparent_power1":      {Name: "Meter Apparent Power L1", Unit: "VA", Calculator: int32Reader(35)},
	"meter_apparent_power2":      {Name: "Meter Apparent Power L2", Unit: "VA", Calculator: int32Reader(37)},
	"meter_apparent_power3":      {Name: "Meter Apparent Power L3", Unit: "VA", Calculator: int32Reader(39)},
	"meter_apparent_power_total": {Name: "Meter Apparent Power Total", Unit: "VA", Calculator: int32Reader(41)},
	"meter_type":                 {Name: "Meter Type", Unit: "", Calculator: uint16Reader(43, 1)},
	"meter_sw_version":           {Name: "Meter Software Version", Unit: "", Calculator: uint16Reader(44, 1)},

	// Extended sensors (reg index 45-57)
	"meter2_active_power": {Name: "Meter 2 Active Power", Unit: "W", Calculator: int32Reader(45)},
	"meter2_e_total_exp":  {Name: "Meter 2 Total Energy (export)", Unit: "kWh", Calculator: floatReader(47, 1000)},
	"meter2_e_total_imp":  {Name: "Meter 2 Total Energy (import)", Unit: "kWh", Calculator: floatReader(49, 1000)},
	"meter2_comm_status":  {Name: "Meter 2 Communication Status", Unit: "", Calculator: uint16Reader(51, 1)},
	"meter_voltage1":      {Name: "Meter L1 Voltage", Unit: "V", Calculator: uint16Reader(52, 0.1)},
	"meter_voltage2":      {Name: "Meter L2 Voltage", Unit: "V", Calculator: uint16Reader(53, 0.1)},
	"meter_voltage3":      {Name: "Meter L3 Voltage", Unit: "V", Calculator: uint16Reader(54, 0.1)},
	"meter_current1":      {Name: "Meter L1 Current", Unit: "A", Calculator: uint16Reader(55, 0.1)},
	"meter_current2":      {Name: "Meter L2 Current", Unit: "A", Calculator: uint16Reader(56, 0.1)},
	"meter_current3":      {Name: "Meter L3 Current", Unit: "A", Calculator: uint16Reader(57, 0.1)},

	// Extended2 sensors (reg index 92+, 8-byte energy totals)
	"meter_e_total_exp1":  {Name: "Meter Total Energy (export) L1", Unit: "kWh", Calculator: energy8Reader(92)},
	"meter_e_total_exp2":  {Name: "Meter Total Energy (export) L2", Unit: "kWh", Calculator: energy8Reader(96)},
	"meter_e_total_exp3":  {Name: "Meter Total Energy (export) L3", Unit: "kWh", Calculator: energy8Reader(100)},
	"meter_e_total_exp_8": {Name: "Meter Total Energy (export)", Unit: "kWh", Calculator: energy8Reader(104)},
	"meter_e_total_imp1":  {Name: "Meter Total Energy (import) L1", Unit: "kWh", Calculator: energy8Reader(108)},
	"meter_e_total_imp2":  {Name: "Meter Total Energy (import) L2", Unit: "kWh", Calculator: energy8Reader(112)},
	"meter_e_total_imp3":  {Name: "Meter Total Energy (import) L3", Unit: "kWh", Calculator: energy8Reader(116)},
	"meter_e_total_imp_8": {Name: "Meter Total Energy (import)", Unit: "kWh", Calculator: energy8Reader(120)},
}

func GetSensorNames() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// Reader helpers for common sensor types.
// All offsets are register indices (multiplied by 2 internally for byte access).

func uint16Reader(regIdx int, scale float64) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+2 > len(data) {
			return float64(0)
		}
		raw := binary.BigEndian.Uint16(data[offset : offset+2])
		if raw == undef16 {
			return float64(0)
		}
		return float64(raw) * scale
	}
}

func int16Reader(regIdx int, scale float64) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+2 > len(data) {
			return float64(0)
		}
		return float64(int16(binary.BigEndian.Uint16(data[offset:offset+2]))) * scale
	}
}

func uint32Reader(regIdx int) func([]byte) any {
	return func(data []byte) any {
		v := readUint32(data, regIdx)
		if v == undef32 {
			return float64(0)
		}
		return float64(v)
	}
}

func int32Reader(regIdx int) func([]byte) any {
	return func(data []byte) any {
		return float64(int32(readUint32(data, regIdx)))
	}
}

func energy4Reader(regIdx int) func([]byte) any {
	return func(data []byte) any {
		v := readUint32(data, regIdx)
		if v == undef32 {
			return float64(0)
		}
		return float64(v) / 10.0
	}
}

func energy2Reader(regIdx int) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+2 > len(data) {
			return float64(0)
		}
		raw := binary.BigEndian.Uint16(data[offset : offset+2])
		if raw == undef16 {
			return float64(0)
		}
		return float64(raw) / 10.0
	}
}

// timestampReader reads a 6-byte timestamp spanning 3 registers at the given index.
// Encoding: register[0] = {year-2000, month}, register[1] = {day, hour}, register[2] = {minute, second}.
func timestampReader(regIdx int) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+6 > len(data) {
			return time.Time{}
		}
		year := 2000 + int(data[offset])
		month := int(data[offset+1])
		day := int(data[offset+2])
		hour := int(data[offset+3])
		minute := int(data[offset+4])
		second := int(data[offset+5])
		return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
	}
}

// highByteReader reads the high byte of a register as a signed integer.
func highByteReader(regIdx int) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+2 > len(data) {
			return float64(0)
		}
		return float64(int8(data[offset]))
	}
}

// lowByteReader reads the low byte of a register as a signed integer.
func lowByteReader(regIdx int) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+2 > len(data) {
			return float64(0)
		}
		return float64(int8(data[offset+1]))
	}
}

// enumHReader reads the high byte of a register and maps it through a label dictionary.
func enumHReader(regIdx int, labels map[int]string) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+2 > len(data) {
			return ""
		}
		v := int8(data[offset])
		s, ok := labels[int(v)]
		if !ok {
			return ""
		}
		return s
	}
}

// enumLReader reads the low byte of a register and maps it through a label dictionary.
func enumLReader(regIdx int, labels map[int]string) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+2 > len(data) {
			return ""
		}
		v := int8(data[offset+1])
		s, ok := labels[int(v)]
		if !ok {
			return ""
		}
		return s
	}
}

// enum2Reader reads a uint16 register value and maps it through a label dictionary.
func enum2Reader(regIdx int, labels map[int]string) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+2 > len(data) {
			return ""
		}
		v := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		s, ok := labels[v]
		if !ok {
			return ""
		}
		return s
	}
}

// enumBitmap4Reader reads 4 bytes (2 registers) as a bitmap and decodes it into
// a comma-separated string of active labels. Bit position 0 is LSB of the uint32.
func enumBitmap4Reader(regIdx int, labels map[int]string) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+4 > len(data) {
			return ""
		}
		v := binary.BigEndian.Uint32(data[offset : offset+4])
		return decodeBitmap(v, labels)
	}
}

// tempReader reads a signed 16-bit temperature value scaled by 0.1.
// Returns 0 for sentinel values (-1 or 32767).
func tempReader(regIdx int) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+2 > len(data) {
			return float64(0)
		}
		v := int16(binary.BigEndian.Uint16(data[offset : offset+2]))
		if v == -1 || v == 32767 {
			return float64(0)
		}
		return float64(v) / 10.0
	}
}

// cellVoltageReader reads an unsigned 16-bit cell voltage value scaled by 0.001.
func cellVoltageReader(regIdx int) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+2 > len(data) {
			return float64(0)
		}
		return float64(binary.BigEndian.Uint16(data[offset:offset+2])) / 1000.0
	}
}

// enumBitmap22Reader reads a 32-bit bitmap spread across two separate registers
// (hiReg and loReg) and decodes it into a comma-separated string of active labels.
func enumBitmap22Reader(hiReg int, loReg int, labels map[int]string) func([]byte) any {
	return func(data []byte) any {
		hiOff := hiReg * 2
		loOff := loReg * 2
		if hiOff+2 > len(data) || loOff+2 > len(data) {
			return ""
		}
		hi := uint32(binary.BigEndian.Uint16(data[hiOff : hiOff+2]))
		lo := uint32(binary.BigEndian.Uint16(data[loOff : loOff+2]))
		v := hi<<16 | lo
		return decodeBitmap(v, labels)
	}
}

// decodeBitmap decodes a 32-bit bitmap into a comma-separated string of active labels.
func decodeBitmap(v uint32, labels map[int]string) string {
	var parts []string
	for i := 0; i < 32; i++ {
		if v&1 == 1 {
			if s, ok := labels[i]; ok && s != "" {
				parts = append(parts, s)
			}
		}
		v >>= 1
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		result := parts[0]
		for _, p := range parts[1:] {
			result += ", " + p
		}
		return result
	}
}

// readUint32 reads a 32-bit unsigned integer spanning 2 consecutive registers.
func readUint32(data []byte, regIdx int) uint32 {
	offset := regIdx * 2
	if offset+4 > len(data) {
		return 0
	}
	return binary.BigEndian.Uint32(data[offset : offset+4])
}

// decimalReader reads a signed 16-bit integer and divides by scale.
func decimalReader(regIdx int, scale float64) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+2 > len(data) {
			return float64(0)
		}
		return float64(int16(binary.BigEndian.Uint16(data[offset:offset+2]))) / scale
	}
}

// floatReader reads a 4-byte IEEE 754 big-endian float and divides by scale.
func floatReader(regIdx int, scale float64) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+4 > len(data) {
			return float64(0)
		}
		return float64(math.Float32frombits(binary.BigEndian.Uint32(data[offset:offset+4]))) / scale
	}
}

// energy8Reader reads 8 bytes (4 registers) as a uint64 and divides by 100.
func energy8Reader(regIdx int) func([]byte) any {
	return func(data []byte) any {
		offset := regIdx * 2
		if offset+8 > len(data) {
			return float64(0)
		}
		v := binary.BigEndian.Uint64(data[offset : offset+8])
		if v == undef64 {
			return float64(0)
		}
		return float64(v) / 100.0
	}
}
