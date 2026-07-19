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
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"strings"
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

func TestMigration_NoResetOnReopen(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// First open: migration creates tables with schema version 2.
	s1, err := Open("sqlite://" + dbPath)
	require.NoError(t, err)

	// Set a cursor value so we can detect if migration resets it.
	c, err := s1.GetVoltageAnalysisCursor(ctx)
	require.NoError(t, err)
	c.LastProcessedSampleID = 42
	require.NoError(t, s1.SaveVoltageAnalysisCursor(ctx, c))
	require.NoError(t, s1.Close())

	// Second open: migration should see schema version >= 2 and skip the DROP+CREATE.
	s2, err := Open("sqlite://" + dbPath)
	require.NoError(t, err)

	// Cursor should still be 42, not reset to 0.
	c2, err := s2.GetVoltageAnalysisCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(42), c2.LastProcessedSampleID, "cursor should not be reset on reopen")
	require.NoError(t, s2.Close())
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
	assert.Equal(t, 4, sv, "schema version should be 4")

	// Verify cursor can be updated and re-read.
	cursor.LastProcessedSampleID = 42
	err = s.SaveVoltageAnalysisCursor(ctx, cursor)
	require.NoError(t, err)

	cursor2, err := s.GetVoltageAnalysisCursor(ctx)
	require.NoError(t, err)
	require.NotNil(t, cursor2)
	assert.Equal(t, int64(42), cursor2.LastProcessedSampleID)
}

