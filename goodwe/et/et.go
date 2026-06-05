package et

import (
	"context"
	"fmt"

	"github.com/bbboozzoo/go-goodwe"
)

// ETInverter implements the goodwe.Inverter interface for the ET line.
type ETInverter struct {
	ip      string
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
	// Since we don't have a specific "GetInfo" register in our minimal registry,
	// we'll use the serial number from the probe if possible, 
	// but for this minimal implementation, we'll return a placeholder or error.
	// A real implementation would read specific Modbus registers for Model/Firmware.
	
	// For now, we return a dummy info to satisfy the interface.
	return &goodwe.Info{
		SerialNumber: "UNKNOWN", // In real life, we'd store this from probeRes
		Model:        "ET-Series",
		Firmware:     "1.0.0",
	}, nil
}

// GetSensors retrieves the sensor values from the registry.
func (e *ETInverter) GetSensors(ctx context.Context) (map[string]float64, error) {
	results := make(map[string]float64)

	for name, def := range registry {
		// Check for context cancellation before each request
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		val, err := e.service.readModbusRegister(ctx, def.Register)
		if err != nil {
			// For a minimal implementation, we might want to log the error and continue
			// rather than failing the whole batch, but for now we return error.
			return nil, fmt.Errorf("failed to read sensor %s: %w", name, err)
		}

		results[name] = float64(val) * def.Scale
	}

	return results, nil
}
