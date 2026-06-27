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

package analysis

import (
	"context"
	"testing"
	"time"

	"github.com/bboozzoo/go-goodwe/pkg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiDayAnalysis(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open("sqlite://" + t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)

	// Day 1: insert 1440 samples (1 sample/min for 24h) with two voltage events.
	day1Start := base
	for i := 0; i < 1440; i++ {
		ts := day1Start.Add(time.Duration(i) * time.Minute)
		v := 230.0
		if i >= 100 && i < 110 {
			v = 200.0 // 10 min out of range (below 207V)
		}
		if i >= 500 && i < 505 {
			v = 260.0 // 5 min out of range (above 253V)
		}
		val := v
		err := store.InsertSample(ctx, "meter_voltage1", "V", ts, &val, nil)
		require.NoError(t, err)
	}

	// Run 1: process day 1.
	err = RunVoltageAnalysis(ctx, store)
	require.NoError(t, err)

	events, total, err := store.QueryVoltageEvents(ctx, 0, 100)
	require.NoError(t, err)
	require.Equal(t, 2, total, "day 1 should have 2 events")
	t.Logf("Day 1 events: %d (IDs: %d, %d)", total, events[0].ID, events[1].ID)

	// Verify cursor advanced past day 1 samples.
	cursor, err := store.GetVoltageAnalysisCursor(ctx)
	require.NoError(t, err)
	require.Greater(t, cursor.LastProcessedSampleID, int64(0), "cursor should advance past day 1")
	t.Logf("Cursor after day 1: %d", cursor.LastProcessedSampleID)

	// Day 2: insert 1440 more samples with one voltage event.
	day2Start := base.Add(24 * time.Hour)
	for i := 0; i < 1440; i++ {
		ts := day2Start.Add(time.Duration(i) * time.Minute)
		v := 230.0
		if i >= 200 && i < 215 {
			v = 205.0 // 15 min out of range (below 207V)
		}
		val := v
		err := store.InsertSample(ctx, "meter_voltage1", "V", ts, &val, nil)
		require.NoError(t, err)
	}

	// Run 2: process day 2.
	err = RunVoltageAnalysis(ctx, store)
	require.NoError(t, err)

	events, total, err = store.QueryVoltageEvents(ctx, 0, 100)
	require.NoError(t, err)
	require.Equal(t, 3, total, "should have 3 total events across 2 days")
	t.Logf("Total events after day 2: %d (IDs: %d, %d, %d)", total, events[0].ID, events[1].ID, events[2].ID)

	// Verify event ordering: newest first (highest ID first).
	if len(events) >= 2 {
		assert.Greater(t, events[0].ID, events[1].ID, "newest event first")
	}

	// Run 3: no new data — should not create new events or error.
	err = RunVoltageAnalysis(ctx, store)
	require.NoError(t, err)
	_, total, err = store.QueryVoltageEvents(ctx, 0, 100)
	require.NoError(t, err)
	require.Equal(t, 3, total, "no new events without new data")
}
