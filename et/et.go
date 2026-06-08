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
	"fmt"
	"log/slog"
	"strings"

	"github.com/bboozzoo/go-goodwe"
)

type ETInverter struct {
	ip        string
	serial    string
	transport Transport
}

// New creates a new ETInverter with the given IP address.
// The transport will be determined lazily; users should generally use
// discovery.Discover() instead of calling New directly.
func New(ip string) *ETInverter {
	return &ETInverter{
		ip: ip,
	}
}

// NewWithTransport creates a new ETInverter with a pre-configured transport.
func NewWithTransport(serial string, transport Transport) *ETInverter {
	return &ETInverter{
		serial:    serial,
		transport: transport,
	}
}

func (e *ETInverter) Connect(ctx context.Context) error {
	if e.transport == nil {
		return fmt.Errorf("no transport configured: use discovery.Discover() instead of New()")
	}
	err := backoff(ctx, func() error {
		// Close any previous connection before retry
		if cerr := e.transport.Close(); cerr != nil {
			slog.Warn("Error closing previous connection", "error", cerr)
		}

		return e.transport.Connect(ctx)
	})
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	return nil
}

func (e *ETInverter) Close() error {
	if e.transport == nil {
		return nil
	}
	return e.transport.Close()
}

// reconnect closes the existing connection and re-establishes it.
func (e *ETInverter) reconnect(ctx context.Context) error {
	slog.Info("Reconnecting to inverter...")
	if err := e.transport.Close(); err != nil {
		slog.Warn("Error closing existing connection", "error", err)
	}
	return e.Connect(ctx)
}

func (e *ETInverter) GetInfo(ctx context.Context) (*goodwe.Info, error) {
	info := &goodwe.Info{
		SerialNumber: e.serial,
	}

	data, err := e.transport.ReadRegisters(ctx, 35000, 33)
	if err != nil {
		slog.Warn("Failed to read device info registers", "error", err)
		return info, nil
	}

	if len(data) < 66 {
		return info, nil
	}

	info.SerialNumber = decodeGoodweString(data[6:22])
	info.Model = decodeGoodweString(data[22:32])
	info.Firmware = decodeGoodweString(data[42:54])
	info.RatedPower = int(binary.BigEndian.Uint16(data[2:4]))
	info.DSPVersion = fmt.Sprintf("%d.%d",
		binary.BigEndian.Uint16(data[32:34]),
		binary.BigEndian.Uint16(data[34:36]),
	)
	info.ARMVersion = fmt.Sprintf("%d", binary.BigEndian.Uint16(data[38:40]))

	return info, nil
}

func (e *ETInverter) GetSensors(ctx context.Context) (map[string]goodwe.SensorValue, error) {
	// TODO 35100, 37000 need to be named constants
	// TODO: 125 here, and 24 around batter need to ba made named constants, e.g.:
	// BaseRegistersOffset = 35100
	// BaseRegistersCount = 125
	// BatteryRegistersOffset = 37000
	data, err := e.readOnceWithFallback(ctx, 35100, 125)
	if err != nil {
		return nil, fmt.Errorf("failed to read bulk telemetry: %w", err)
	}

	results := make(map[string]goodwe.SensorValue)

	for name, def := range registry {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		results[name] = goodwe.SensorValue{
			Value: def.Calculator(data),
			Unit:  def.Unit,
			Name:  def.Name,
		}
	}

	// Attempt to read battery info (37000, 24 regs). Skip silently if battery is absent.
	batteryData, err := e.transport.ReadRegisters(ctx, 37000, 24)
	if err != nil {
		// ILLEGAL_DATA_ADDRESS (0x02) means no battery connected
		if !isIllegalDataAddress(err) {
			slog.Warn("Failed to read battery info", "error", err)
		}
	} else {
		for name, def := range batteryRegistry {
			select {
			case <-ctx.Done():
				return results, ctx.Err()
			default:
			}

			results[name] = goodwe.SensorValue{
				Value: def.Calculator(batteryData),
				Unit:  def.Unit,
				Name:  def.Name,
			}
		}
	}

	// Attempt to read meter data (36000). Try 125 regs, fall back to 58, then 45.
	meterData, err := e.transport.ReadRegisters(ctx, 36000, 125)
	if err != nil && isIllegalDataAddress(err) {
		meterData, err = e.transport.ReadRegisters(ctx, 36000, 58)
	}
	if err != nil && isIllegalDataAddress(err) {
		meterData, err = e.transport.ReadRegisters(ctx, 36000, 45)
	}
	if err != nil {
		slog.Warn("Failed to read meter data", "error", err)
	} else {
		for name, def := range meterRegistry {
			select {
			case <-ctx.Done():
				return results, ctx.Err()
			default:
			}
			// Calculator handles bounds checking internally.
			results[name] = goodwe.SensorValue{
				Value: def.Calculator(meterData),
				Unit:  def.Unit,
				Name:  def.Name,
			}
		}
	}

	// Read MPPT data (35301, 61 regs). Skip silently if unavailable.
	mpptData, err := e.transport.ReadRegisters(ctx, 35301, 61)
	if err != nil {
		if !isIllegalDataAddress(err) {
			slog.Warn("Failed to read MPPT data", "error", err)
		}
	} else {
		for name, def := range mpptRegistry {
			select {
			case <-ctx.Done():
				return results, ctx.Err()
			default:
			}
			results[name] = goodwe.SensorValue{
				Value: def.Calculator(mpptData),
				Unit:  def.Unit,
				Name:  def.Name,
			}
		}
	}

	return results, nil
}