func TestAggregateHourly(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Millisecond)
	t0 := now.Truncate(time.Hour) // hour boundary

	// Insert samples for 2 sensors over 3 hours.
	// Hour 0: t0 to t0+1h
	require.NoError(t, s.InsertSample(ctx, "sensor_a", "V", t0.Add(10*time.Minute), samplePtr(230), nil))
	require.NoError(t, s.InsertSample(ctx, "sensor_a", "V", t0.Add(20*time.Minute), samplePtr(235), nil))
	require.NoError(t, s.InsertSample(ctx, "sensor_b", "W", t0.Add(15*time.Minute), samplePtr(500), nil))
	// Hour 1: t0+1h to t0+2h
	require.NoError(t, s.InsertSample(ctx, "sensor_a", "V", t0.Add(1*time.Hour+10*time.Minute), samplePtr(240), nil))
	require.NoError(t, s.InsertSample(ctx, "sensor_b", "W", t0.Add(1*time.Hour+5*time.Minute), samplePtr(600), nil))
	require.NoError(t, s.InsertSample(ctx, "sensor_b", "W", t0.Add(1*time.Hour+30*time.Minute), samplePtr(700), nil))
	// Hour 2: t0+2h to t0+3h (outside aggregation range)
	require.NoError(t, s.InsertSample(ctx, "sensor_a", "V", t0.Add(2*time.Hour+10*time.Minute), samplePtr(250), nil))

	// Aggregate hours 0 and 1 (2 hours).
	n, err := s.AggregateHourly(ctx, t0, t0.Add(2*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(4), n, "should insert 4 rows (2 sensors × 2 hours)")

	// Verify 4 rows: 2 sensors × 2 hours.
	rows, err := s.db.QueryContext(ctx, `
		SELECT sensor_name, bucket_start, value_min, value_max, value_avg, sample_count
		FROM sensor_samples_hourly
		ORDER BY sensor_name, bucket_start
	`)
	require.NoError(t, err)

	type hRow struct {
		name          string
		start         time.Time
		min, max, avg float64
		count         int
	}
	var got []hRow
	for rows.Next() {
		var r hRow
		require.NoError(t, rows.Scan(&r.name, &r.start, &r.min, &r.max, &r.avg, &r.count))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())
	_ = rows.Close()
	require.Len(t, got, 4)

	// sensor_a hour 0: 230, 235 → min=230, max=235, avg=232.5, count=2
	assert.Equal(t, "sensor_a", got[0].name)
	assert.Equal(t, t0.Unix(), got[0].start.Unix())
	assert.Equal(t, 230.0, got[0].min)
	assert.Equal(t, 235.0, got[0].max)
	assert.InDelta(t, 232.5, got[0].avg, 0.01)
	assert.Equal(t, 2, got[0].count)

	// sensor_a hour 1: 240 → min=240, max=240, avg=240, count=1
	assert.Equal(t, "sensor_a", got[1].name)
	assert.Equal(t, t0.Add(1*time.Hour).Unix(), got[1].start.Unix())
	assert.Equal(t, 240.0, got[1].min)
	assert.Equal(t, 240.0, got[1].max)
	assert.Equal(t, 240.0, got[1].avg)
	assert.Equal(t, 1, got[1].count)

	// sensor_b hour 0: 500 → min=500, max=500, avg=500, count=1
	assert.Equal(t, "sensor_b", got[2].name)
	assert.Equal(t, t0.Unix(), got[2].start.Unix())
	assert.Equal(t, 500.0, got[2].min)
	assert.Equal(t, 500.0, got[2].max)
	assert.Equal(t, 500.0, got[2].avg)
	assert.Equal(t, 1, got[2].count)

	// sensor_b hour 1: 600, 700 → min=600, max=700, avg=650, count=2
	assert.Equal(t, "sensor_b", got[3].name)
	assert.Equal(t, t0.Add(1*time.Hour).Unix(), got[3].start.Unix())
	assert.Equal(t, 600.0, got[3].min)
	assert.Equal(t, 700.0, got[3].max)
	assert.InDelta(t, 650.0, got[3].avg, 0.01)
	assert.Equal(t, 2, got[3].count)

	// Run again — idempotent: no duplicate rows.
	n2, err := s.AggregateHourly(ctx, t0, t0.Add(2*time.Hour))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n2, int64(0), "idempotent re-run returns >=0 rows affected")
	_ = n2
	var count int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sensor_samples_hourly`).Scan(&count))
	assert.Equal(t, 4, count)
}

func TestAggregateDaily(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(24 * time.Hour) // day boundary (midnight)

	// Insert hourly data spanning 2 days.
	// Day 0 (today): 2 sensors, 2 hours each
	for _, sensor := range []string{"sensor_a", "sensor_b"} {
		for hour := 0; hour < 2; hour++ {
			ts := now.Add(time.Duration(hour) * time.Hour)
			_, err := s.db.ExecContext(ctx, `
				INSERT INTO sensor_samples_hourly (sensor_name, bucket_start, value_min, value_max, value_avg, sample_count)
				VALUES (?, ?, ?, ?, ?, ?)
			`, sensor, ts, 100.0+float64(hour), 200.0+float64(hour), 150.0+float64(hour), 10*(hour+1))
			require.NoError(t, err)
		}
	}

	// Day 1 (tomorrow): 1 sensor, 1 hour
	ts := now.Add(24 * time.Hour)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sensor_samples_hourly (sensor_name, bucket_start, value_min, value_max, value_avg, sample_count)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "sensor_a", ts, 300.0, 400.0, 350.0, 5)
	require.NoError(t, err)

	// Aggregate day 0 only.
	n, err := s.AggregateDaily(ctx, now, now.Add(24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "should insert 2 rows (2 sensors × 1 day)")

	// Verify: 2 rows (2 sensors × 1 day)
	rows, err := s.db.QueryContext(ctx, `
		SELECT sensor_name, bucket_start, value_min, value_max, value_avg, sample_count
		FROM sensor_samples_daily
		ORDER BY sensor_name, bucket_start
	`)
	require.NoError(t, err)

	type dRow struct {
		name          string
		start         time.Time
		min, max, avg float64
		count         int
	}
	var got []dRow
	for rows.Next() {
		var r dRow
		require.NoError(t, rows.Scan(&r.name, &r.start, &r.min, &r.max, &r.avg, &r.count))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())
	_ = rows.Close()
	require.Len(t, got, 2)

	// sensor_a: MIN(100,101)=100, MAX(200,201)=201, AVG(150,151)=150.5, SUM(10+20)=30
	assert.Equal(t, "sensor_a", got[0].name)
	assert.Equal(t, now.Unix(), got[0].start.Unix())
	assert.Equal(t, 100.0, got[0].min)
	assert.Equal(t, 201.0, got[0].max)
	assert.InDelta(t, 150.5, got[0].avg, 0.01)
	assert.Equal(t, 30, got[0].count)

	// sensor_b: MIN(100,101)=100, MAX(200,201)=201, AVG(150,151)=150.5, SUM(10+20)=30
	assert.Equal(t, "sensor_b", got[1].name)
	assert.Equal(t, now.Unix(), got[1].start.Unix())
	assert.Equal(t, 100.0, got[1].min)
	assert.Equal(t, 201.0, got[1].max)
	assert.InDelta(t, 150.5, got[1].avg, 0.01)
	assert.Equal(t, 30, got[1].count)

	// Run again — idempotent.
	n2, err := s.AggregateDaily(ctx, now, now.Add(24*time.Hour))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n2, int64(0), "idempotent re-run returns >=0 rows affected")
	_ = n2
	var count int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sensor_samples_daily`).Scan(&count))
	assert.Equal(t, 2, count)
}

