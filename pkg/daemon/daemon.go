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

	"github.com/bboozzoo/go-goodwe"
	"github.com/bboozzoo/go-goodwe/pkg/api"
	"github.com/bboozzoo/go-goodwe/pkg/db"
)

// Daemon manages the poll loop and aggregation scheduler.
type Daemon struct {
	inverter goodwe.Inverter // may be nil when no inverter is configured
	store    *db.Store       // may be nil when no database is configured

	mu               sync.RWMutex
	connState        api.InverterConnState
	connErr          error
	verificationErr  error                // set when inverter serial doesn't match DB
	inverterIdentity *db.InverterIdentity // stored identity from DB (may be nil)
}

// New creates a new Daemon. inverter and store may be nil; the poll loop
// is a no-op until an inverter is provided.
func New(inverter goodwe.Inverter, store *db.Store) *Daemon {
	d := &Daemon{
		inverter: inverter,
		store:    store,
	}
	if inverter == nil {
		d.connState = api.InverterStateDisabled
	} else {
		d.connState = api.InverterStateConnecting
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

// Run connects to the inverter, verifies its identity against the database,
// and starts the poll loop. It blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	if d.inverter == nil {
		slog.Info("No inverter configured, poll loop disabled")
		d.setState(api.InverterStateDisabled, nil)
		<-ctx.Done()
		return nil
	}

	slog.Info("Connecting to inverter...")
	if err := d.inverter.Connect(ctx); err != nil {
		d.setState(api.InverterStateFailed, err)
		return fmt.Errorf("inverter connect: %w", err)
	}
	slog.Info("Connected to inverter")

	// Ensure disconnection on exit.
	defer func() {
		slog.Info("Disconnecting from inverter...")
		if err := d.inverter.Close(); err != nil {
			slog.Warn("Error closing inverter connection", "error", err)
		}
	}()

	// Read inverter info and verify identity against the database.
	if err := d.verifyIdentity(ctx); err != nil {
		return fmt.Errorf("identity verification failed: %w", err)
	}

	if d.VerificationError() != nil {
		slog.Warn("Inverter identity mismatch detected; polling disabled",
			"error", d.VerificationError())
		// Set to connected but with verification error so health endpoint
		// shows the mismatch.
		d.setState(api.InverterStateConnected, nil)
		<-ctx.Done()
		return nil
	}

	d.setState(api.InverterStateConnected, nil)
	slog.Info("Daemon poll loop started")
	<-ctx.Done()
	slog.Info("Daemon poll loop stopped")
	return nil
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
		// No database; accept any identity.
		slog.Info("Inverter identity (no database)", "serial", info.SerialNumber)
		return nil
	}

	stored, err := d.store.GetInverterIdentity(ctx)
	if err != nil {
		return fmt.Errorf("read stored identity: %w", err)
	}

	if stored == nil {
		// First connection — store the identity.
		slog.Info("Storing inverter identity", "serial", info.SerialNumber, "model", info.Model)
		if err := d.store.SetInverterIdentity(ctx, info.SerialNumber, info.Model); err != nil {
			return fmt.Errorf("store inverter identity: %w", err)
		}
		d.inverterIdentity = &db.InverterIdentity{
			Serial: info.SerialNumber,
			Model:  info.Model,
		}
		return nil
	}

	// Stored identity exists — verify serial matches.
	d.inverterIdentity = stored
	if stored.Serial != info.SerialNumber {
		d.verificationErr = fmt.Errorf(
			"serial mismatch: expected %q, got %q (inverter may have been replaced)",
			stored.Serial, info.SerialNumber)
		slog.Error("Inverter serial mismatch",
			"expected", stored.Serial,
			"actual", info.SerialNumber,
		)
		return nil // non-fatal: we return the error via VerificationError()
	}

	// Serial matches — update last_seen.
	slog.Info("Inverter identity verified", "serial", info.SerialNumber)
	if err := d.store.SetInverterIdentity(ctx, info.SerialNumber, info.Model); err != nil {
		slog.Warn("Failed to update inverter last_seen", "error", err)
	}
	return nil
}

// setState updates the connection state and error atomically.
func (d *Daemon) setState(state api.InverterConnState, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connState = state
	if err != nil {
		d.connErr = err
	}
}

// Close cleans up daemon resources. Called after Run() returns.
func (d *Daemon) Close() error {
	slog.Info("Daemon resources cleaned up")
	return nil
}
