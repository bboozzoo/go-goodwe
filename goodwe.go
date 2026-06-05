package goodwe

import (
	"context"
)

// Info contains basic metadata about the inverter.
type Info struct {
	SerialNumber string
	Model        string
	Firmware     string
}

// Inverter defines the contract for any GoodWe inverter implementation.
type Inverter interface {
	// Connect performs the probe and establishes the connection.
	Connect(ctx context.Context) error

	// Close gracefully shuts down the connection.
	Close() error

	// GetInfo retrieves basic metadata.
	GetInfo(ctx context.Context) (*Info, error)

	// GetSensors retrieves current sensor values.
	GetSensors(ctx context.Context) (map[string]float64, error)
}