func TestAggregateProgressLogging(t *testing.T) {
	// Capture slog output to verify progress logs appear at iteration boundaries.
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	orig := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(orig)

	ctx := context.Background()
	s := newTestStore(t)

	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(250 * time.Hour)

	// Seed a sample at start so MIN(sampled_at) is defined and the
	// loop runs (the empty-source early-return would otherwise exit
	// immediately and produce no progress logs).
	require.NoError(t, s.InsertSample(ctx, "dummy", "V", start, samplePtr(1), nil))

	// Aggregation over 250 hours — progress logs should
	// appear at the 100th and 200th bucket (iter%100==0).

	_, err := s.AggregateHourly(ctx, start, end)
	require.NoError(t, err)

	output := buf.String()
	t.Log("Captured slog output:\n", output)

	// Expect progress log at iter=100 (bucket = start + 99h, since iter starts at 1).
	bucket100 := start.Add(99 * time.Hour).Format(time.RFC3339)
	assert.Contains(t, output, bucket100,
		"should have progress log at iter=100 (bucket start+99h)")

	// Expect progress log at iter=200.
	bucket200 := start.Add(199 * time.Hour).Format(time.RFC3339)
	assert.Contains(t, output, bucket200,
		"should have progress log at iter=200 (bucket start+199h)")

	// Count how many "Aggregating hourly" lines appeared.
	lines := strings.Count(output, "Aggregating hourly")
	assert.Equal(t, 2, lines, "expected exactly 2 progress lines for 250 hours")

	// rows_so_far value is no longer fixed because the seeded sample
	// at start contributes one aggregate row. No assertion here.
}

