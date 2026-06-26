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
)

// mockStore implements VoltageStore in memory for testing.
type mockStore struct {
	cursor  db.VoltageAnalysisCursor
	samples map[string][]db.Sample
	events  []db.VoltageEvent
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

func (m *mockStore) GetNewVoltageSamplesForSensor(_ context.Context, sensorName string, sinceNano int64) ([]db.Sample, error) {
	samples := m.samples[sensorName]
	var result []db.Sample
	for _, s := range samples {
		if s.SampledAt.UnixNano() > sinceNano {
			result = append(result, s)
		}
	}
	return result, nil
}

// helpers

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
		cursor:  db.VoltageAnalysisCursor{LastProcessedNano: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 230, 231, 229, 230, 232)},
	}
	if err := RunVoltageAnalysis(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(store.events))
	}
}

func TestSingleEvent(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedNano: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 230, 200, 200, 200, 230)},
	}
	if err := RunVoltageAnalysis(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.events))
	}
	e := store.events[0]
	if e.MinVoltage != 200.0 {
		t.Fatalf("expected min 200.0, got %f", e.MinVoltage)
	}
	if e.MaxVoltage != 200.0 {
		t.Fatalf("expected max 200.0, got %f", e.MaxVoltage)
	}
	if e.DurationSeconds == nil || *e.DurationSeconds < 100 {
		t.Fatalf("expected duration > 100s, got %v", e.DurationSeconds)
	}
	if e.EndTime == nil {
		t.Fatal("expected event to be closed")
	}
}

func TestMultipleEvents(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedNano: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 230, 200, 200, 230, 260, 260, 230)},
	}
	if err := RunVoltageAnalysis(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(store.events))
	}
	if store.events[0].MinVoltage != 200.0 {
		t.Fatalf("event 0: expected min 200.0, got %f", store.events[0].MinVoltage)
	}
	if store.events[1].MinVoltage != 260.0 {
		t.Fatalf("event 1: expected min 260.0, got %f", store.events[1].MinVoltage)
	}
}

func TestBoundaryInclusive(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedNano: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 207.0, 253.0, 230, 207.0)},
	}
	if err := RunVoltageAnalysis(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 0 {
		t.Fatalf("expected 0 events for boundary values, got %d", len(store.events))
	}
}

func TestOngoingResume(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)

	// Event starting in first run, completing in second run.
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedNano: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 230, 200, 200)},
	}
	// First run: voltage goes out of range but never comes back → ongoing event.
	if err := RunVoltageAnalysis(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected 1 event after first run, got %d", len(store.events))
	}
	if store.cursor.OngoingL1EventID == nil {
		t.Fatal("OngoingL1EventID should be set")
	}
	if store.events[0].EndTime != nil {
		t.Fatal("expected event to be ongoing (no EndTime)")
	}

	// Second run: more out-of-range samples at later timestamps, then back in range.
	store.samples["meter_voltage1"] = makeSamplesAt(base.Add(5*time.Minute), time.Minute, 200, 200, 230)
	if err := RunVoltageAnalysis(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected 1 event after second run, got %d", len(store.events))
	}
	if store.events[0].EndTime == nil {
		t.Fatal("expected event to be closed after second run")
	}
	if store.events[0].MinVoltage != 200.0 {
		t.Fatalf("expected min 200.0, got %f", store.events[0].MinVoltage)
	}
}

func TestMultiplePhases(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor: db.VoltageAnalysisCursor{LastProcessedNano: 0, LastRunAt: base},
		samples: map[string][]db.Sample{
			"meter_voltage1": makeSamplesAt(base, time.Minute, 230, 200, 230),
			"meter_voltage2": makeSamplesAt(base, time.Minute, 230, 230, 230),
		},
	}
	if err := RunVoltageAnalysis(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected 1 event (L1), got %d", len(store.events))
	}
	if store.events[0].Phase != "meter_voltage1" {
		t.Fatalf("expected event on meter_voltage1, got %s", store.events[0].Phase)
	}
}

func TestAllOutOfRange(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedNano: 0, LastRunAt: base},
		samples: map[string][]db.Sample{"meter_voltage1": makeSamplesAt(base, time.Minute, 200, 200, 200)},
	}
	if err := RunVoltageAnalysis(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.events))
	}
	if store.events[0].EndTime != nil {
		t.Fatal("expected event to be ongoing (no EndTime)")
	}
	if store.cursor.OngoingL1EventID == nil {
		t.Fatal("expected OngoingL1EventID to be set")
	}
}

func TestEmptyData(t *testing.T) {
	base := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	store := &mockStore{
		cursor:  db.VoltageAnalysisCursor{LastProcessedNano: 0, LastRunAt: base},
		samples: map[string][]db.Sample{},
	}
	cursorBefore := store.cursor.LastProcessedNano
	if err := RunVoltageAnalysis(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(store.events))
	}
	if store.cursor.LastProcessedNano != cursorBefore {
		t.Fatal("expected cursor to be unchanged")
	}
}
