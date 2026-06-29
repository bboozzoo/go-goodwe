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

// mockStore implements VoltageStore in memory for testing.
type mockStore struct {
	cursor    db.VoltageAnalysisCursor
	samples   map[string][]db.Sample
	events    []db.VoltageEvent
	idCounter int64
}

func (m *mockStore) GetVoltageAnalysisCursor(_ context.Context) (*db.VoltageAnalysisCursor, error) {
	return &m.cursor, nil
}

func (m *mockStore) SaveVoltageAnalysisCursor(_ context.Context, c *db.VoltageAnalysisCursor) error {
	m.cursor = *c
	return nil
}

func (m *mockStore) InsertVoltageEvent(_ context.Context, phase string, _ int64, startTime time.Time, minV, maxV, avgV float64) (int64, error) {
	id := int64(len(m.events) + 1)
	m.events = append(m.events, db.VoltageEvent{
		ID:         id,
		Phase:      phase,
		StartTime:  startTime,
		MinVoltage: minV,
		MaxVoltage: maxV,
		AvgVoltage: avgV,
	})
	return id, nil
}

func (m *mockStore) GetVoltageEvent(_ context.Context, eventID int64) (*db.VoltageEvent, error) {
	idx := int(eventID) - 1
	if idx < 0 || idx >= len(m.events) {
		return nil, nil
	}
	return &m.events[idx], nil
}

func (m *mockStore) CloseVoltageEvent(_ context.Context, eventID int64, _ int64, endTime time.Time, durationSec int) error {
	idx := int(eventID) - 1
	if idx < 0 || idx >= len(m.events) {
		return nil
	}
	m.events[idx].EndTime = &endTime
	m.events[idx].DurationSeconds = &durationSec
	return nil
}

func (m *mockStore) UpdateVoltageEvent(_ context.Context, eventID int64, minV, maxV, avgV float64) error {
	idx := int(eventID) - 1
	if idx < 0 || idx >= len(m.events) {
		return nil
	}
	m.events[idx].MinVoltage = minV
	m.events[idx].MaxVoltage = maxV
	m.events[idx].AvgVoltage = avgV
	return nil
}

func (m *mockStore) GetNewVoltageSampleRows(_ context.Context, sensorName string, sinceID int64) ([]db.SampleRow, error) {
	samples := m.samples[sensorName]
	var result []db.SampleRow
	for _, s := range samples {
		m.idCounter++
		if m.idCounter > sinceID {
			result = append(result, db.SampleRow{ID: m.idCounter, Sample: s})
		}
	}
	return result, nil
}

// makeSamplesAt creates samples with given values, starting from base time, spaced by step.
func makeSamplesAt(base time.Time, step time.Duration, vals ...float64) []db.Sample {
	samples := make([]db.Sample, len(vals))
	for i, v := range vals {
		val := v
		samples[i] = db.Sample{Value: &val, Unit: "V", SampledAt: base.Add(time.Duration(i) * step)}
	}
	return samples
}

// Tests

func TestNoEvents(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedSampleID: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 230, 231, 229, 230, 232)},
	}
	err := RunVoltageAnalysis(context.Background(), store)
	require.NoError(t, err)
	assert.Empty(t, store.events)
}

func TestSingleEvent(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedSampleID: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 230, 200, 200, 200, 230)},
	}
	err := RunVoltageAnalysis(context.Background(), store)
	require.NoError(t, err)
	require.Len(t, store.events, 1)

	e := store.events[0]
	assert.Equal(t, 200.0, e.MinVoltage)
	assert.Equal(t, 200.0, e.MaxVoltage)
	require.NotNil(t, e.DurationSeconds)
	assert.Greater(t, *e.DurationSeconds, 100)
	require.NotNil(t, e.EndTime, "expected event to be closed")
}

func TestMultipleEvents(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedSampleID: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 230, 200, 200, 230, 260, 260, 230)},
	}
	err := RunVoltageAnalysis(context.Background(), store)
	require.NoError(t, err)
	require.Len(t, store.events, 2)
	assert.Equal(t, 200.0, store.events[0].MinVoltage, "event 0 min")
	assert.Equal(t, 260.0, store.events[1].MinVoltage, "event 1 min")
}

func TestBoundaryInclusive(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedSampleID: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 207.0, 253.0, 230, 207.0)},
	}
	err := RunVoltageAnalysis(context.Background(), store)
	require.NoError(t, err)
	assert.Empty(t, store.events, "boundary values should not trigger events")
}

func TestOngoingResume(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)

	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedSampleID: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 230, 200, 200)},
	}
	// First run: voltage goes out of range but never comes back.
	err := RunVoltageAnalysis(context.Background(), store)
	require.NoError(t, err)
	require.Len(t, store.events, 1)
	require.NotNil(t, store.cursor.OngoingL1EventID, "OngoingL1EventID should be set")
	assert.Nil(t, store.events[0].EndTime, "event should be ongoing (no EndTime)")

	// Second run: more out-of-range samples at later timestamps, then back in range.
	store.samples["meter_voltage1"] = makeSamplesAt(base.Add(5*time.Minute), time.Minute, 200, 200, 230)
	err = RunVoltageAnalysis(context.Background(), store)
	require.NoError(t, err)
	require.Len(t, store.events, 1)
	require.NotNil(t, store.events[0].EndTime, "event should be closed after second run")
	require.NotNil(t, store.events[0].DurationSeconds, "duration should be set")
	assert.InDelta(t, 360, *store.events[0].DurationSeconds, 10, "duration should be ~6 minutes")
	assert.Equal(t, 200.0, store.events[0].MinVoltage)
}

func TestMultiplePhases(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor: db.VoltageAnalysisCursor{LastProcessedSampleID: 0, LastRunAt: base},
		samples: map[string][]db.Sample{
			"meter_voltage1": makeSamplesAt(base, time.Minute, 230, 200, 230),
			"meter_voltage2": makeSamplesAt(base, time.Minute, 230, 230, 230),
		},
	}
	err := RunVoltageAnalysis(context.Background(), store)
	require.NoError(t, err)
	require.Len(t, store.events, 1)
	assert.Equal(t, "meter_voltage1", store.events[0].Phase)
}

func TestAllOutOfRange(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedSampleID: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 200, 200, 200)},
	}
	err := RunVoltageAnalysis(context.Background(), store)
	require.NoError(t, err)
	require.Len(t, store.events, 1)
	assert.Nil(t, store.events[0].EndTime, "expected event to be ongoing (no EndTime)")
	require.NotNil(t, store.cursor.OngoingL1EventID, "OngoingL1EventID should be set")
}

func TestEmptyData(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedSampleID: 0, LastRunAt: base},
		samples: map[string][]db.Sample{},
	}
	cursorBefore := store.cursor.LastProcessedSampleID
	err := RunVoltageAnalysis(context.Background(), store)
	require.NoError(t, err)
	assert.Empty(t, store.events)
	assert.Equal(t, cursorBefore, store.cursor.LastProcessedSampleID, "cursor should be unchanged")
}
