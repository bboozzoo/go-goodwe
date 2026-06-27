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

package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	// Use a unique temp directory so parallel tests don't collide.
	store, err := Open("sqlite://" + filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func samplePtr(v float64) *float64 { return &v }
func textPtr(v string) *string     { return &v }

func TestInsertAndQuerySamples(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Millisecond)

	// Insert a numeric sample.
	err := s.InsertSample(ctx, "battery_soc", "%", now, samplePtr(60.5), nil)
	require.NoError(t, err)

	// Insert a string/label sample.
	err = s.InsertSample(ctx, "work_mode_label", "", now, nil, textPtr("Normal (On-Grid)"))
	require.NoError(t, err)

	// Insert another numeric sample with a different name.
	err = s.InsertSample(ctx, "ppv", "W", now.Add(-time.Hour), samplePtr(1200), nil)
	require.NoError(t, err)

	// Query by name.
	t.Run("query by name", func(t *testing.T) {
		samples, err := s.QueryRawSamples(ctx, "battery_soc", now.Add(-time.Hour), now.Add(time.Hour), 100)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "%", samples[0].Unit)
		require.NotNil(t, samples[0].Value)
		assert.Equal(t, 60.5, *samples[0].Value)
		assert.Nil(t, samples[0].ValueText)
	})

	t.Run("query string sample", func(t *testing.T) {
		samples, err := s.QueryRawSamples(ctx, "work_mode_label", now.Add(-time.Hour), now.Add(time.Hour), 100)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		require.NotNil(t, samples[0].ValueText)
		assert.Equal(t, "Normal (On-Grid)", *samples[0].ValueText)
		assert.Nil(t, samples[0].Value)
	})

	t.Run("query with time range", func(t *testing.T) {
		// ppv was inserted 1 hour ago — query only the last 30 minutes.
		samples, err := s.QueryRawSamples(ctx, "ppv", now.Add(-30*time.Minute), now, 100)
		require.NoError(t, err)
		assert.Len(t, samples, 0)
	})

	t.Run("query with limit", func(t *testing.T) {
		// Insert a few more.
		for i := 0; i < 5; i++ {
			err := s.InsertSample(ctx, "battery_soc", "%", now.Add(time.Duration(i)*time.Second), samplePtr(float64(60+i)), nil)
			require.NoError(t, err)
		}
		samples, err := s.QueryRawSamples(ctx, "battery_soc", now, now.Add(time.Hour), 2)
		require.NoError(t, err)
		assert.Len(t, samples, 2)
	})

	t.Run("empty table returns empty slice", func(t *testing.T) {
		samples, err := s.QueryRawSamples(ctx, "nonexistent", time.Now().Add(-24*time.Hour), time.Now(), 100)
		require.NoError(t, err)
		assert.Empty(t, samples)
	})

	t.Run("query returns most recent samples when limited", func(t *testing.T) {
		// Insert samples with increasing timestamps.
		base := now.Add(-10 * time.Second)
		for i := 0; i < 5; i++ {
			err := s.InsertSample(ctx, "recent_test", "W", base.Add(time.Duration(i)*time.Second), samplePtr(float64(100+i)), nil)
			require.NoError(t, err)
		}

		// Query with limit smaller than total — should return the most recent ones.
		samples, err := s.QueryRawSamples(ctx, "recent_test", base, base.Add(10*time.Second), 3)
		require.NoError(t, err)
		require.Len(t, samples, 3)

		// They should be the 3 most recent samples, in ascending order.
		require.NotNil(t, samples[0].Value)
		require.NotNil(t, samples[1].Value)
		require.NotNil(t, samples[2].Value)
		assert.Equal(t, 102.0, *samples[0].Value, "oldest of the 3 most recent")
		assert.Equal(t, 103.0, *samples[1].Value)
		assert.Equal(t, 104.0, *samples[2].Value, "most recent")

		// Timestamps must also be in ascending order.
		assert.True(t, samples[0].SampledAt.Before(samples[1].SampledAt))
		assert.True(t, samples[1].SampledAt.Before(samples[2].SampledAt))
	})
}

