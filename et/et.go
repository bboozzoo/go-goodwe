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
	"fmt"
	"log/slog"
	"strings"

	"github.com/bboozzoo/go-goodwe"
)

type ETInverter struct {
	ip      string
	serial  string
	service *service
}

func New(ip string) *ETInverter {
	return &ETInverter{
		ip:      ip,
		service: newService(ip),
	}
}

func (e *ETInverter) Connect(ctx context.Context) error {
	var probeRes *probeResult
	err := backoff(ctx, func() error {
		var pErr error
		probeRes, pErr = e.service.probe(ctx)
		return pErr
	})

	if err != nil {
		return fmt.Errorf("connection failed during probe: %w", err)
	}

	e.serial = probeRes.SerialNumber

	err = e.service.connectDTLS(ctx, probeRes.DTLSPort)
	if err != nil {
		return fmt.Errorf("connection failed during DTLS handshake: %w", err)
	}

	return nil
}

func (e *ETInverter) Close() error {
	return e.service.close()
}

func (e *ETInverter) GetInfo(ctx context.Context) (*goodwe.Info, error) {
	return &goodwe.Info{
		SerialNumber: e.serial,
		Model:        "ET-Series",
		Firmware:     "1.0.0",
	}, nil
}

func (e *ETInverter) GetSensors(ctx context.Context) (map[string]goodwe.SensorValue, error) {
	data, err := e.service.readModbusBulk(ctx, 35100, 125)
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
	batteryData, err := e.service.readModbusBulk(ctx, 37000, 24)
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
	meterData, err := e.service.readModbusBulk(ctx, 36000, 125)
	if err != nil && isIllegalDataAddress(err) {
		meterData, err = e.service.readModbusBulk(ctx, 36000, 58)
	}
	if err != nil && isIllegalDataAddress(err) {
		meterData, err = e.service.readModbusBulk(ctx, 36000, 45)
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
	mpptData, err := e.service.readModbusBulk(ctx, 35301, 61)
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

// isIllegalDataAddress checks if a Modbus error is ILLEGAL_DATA_ADDRESS (0x02).
func isIllegalDataAddress(err error) bool {
	return strings.Contains(err.Error(), "exception 0x02")
}
