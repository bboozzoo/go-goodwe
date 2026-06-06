// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Maciej Borzecki <maciek.borzecki@gmail.com>
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

	"github.com/bboozzoo/go-goodwe"
)

// ETInverter implements the goodwe.Inverter interface for the ET line.
type ETInverter struct {
	ip      string
	serial  string
	service *service
}

// New creates a new ETInverter instance.
func New(ip string) *ETInverter {
	return &ETInverter{
		ip:      ip,
		service: newService(ip),
	}
}

// Connect performs the probe and DTLS connection.
func (e *ETInverter) Connect(ctx context.Context) error {
	// 1. Probe with backoff
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

	// 2. Connect via DTLS
	err = e.service.connectDTLS(ctx, probeRes.DTLSPort)
	if err != nil {
		return fmt.Errorf("connection failed during DTLS handshake: %w", err)
	}

	return nil
}

// Close cleans up the connection.
func (e *ETInverter) Close() error {
	return e.service.close()
}

// GetInfo retrieves the inverter information.
func (e *ETInverter) GetInfo(ctx context.Context) (*goodwe.Info, error) {
	return &goodwe.Info{
		SerialNumber: e.serial,
		Model:        "ET-Series",
		Firmware:     "1.0.0",
	}, nil
}

// GetSensors retrieves the sensor values from the registry via a single bulk request.
func (e *ETInverter) GetSensors(ctx context.Context) (map[string]any, error) {
	// Perform a single bulk request to get all telemetry in one go.
	// Based on user feedback, target register 35100 with a quantity of 125.
	data, err := e.service.readModbusBulk(ctx, 35100, 125)
	if err != nil {
		return nil, fmt.Errorf("failed to read bulk telemetry: %w", err)
	}

	results := make(map[string]any)

	for name, def := range registry {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		results[name] = def.Calculator(data)
	}

	return results, nil
}
