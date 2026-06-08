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
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"
)

// tcpTransport implements Transport over plain TCP with Modbus TCP framing.
type tcpTransport struct {
	ip   string
	port int
	conn net.Conn
	txID uint16
}

// NewTCPTransport creates a new TCP transport for the given IP and port.
func NewTCPTransport(ip string, port int) Transport {
	return &tcpTransport{
		ip:   ip,
		port: port,
		txID: 1,
	}
}

// Connect establishes the TCP connection to the inverter.
func (t *tcpTransport) Connect(ctx context.Context) error {
	addr := net.JoinHostPort(t.ip, strconv.Itoa(t.port))
	slog.Debug("Attempting TCP connection", "address", addr)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("tcp dial failed: %w", err)
	}

	t.conn = conn
	slog.Debug("TCP connection established")
	return nil
}

// Close closes the TCP connection.
func (t *tcpTransport) Close() error {
	if t.conn != nil {
		slog.Debug("Closing TCP connection")
		return t.conn.Close()
	}
	return nil
}

// ReadRegisters performs a Modbus TCP read holding registers request.
func (t *tcpTransport) ReadRegisters(ctx context.Context, startReg uint16, quantity uint16) ([]byte, error) {
	if t.conn == nil {
		return nil, errors.New("no TCP connection established")
	}

	req := t.buildRequest(startReg, quantity)
	slog.Debug("Sending Modbus TCP request", "start", startReg, "qty", quantity, "payload", hex.EncodeToString(req))

	_, err := t.conn.Write(req)
	if err != nil {
		return nil, fmt.Errorf("modbus write error: %w", err)
	}

	// Read response: MBAP(7) + Func(1) + ByteCount(1) + Data(N)
	respBuf := make([]byte, 4096)
	n, err := t.conn.Read(respBuf)
	if err != nil {
		return nil, fmt.Errorf("modbus read error: %w", err)
	}

	responseBytes := respBuf[:n]
	slog.Debug("Received Modbus TCP response", "payload", hex.EncodeToString(responseBytes))

	return parseModbusTCPResponse(responseBytes)
}

// buildRequest creates a Modbus TCP read holding registers request frame.
// MBAP header: TransID(2) | ProtoID(2) | Len(2) | UnitID(1) | Func(1) | RegAddr(2) | Qty(2)
func (t *tcpTransport) buildRequest(startReg, quantity uint16) []byte {
	t.txID++
	buf := make([]byte, 12)

	// MBAP header
	binary.BigEndian.PutUint16(buf[0:2], t.txID) // Transaction ID
	binary.BigEndian.PutUint16(buf[2:4], 0)      // Protocol ID (0 = Modbus)
	binary.BigEndian.PutUint16(buf[4:6], 6)      // Length (bytes after MBAP header)

	buf[6] = 0xF7 // Unit ID (ET protocol quirk)
	buf[7] = 0x03 // Function Code (Read Holding Registers)
	binary.BigEndian.PutUint16(buf[8:10], startReg)
	binary.BigEndian.PutUint16(buf[10:12], quantity)

	return buf
}

// parseModbusTCPResponse parses a Modbus TCP response and returns the raw register data bytes.
func parseModbusTCPResponse(data []byte) ([]byte, error) {
	if len(data) < 9 {
		return nil, fmt.Errorf("modbus TCP response too short: %d bytes", len(data))
	}

	// Skip MBAP header (7 bytes), check function code at offset 7
	funcCode := data[7]
	if funcCode&0x80 != 0 {
		exceptionCode := data[8]
		return nil, fmt.Errorf("modbus error response: function 0x%02X, exception 0x%02X", funcCode, exceptionCode)
	}

	byteCount := int(data[8])
	if len(data) < 9+byteCount {
		return nil, fmt.Errorf("incomplete modbus data: expected %d bytes, got %d", 9+byteCount, len(data))
	}

	return data[9 : 9+byteCount], nil
}
