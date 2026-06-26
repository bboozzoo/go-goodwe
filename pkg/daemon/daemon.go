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

package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bboozzoo/go-goodwe"
	"github.com/bboozzoo/go-goodwe/discovery"
	"github.com/bboozzoo/go-goodwe/pkg/analysis"
	"github.com/bboozzoo/go-goodwe/pkg/api"
	"github.com/bboozzoo/go-goodwe/pkg/db"
)

const (
	backoffInitial = 5 * time.Second
	backoffMax     = 5 * time.Minute
)

// Daemon manages the poll loop and aggregation scheduler.
type Daemon struct {
	inverter     goodwe.Inverter // may be nil when no inverter is configured
	store        *db.Store       // may be nil when no database is configured
	inverterIP   string          // IP address of the inverter, for diagnostics
	pollInterval time.Duration   // zero means no polling

	mu               sync.RWMutex
	connState        api.InverterConnState
	connErr          error
	verificationErr  error                // set when inverter serial doesn't match DB
	inverterIdentity *db.InverterIdentity // stored identity from DB (may be nil)
}

// New creates a new Daemon. inverter and store may be nil; the poll loop
// is a no-op until an inverter is provided. pollInterval of 0 disables polling.
func New(inverter goodwe.Inverter, store *db.Store, inverterIP string, pollInterval time.Duration) *Daemon {
	d := &Daemon{
		inverter:     inverter,
		store:        store,
		inverterIP:   inverterIP,
		pollInterval: pollInterval,
	}
	if inverter == nil {
		d.connState = api.InverterStateDisabled
	} else {
		d.connState = api.InverterStateDisconnected
	}
	return d
}

// InverterState returns the current inverter connection state.
func (d *Daemon) InverterState() api.InverterConnState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.connState
}

// ConnError returns any connection-level error.
func (d *Daemon) ConnError() error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.connErr
}

// VerificationError returns any identity mismatch error.
// Returns nil if the identity was verified successfully or not yet checked.
func (d *Daemon) VerificationError() error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.verificationErr
}

// InverterIdentity returns the stored inverter identity from the database.
// May be nil if not yet set or no database configured.
func (d *Daemon) InverterIdentity() *db.InverterIdentity {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.inverterIdentity
}

// Run implements the inverter connection state machine.
//
//	disconnected -> try connect()
//	   if connected -> polling
//	   if error -> backoff
//	backoff -> wait -> disconnected
//	polling -> try poll
//	   if success -> waiting
//	   if error -> disconnect -> disconnected
//	waiting -> wait for poll tick -> polling
func (d *Daemon) Run(ctx context.Context) error {
	if d.inverter == nil {
		slog.Info("No inverter configured, poll loop disabled")
		d.setState(api.InverterStateDisabled, nil)
		<-ctx.Done()
		return nil
	}

	if d.pollInterval <= 0 {
		slog.Info("Polling disabled (no -poll interval set)")
		d.setState(api.InverterStateDisconnected, nil)
		<-ctx.Done()
		return nil
	}

	slog.Info("Poll loop started", "interval", d.pollInterval)
	defer slog.Info("Poll loop stopped")

	d.setState(api.InverterStateDisconnected, nil)

	backoff := backoffInitial
	timer := time.NewTimer(0) // fire immediately for first connect
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			d.closeConnection()
			return nil

		case <-timer.C:
			switch d.getState() {
			case api.InverterStateDisconnected:
				d.doConnect(ctx)
				// Reconnected successfully — poll immediately instead of
				// waiting for the next pollInterval tick.
				if d.getState() == api.InverterStateConnected {
					d.doPoll(ctx)
				}

			case api.InverterStateConnected:
				d.doPoll(ctx)
			}

			// Schedule next action based on state.
			var next time.Duration
			switch d.getState() {
			case api.InverterStateConnected:
				next = d.pollInterval
				backoff = backoffInitial // reset backoff on success
			case api.InverterStateDisconnected:
				next = backoff
				backoff *= 2
				if backoff > backoffMax {
					backoff = backoffMax
				}
			case api.InverterStateFailed:
				// Serial mismatch — stop polling entirely.
				<-ctx.Done()
				return nil
			default:
				next = d.pollInterval
			}

			if d.getState() != api.InverterStateFailed {
				timer.Reset(next)
			}
		}
	}
}

// doConnect attempts to connect to the inverter and verify its identity.
// Updates state to Connected on success, or Disconnected on failure.
func (d *Daemon) doConnect(ctx context.Context) {
	d.setState(api.InverterStateConnecting, nil)
	slog.Info("Connecting to inverter...")

	if err := d.inverter.Connect(ctx); err != nil {
		if d.inverterIP != "" && discovery.Ping(ctx, d.inverterIP) {
			slog.Warn("Connection failed — dongle responded to probe but DTLS connection refused", "error", err)
		} else {
			slog.Warn("Connection failed", "error", err)
		}
		d.setState(api.InverterStateDisconnected, err)
		return
	}

	// Verify identity (this also stores it on first connect).
	if err := d.verifyIdentity(ctx); err != nil {
		slog.Warn("Identity verification failed, disconnecting", "error", err)
		d.closeConnection()
		d.setState(api.InverterStateDisconnected, err)
		return
	}

	// Check for serial mismatch.
	if err := d.VerificationError(); err != nil {
		slog.Warn("Serial mismatch detected, polling disabled", "error", err)
		d.setState(api.InverterStateFailed, err)
		return
	}

	slog.Info("Connected to inverter")
	d.setState(api.InverterStateConnected, nil)
}

