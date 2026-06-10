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

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/bboozzoo/go-goodwe"
	"github.com/bboozzoo/go-goodwe/et"
)

// InverterConnState describes the current state of the inverter connection.
type InverterConnState int

const (
	InverterStateDisabled   InverterConnState = iota // no inverter configured
	InverterStateConnecting                          // connecting to inverter in progress
	InverterStateConnected                           // connected and identity verified
	InverterStateFailed                              // connection or identity error
)

func (s InverterConnState) String() string {
	switch s {
	case InverterStateDisabled:
		return "disabled"
	case InverterStateConnecting:
		return "connecting"
	case InverterStateConnected:
		return "connected"
	case InverterStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// DaemonStatus is the interface the daemon exposes for the API handler
// to read inverter state and verification errors.
type DaemonStatus interface {
	InverterState() InverterConnState
	ConnError() error // nil unless state is Failed
	VerificationError() error
}

// Handler serves the REST API endpoints.
type Handler struct {
	inverter goodwe.Inverter // may be nil when no inverter is configured
	daemon   DaemonStatus    // may be nil
	debug    bool
	mux      http.Handler
}

// New creates an API handler. inverter and daemonStatus may be nil; in that
// case endpoints that require an inverter return a 503 status.
func New(inverter goodwe.Inverter, daemonStatus DaemonStatus, debug bool) *Handler {
	h := &Handler{inverter: inverter, daemon: daemonStatus, debug: debug}
	h.mux = h.buildRoutes()
	return h
}

// ServeHTTP implements http.Handler. Delegates to the route mux.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) buildRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", h.handleHealth)
	mux.HandleFunc("GET /api/sensors", h.handleListSensors)
	mux.HandleFunc("GET /api/info", h.handleInfo)
	mux.HandleFunc("GET /api/", h.handleNotFound)
	mux.HandleFunc("GET /dashboard", h.handleDashboard)

	// Wrap in middleware chain: innermost first.
	var handler http.Handler = mux
	handler = corsMiddleware(handler)
	handler = loggingMiddleware(h.debug, handler)
	return handler
}

// healthResponse is the JSON body for the health endpoint.
type healthResponse struct {
	Status    string         `json:"status"`
	Timestamp string         `json:"timestamp"`
	Inverter  *inverterState `json:"inverter,omitempty"`
}

type inverterState struct {
	Connected bool   `json:"connected"`
	Error     string `json:"error,omitempty"`
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	var invState *inverterState

	ds := h.daemon
	if ds != nil {
		switch ds.InverterState() {
		case InverterStateDisabled:
			invState = &inverterState{Connected: false}
		case InverterStateConnecting:
			status = "degraded"
			invState = &inverterState{
				Connected: false,
				Error:     "connecting to inverter...",
			}
		case InverterStateConnected:
			if err := ds.VerificationError(); err != nil {
				status = "degraded"
				invState = &inverterState{
					Connected: false,
					Error:     err.Error(),
				}
			} else {
				invState = &inverterState{Connected: true}
			}
		case InverterStateFailed:
			status = "degraded"
			errMsg := "inverter connection failed"
			if err := ds.ConnError(); err != nil {
				errMsg = err.Error()
			}
			invState = &inverterState{
				Connected: false,
				Error:     errMsg,
			}
		}
	} else if h.inverter != nil {
		// No daemon status available — assume connected.
		invState = &inverterState{Connected: true}
	}

	resp := healthResponse{
		Status:    status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Inverter:  invState,
	}
	writeJSON(w, http.StatusOK, resp)
}

// sensorEntry is one sensor in the /api/sensors response.
type sensorEntry struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

func (h *Handler) handleListSensors(w http.ResponseWriter, r *http.Request) {
	groups := et.GetSensorNamesByBlock()

	// Order categories deterministically.
	var sensors []sensorEntry
	for _, cat := range []string{"Main Telemetry", "Battery", "Meter", "MPPT"} {
		names, ok := groups[cat]
		if !ok {
			continue
		}
		for _, name := range names {
			sensors = append(sensors, sensorEntry{
				Name:     name,
				Category: cat,
			})
		}
	}

	writeJSON(w, http.StatusOK, sensors)
}

// inverterInfo is the JSON body for GET /api/info.
type inverterInfo struct {
	Serial   string `json:"serial"`
	Model    string `json:"model"`
	Firmware string `json:"firmware"`
	Rated    int    `json:"rated_power"`
	DSP      string `json:"dsp_version"`
	ARM      string `json:"arm_version"`
	Error    string `json:"error,omitempty"`
}

func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request) {
	if h.inverter == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "no inverter configured")
		return
	}

	// Check daemon state before attempting to query the inverter.
	ds := h.daemon
	if ds != nil {
		switch ds.InverterState() {
		case InverterStateDisabled:
			writeJSONError(w, http.StatusServiceUnavailable, "no inverter configured")
			return
		case InverterStateConnecting:
			writeJSONError(w, http.StatusServiceUnavailable, "connecting to inverter...")
			return
		case InverterStateFailed:
			errMsg := "inverter connection failed"
			if err := ds.ConnError(); err != nil {
				errMsg = err.Error()
			}
			writeJSONError(w, http.StatusServiceUnavailable, errMsg)
			return
		case InverterStateConnected:
			// proceed with the query
		}
	}

	// Check for identity verification errors.
	var errStr string
	if ds != nil {
		if err := ds.VerificationError(); err != nil {
			errStr = err.Error()
		}
	}

	info, err := h.inverter.GetInfo(r.Context())
	if err != nil {
		slog.Warn("Failed to get inverter info", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to get inverter info")
		return
	}

	resp := inverterInfo{
		Serial:   info.SerialNumber,
		Model:    info.Model,
		Firmware: info.Firmware,
		Rated:    info.RatedPower,
		DSP:      info.DSPVersion,
		ARM:      info.ARMVersion,
		Error:    errStr,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("<html><body><h1>GoodWe Dashboard</h1><p>Dashboard will be available here.</p></body></html>")); err != nil {
		slog.Warn("Failed to write dashboard response", "error", err)
	}
}

func (h *Handler) handleNotFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// corsMiddleware adds permissive CORS headers for dashboard access.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs every request.
// - All requests are logged at DEBUG level (controlled by the debug flag).
// - Error responses (status >= 400) are always logged at WARN level regardless of debug flag.
func loggingMiddleware(debug bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lrw, r)

		latency := time.Since(start)
		if lrw.status >= 400 {
			slog.Warn("HTTP request error",
				"method", r.Method,
				"path", r.URL.Path,
				"status", lrw.status,
				"latency", latency.String(),
			)
		} else if debug {
			slog.Debug("HTTP request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", lrw.status,
				"latency", latency.String(),
			)
		}
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.status = code
	lrw.ResponseWriter.WriteHeader(code)
}

// writeJSON is a helper to write a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("Failed to encode JSON response", "error", err)
	}
}

// writeJSONError writes a JSON error response.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
