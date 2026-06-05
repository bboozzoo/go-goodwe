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
	defer conn.Close()

	// Set deadline based on context
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	conn.SetDeadline(deadline)

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
	re := regexp.MustCompile(`dtls_port:(\d+),`)
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
func (s *service) readModbusBulk(ctx context.Context, startReg uint16, quantity uint16) ([]uint16, error) {
	if s.dtlsConn == nil {
		return nil, errors.New("no DTLS connection established")
	}

	// Modbus RTU over DTLS:
	// Slave ID (1) | Function Code (1) | Register Address (2) | Quantity (2) | CRC (2)
	request := make([]byte, 8)
	request[0] = 0xF7                               // Slave ID 247
	request[1] = 0x03                               // Function Code (Read Holding Registers)
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

	responseBytes := respBuf[:n]
	slog.Debug("Received Modbus RTU bulk response", "payload", hex.EncodeToString(responseBytes))

	if n < 7 {
		return nil, fmt.Errorf("modbus response too short: %d bytes", n)
	}

	// Validate CRC
	expectedCRC := binary.LittleEndian.Uint16(responseBytes[n-2 : n])
	actualCRC := calculateCRC16(responseBytes[0 : n-2])
	if expectedCRC != actualCRC {
		return nil, fmt.Errorf("modbus CRC mismatch: expected %04X, got %04X", expectedCRC, actualCRC)
	}

	// Validate Function Code
	funcCode := responseBytes[1]
	if funcCode&0x80 != 0 {
		return nil, fmt.Errorf("modbus error response: code 0x%02X", funcCode)
	}

	byteCount := int(responseBytes[2])
	if n < 3+byteCount+2 {
		return nil, fmt.Errorf("incomplete modbus data: expected %d bytes, got %d", 3+byteCount+2, n)
	}

	results := make([]uint16, byteCount/2)
	for i := 0; i < len(results); i++ {
		results[i] = binary.BigEndian.Uint16(responseBytes[3+(i*2) : 3+(i*2)+2])
	}

	return results, nil
}

func (s *service) close() error {
	if s.dtlsConn != nil {
		slog.Debug("Closing DTLS connection")
		return s.dtlsConn.Close()
	}
	return nil
}
