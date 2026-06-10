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
	Serial    string
	Model     string
	FirstSeen time.Time
	LastSeen  time.Time
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
		`SELECT serial, model, first_seen, last_seen FROM inverter_identity LIMIT 1`)

	var ident InverterIdentity
	if err := row.Scan(&ident.Serial, &ident.Model, &ident.FirstSeen, &ident.LastSeen); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read inverter identity: %w", err)
	}
	return &ident, nil
}

// SetInverterIdentity inserts or updates the inverter identity.
func (s *Store) SetInverterIdentity(ctx context.Context, serial, model string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO inverter_identity (serial, model, first_seen, last_seen)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(serial) DO UPDATE SET
		   model = excluded.model,
		   last_seen = excluded.last_seen`,
		serial, model, now, now)
	if err != nil {
		return fmt.Errorf("failed to set inverter identity: %w", err)
	}
	return nil
}

// migrate creates tables if they don't exist.
func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS inverter_identity (
			serial     TEXT PRIMARY KEY,
			model      TEXT NOT NULL DEFAULT '',
			first_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_seen  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
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
