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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// InverterIdentity holds the stored identity of the inverter.
type InverterIdentity struct {
	Serial     string
	Model      string
	Firmware   string
	DSPVersion string
	ARMVersion string
	RatedPower int
	FirstSeen  time.Time
	LastSeen   time.Time
}

// Sample is a single sensor reading stored in the database.
type Sample struct {
	Value     *float64  `json:"value,omitempty"`
	ValueText *string   `json:"value_text,omitempty"`
	Unit      string    `json:"unit"`
	SampledAt time.Time `json:"sampled_at"`
}

// VoltageAnalysisCursor tracks the progress of voltage quality analysis.
// SampleRow pairs a sample with its rowid for cursor-based progress tracking.
type SampleRow struct {
	ID     int64
	Sample Sample
}

// VoltageAnalysisCursor tracks the progress of voltage quality analysis.
type VoltageAnalysisCursor struct {
	ID                    int64
	LastProcessedSampleID int64
	OngoingL1EventID      *int64
	OngoingL2EventID      *int64
	OngoingL3EventID      *int64
	LastRunAt             time.Time
}

// VoltageEvent represents a detected voltage quality event (out-of-range).
type VoltageEvent struct {
	ID              int64      `json:"id"`
	Phase           string     `json:"phase"`
	StartSampleID   int64      `json:"-"`
	StartTime       time.Time  `json:"start_time"`
	EndSampleID     *int64     `json:"-"`
	EndTime         *time.Time `json:"end_time,omitempty"`
	MinVoltage      float64    `json:"min_voltage"`
	MaxVoltage      float64    `json:"max_voltage"`
	AvgVoltage      float64    `json:"avg_voltage"`
	DurationSeconds *int       `json:"duration_seconds,omitempty"`
}

// Store provides access to the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given DSN.
// DSN format: sqlite:///path/to/file.db
func Open(dsn string) (*Store, error) {
	path, err := parseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN %q: %w", dsn, err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database %q: %w", path, err)
	}

	// Enable WAL mode for concurrent reads during writes.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Run schema migrations.
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database.
// SchemaVersion returns the current database schema version.
func (s *Store) SchemaVersion() int {
	var v int
	_ = s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
	return v
}

func (s *Store) Close() error {
	return s.db.Close()
}

// GetInverterIdentity returns the stored inverter identity, or nil if not yet set.
func (s *Store) GetInverterIdentity(ctx context.Context) (*InverterIdentity, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT serial, model, firmware, dsp_version, arm_version, rated_power, first_seen, last_seen
		 FROM inverter_identity LIMIT 1`)

	var ident InverterIdentity
	if err := row.Scan(&ident.Serial, &ident.Model, &ident.Firmware, &ident.DSPVersion,
		&ident.ARMVersion, &ident.RatedPower, &ident.FirstSeen, &ident.LastSeen); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read inverter identity: %w", err)
	}
	return &ident, nil
}

// SetInverterIdentity inserts or updates the inverter identity.
func (s *Store) SetInverterIdentity(ctx context.Context, serial, model, firmware, dspVersion, armVersion string, ratedPower int) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO inverter_identity (serial, model, firmware, dsp_version, arm_version, rated_power, first_seen, last_seen)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(serial) DO UPDATE SET
		   model = excluded.model,
		   firmware = excluded.firmware,
		   dsp_version = excluded.dsp_version,
		   arm_version = excluded.arm_version,
		   rated_power = excluded.rated_power,
		   last_seen = excluded.last_seen`,
		serial, model, firmware, dspVersion, armVersion, ratedPower, now, now)
	if err != nil {
		return fmt.Errorf("failed to set inverter identity: %w", err)
	}
	return nil
}

