package et

import (
	"context"
	"fmt"

	"github.com/bboozzoo/go-goodwe"
)

// ETInverter implements the goodwe.Inverter interface for the ET line.
type ETInverter struct {
	ip      string
	serial  string
	service *service
}

// New creates a new ETInverter instance.
func New(ip string) *ETInverter {
	return &ETInverter{
		ip:      ip,
		service: newService(ip),
	}
}

// Connect performs the probe and DTLS connection.
func (e *ETInverter) Connect(ctx context.Context) error {
	// 1. Probe with backoff
	var probeRes *probeResult
	err := backoff(ctx, func() error {
		var pErr error
		probeRes, pErr = e.service.probe(ctx)
		return pErr
	})

	if err != nil {
		return fmt.Errorf("connection failed during probe: %w", err)
	}

	e.serial = probeRes.SerialNumber

	// 2. Connect via DTLS
	err = e.service.connectDTLS(ctx, probeRes.DTLSPort)
	if err != nil {
		return fmt.Errorf("connection failed during DTLS handshake: %w", err)
	}

	return nil
}

// Close cleans up the connection.
func (e *ETInverter) Close() error {
	return e.service.close()
}

// GetInfo retrieves the inverter information.
func (e *ETInverter) GetInfo(ctx context.Context) (*goodwe.Info, error) {
	return &goodwe.Info{
		SerialNumber: e.serial,
		Model:        "ET-Series",
		Firmware:     "1.0.0",
	}, nil
}

// GetSensors retrieves the sensor values from the registry via a single bulk request.
func (e *ETInverter) GetSensors(ctx context.Context) (map[string]float64, error) {
	// Perform a single bulk request to get all telemetry in one go.
	// Based on user feedback, target register 35100 with a quantity of 125.
	data, err := e.service.readModbusBulk(ctx, 35100, 125)
	if err != nil {
		return nil, fmt.Errorf("failed to read bulk telemetry: %w", err)
	}

	results := make(map[string]float64)

	for name, def := range registry {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		if def.Calculator != nil {
			results[name] = def.Calculator(data)
		} else if def.Offset >= 0 && def.Offset < len(data) {
			results[name] = float64(data[def.Offset]) * def.Scale
		}
	}

	return results, nil
}
