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
	"time"
)

const (
	undef16 uint16 = 0xFFFF
	undef32 uint32 = 0xFFFFFFFF
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
	"total_inverter_power": {Name: "Total Inverter Power", Unit: "W", Calculator: int16Reader(38, 1)},
	"active_power":         {Name: "Active Power", Unit: "W", Calculator: int16Reader(40, 1)},
	"reactive_power":       {Name: "Reactive Power", Unit: "var", Calculator: int16Reader(42, 1)},
	"apparent_power":       {Name: "Apparent Power", Unit: "VA", Calculator: int16Reader(44, 1)},

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
	"vbattery1":      {Name: "Battery Voltage", Unit: "V", Calculator: uint16Reader(80, 0.1)},
	"ibattery1":      {Name: "Battery Current", Unit: "A", Calculator: int16Reader(81, 0.1)},
	"pbattery1":      {Name: "Battery Power", Unit: "W", Calculator: int32Reader(82)},
	"battery_mode":   {Name: "Battery Mode Code", Unit: "", Calculator: uint16Reader(84, 1)},
	"warning_code":   {Name: "Warning Code", Unit: "", Calculator: uint16Reader(85, 1)},
	"safety_country": {Name: "Safety Country Code", Unit: "", Calculator: uint16Reader(86, 1)},
	"work_mode":      {Name: "Work Mode Code", Unit: "", Calculator: uint16Reader(87, 1)},
	"operation_mode": {Name: "Operation Mode", Unit: "", Calculator: uint16Reader(88, 1)},
	"error_codes":    {Name: "Error Codes", Unit: "", Calculator: uint32Reader(89)},

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

	// PV mode codes (byte-level encoding)
	"pv4_mode": {Name: "PV4 Mode Code", Unit: "", Calculator: highByteReader(19)},
	"pv3_mode": {Name: "PV3 Mode Code", Unit: "", Calculator: lowByteReader(19)},
	"pv2_mode": {Name: "PV2 Mode Code", Unit: "", Calculator: highByteReader(20)},
	"pv1_mode": {Name: "PV1 Mode Code", Unit: "", Calculator: lowByteReader(20)},

	// Calculated from active_power
	"grid_in_out": {
		Name: "Grid In/Out",
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

// readUint32 reads a 32-bit unsigned integer spanning 2 consecutive registers.
func readUint32(data []byte, regIdx int) uint32 {
	offset := regIdx * 2
	if offset+4 > len(data) {
		return 0
	}
	return binary.BigEndian.Uint32(data[offset : offset+4])
}