// InsertSample stores a single sensor reading.
func (s *Store) InsertSample(ctx context.Context, name, unit string, sampledAt time.Time, value *float64, valueText *string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sensor_samples (sensor_name, value, value_text, unit, sampled_at)
		 VALUES (?, ?, ?, ?, ?)`,
		name, value, valueText, unit, sampledAt)
	if err != nil {
		return fmt.Errorf("insert sample: %w", err)
	}
	return nil
}

// QueryRawSamples returns raw sensor samples matching the given criteria,
// most recent first (capped by limit), then reversed into ascending order.
func (s *Store) QueryRawSamples(ctx context.Context, name string, since, until time.Time, limit int) ([]Sample, error) {
	query := `SELECT value, value_text, unit, sampled_at FROM sensor_samples
		 WHERE sensor_name = ? AND sampled_at >= ? AND sampled_at <= ?
		 ORDER BY sampled_at DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, name, since, until, limit)
	if err != nil {
		return nil, fmt.Errorf("query samples: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var samples []Sample
	for rows.Next() {
		var s Sample
		if err := rows.Scan(&s.Value, &s.ValueText, &s.Unit, &s.SampledAt); err != nil {
			return nil, fmt.Errorf("scan sample: %w", err)
		}
		samples = append(samples, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	if samples == nil {
		samples = []Sample{}
	}
	// Reverse back to ascending order (query was DESC to get most recent first).
	for i, j := 0, len(samples)-1; i < j; i, j = i+1, j-1 {
		samples[i], samples[j] = samples[j], samples[i]
	}
	return samples, nil
}

// LastSampleTime returns the timestamp of the most recent sample in the database.
// Returns nil if there are no samples.
func (s *Store) LastSampleTime(ctx context.Context) (*time.Time, error) {
	row := s.db.QueryRowContext(ctx, `SELECT MAX(sampled_at) FROM sensor_samples`)
	var sampledAt sql.NullString
	if err := row.Scan(&sampledAt); err != nil {
		return nil, fmt.Errorf("query last sample time: %w", err)
	}
	if sampledAt.Valid && sampledAt.String != "" {
		// SQLite stores TIMESTAMP as TEXT; parse with RFC3339 fractional seconds.
		t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", sampledAt.String)
		if err != nil {
			// Fall back to RFC3339 parsing.
			t, err = time.Parse(time.RFC3339Nano, sampledAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse last sample time %q: %w", sampledAt.String, err)
			}
		}
		return &t, nil
	}
	return nil, nil
}

// LatestSample returns the single most recent sample for a given sensor.
// Returns nil if no samples exist.
func (s *Store) LatestSample(ctx context.Context, name string) (*Sample, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT value, value_text, unit, sampled_at FROM sensor_samples
		 WHERE sensor_name = ?
		 ORDER BY sampled_at DESC LIMIT 1`, name)

	var samp Sample
	if err := row.Scan(&samp.Value, &samp.ValueText, &samp.Unit, &samp.SampledAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("latest sample: %w", err)
	}
	return &samp, nil
}

// SampleAt returns the sample closest to (but not after) the given timestamp.
// Returns nil if no samples exist before or at the given time.
func (s *Store) SampleAt(ctx context.Context, name string, at time.Time) (*Sample, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT value, value_text, unit, sampled_at FROM sensor_samples
		 WHERE sensor_name = ? AND sampled_at <= ?
		 ORDER BY sampled_at DESC LIMIT 1`, name, at)

	var samp Sample
	if err := row.Scan(&samp.Value, &samp.ValueText, &samp.Unit, &samp.SampledAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("sample at: %w", err)
	}
	return &samp, nil
}

// PurgeBadSamples deletes sensor samples with physically impossible values.
// ratedPower is the inverter's rated power in watts, used to cap power readings.

func (s *Store) GetVoltageAnalysisCursor(ctx context.Context) (*VoltageAnalysisCursor, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, last_processed_sample_id, ongoing_l1_event_id, ongoing_l2_event_id,
		       ongoing_l3_event_id, last_run_at FROM voltage_analysis_cursor WHERE id = 1`)
	var c VoltageAnalysisCursor
	if err := row.Scan(&c.ID, &c.LastProcessedSampleID, &c.OngoingL1EventID,
		&c.OngoingL2EventID, &c.OngoingL3EventID, &c.LastRunAt); err != nil {
		return nil, fmt.Errorf("get voltage cursor: %w", err)
	}
	return &c, nil
}

func (s *Store) SaveVoltageAnalysisCursor(ctx context.Context, c *VoltageAnalysisCursor) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE voltage_analysis_cursor SET
			last_processed_sample_id = ?, ongoing_l1_event_id = ?, ongoing_l2_event_id = ?,
			ongoing_l3_event_id = ?, last_run_at = ? WHERE id = 1`,
		c.LastProcessedSampleID, c.OngoingL1EventID, c.OngoingL2EventID,
		c.OngoingL3EventID, c.LastRunAt)
	if err != nil {
		return fmt.Errorf("save voltage cursor: %w", err)
	}
	return nil
}

func (s *Store) InsertVoltageEvent(ctx context.Context, phase string, startSampleID int64, startTime time.Time, minV, maxV, avgV float64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO voltage_events (phase, start_sample_id, start_time, min_voltage, max_voltage, avg_voltage)
		VALUES (?, ?, ?, ?, ?, ?)`,
		phase, startSampleID, startTime, minV, maxV, avgV)
	if err != nil {
		return 0, fmt.Errorf("insert voltage event: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) GetVoltageEvent(ctx context.Context, eventID int64) (*VoltageEvent, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, phase, start_time FROM voltage_events WHERE id = ?`, eventID)
	var e VoltageEvent
	if err := row.Scan(&e.ID, &e.Phase, &e.StartTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get voltage event: %w", err)
	}
	return &e, nil
}

