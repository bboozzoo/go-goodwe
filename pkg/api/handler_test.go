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
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bboozzoo/go-goodwe"
	"github.com/bboozzoo/go-goodwe/pkg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- mocks ----

type mockInverter struct {
	onConnect    func(ctx context.Context) error
	onClose      func() error
	onGetInfo    func(ctx context.Context) (*goodwe.Info, error)
	onGetSensors func(ctx context.Context) (map[string]goodwe.SensorValue, error)
	onReadSensor func(ctx context.Context, name string) (goodwe.SensorValue, error)
}

func (m *mockInverter) Connect(ctx context.Context) error {
	if m.onConnect != nil {
		return m.onConnect(ctx)
	}
	return nil
}
func (m *mockInverter) Close() error {
	if m.onClose != nil {
		return m.onClose()
	}
	return nil
}
func (m *mockInverter) GetInfo(ctx context.Context) (*goodwe.Info, error) {
	if m.onGetInfo != nil {
		return m.onGetInfo(ctx)
	}
	return &goodwe.Info{SerialNumber: "TEST001", Model: "GW-TEST", RatedPower: 10000}, nil
}
func (m *mockInverter) GetSensors(ctx context.Context) (map[string]goodwe.SensorValue, error) {
	if m.onGetSensors != nil {
		return m.onGetSensors(ctx)
	}
	return nil, nil
}
func (m *mockInverter) ReadSensor(ctx context.Context, name string) (goodwe.SensorValue, error) {
	if m.onReadSensor != nil {
		return m.onReadSensor(ctx, name)
	}
	return goodwe.SensorValue{}, fmt.Errorf("unknown sensor: %s", name)
}

type mockDaemonStatus struct {
	state           InverterConnState
	connErr         error
	verificationErr error
}

func (m *mockDaemonStatus) InverterState() InverterConnState { return m.state }
func (m *mockDaemonStatus) ConnError() error                 { return m.connErr }
func (m *mockDaemonStatus) VerificationError() error         { return m.verificationErr }

type mockSensorStore struct {
	onGetIdentity              func(ctx context.Context) (*db.InverterIdentity, error)
	onQuerySamples            func(ctx context.Context, name string, since, until time.Time, limit int) ([]db.Sample, error)
	onLatestSample            func(ctx context.Context, name string) (*db.Sample, error)
	onLastTime                func(ctx context.Context) (*time.Time, error)
	onSampleAt                func(ctx context.Context, name string, at time.Time) (*db.Sample, error)
	onQueryVoltageEvents      func(ctx context.Context, before int64, limit int) ([]db.VoltageEvent, int, error)
	onGetVoltageAnalysisCursor func(ctx context.Context) (*db.VoltageAnalysisCursor, error)
}

func (m *mockSensorStore) GetInverterIdentity(ctx context.Context) (*db.InverterIdentity, error) {
	if m.onGetIdentity != nil {
		return m.onGetIdentity(ctx)
	}
	return nil, nil
}
func (m *mockSensorStore) QueryRawSamples(ctx context.Context, name string, since, until time.Time, limit int) ([]db.Sample, error) {
	if m.onQuerySamples != nil {
		return m.onQuerySamples(ctx, name, since, until, limit)
	}
	return []db.Sample{}, nil
}
func (m *mockSensorStore) LatestSample(ctx context.Context, name string) (*db.Sample, error) {
	if m.onLatestSample != nil {
		return m.onLatestSample(ctx, name)
	}
	return nil, nil
}
func (m *mockSensorStore) LastSampleTime(ctx context.Context) (*time.Time, error) {
	if m.onLastTime != nil {
		return m.onLastTime(ctx)
	}
	return nil, nil
}
func (m *mockSensorStore) SampleAt(ctx context.Context, name string, at time.Time) (*db.Sample, error) {
	if m.onSampleAt != nil {
		return m.onSampleAt(ctx, name, at)
	}
	return nil, nil
}
func (m *mockSensorStore) QueryVoltageEvents(ctx context.Context, before int64, limit int) ([]db.VoltageEvent, int, error) {
	if m.onQueryVoltageEvents != nil {
		return m.onQueryVoltageEvents(ctx, before, limit)
	}
	return nil, 0, nil
}
func (m *mockSensorStore) GetVoltageAnalysisCursor(ctx context.Context) (*db.VoltageAnalysisCursor, error) {
	if m.onGetVoltageAnalysisCursor != nil {
		return m.onGetVoltageAnalysisCursor(ctx)
	}
	return nil, nil
}