// ReadSensor reads a single sensor by name, reading only the minimal register block.
func (e *ETInverter) ReadSensor(ctx context.Context, name string) (goodwe.SensorValue, error) {
	sb, ok := sensorLookup[name]
	if !ok {
		return goodwe.SensorValue{}, fmt.Errorf("unknown sensor: %s", name)
	}

	// Try primary read size, with one reconnect on failure.
	data, err := e.readOnceWithFallback(ctx, sb.startReg, sb.readQty)
	if err != nil {
		// Meter supports smaller read sizes on ILLEGAL_DATA_ADDRESS.
		if sb.block == blockMeter && isIllegalDataAddress(err) {
			var fallbackQty uint16
			switch {
			case sb.readQty >= 125:
				fallbackQty = 58
			case sb.readQty >= 58:
				fallbackQty = 45
			}
			if fallbackQty > 0 {
				// No reconnect here — the connection is fine, inverter just doesn't
				// support this many registers.
				data, err = e.transport.ReadRegisters(ctx, sb.startReg, fallbackQty)
				if err != nil && isIllegalDataAddress(err) && fallbackQty == 58 {
					data, err = e.transport.ReadRegisters(ctx, sb.startReg, 45)
				}
			}
		}
	}
	if err != nil {
		return goodwe.SensorValue{}, fmt.Errorf("failed to read sensor %s: %w", name, err)
	}

	return goodwe.SensorValue{
		Value: sb.def.Calculator(data),
		Unit:  sb.def.Unit,
		Name:  sb.def.Name,
	}, nil
}

// readOnceWithFallback tries a bulk read, reconnecting once on connection errors.
func (e *ETInverter) readOnceWithFallback(ctx context.Context, startReg, quantity uint16) ([]byte, error) {
	data, err := e.transport.ReadRegisters(ctx, startReg, quantity)
	if err != nil && !isIllegalDataAddress(err) {
		slog.Warn("Modbus read failed, attempting reconnect", "error", err)
		if rerr := e.reconnect(ctx); rerr != nil {
			return nil, fmt.Errorf("reconnect failed: %w (original error: %v)", rerr, err)
		}
		data, err = e.transport.ReadRegisters(ctx, startReg, quantity)
	}
	return data, err
}

// isIllegalDataAddress checks if a Modbus error is ILLEGAL_DATA_ADDRESS (0x02).
func isIllegalDataAddress(err error) bool {
	// TODO should use a proper typed error:
	// ErrModbusIllegalDataAddress
	return strings.Contains(err.Error(), "exception 0x02")
}

// decodeGoodweString decodes a GoodWe device info string field.
// Mirrors Python's _decode(): UTF-16BE if any byte < 0x20, otherwise ASCII.
func decodeGoodweString(data []byte) string {
	hasLow := false
	for _, b := range data {
		if b < 32 {
			hasLow = true
			break
		}
	}
	if hasLow {
		runes := make([]rune, 0, len(data)/2)
		for i := 0; i+1 < len(data); i += 2 {
			r := rune(binary.BigEndian.Uint16(data[i:]))
			if r == 0 {
				continue
			}
			runes = append(runes, r)
		}
		return strings.TrimRight(string(runes), " \t\n\r\x00")
	}
	return strings.TrimRight(string(data), " \t\n\r\x00")
}
