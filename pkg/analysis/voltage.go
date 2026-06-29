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

// Package analysis provides voltage quality analysis for the inverter's meter readings.
package analysis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bboozzoo/go-goodwe/pkg/db"
)

const (
	// VoltageMin is the lower bound per PN-IEC 60038: 230V - 10%.
	VoltageMin = 207.0
	// VoltageMax is the upper bound per PN-IEC 60038: 230V + 10%.
	VoltageMax = 253.0
)

// VoltageStore is the interface the analysis engine needs from the database.
type VoltageStore interface {
	GetVoltageAnalysisCursor(ctx context.Context) (*db.VoltageAnalysisCursor, error)
	SaveVoltageAnalysisCursor(ctx context.Context, c *db.VoltageAnalysisCursor) error
	InsertVoltageEvent(ctx context.Context, phase string, startSampleID int64, startTime time.Time, minV, maxV, avgV float64) (int64, error)
	CloseVoltageEvent(ctx context.Context, eventID int64, endSampleID int64, endTime time.Time, durationSec int) error
	UpdateVoltageEvent(ctx context.Context, eventID int64, minV, maxV, avgV float64) error
	GetVoltageEvent(ctx context.Context, eventID int64) (*db.VoltageEvent, error)
	GetNewVoltageSampleRows(ctx context.Context, sensorName string, sinceID int64) ([]db.SampleRow, error)
}

// phaseState tracks the current analysis state for one electrical phase.
type phaseState struct {
	eventID   *int64
	startTime time.Time
	minV      float64
	maxV      float64
	sumV      float64
	count     int
}

// RunVoltageAnalysis scans new sensor samples for meter_voltage1/2/3 and
// detects events where voltage is outside the 207V–253V range.
func RunVoltageAnalysis(ctx context.Context, store VoltageStore) error {
	cursor, err := store.GetVoltageAnalysisCursor(ctx)
	if err != nil {
		return fmt.Errorf("get cursor: %w", err)
	}

	slog.Info("Voltage analysis starting",
		"last_processed_sample_id", cursor.LastProcessedSampleID,
		"ongoing_l1", cursor.OngoingL1EventID != nil,
		"ongoing_l2", cursor.OngoingL2EventID != nil,
		"ongoing_l3", cursor.OngoingL3EventID != nil)

	sensors := []string{"meter_voltage1", "meter_voltage2", "meter_voltage3"}
	ongoingPtrs := []**int64{&cursor.OngoingL1EventID, &cursor.OngoingL2EventID, &cursor.OngoingL3EventID}
	maxSeenID := cursor.LastProcessedSampleID

	for i, sensor := range sensors {
		samples, err := store.GetNewVoltageSampleRows(ctx, sensor, cursor.LastProcessedSampleID)
		if err != nil {
			return fmt.Errorf("get samples for %s: %w", sensor, err)
		}

		slog.Info("Voltage analysis sensor", "sensor", sensor, "samples", len(samples))

		var state phaseState

		// Resume ongoing event from previous run.
		if *ongoingPtrs[i] != nil {
			state.eventID = *ongoingPtrs[i]
			evt, err := store.GetVoltageEvent(ctx, *state.eventID)
			if err != nil {
				return fmt.Errorf("get event %d: %w", *state.eventID, err)
			}
			if evt != nil {
				state.startTime = evt.StartTime
			}
		}

		for _, s := range samples {
			// Track progress via rowid.
			if s.ID > maxSeenID {
				maxSeenID = s.ID
			}
			if s.Sample.Value == nil {
				continue
			}
			v := *s.Sample.Value
			isOut := v < VoltageMin || v > VoltageMax

			if isOut {
				if state.eventID == nil {
					// Start new event.
					state.startTime = s.Sample.SampledAt
					state.minV = v
					state.maxV = v
					state.sumV = v
					state.count = 1
					eid, err := store.InsertVoltageEvent(ctx, sensor, 0, state.startTime, v, v, v)
					if err != nil {
						return fmt.Errorf("insert event: %w", err)
					}
					state.eventID = &eid
				} else {
					// Accumulate into ongoing event.
					if state.count == 0 {
						// First sample after create or resume — initialize from this sample.
						state.minV = v
						state.maxV = v
						state.sumV = v
						state.count = 1
					} else {
						if v < state.minV {
							state.minV = v
						}
						if v > state.maxV {
							state.maxV = v
						}
						state.sumV += v
						state.count++
					}
					avg := state.sumV / float64(state.count)
					if err := store.UpdateVoltageEvent(ctx, *state.eventID, state.minV, state.maxV, avg); err != nil {
						return fmt.Errorf("update event: %w", err)
					}
				}
			} else {
				// Voltage back in range.
				if state.eventID != nil {
					durSec := int(s.Sample.SampledAt.Sub(state.startTime).Seconds())
					if err := store.CloseVoltageEvent(ctx, *state.eventID, 0, s.Sample.SampledAt, durSec); err != nil {
						return fmt.Errorf("close event: %w", err)
					}
					state.eventID = nil
				}
			}
		}

		// If event is still ongoing, keep it for the next run.
		if state.eventID != nil {
			*ongoingPtrs[i] = state.eventID
		} else {
			*ongoingPtrs[i] = nil
		}
	}

	cursor.LastProcessedSampleID = maxSeenID
	cursor.LastRunAt = time.Now().UTC()
	slog.Info("Voltage analysis complete", "new_cursor", cursor.LastProcessedSampleID)
	return store.SaveVoltageAnalysisCursor(ctx, cursor)
}