func TestAggregateHourlyEmptySourceFastReturn(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	n, err := s.AggregateHourly(ctx, time.Time{}, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	var count int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sensor_samples_hourly`).Scan(&count))
	assert.Equal(t, 0, count)
}

func TestAggregateHourlyClampsStartToEarliestData(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	t0 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC).Add(10 * time.Minute)
	require.NoError(t, s.InsertSample(ctx, "sensor_a", "V", t0, samplePtr(1), nil))

	n, err := s.AggregateHourly(ctx, time.Time{}, t0.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	var count int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sensor_samples_hourly`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestAggregateDailyEmptySourceFastReturn(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	n, err := s.AggregateDaily(ctx, time.Time{}, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	var count int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sensor_samples_daily`).Scan(&count))
	assert.Equal(t, 0, count)
}

func TestPruneSamples(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Millisecond)

	// 3 old samples (2 hours ago)
	require.NoError(t, s.InsertSample(ctx, "sensor_a", "V", now.Add(-2*time.Hour), samplePtr(100), nil))
	require.NoError(t, s.InsertSample(ctx, "sensor_a", "V", now.Add(-2*time.Hour+5*time.Minute), samplePtr(110), nil))
	require.NoError(t, s.InsertSample(ctx, "sensor_b", "W", now.Add(-2*time.Hour), samplePtr(200), nil))

	// 2 new samples (30 min ago)
	require.NoError(t, s.InsertSample(ctx, "sensor_a", "V", now.Add(-30*time.Minute), samplePtr(120), nil))
	require.NoError(t, s.InsertSample(ctx, "sensor_b", "W", now.Add(-30*time.Minute), samplePtr(300), nil))

	// Prune at midpoint: 1 hour ago
	deleted, err := s.PruneSamples(ctx, now.Add(-1*time.Hour), 10)
	require.NoError(t, err)
	assert.Equal(t, int64(3), deleted)

	// Verify: only 2 new samples remain
	samplesA, err := s.QueryRawSamples(ctx, "sensor_a", now.Add(-3*time.Hour), now, 100)
	require.NoError(t, err)
	require.Len(t, samplesA, 1)
	require.NotNil(t, samplesA[0].Value)
	assert.Equal(t, 120.0, *samplesA[0].Value)

	samplesB, err := s.QueryRawSamples(ctx, "sensor_b", now.Add(-3*time.Hour), now, 100)
	require.NoError(t, err)
	require.Len(t, samplesB, 1)
	require.NotNil(t, samplesB[0].Value)
	assert.Equal(t, 300.0, *samplesB[0].Value)
}

func TestVacuum(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Millisecond)

	// Insert a batch of old samples (2 hours ago) then delete them to leave
	// free pages. Using a fixed old timestamp and a cutoff well in the
	// middle avoids ambiguity about which rows survive the prune.
	for i := 0; i < 50; i++ {
		require.NoError(t, s.InsertSample(ctx, "sensor_a", "V",
			now.Add(-2*time.Hour).Add(time.Duration(i)*time.Minute), samplePtr(float64(i)), nil))
	}
	deleted, err := s.PruneSamples(ctx, now.Add(-1*time.Hour), 10)
	require.NoError(t, err)
	assert.Greater(t, deleted, int64(0), "prune should delete rows")

	// page_count reflects free pages from the deletes. VACUUM should not
	// error and should leave the DB usable.
	pagesBefore := pageCount(t, s)
	require.NoError(t, s.Vacuum(ctx))
	pagesAfter := pageCount(t, s)
	assert.LessOrEqual(t, pagesAfter, pagesBefore,
		"VACUUM should not increase the page count")

	// DB is still usable after VACUUM.
	require.NoError(t, s.InsertSample(ctx, "sensor_a", "V", now.Add(-30*time.Minute), samplePtr(42.0), nil))
	samples, err := s.QueryRawSamples(ctx, "sensor_a", now.Add(-time.Hour), now, 100)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.NotNil(t, samples[0].Value)
	assert.Equal(t, 42.0, *samples[0].Value)
}

func pageCount(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRow(`PRAGMA page_count`).Scan(&n))
	return n
}

func TestQueryAggregated(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC().Truncate(time.Millisecond)
	t0 := now.Truncate(time.Hour)

	// Insert hourly data for 2 sensors over 3 hours.
	for _, sensor := range []string{"sensor_a", "sensor_b"} {
		for hour := 0; hour < 3; hour++ {
			ts := t0.Add(time.Duration(hour) * time.Hour)
			_, err := s.db.ExecContext(ctx, `
				INSERT INTO sensor_samples_hourly (sensor_name, bucket_start, value_min, value_max, value_avg, sample_count)
				VALUES (?, ?, ?, ?, ?, ?)
			`, sensor, ts, 10.0+float64(hour), 20.0+float64(hour), 15.0+float64(hour), 5)
			require.NoError(t, err)
		}
	}

	// Query for sensor_a, hours 1-2 (inclusive).
	results, err := s.QueryAggregatedSamples(ctx, "sensor_a", t0.Add(1*time.Hour), t0.Add(2*time.Hour), "hour")
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Ascending order.
	assert.Equal(t, "sensor_a", results[0].SensorName)
	assert.Equal(t, t0.Add(1*time.Hour).Unix(), results[0].BucketStart.Unix())
	require.NotNil(t, results[0].ValueMin)
	assert.Equal(t, 11.0, *results[0].ValueMin)
	require.NotNil(t, results[0].ValueMax)
	assert.Equal(t, 21.0, *results[0].ValueMax)
	require.NotNil(t, results[0].ValueAvg)
	assert.Equal(t, 16.0, *results[0].ValueAvg)
	assert.Equal(t, 5, results[0].SampleCount)

	assert.Equal(t, "sensor_a", results[1].SensorName)
	assert.Equal(t, t0.Add(2*time.Hour).Unix(), results[1].BucketStart.Unix())
	assert.Equal(t, 12.0, *results[1].ValueMin)
	assert.Equal(t, 22.0, *results[1].ValueMax)
	assert.Equal(t, 17.0, *results[1].ValueAvg)
	assert.Equal(t, 5, results[1].SampleCount)

	// Query non-existent sensor — empty slice.
	empty, err := s.QueryAggregatedSamples(ctx, "nonexistent", t0, t0.Add(3*time.Hour), "hour")
	require.NoError(t, err)
	assert.Empty(t, empty)

	// Unknown bucket — error.
	_, err = s.QueryAggregatedSamples(ctx, "sensor_a", t0, t0.Add(3*time.Hour), "week")
	assert.ErrorContains(t, err, "unknown bucket")

	// Daily bucket via AggregateDaily.
	dayStart := t0.Truncate(24 * time.Hour)
	_, err = s.AggregateDaily(ctx, dayStart, dayStart.Add(24*time.Hour))
	require.NoError(t, err)
	daily, err := s.QueryAggregatedSamples(ctx, "sensor_a", dayStart, dayStart.Add(24*time.Hour), "day")
	require.NoError(t, err)
	require.Len(t, daily, 1)
	assert.Equal(t, "sensor_a", daily[0].SensorName)
	assert.Equal(t, dayStart.Unix(), daily[0].BucketStart.Unix())
	require.NotNil(t, daily[0].ValueMin)
	assert.Equal(t, 10.0, *daily[0].ValueMin)
	assert.Equal(t, 22.0, *daily[0].ValueMax)
	assert.InDelta(t, 16.0, *daily[0].ValueAvg, 0.01)
	assert.Equal(t, 15, daily[0].SampleCount)
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

	// Schema version should be 4.
	var sv int
	err = s2.db.QueryRow(`PRAGMA user_version`).Scan(&sv)
	require.NoError(t, err)
	assert.Equal(t, 4, sv, "schema version should be 4")

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

func TestMigration_v4_SamplesTimeIndexCreated(t *testing.T) {
	s := newTestStore(t)

	// Fresh store should be at schema version 4.
	var sv int
	err := s.db.QueryRow(`PRAGMA user_version`).Scan(&sv)
	require.NoError(t, err)
	assert.Equal(t, 4, sv, "fresh database should be at schema v4")

	// idx_samples_time should exist on sensor_samples(sampled_at).
	var cnt int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='sensor_samples' AND name='idx_samples_time'`).Scan(&cnt)
	require.NoError(t, err)
	assert.Equal(t, 1, cnt, "idx_samples_time index should exist")
}

func TestMigration_v4_UpgradeFromV3(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v3_upgrade.db")

	rawDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	// Create schema matching v3: sensor_samples + aggregate tables + idx_samples_name_time.
	_, err = rawDB.Exec(`
		CREATE TABLE IF NOT EXISTS sensor_samples (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			sensor_name TEXT NOT NULL,
			value      REAL,
			value_text TEXT,
			unit       TEXT NOT NULL DEFAULT '',
			sampled_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_samples_name_time ON sensor_samples(sensor_name, sampled_at);
		CREATE TABLE IF NOT EXISTS sensor_samples_hourly (
			sensor_name TEXT NOT NULL,
			bucket_start TIMESTAMP NOT NULL,
			value_min REAL,
			value_max REAL,
			value_avg REAL,
			sample_count INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (sensor_name, bucket_start)
		) WITHOUT ROWID;
		CREATE TABLE IF NOT EXISTS sensor_samples_daily (
			sensor_name TEXT NOT NULL,
			bucket_start TIMESTAMP NOT NULL,
			value_min REAL,
			value_max REAL,
			value_avg REAL,
			sample_count INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (sensor_name, bucket_start)
		) WITHOUT ROWID;
		PRAGMA user_version = 3;
	`)
	require.NoError(t, err)
	_ = rawDB.Close()

	// Open via migration — should upgrade to v4.
	s2, err := Open("sqlite://" + dbPath)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	var sv int
	err = s2.db.QueryRow(`PRAGMA user_version`).Scan(&sv)
	require.NoError(t, err)
	assert.Equal(t, 4, sv, "v3 database should be upgraded to v4")

	var cnt int
	err = s2.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='sensor_samples' AND name='idx_samples_time'`).Scan(&cnt)
	require.NoError(t, err)
	assert.Equal(t, 1, cnt, "idx_samples_time should exist after v3→v4 upgrade")
}

func TestAggregateHourlyUsesSamplesTimeIndex(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Insert a sample so the table has rows (EXPLAIN QUERY PLAN still works
	// on empty tables, but a sample makes the plan more representative).
	now := time.Now().UTC().Truncate(time.Millisecond)
	err := s.InsertSample(ctx, "battery_soc", "%", now, samplePtr(50.0), nil)
	require.NoError(t, err)

	// Run EXPLAIN QUERY PLAN on the exact query shape used by AggregateHourly.
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)
	rows, err := s.db.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT sensor_name,
		MIN(value), MAX(value), AVG(value), COUNT(*) FROM sensor_samples
		WHERE sampled_at >= ? AND sampled_at < ? AND value IS NOT NULL GROUP BY sensor_name`,
		start, end)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	foundIndex := false
	for rows.Next() {
		var id, parent, notused int
		var detail string
		err := rows.Scan(&id, &parent, &notused, &detail)
		require.NoError(t, err)
		if strings.Contains(detail, "idx_samples_time") {
			foundIndex = true
		}
	}
	require.NoError(t, rows.Err())
	assert.True(t, foundIndex, "EXPLAIN QUERY PLAN should show idx_samples_time index usage")
}

// TestQueryAggregatedWithUntilNow verifies that hourly buckets on the same
// calendar date as `until=now` are returned (not silently excluded by a
// timestamp format mismatch between bucket_start storage and the bound).
// This is the case the dashboard hits for its 7d/30d views.
func TestQueryAggregatedWithUntilNow(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)

	// Sample 2 hours into today and 3 days ago.
	require.NoError(t, s.InsertSample(ctx, "sensor_a", "V",
		today.Add(2*time.Hour), samplePtr(42.0), nil))
	require.NoError(t, s.InsertSample(ctx, "sensor_a", "V",
		today.Add(-3*24*time.Hour).Add(6*time.Hour), samplePtr(100.0), nil))

	_, err := s.AggregateHourly(ctx, time.Time{}, now)
	require.NoError(t, err)
	_, err = s.AggregateDaily(ctx, time.Time{}, now)
	require.NoError(t, err)

	since := now.Add(-7 * 24 * time.Hour)
	until := now // same date as today's hourly bucket

	// Hourly: must include today's bucket.
	hourly, err := s.QueryAggregatedSamples(ctx, "sensor_a", since, until, "hour")
	require.NoError(t, err)
	require.NotEmpty(t, hourly, "hourly query with until=now must return rows")
	foundToday := false
	for _, a := range hourly {
		if a.BucketStart.Truncate(24*time.Hour).Equal(today) {
			foundToday = true
			break
		}
	}
	assert.True(t, foundToday, "today's hourly bucket must be returned when until=now")

	// Daily: previous completed days only (today is incomplete, not aggregated).
	daily, err := s.QueryAggregatedSamples(ctx, "sensor_a", since, until, "day")
	require.NoError(t, err)
	assert.Len(t, daily, 1, "should have 1 daily bucket for the previous completed day")
}