// doPoll performs a single poll cycle. On error it disconnects and
// transitions to Disconnected so the state machine retries from scratch.
func (d *Daemon) doPoll(ctx context.Context) {
	slog.Debug("Poll cycle starting")

	sensors, err := d.inverter.GetSensors(ctx)
	if err != nil {
		slog.Warn("Poll cycle failed, disconnecting", "error", err)
		d.closeConnection()
		d.setState(api.InverterStateDisconnected, err)
		return
	}

	now := time.Now().UTC()
	var stored int
	for name, sv := range sensors {
		if d.store == nil {
			break
		}
		var val *float64
		var valText *string
		switch v := sv.Value.(type) {
		case float64:
			val = &v
		case string:
			valText = &v
		default:
			continue
		}
		if err := d.store.InsertSample(ctx, name, sv.Unit, now, val, valText); err != nil {
			slog.Warn("Failed to store sample", "sensor", name, "error", err)
			continue
		}
		stored++
	}

	// Run voltage analysis on newly stored samples.
	if d.store != nil {
		if err := analysis.RunVoltageAnalysis(ctx, d.store); err != nil {
			slog.Warn("Voltage analysis failed", "error", err)
		}
	}

	slog.Info("Poll cycle complete", "sensors_read", len(sensors), "stored", stored)
}

// closeConnection closes the inverter connection, ignoring errors.
func (d *Daemon) closeConnection() {
	if err := d.inverter.Close(); err != nil {
		slog.Debug("Error closing inverter connection (expected if already closed)", "error", err)
	}
}

// getState returns the current state (read-only lock).
func (d *Daemon) getState() api.InverterConnState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.connState
}

// setState updates the connection state and error atomically.
func (d *Daemon) setState(state api.InverterConnState, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	slog.Debug("State transition", "from", d.connState, "to", state)
	d.connState = state
	d.connErr = nil
	if err != nil {
		d.connErr = err
	}
}

// verifyIdentity reads the inverter info and checks/stores its serial number
// against the database. Runs once on startup.
func (d *Daemon) verifyIdentity(ctx context.Context) error {
	info, err := d.inverter.GetInfo(ctx)
	if err != nil {
		return fmt.Errorf("get inverter info: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.store == nil {
		slog.Info("Inverter identity (no database)", "serial", info.SerialNumber)
		return nil
	}

	stored, err := d.store.GetInverterIdentity(ctx)
	if err != nil {
		return fmt.Errorf("read stored identity: %w", err)
	}

	if stored == nil {
		slog.Info("Storing inverter identity",
			"serial", info.SerialNumber,
			"model", info.Model,
			"rated_power", info.RatedPower)
		if err := d.store.SetInverterIdentity(ctx, info.SerialNumber, info.Model,
			info.Firmware, info.DSPVersion, info.ARMVersion, info.RatedPower); err != nil {
			return fmt.Errorf("store inverter identity: %w", err)
		}
		d.inverterIdentity = &db.InverterIdentity{
			Serial:     info.SerialNumber,
			Model:      info.Model,
			Firmware:   info.Firmware,
			DSPVersion: info.DSPVersion,
			ARMVersion: info.ARMVersion,
			RatedPower: info.RatedPower,
		}
		if err := d.store.PurgeBadSamples(ctx, info.RatedPower); err != nil {
			slog.Warn("Failed to purge bad samples", "error", err)
		}
		return nil
	}

	d.inverterIdentity = stored
	if stored.Serial != info.SerialNumber {
		d.verificationErr = fmt.Errorf(
			"serial mismatch: expected %q, got %q (inverter may have been replaced)",
			stored.Serial, info.SerialNumber)
		slog.Error("Inverter serial mismatch",
			"expected", stored.Serial,
			"actual", info.SerialNumber,
		)
		return nil
	}

	slog.Info("Inverter identity verified", "serial", info.SerialNumber)
	if err := d.store.SetInverterIdentity(ctx, info.SerialNumber, info.Model,
		info.Firmware, info.DSPVersion, info.ARMVersion, info.RatedPower); err != nil {
		slog.Warn("Failed to update inverter identity", "error", err)
	}
	if err := d.store.PurgeBadSamples(ctx, info.RatedPower); err != nil {
		slog.Warn("Failed to purge bad samples", "error", err)
	}
	return nil
}

// Close cleans up daemon resources. Called after Run() returns.
func (d *Daemon) Close() error {
	slog.Info("Daemon resources cleaned up")
	return nil
}
