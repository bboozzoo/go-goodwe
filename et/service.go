// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, Maciej Borzecki <maciek.borzecki@gmail.com>
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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pion/dtls/v2"
)

// service handles low-level communication with the inverter.
type service struct {
	ip        string
	probePort int
	dtlsPort  int
	dtlsConn  net.Conn
}

// newService initializes a new service instance.
func newService(ip string) *service {
	return &service{
		ip:        ip,
		probePort: 48899, // Default probe port based on example
	}
}

// probe sends the WIFIKIT UDP packet and parses the response.
func (s *service) probe(ctx context.Context) (*probeResult, error) {
	// The probe packet: "WIFIKIT-214028-READ"
	probeMsg := []byte("WIFIKIT-214028-READ")

	slog.Debug("Sending UDP probe", "payload", hex.EncodeToString(probeMsg))

	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:%d", s.ip, s.probePort), 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to dial UDP: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Warn("Failed to close UDP probe connection", "error", err)
		}
	}()

	// Set deadline based on context
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	_ = conn.SetDeadline(deadline)

	_, err = conn.Write(probeMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to write probe: %w", err)
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read probe response: %w", err)
	}

	responseBytes := buf[:n]
	slog.Debug("Received UDP probe response", "payload", hex.EncodeToString(responseBytes))

	response := string(responseBytes)
	return s.parseProbeResponse(response)
}

// parseProbeResponse extracts serial and DTLS port from the string.
func (s *service) parseProbeResponse(resp string) (*probeResult, error) {
	if strings.Contains(resp, "@busy") {
		return nil, fmt.Errorf("inverter busy, try again later: %s", resp)
	}

	re := regexp.MustCompile(`dtls_port:(\d+)`)
	matches := re.FindStringSubmatch(resp)
	if len(matches) < 2 {
		return nil, fmt.Errorf("could not find dtls_port in response: %s", resp)
	}

	port, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("invalid dtls_port: %w", err)
	}

	parts := strings.Split(resp, ",")
	serial := ""
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		if len(lastPart) >= 12 {
			serial = lastPart[len(lastPart)-12:]
		} else {
			serial = lastPart
		}
	}

	return &probeResult{
		SerialNumber: serial,
		DTLSPort:     port,
	}, nil
}

// connectDTLS establishes the secure connection using pion/dtls.
func (s *service) connectDTLS(ctx context.Context, port int) error {
	addr := fmt.Sprintf("%s:%d", s.ip, port)
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

	s.dtlsConn = conn
	s.dtlsPort = port
	slog.Debug("DTLS connection established")
	return nil
}

// TODO move this to the modbus package
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

// readModbusBulk performs a single Modbus bulk register read over the DTLS connection using RTU framing.
func (s *service) readModbusBulk(ctx context.Context, startReg uint16, quantity uint16) ([]byte, error) {
	if s.dtlsConn == nil {
		return nil, errors.New("no DTLS connection established")
	}

	// TODO: create modbus package, we need a struct describing the RTU frame, e.g.
	// struct RTU {
	//     Slave uint8
	//     FunctionCode uint8
	//     Data []byte   // 0-255 bytes
	//}
	//
	//there should be a MarshalBinary() and UnmarshalBinary methods on the
	//frame. MarshalBinary() should append CRC16 at the end. Unmarshal binary
	//should verify the CRC and return an error if there is an error. THere
	//needs to be a named error.
	//
	// Modbus RTU over DTLS:
	// Slave ID (1) | Function Code (1) | Register Address (2) | Quantity (2) | CRC (2)
	request := make([]byte, 8)
	// TODO: slave ID needs to be documented as protocol quirk of ET inverter
	request[0] = 0xF7 // Slave ID 247
	// TODO: the RTU frame struct could have a method e.g.:
	//
	// ReadHoldingRegisters configures the RTU frame with values appropriate for
	// Read Holding Registers function with a given starting register and their
	// quantity.
	// func (r *RTU) ReadHoldingRegisters(start, quantity)
	request[1] = 0x03 // Function Code (Read Holding Registers)
	binary.BigEndian.PutUint16(request[2:4], startReg)
	binary.BigEndian.PutUint16(request[4:6], quantity)

	crc := calculateCRC16(request[0:6])
	binary.LittleEndian.PutUint16(request[6:8], crc)

	slog.Debug("Sending Modbus RTU bulk request", "start", startReg, "qty", quantity, "payload", hex.EncodeToString(request))

	_, err := s.dtlsConn.Write(request)
	if err != nil {
		return nil, fmt.Errorf("modbus write error: %w", err)
	}

	// Expected RTU: SlaveID(1), Func(1), ByteCount(1), Data(N), CRC(2)
	respBuf := make([]byte, 4096)
	n, err := s.dtlsConn.Read(respBuf)
	if err != nil {
		return nil, fmt.Errorf("modbus read error: %w", err)
	}
	// TODO and now:
	// assert header is 0xaa 0x55, and then:
	// var frame modbus.RTU
	// if err := frame.UnmarshalBinary(respBuf[2:]); err != nil {
	//    return fmt.Errorf("received invalid frame")
	// }

	// TODO better handle fixed 0xaa 0x55 header
	responseBytes := respBuf[:n]
	slog.Debug("Received Modbus RTU bulk response", "payload", hex.EncodeToString(responseBytes))

	return parseModbusBulkResponse(responseBytes)
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

func (s *service) close() error {
	if s.dtlsConn != nil {
		slog.Debug("Closing DTLS connection")
		return s.dtlsConn.Close()
	}
	return nil
}
