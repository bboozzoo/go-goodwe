package et

import (
	"errors"
)

var (
	ErrNotConnected   = errors.New("inverter not connected")
	ErrProbeFailed    = errors.New("inverter probe failed")
	ErrDTLSHandshake  = errors.New("dtls handshake failed")
	ErrModbusRequest  = errors.New("modbus request failed")
	ErrSensorNotFound = errors.New("requested sensor not found in registry")
)

// internal probe response structure
type probeResult struct {
	SerialNumber string
	DTLSPort     int
}
