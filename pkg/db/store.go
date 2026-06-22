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

// PurgeBadSamples deletes sensor samples with physically impossible values.
// ratedPower is the inverter's rated power in watts, used to cap power readings.
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