// ---- helpers ----

func newHandler(inv goodwe.Inverter, ds DaemonStatus, ss SensorStore) *Handler {
	return New(inv, ds, ss, "10.0.0.1", "test-version", false, nil, 0)
}

func getBody(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &m)
	require.NoError(t, err)
	return m
}

func getBodyArray(t *testing.T, rr *httptest.ResponseRecorder) []any {
	t.Helper()
	var a []any
	err := json.Unmarshal(rr.Body.Bytes(), &a)
	require.NoError(t, err)
	return a
}

// ---- health endpoint ----

func TestHealth_Connected(t *testing.T) {
	ds := &mockDaemonStatus{state: InverterStateConnected}
	h := newHandler(&mockInverter{}, ds, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/health", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	assert.Equal(t, "ok", body["status"])
	inv := body["inverter"].(map[string]any)
	assert.Equal(t, true, inv["connected"])
	assert.Equal(t, "test-version", rr.Header().Get("Goodwe-Daemon-Version"))
}

func TestHealth_Connecting(t *testing.T) {
	ds := &mockDaemonStatus{state: InverterStateConnecting}
	h := newHandler(&mockInverter{}, ds, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/health", nil)
	h.ServeHTTP(rr, req)

	body := getBody(t, rr)
	assert.Equal(t, "degraded", body["status"])
	inv := body["inverter"].(map[string]any)
	assert.Equal(t, false, inv["connected"])
	assert.Contains(t, inv["error"], "connecting")
}

func TestHealth_Disconnected(t *testing.T) {
	ds := &mockDaemonStatus{state: InverterStateDisconnected}
	h := newHandler(&mockInverter{}, ds, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/health", nil)
	h.ServeHTTP(rr, req)

	body := getBody(t, rr)
	assert.Equal(t, "degraded", body["status"])
	inv := body["inverter"].(map[string]any)
	assert.Equal(t, false, inv["connected"])
	assert.Contains(t, inv["error"], "disconnected")
}

func TestHealth_Failed(t *testing.T) {
	ds := &mockDaemonStatus{
		state:   InverterStateFailed,
		connErr: fmt.Errorf("connection timeout"),
	}
	h := newHandler(&mockInverter{}, ds, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/health", nil)
	h.ServeHTTP(rr, req)

	body := getBody(t, rr)
	assert.Equal(t, "degraded", body["status"])
	inv := body["inverter"].(map[string]any)
	assert.Equal(t, false, inv["connected"])
	assert.Contains(t, inv["error"], "connection timeout")
}

func TestHealth_VerificationError(t *testing.T) {
	ds := &mockDaemonStatus{
		state:           InverterStateConnected,
		verificationErr: fmt.Errorf("serial mismatch: expected X, got Y"),
	}
	h := newHandler(&mockInverter{}, ds, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/health", nil)
	h.ServeHTTP(rr, req)

	body := getBody(t, rr)
	assert.Equal(t, "degraded", body["status"])
	inv := body["inverter"].(map[string]any)
	assert.Equal(t, false, inv["connected"])
	assert.Contains(t, inv["error"], "serial mismatch")
}

func TestHealth_NoInverter(t *testing.T) {
	h := New(nil, nil, nil, "", "", false, nil, 0)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/health", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	assert.Equal(t, "ok", body["status"])
	// inverter field should be absent when there's no daemon status and no inverter
	_, ok := body["inverter"]
	assert.False(t, ok)
	// Version header should be set even without inverter.
	assert.Equal(t, "", rr.Header().Get("Goodwe-Daemon-Version"))
}

// ---- info endpoint ----

func TestInfo_FromDB(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	ss := &mockSensorStore{
		onGetIdentity: func(ctx context.Context) (*db.InverterIdentity, error) {
			return &db.InverterIdentity{
				Serial: "SER001", Model: "GW-TEST", Firmware: "fw1",
				DSPVersion: "dsp1", ARMVersion: "arm1", RatedPower: 10000,
			}, nil
		},
		onLastTime: func(ctx context.Context) (*time.Time, error) { return &now, nil },
	}
	h := newHandler(&mockInverter{}, nil, ss)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/info", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	assert.Equal(t, "SER001", body["serial"])
	assert.Equal(t, "GW-TEST", body["model"])
	assert.Equal(t, "fw1", body["firmware"])
	assert.Equal(t, "dsp1", body["dsp_version"])
	assert.Equal(t, "arm1", body["arm_version"])
	assert.Equal(t, float64(10000), body["rated_power"])
	assert.Equal(t, "10.0.0.1", body["inverter_ip"])
	assert.Equal(t, "test-version", body["daemon_version"])
	assert.Contains(t, body["last_poll_time"], now.Format("2006-01-02"))
	assert.Equal(t, "test-version", rr.Header().Get("Goodwe-Daemon-Version"))
}

func TestInfo_NoInverter(t *testing.T) {
	h := New(nil, nil, nil, "", "", false, nil, 0)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/info", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, 503, rr.Code)
	body := getBody(t, rr)
	assert.Contains(t, body["error"], "no inverter configured")
}

func TestInfo_FallbackToInverter(t *testing.T) {
	// No sensor store — should fall back to inverter GetInfo.
	h := newHandler(&mockInverter{}, &mockDaemonStatus{state: InverterStateConnected}, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/info", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	assert.Equal(t, "TEST001", body["serial"])
	assert.Equal(t, "GW-TEST", body["model"])
	assert.Equal(t, "test-version", body["daemon_version"])
}

// ---- sensors endpoint ----

func TestSensors_List(t *testing.T) {
	h := newHandler(&mockInverter{}, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/sensors", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	sensors := getBodyArray(t, rr)
	assert.Greater(t, len(sensors), 150) // we have ~199 sensors

	// Check known sensors are present.
	names := make([]string, len(sensors))
	for i, s := range sensors {
		m := s.(map[string]any)
		names[i] = m["name"].(string)
		assert.Contains(t, m, "category")
	}
	assert.Contains(t, names, "battery_soc")
	assert.Contains(t, names, "ppv")
	assert.Contains(t, names, "work_mode_label")
}

func TestSensors_Format(t *testing.T) {
	h := newHandler(&mockInverter{}, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/sensors", nil)
	h.ServeHTTP(rr, req)

	sensors := getBodyArray(t, rr)
	entry := sensors[0].(map[string]any)
	assert.Contains(t, entry, "name")
	assert.Contains(t, entry, "category")
	// No extra fields.
	assert.Len(t, entry, 2)
}

// ---- data/{sensor} endpoint ----

func TestGetData_Success(t *testing.T) {
	inv := &mockInverter{
		onReadSensor: func(ctx context.Context, name string) (goodwe.SensorValue, error) {
			return goodwe.SensorValue{Name: "PV Power", Value: float64(1234), Unit: "W"}, nil
		},
	}
	h := newHandler(inv, &mockDaemonStatus{state: InverterStateConnected}, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data/ppv", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	assert.Equal(t, "PV Power", body["name"])
	assert.Equal(t, float64(1234), body["value"])
	assert.Equal(t, "W", body["unit"])
	assert.Contains(t, body, "timestamp")
}

func TestGetData_UnknownSensor(t *testing.T) {
	inv := &mockInverter{
		onReadSensor: func(ctx context.Context, name string) (goodwe.SensorValue, error) {
			return goodwe.SensorValue{}, fmt.Errorf("unknown sensor: %s", name)
		},
	}
	h := newHandler(inv, &mockDaemonStatus{state: InverterStateConnected}, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data/nonexistent", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, 500, rr.Code)
}

func TestGetData_NoInverter(t *testing.T) {
	h := New(nil, nil, nil, "", "", false, nil, 0)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data/battery_soc", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 503, rr.Code)
}

// ---- data/{sensor}/aggregate endpoint ----

func TestAggregate_Latest(t *testing.T) {
	val := 60.5
	now := time.Now().UTC().Truncate(time.Millisecond)
	ss := &mockSensorStore{
		onLatestSample: func(ctx context.Context, name string) (*db.Sample, error) {
			return &db.Sample{Value: &val, Unit: "%", SampledAt: now}, nil
		},
	}
	h := newHandler(&mockInverter{}, nil, ss)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data/battery_soc/aggregate?latest=true", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	assert.Equal(t, "battery_soc", body["sensor"])
	samples := body["samples"].([]any)
	require.Len(t, samples, 1)
	s := samples[0].(map[string]any)
	assert.Equal(t, float64(60.5), s["value"])
}

func TestAggregate_NoStore(t *testing.T) {
	h := newHandler(&mockInverter{}, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data/battery_soc/aggregate", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 501, rr.Code)
}

func TestAggregate_InvalidSince(t *testing.T) {
	ss := &mockSensorStore{}
	h := newHandler(&mockInverter{}, nil, ss)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data/battery_soc/aggregate?since=not-a-date", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 400, rr.Code)
	body := getBody(t, rr)
	assert.Contains(t, body["error"], "since format")
}

func TestAggregate_InvalidUntil(t *testing.T) {
	ss := &mockSensorStore{}
	h := newHandler(&mockInverter{}, nil, ss)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data/battery_soc/aggregate?since=2026-01-01T00:00:00Z&until=bad", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 400, rr.Code)
}

func TestAggregate_EmptySensor(t *testing.T) {
	h := newHandler(&mockInverter{}, nil, &mockSensorStore{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data//aggregate", nil)
	h.ServeHTTP(rr, req)
	// Go 1.22+ mux requires non-empty path segments; // doesn't match
	// the route so falls through to the root handler -> 307 redirect.
	assert.Equal(t, 307, rr.Code)
}

func TestAggregate_DeltaInvalidFormat(t *testing.T) {
	ss := &mockSensorStore{}
	h := newHandler(&mockInverter{}, nil, ss)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data/e_total_exp/aggregate?delta=not-a-date&limit=1", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 400, rr.Code)
	body := getBody(t, rr)
	assert.Contains(t, body["error"], "delta format")
}

func TestAggregate_DeltaNoRefSample(t *testing.T) {
	ss := &mockSensorStore{
		onSampleAt: func(ctx context.Context, name string, at time.Time) (*db.Sample, error) {
			return nil, nil // no sample at delta timestamp
		},
	}
	h := newHandler(&mockInverter{}, nil, ss)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data/e_total_exp/aggregate?delta=2026-01-01T00:00:00Z&limit=1", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 400, rr.Code)
	body := getBody(t, rr)
	assert.Contains(t, body["error"], "no sample found")
}

func TestAggregate_DeltaAdjustsValue(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	refVal := 1000.0
	curVal := 1042.5
	// Reference sample at T-12h; delta requested at T-12h+30s → 30s gap
	refTime := now.Add(-12 * time.Hour)
	deltaTime := refTime.Add(30 * time.Second)
	ss := &mockSensorStore{
		onSampleAt: func(ctx context.Context, name string, at time.Time) (*db.Sample, error) {
			return &db.Sample{Value: &refVal, Unit: "kWh", SampledAt: refTime}, nil
		},
		onQuerySamples: func(ctx context.Context, name string, since, until time.Time, limit int) ([]db.Sample, error) {
			return []db.Sample{
				{Value: &curVal, Unit: "kWh", SampledAt: now},
			}, nil
		},
	}
	h := newHandler(&mockInverter{}, nil, ss)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data/e_total_exp/aggregate?delta="+deltaTime.Format(time.RFC3339)+"&limit=1", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	assert.Equal(t, "e_total_exp", body["sensor"])
	samples := body["samples"].([]any)
	require.Len(t, samples, 1)
	s := samples[0].(map[string]any)
	// curVal - refVal = 1042.5 - 1000.0 = 42.5
	assert.Equal(t, 42.5, s["value"])
	// Check delta metadata
	delta, ok := body["delta"].(map[string]any)
	require.True(t, ok, "delta metadata should be present")
	assert.Equal(t, deltaTime.Format(time.RFC3339), delta["at"])
	assert.Equal(t, refTime.Format(time.RFC3339), delta["reference_at"])
	assert.InDelta(t, 30.0, delta["gap_seconds"], 1.0)
}

func TestAggregate_DeltaAdjustsMultipleSamples(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	refVal := 1000.0
	refTime := now.Add(-24 * time.Hour)
	deltaTime := refTime.Add(30 * time.Second)
	ss := &mockSensorStore{
		onSampleAt: func(ctx context.Context, name string, at time.Time) (*db.Sample, error) {
			return &db.Sample{Value: &refVal, Unit: "kWh", SampledAt: refTime}, nil
		},
		onQuerySamples: func(ctx context.Context, name string, since, until time.Time, limit int) ([]db.Sample, error) {
			v1, v2, v3 := 1005.0, 1010.0, 1015.0
			return []db.Sample{
				{Value: &v1, Unit: "kWh", SampledAt: now.Add(-2 * time.Hour)},
				{Value: &v2, Unit: "kWh", SampledAt: now.Add(-1 * time.Hour)},
				{Value: &v3, Unit: "kWh", SampledAt: now},
			}, nil
		},
	}
	h := newHandler(&mockInverter{}, nil, ss)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data/e_total_exp/aggregate?delta="+deltaTime.Format(time.RFC3339)+"&limit=3", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	samples := body["samples"].([]any)
	require.Len(t, samples, 3)
	s0 := samples[0].(map[string]any)
	s1 := samples[1].(map[string]any)
	s2 := samples[2].(map[string]any)
	assert.Equal(t, 5.0, s0["value"])  // 1005 - 1000
	assert.Equal(t, 10.0, s1["value"]) // 1010 - 1000
	assert.Equal(t, 15.0, s2["value"]) // 1015 - 1000
	// Check delta metadata
	delta, ok := body["delta"].(map[string]any)
	require.True(t, ok, "delta metadata should be present")
	assert.Equal(t, deltaTime.Format(time.RFC3339), delta["at"])
	assert.Equal(t, refTime.Format(time.RFC3339), delta["reference_at"])
	assert.InDelta(t, 30.0, delta["gap_seconds"], 1.0)
}

// ---- redirect ----

func TestRootRedirect(t *testing.T) {
	h := newHandler(&mockInverter{}, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 302, rr.Code)
	loc := rr.Header().Get("Location")
	assert.Equal(t, "/dashboard", loc)
}

// ---- CORS headers ----

func TestCORSPreflight(t *testing.T) {
	h := newHandler(&mockInverter{}, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/api/health", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 204, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSHeaders(t *testing.T) {
	h := newHandler(&mockInverter{}, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/health", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
}

// ---- 404 routing ----

func TestUnknownAPIRoute(t *testing.T) {
	h := newHandler(&mockInverter{}, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/nonexistent", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 404, rr.Code)
}

func TestWrongMethod(t *testing.T) {
	// POST to a GET-only endpoint should get 405 from Go 1.22+ mux.
	h := newHandler(&mockInverter{}, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/health", strings.NewReader(""))
	h.ServeHTTP(rr, req)
	assert.Equal(t, 405, rr.Code)
}

// ---- gzip middleware ----

func TestGzipEncoding(t *testing.T) {
	h := newHandler(&mockInverter{}, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/sensors", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	h.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "gzip", rr.Header().Get("Content-Encoding"))
	// Body should be gzip-compressed (non-empty, different from expected).
	assert.NotEmpty(t, rr.Body.Bytes())
}

func TestGzipNotRequested(t *testing.T) {
	h := newHandler(&mockInverter{}, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/health", nil)
	// No Accept-Encoding header.
	h.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	assert.Empty(t, rr.Header().Get("Content-Encoding"))
}

// ---- downsample -------

func TestDownsample_Empty(t *testing.T) {
	result := downsample(nil, 100)
	assert.Empty(t, result)
}

func TestDownsample_BelowMax(t *testing.T) {
	samples := makeSamples(5)
	result := downsample(samples, 10)
	assert.Equal(t, samples, result, "should return original when len <= max")
}

func TestDownsample_AtMax(t *testing.T) {
	samples := makeSamples(10)
	result := downsample(samples, 10)
	assert.Equal(t, samples, result, "should return original when len == max")
}

func TestDownsample_ReducesCount(t *testing.T) {
	samples := makeSamples(1000)
	result := downsample(samples, 50)
	assert.LessOrEqual(t, len(result), 50, "should not exceed max")
	assert.Greater(t, len(result), 0, "should contain at least one sample")
}

func TestDownsample_PreservesFirstElement(t *testing.T) {
	samples := makeSamples(100)
	result := downsample(samples, 10)
	assert.Equal(t, samples[0].Value, result[0].Value, "first element should be preserved")
}

func TestDownsample_EvenSpacing(t *testing.T) {
	samples := makeSamples(100)
	result := downsample(samples, 20)
	// With 100 samples and max 20, step = (100+20-1)/20 = 5.
	// Result should be indices 0,5,10,...,95 = 20 elements.
	assert.Equal(t, 20, len(result))
	for i := range result {
		expectedIdx := i * 5
		if expectedIdx >= len(samples) {
			t.Fatalf("index %d out of bounds", expectedIdx)
		}
		assert.Equal(t, samples[expectedIdx].Value, result[i].Value, "element %d should match index %d", i, expectedIdx)
	}
}

func TestDownsample_ReturnsAtMostMax(t *testing.T) {
	t.Run("2000->1999", func(t *testing.T) {
		samples := makeSamples(2000)
		result := downsample(samples, 1999)
		assert.LessOrEqual(t, len(result), 1999)
	})
	t.Run("100->99", func(t *testing.T) {
		samples := makeSamples(100)
		result := downsample(samples, 99)
		assert.LessOrEqual(t, len(result), 99)
	})
	t.Run("7->3", func(t *testing.T) {
		samples := makeSamples(7)
		result := downsample(samples, 3)
		assert.LessOrEqual(t, len(result), 3)
		// step = (7+3-1)/3 = 3 → indices 0,3,6
		assert.Equal(t, 3, len(result))
		assert.Equal(t, samples[0], result[0])
		assert.Equal(t, samples[3], result[1])
		assert.Equal(t, samples[6], result[2])
	})
	t.Run("1->1", func(t *testing.T) {
		samples := makeSamples(1)
		result := downsample(samples, 1)
		assert.Equal(t, samples, result)
	})
}

func TestGridVoltage_NoStore(t *testing.T) {
	h := New(nil, nil, nil, "", "", false, nil, 0)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/analysis/grid_voltage", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 501, rr.Code)
}

func TestGridVoltage_Empty(t *testing.T) {
	ss := &mockSensorStore{}
	h := New(nil, nil, ss, "", "", false, ss, 0)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/analysis/grid_voltage?limit=20", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	events := body["events"].([]any)
	assert.Empty(t, events)
	cursor := body["cursor"].(map[string]any)
	assert.Equal(t, false, cursor["has_more"])
}

func TestGridVoltage_FirstPage(t *testing.T) {
	ss := &mockSensorStore{
		onQueryVoltageEvents: func(ctx context.Context, before int64, limit int) ([]db.VoltageEvent, int, error) {
			var events []db.VoltageEvent
			for i := int64(30); i > 10; i-- {
				minV := 200.0
				dur := 120
				now := time.Now().UTC()
				end := now.Add(-time.Duration(i) * time.Minute)
				start := end.Add(-2 * time.Hour)
				events = append(events, db.VoltageEvent{
					ID: i, Phase: "meter_voltage1",
					StartTime: start, EndTime: &end,
					MinVoltage: minV, MaxVoltage: 200.0, AvgVoltage: 200.0,
					DurationSeconds: &dur,
				})
			}
			return events[:limit], 30, nil
		},
		onGetVoltageAnalysisCursor: func(ctx context.Context) (*db.VoltageAnalysisCursor, error) {
			return &db.VoltageAnalysisCursor{LastRunAt: time.Now().UTC()}, nil
		},
	}
	h := New(nil, nil, ss, "", "", false, ss, 0)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/analysis/grid_voltage?limit=20", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	events := body["events"].([]any)
	assert.Len(t, events, 20)
	cursor := body["cursor"].(map[string]any)
	assert.Equal(t, true, cursor["has_more"])
	assert.NotZero(t, cursor["next"])
}

func TestGridVoltage_SecondPage(t *testing.T) {
	ss := &mockSensorStore{
		onQueryVoltageEvents: func(ctx context.Context, before int64, limit int) ([]db.VoltageEvent, int, error) {
			var events []db.VoltageEvent
			for i := int64(10); i > 0; i-- {
				minV := 200.0
				dur := 120
				now := time.Now().UTC()
				end := now.Add(-time.Duration(i) * time.Minute)
				start := end.Add(-2 * time.Hour)
				events = append(events, db.VoltageEvent{
					ID: i, Phase: "meter_voltage1",
					StartTime: start, EndTime: &end,
					MinVoltage: minV, MaxVoltage: 200.0, AvgVoltage: 200.0,
					DurationSeconds: &dur,
				})
			}
			return events, 30, nil
		},
	}
	h := New(nil, nil, ss, "", "", false, ss, 0)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/analysis/grid_voltage?before=10&limit=20", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	events := body["events"].([]any)
	assert.Len(t, events, 10)
	cursor := body["cursor"].(map[string]any)
	assert.Equal(t, false, cursor["has_more"])
}

func TestGridVoltage_InvalidLimit(t *testing.T) {
	ss := &mockSensorStore{}
	h := New(nil, nil, ss, "", "", false, ss, 0)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/analysis/grid_voltage?limit=200", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 400, rr.Code)
	body := getBody(t, rr)
	assert.Contains(t, body["error"], "limit")
}

func TestGridVoltage_InvalidBefore(t *testing.T) {
	ss := &mockSensorStore{}
	h := New(nil, nil, ss, "", "", false, ss, 0)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/analysis/grid_voltage?before=-1", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 400, rr.Code)
	body := getBody(t, rr)
	assert.Contains(t, body["error"], "before")
}

func TestGridVoltage_AnalysisMetadata(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	ss := &mockSensorStore{
		onGetVoltageAnalysisCursor: func(ctx context.Context) (*db.VoltageAnalysisCursor, error) {
			return &db.VoltageAnalysisCursor{LastRunAt: now}, nil
		},
	}
	h := New(nil, nil, ss, "", "", false, ss, 0)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/analysis/grid_voltage?limit=1", nil)
	h.ServeHTTP(rr, req)
	assert.Equal(t, 200, rr.Code)
	body := getBody(t, rr)
	an := body["analysis"].(map[string]any)
	lastRun := an["last_run_at"].(string)
	assert.Contains(t, lastRun, now.Format("2006-01-02"))
}

// makeSamples creates n samples with sequential float64 values (0..n-1).
func makeSamples(n int) []db.Sample {
	s := make([]db.Sample, n)
	for i := 0; i < n; i++ {
		v := float64(i)
		s[i] = db.Sample{Value: &v, Unit: "W", SampledAt: time.Now()}
	}
	return s
}