func (s *Store) CloseVoltageEvent(ctx context.Context, eventID int64, endSampleID int64, endTime time.Time, durationSec int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE voltage_events SET end_sample_id = ?, end_time = ?, duration_seconds = ? WHERE id = ?`,
		endSampleID, endTime, durationSec, eventID)
	if err != nil {
		return fmt.Errorf("close voltage event: %w", err)
	}
	return nil
}

func (s *Store) UpdateVoltageEvent(ctx context.Context, eventID int64, minV, maxV, avgV float64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE voltage_events SET min_voltage = ?, max_voltage = ?, avg_voltage = ? WHERE id = ?`,
		minV, maxV, avgV, eventID)
	if err != nil {
		return fmt.Errorf("update voltage event: %w", err)
	}
	return nil
}

func (s *Store) QueryVoltageEvents(ctx context.Context, before int64, limit int) ([]VoltageEvent, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM voltage_events`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count voltage events: %w", err)
	}
	var rows *sql.Rows
	var err error
	if before > 0 {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, phase, start_sample_id, start_time, end_sample_id, end_time,
			       min_voltage, max_voltage, avg_voltage, duration_seconds
			FROM voltage_events WHERE id < ? ORDER BY id DESC LIMIT ?`, before, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, phase, start_sample_id, start_time, end_sample_id, end_time,
			       min_voltage, max_voltage, avg_voltage, duration_seconds
			FROM voltage_events ORDER BY id DESC LIMIT ?`, limit)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("query voltage events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var events []VoltageEvent
	for rows.Next() {
		var e VoltageEvent
		if err := rows.Scan(&e.ID, &e.Phase, &e.StartSampleID, &e.StartTime,
			&e.EndSampleID, &e.EndTime, &e.MinVoltage, &e.MaxVoltage, &e.AvgVoltage, &e.DurationSeconds); err != nil {
			return nil, 0, fmt.Errorf("scan voltage event: %w", err)
		}
		events = append(events, e)
	}
	return events, total, rows.Err()
}

func (s *Store) GetNewVoltageSampleRows(ctx context.Context, sensorName string, sinceID int64) ([]SampleRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rowid, value, value_text, unit, sampled_at FROM sensor_samples
		WHERE sensor_name = ? AND rowid > ? ORDER BY rowid ASC`,
		sensorName, sinceID)
	if err != nil {
		return nil, fmt.Errorf("query voltage samples: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var samples []SampleRow
	for rows.Next() {
		var sr SampleRow
		if err := rows.Scan(&sr.ID, &sr.Sample.Value, &sr.Sample.ValueText, &sr.Sample.Unit, &sr.Sample.SampledAt); err != nil {
			return nil, fmt.Errorf("scan sample: %w", err)
		}
		samples = append(samples, sr)
	}
	return samples, rows.Err()
}
func (s *Store) PurgeBadSamples(ctx context.Context, ratedPower int) error {
	if ratedPower <= 0 {
		ratedPower = 15000 // conservative default for unknown inverters
	}
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM sensor_samples WHERE
			(unit = 'W'  AND value > ?) OR
			(unit = '%'  AND (value > 100 OR value < 0)) OR
			(unit = 'V'  AND value > 600) OR
			(unit = 'A'  AND value > 50) OR
			(unit = 'C'  AND (value > 200 OR value < -50)) OR
			(unit = 'Hz' AND (value > 60 OR value < 40)) OR
			(unit = 'kWh' AND value < 0) OR
			(unit = 'var' AND (ABS(value) > 50000)) OR
			(unit = 'VA' AND (value > 50000 OR value < 0))
	`, ratedPower*2) // allow 2x rated power as surge margin
	if err != nil {
		return fmt.Errorf("purge bad samples: %w", err)
	}
	return nil
}

// migrate creates tables if they don't exist and applies schema upgrades.
func migrate(db *sql.DB) error {
	// Check current schema version.
	var schemaVersion int
	_ = db.QueryRow(`PRAGMA user_version`).Scan(&schemaVersion)

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS inverter_identity (
			serial     TEXT PRIMARY KEY,
			model      TEXT NOT NULL DEFAULT '',
			rated_power INTEGER NOT NULL DEFAULT 0,
			first_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_seen  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS sensor_samples (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			sensor_name TEXT NOT NULL,
			value      REAL,
			value_text TEXT,
			unit       TEXT NOT NULL DEFAULT '',
			sampled_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_samples_name_time ON sensor_samples(sensor_name, sampled_at);
	`); err != nil {
		return err
	}
	// Add columns for databases created before they existed (non-fatal if already present).
	_, _ = db.Exec(`ALTER TABLE inverter_identity ADD COLUMN rated_power INTEGER NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE inverter_identity ADD COLUMN firmware TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE inverter_identity ADD COLUMN dsp_version TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE inverter_identity ADD COLUMN arm_version TEXT NOT NULL DEFAULT ''`)

	// Migrate voltage analysis cursor from nano timestamp to sample rowid.
	_, _ = db.Exec(`DROP TABLE IF EXISTS voltage_analysis_cursor`)
	_, _ = db.Exec(`DROP TABLE IF EXISTS voltage_events`)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS voltage_analysis_cursor (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			last_processed_sample_id INTEGER NOT NULL DEFAULT 0,
			ongoing_l1_event_id INTEGER,
			ongoing_l2_event_id INTEGER,
			ongoing_l3_event_id INTEGER,
			last_run_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00'
		);
		INSERT OR IGNORE INTO voltage_analysis_cursor (id, last_processed_sample_id, last_run_at) VALUES (1, 0, '1970-01-01 00:00:00');
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
		CREATE INDEX IF NOT EXISTS idx_voltage_events_phase_start ON voltage_events(phase, start_time DESC);
	`)

	// Update schema version for future migration tracking.
	if schemaVersion < 2 {
		_, _ = db.Exec(`PRAGMA user_version = 2`)
	}

	// Re-read version for logging.
	var finalVersion int
	_ = db.QueryRow(`PRAGMA user_version`).Scan(&finalVersion)
	slog.Info("Schema migration complete", "version", finalVersion)
	return nil
}

// parseDSN converts a DSN like "sqlite://~/.goodwe/goodwe.db" to a file path.
func parseDSN(dsn string) (string, error) {
	if !strings.HasPrefix(dsn, "sqlite://") {
		return "", fmt.Errorf("unsupported scheme (expected sqlite://)")
	}
	path := dsn[len("sqlite://"):]
	if path == "" {
		return "", fmt.Errorf("empty path in DSN")
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand ~: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}
	// Ensure parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("cannot create directory %q: %w", dir, err)
	}
	return path, nil
}
