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
	"log/slog"

	"github.com/bboozzoo/go-goodwe"
)

// Daemon manages the poll loop and aggregation scheduler.
type Daemon struct {
	inverter goodwe.Inverter // may be nil when no inverter is configured
}

// New creates a new Daemon. inverter may be nil; the poll loop is a no-op
// until an inverter is provided.
func New(inverter goodwe.Inverter) *Daemon {
	return &Daemon{inverter: inverter}
}

// Run connects to the inverter and starts the poll loop.
// It blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	if d.inverter == nil {
		slog.Info("No inverter configured, poll loop disabled")
		<-ctx.Done()
		return nil
	}

	slog.Info("Connecting to inverter...")
	if err := d.inverter.Connect(ctx); err != nil {
		return err
	}
	slog.Info("Connected to inverter")

	go func() {
		<-ctx.Done()
		slog.Info("Disconnecting from inverter...")
		if err := d.inverter.Close(); err != nil {
			slog.Warn("Error closing inverter connection", "error", err)
		}
	}()

	slog.Info("Daemon poll loop started")
	<-ctx.Done()
	slog.Info("Daemon poll loop stopped")
	return nil
}

// Close cleans up daemon resources. Called after Run() returns.
func (d *Daemon) Close() error {
	slog.Info("Daemon resources cleaned up")
	return nil
}
