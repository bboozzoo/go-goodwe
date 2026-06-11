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

package et

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pion/dtls/v2"
)

const (
	dtlsReadTimeout  = 5 * time.Second
	dtlsWriteTimeout = 5 * time.Second
)

// dtlsTransport implements Transport over DTLS with Modbus RTU framing.
type dtlsTransport struct {
	mu   sync.Mutex
	ip   string
	port int
	conn net.Conn
}

// NewDTLSTransport creates a new DTLS transport for the given IP and port.
func NewDTLSTransport(ip string, port int) Transport {
	return &dtlsTransport{
		ip:   ip,
		port: port,
	}
}

// Connect establishes the DTLS connection to the inverter.
func (t *dtlsTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Close any stale connection first.
	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}

	addr := net.JoinHostPort(t.ip, strconv.Itoa(t.port))
	slog.Debug("Attempting DTLS connection", "address", addr)

	config := &dtls.Config{
		CipherSuites: []dtls.CipherSuiteID{
			dtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			dtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		InsecureSkipVerify: true,
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := dtls.Dial("udp", udpAddr, config)
	if err != nil {
		return fmt.Errorf("dtls dial failed: %w", err)
	}

	t.conn = conn
	slog.Debug("DTLS connection established")
	return nil
}

// Close closes the DTLS connection.
func (t *dtlsTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn != nil {
		slog.Debug("Closing DTLS connection")
		err := t.conn.Close()
		t.conn = nil
		return err
	}
	return nil
}

// ReadRegisters performs a Modbus RTU bulk register read over the DTLS connection.
// Automatically reconnects if the connection has been closed by the remote end.
func (t *dtlsTransport) ReadRegisters(ctx context.Context, startReg uint16, quantity uint16) ([]byte, error) {
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()

	if conn == nil {
		return t.readWithReconnect(ctx, startReg, quantity)
	}

	data, err := t.doRead(conn, startReg, quantity)
	if err != nil && isConnClosed(err) {
		slog.Debug("DTLS connection closed, reconnecting...", "error", err)
		return t.readWithReconnect(ctx, startReg, quantity)
	}
	return data, err
}

// readWithReconnect reconnects and performs the read.
func (t *dtlsTransport) readWithReconnect(ctx context.Context, startReg, quantity uint16) ([]byte, error) {
	if err := t.Connect(ctx); err != nil {
		return nil, fmt.Errorf("reconnect failed: %w", err)
	}

	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()

	return t.doRead(conn, startReg, quantity)
}

// doRead performs a single Modbus RTU read over the given connection.
func (t *dtlsTransport) doRead(conn net.Conn, startReg, quantity uint16) ([]byte, error) {
	// Modbus RTU over DTLS:
	// Slave ID (1) | Function Code (1) | Register Address (2) | Quantity (2) | CRC (2)
	request := make([]byte, 8)
	request[0] = 0xF7 // Slave ID 247 (ET protocol quirk: 0x01 requests are ignored)
	request[1] = 0x03 // Function Code (Read Holding Registers)
	binary.BigEndian.PutUint16(request[2:4], startReg)
	binary.BigEndian.PutUint16(request[4:6], quantity)

	crc := calculateCRC16(request[0:6])
	binary.LittleEndian.PutUint16(request[6:8], crc)

	slog.Debug("Sending Modbus RTU bulk request", "start", startReg, "qty", quantity, "payload", hex.EncodeToString(request))

	if err := conn.SetWriteDeadline(time.Now().Add(dtlsWriteTimeout)); err != nil {
		return nil, fmt.Errorf("set write deadline: %w", err)
	}
	if _, err := conn.Write(request); err != nil {
		return nil, fmt.Errorf("modbus write error: %w", err)
	}

	// Expected RTU: AA55(2) | SlaveID(1) | Func(1) | ByteCount(1) | Data(N) | CRC(2)
	respBuf := make([]byte, 4096)
	if err := conn.SetReadDeadline(time.Now().Add(dtlsReadTimeout)); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}
	n, err := conn.Read(respBuf)
	if err != nil {
		return nil, fmt.Errorf("modbus read error: %w", err)
	}

	responseBytes := respBuf[:n]
	slog.Debug("Received Modbus RTU bulk response", "payload", hex.EncodeToString(responseBytes))

	return parseModbusBulkResponse(responseBytes)
}

// isConnClosed checks if the error indicates the connection was closed.
func isConnClosed(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "conn is closed") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "reset by peer")
}

// calculateCRC16 calculates the Modbus CRC16 checksum.
func calculateCRC16(data []byte) uint16 {
	var crc uint16 = 0xFFFF
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&0x0001 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

// parseModbusBulkResponse parses a raw Modbus RTU response (with aa55 pre-header)
// and returns the raw register data bytes.
func parseModbusBulkResponse(responseBytes []byte) ([]byte, error) {
	n := len(responseBytes)
	if n < 7 {
		return nil, fmt.Errorf("modbus response too short: %d bytes", n)
	}

	// Validate CRC
	expectedCRC := binary.LittleEndian.Uint16(responseBytes[n-2 : n])
	// The expected CRC does not include the fixed 0xaa 0x55 header
	actualCRC := calculateCRC16(responseBytes[2 : n-2])
	if expectedCRC != actualCRC {
		return nil, fmt.Errorf("modbus CRC mismatch: expected %04X, got %04X", expectedCRC, actualCRC)
	}

	// Validate Function Code
	funcCode := responseBytes[3]
	if funcCode&0x80 != 0 {
		exceptionCode := responseBytes[4]
		return nil, fmt.Errorf("modbus error response: function 0x%02X, exception 0x%02X", funcCode, exceptionCode)
	}

	byteCount := int(responseBytes[4])
	if n < 5+byteCount+2 {
		return nil, fmt.Errorf("incomplete modbus data: expected %d bytes, got %d", 5+byteCount+2, n)
	}

	return responseBytes[5 : 5+byteCount], nil
}