func TestLatestSample(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Millisecond)

	// No samples yet.
	sample, err := s.LatestSample(ctx, "battery_soc")
	require.NoError(t, err)
	assert.Nil(t, sample)

	// Insert three samples with different times.
	_ = s.InsertSample(ctx, "battery_soc", "%", now.Add(-2*time.Hour), samplePtr(50), nil)
	_ = s.InsertSample(ctx, "battery_soc", "%", now.Add(-1*time.Hour), samplePtr(55), nil)
	_ = s.InsertSample(ctx, "battery_soc", "%", now, samplePtr(60), nil)

	sample, err = s.LatestSample(ctx, "battery_soc")
	require.NoError(t, err)
	require.NotNil(t, sample)
	require.NotNil(t, sample.Value)
	assert.Equal(t, 60.0, *sample.Value)
	assert.Equal(t, "%", sample.Unit)
}

func TestLastSampleTime(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Empty.
	last, err := s.LastSampleTime(ctx)
	require.NoError(t, err)
	assert.Nil(t, last)

	now := time.Now().UTC().Truncate(time.Millisecond)

	_ = s.InsertSample(ctx, "a", "V", now.Add(-time.Hour), samplePtr(230), nil)
	_ = s.InsertSample(ctx, "b", "W", now, samplePtr(500), nil)

	last, err = s.LastSampleTime(ctx)
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.Equal(t, now.Unix(), last.Unix())
}

func TestPurgeBadSamples(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC()
	ratedPower := 15000 // matches GW15K-ET

	type sample struct {
		name  string
		value float64
		unit  string
		keep  bool // should the sample survive purging?
	}

	cases := []sample{
		// W — keep below 2x rated, delete above
		{name: "ppv", value: 1000, unit: "W", keep: true},
		{name: "ppv_bad", value: 999999, unit: "W", keep: false},
		{name: "ppv_edge", value: 30000, unit: "W", keep: true}, // 2*15000 = 30000, not >
		// % — 0-100 only
		{name: "soc_good", value: 50, unit: "%", keep: true},
		{name: "soc_high", value: 150, unit: "%", keep: false},
		{name: "soc_neg", value: -10, unit: "%", keep: false},
		// V — max 600
		{name: "v_grid", value: 230, unit: "V", keep: true},
		{name: "v_bad", value: 1000, unit: "V", keep: false},
		// A — max 50
		{name: "i_good", value: 10, unit: "A", keep: true},
		{name: "i_bad", value: 100, unit: "A", keep: false},
		// C — -50 to 200
		{name: "t_good", value: 25, unit: "C", keep: true},
		{name: "t_hot", value: 999, unit: "C", keep: false},
		{name: "t_cold", value: -100, unit: "C", keep: false},
		// Hz — 40-60
		{name: "f_good", value: 50, unit: "Hz", keep: true},
		{name: "f_bad", value: 999, unit: "Hz", keep: false},
		// kWh — no negative
		{name: "e_good", value: 1000, unit: "kWh", keep: true},
		{name: "e_bad", value: -5, unit: "kWh", keep: false},
		// var — abs <= 50000
		{name: "var_good", value: 1000, unit: "var", keep: true},
		{name: "var_big", value: 99999, unit: "var", keep: false},
		// VA — 0-50000
		{name: "va_good", value: 5000, unit: "VA", keep: true},
		{name: "va_bad", value: 99999, unit: "VA", keep: false},
	}

	for _, c := range cases {
		err := s.InsertSample(ctx, c.name, c.unit, now, samplePtr(c.value), nil)
		require.NoError(t, err)
	}

	err := s.PurgeBadSamples(ctx, ratedPower)
	require.NoError(t, err)

	for _, c := range cases {
		samples, err := s.QueryRawSamples(ctx, c.name, now.Add(-time.Hour), now.Add(time.Hour), 1)
		require.NoError(t, err)
		if c.keep {
			assert.Len(t, samples, 1, "expected %s to be kept", c.name)
		} else {
			assert.Empty(t, samples, "expected %s to be purged", c.name)
		}
	}
}

func TestInverterIdentityCRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Empty.
	ident, err := s.GetInverterIdentity(ctx)
	require.NoError(t, err)
	assert.Nil(t, ident)

	// Set.
	err = s.SetInverterIdentity(ctx, "SERIAL001", "GW15K-ET", "04062-07-S00", "07", "13", 15000)
	require.NoError(t, err)

	// Read back.
	ident, err = s.GetInverterIdentity(ctx)
	require.NoError(t, err)
	require.NotNil(t, ident)
	assert.Equal(t, "SERIAL001", ident.Serial)
	assert.Equal(t, "GW15K-ET", ident.Model)
	assert.Equal(t, "04062-07-S00", ident.Firmware)
	assert.Equal(t, "07", ident.DSPVersion)
	assert.Equal(t, "13", ident.ARMVersion)
	assert.Equal(t, 15000, ident.RatedPower)
	assert.False(t, ident.FirstSeen.IsZero())
	assert.False(t, ident.LastSeen.IsZero())

	// FirstSeen should equal LastSeen on first insert.
	assert.Equal(t, ident.FirstSeen.Unix(), ident.LastSeen.Unix())

	// Update with same serial.
	err = s.SetInverterIdentity(ctx, "SERIAL001", "GW15K-ET20", "new-fw", "08", "14", 12000)
	require.NoError(t, err)

	ident, err = s.GetInverterIdentity(ctx)
	require.NoError(t, err)
	require.NotNil(t, ident)
	assert.Equal(t, "SERIAL001", ident.Serial)            // unchanged
	assert.Equal(t, "GW15K-ET20", ident.Model)            // updated
	assert.Equal(t, "new-fw", ident.Firmware)             // updated
	assert.Equal(t, "08", ident.DSPVersion)               // updated
	assert.Equal(t, "14", ident.ARMVersion)               // updated
	assert.Equal(t, 12000, ident.RatedPower)              // updated
	assert.True(t, ident.LastSeen.After(ident.FirstSeen)) // last_seen moved forward
}

func TestParseDSN(t *testing.T) {
	// Valid absolute path.
	path, err := parseDSN("sqlite:///tmp/goodwe.db")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/goodwe.db", path)

	// Relative path (no ~).
	path, err = parseDSN("sqlite://data/history.db")
	require.NoError(t, err)
	assert.Equal(t, "data/history.db", path)

	// Empty path.
	_, err = parseDSN("sqlite://")
	assert.ErrorContains(t, err, "empty path")

	// Wrong scheme.
	_, err = parseDSN("postgres://host/db")
	assert.ErrorContains(t, err, "unsupported scheme")
}

func TestConcurrentReadWrite(t *testing.T) {
	// Basic sanity: write and read concurrently don't crash.
	ctx := context.Background()
	s := newTestStore(t)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			_ = s.InsertSample(ctx, "test", "V", time.Now(), samplePtr(float64(i)), nil)
		}
		close(done)
	}()

	for i := 0; i < 20; i++ {
		_, _ = s.QueryRawSamples(ctx, "test", time.Now().Add(-time.Hour), time.Now(), 1000)
	}
	<-done
}

func TestLatestSampleNoData(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	sample, err := s.LatestSample(ctx, "does_not_exist")
	require.NoError(t, err)
	assert.Nil(t, sample)
}

func TestSampleAt(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Millisecond)

	// No samples yet.
	sample, err := s.SampleAt(ctx, "battery_soc", now)
	require.NoError(t, err)
	assert.Nil(t, sample)

	// Insert three samples.
	_ = s.InsertSample(ctx, "battery_soc", "%", now.Add(-2*time.Hour), samplePtr(50), nil)
	_ = s.InsertSample(ctx, "battery_soc", "%", now.Add(-1*time.Hour), samplePtr(55), nil)
	_ = s.InsertSample(ctx, "battery_soc", "%", now, samplePtr(60), nil)

	// Sample at a time before all samples → nil.
	sample, err = s.SampleAt(ctx, "battery_soc", now.Add(-3*time.Hour))
	require.NoError(t, err)
	assert.Nil(t, sample)

	// Sample between first and second → returns the first (closest <= at).
	sample, err = s.SampleAt(ctx, "battery_soc", now.Add(-90*time.Minute))
	require.NoError(t, err)
	require.NotNil(t, sample)
	require.NotNil(t, sample.Value)
	assert.Equal(t, 50.0, *sample.Value)

	// Sample exactly at second timestamp → returns the second.
	sample, err = s.SampleAt(ctx, "battery_soc", now.Add(-1*time.Hour))
	require.NoError(t, err)
	require.NotNil(t, sample)
	require.NotNil(t, sample.Value)
	assert.Equal(t, 55.0, *sample.Value)

	// Sample after all → returns the latest.
	sample, err = s.SampleAt(ctx, "battery_soc", now.Add(time.Hour))
	require.NoError(t, err)
	require.NotNil(t, sample)
	require.NotNil(t, sample.Value)
	assert.Equal(t, 60.0, *sample.Value)
}

func TestStringValueRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Millisecond)
	err := s.InsertSample(ctx, "test_string", "", now, nil, textPtr("hello world"))
	require.NoError(t, err)

	samples, err := s.QueryRawSamples(ctx, "test_string", now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.NotNil(t, samples[0].ValueText)
	assert.Equal(t, "hello world", *samples[0].ValueText)
	assert.Nil(t, samples[0].Value)
}

func TestMigration_VoltageTablesCreated(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Verify the voltage_analysis_cursor was created with initial row.
	cursor, err := s.GetVoltageAnalysisCursor(ctx)
	require.NoError(t, err)
	require.NotNil(t, cursor)
	assert.Equal(t, int64(1), cursor.ID)
	assert.Equal(t, int64(0), cursor.LastProcessedSampleID)
	assert.Nil(t, cursor.OngoingL1EventID)
	assert.Nil(t, cursor.OngoingL2EventID)
	assert.Nil(t, cursor.OngoingL3EventID)

	// Verify voltage_events table exists and is queryable.
	events, total, err := s.QueryVoltageEvents(ctx, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, events)
	assert.Equal(t, 0, total)

	// Verify schema version is set.
	var sv int
	err = s.db.QueryRow(`PRAGMA user_version`).Scan(&sv)
	require.NoError(t, err)
	assert.Equal(t, 2, sv, "schema version should be 2")

	// Verify cursor can be updated and re-read.
	cursor.LastProcessedSampleID = 42
	err = s.SaveVoltageAnalysisCursor(ctx, cursor)
	require.NoError(t, err)

	cursor2, err := s.GetVoltageAnalysisCursor(ctx)
	require.NoError(t, err)
	require.NotNil(t, cursor2)
	assert.Equal(t, int64(42), cursor2.LastProcessedSampleID)
}

func TestMigration_OldSchemaUpgrade(t *testing.T) {
	// Create a database with the OLD schema (last_processed_nano), close it,
	// then open it again — the migration should upgrade it.
	dbPath := filepath.Join(t.TempDir(), "old_schema.db")

	rawDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = rawDB.Exec(`
		CREATE TABLE IF NOT EXISTS voltage_analysis_cursor (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			last_processed_nano INTEGER NOT NULL DEFAULT 0,
			ongoing_l1_event_id INTEGER,
			ongoing_l2_event_id INTEGER,
			ongoing_l3_event_id INTEGER,
			last_run_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00'
		);
		INSERT OR IGNORE INTO voltage_analysis_cursor (id, last_processed_nano, last_run_at) VALUES (1, 42, '2026-06-27 00:00:00');
		CREATE TABLE IF NOT EXISTS voltage_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			phase TEXT NOT NULL,
			start_sample_id INTEGER NOT NULL DEFAULT 0,
			start_time TIMESTAMP NOT NULL,
			end_sample_id INTEGER,
			end_time TIMESTAMP,
			min_voltage REAL NOT NULL,
			max_voltage REAL NOT NULL,
			avg_voltage REAL NOT NULL,
			duration_seconds INTEGER
		);
		INSERT INTO voltage_events (phase, start_sample_id, start_time, min_voltage, max_voltage, avg_voltage)
		VALUES ('meter_voltage1', 1, '2026-06-27 00:00:00', 200.0, 200.0, 200.0);
	`)
	require.NoError(t, err)
	_ = rawDB.Close()

	// Open via migration — should drop old tables and recreate with new schema.
	s2, err := Open("sqlite://" + dbPath)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	ctx := context.Background()

	// Schema version should be 2.
	var sv int
	err = s2.db.QueryRow(`PRAGMA user_version`).Scan(&sv)
	require.NoError(t, err)
	assert.Equal(t, 2, sv, "schema version should be 2")

	// Cursor should be reset.
	cursor, err := s2.GetVoltageAnalysisCursor(ctx)
	require.NoError(t, err)
	require.NotNil(t, cursor)
	assert.Equal(t, int64(0), cursor.LastProcessedSampleID, "cursor should be reset to 0")

	// Old voltage events should be gone.
	events, total, err := s2.QueryVoltageEvents(ctx, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, events)
	assert.Equal(t, 0, total, "old events should be deleted")

	// New cursor should accept the new column name.
	cursor.LastProcessedSampleID = 7
	err = s2.SaveVoltageAnalysisCursor(ctx, cursor)
	require.NoError(t, err)

	cursor2, err := s2.GetVoltageAnalysisCursor(ctx)
	require.NoError(t, err)
	require.NotNil(t, cursor2)
	assert.Equal(t, int64(7), cursor2.LastProcessedSampleID)
}
