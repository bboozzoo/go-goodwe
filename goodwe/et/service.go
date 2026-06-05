package et

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pion/dtls/v2"
)

// service handles low-level communication with the inverter.
type service struct {
	ip         string
	probePort  int
	dtlsPort   int
	dtlsConn   net.Conn
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

	response := string(buf[:n])
	return s.parseProbeResponse(response)
}

// parseProbeResponse extracts serial and DTLS port from the string.
// Expected format example: "dongle@sn,dtls_port:8899,9012KEUB258L0189"
func (s *service) parseProbeResponse(resp string) (*probeResult, error) {
	// This regex is a bit loose to account for variations in the example string
	// Looking for: dtls_port:(\d+), and then the serial number part.
	re := regexp.MustCompile(`dtls_port:(\d+),`)
	matches := re.FindStringSubmatch(resp)
	if len(matches) < 2 {
		return nil, fmt.Errorf("could not find dtls_port in response: %s", resp)
	}

	port, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("invalid dtls_port: %w", err)
	}

	// Attempt to extract serial number. In the example: "KEUB258L0189"
	// The example shows "dongle@sn,dtls_port:8899,9012KEUB258L0189"
	// Let's try to grab the last part.
	parts := strings.Split(resp, ",")
	serial := ""
	if len(parts) > 0 {
		// In the example, the serial seems to be part of the last segment or after some noise.
		// This is brittle, but follows the provided pattern.
		lastPart := parts[len(parts)-1]
		// If it contains the "9012" prefix from the example, we might need to strip it.
		// For now, let's just take the last 12 characters as a heuristic for serials.
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

	// DTLS config - In a real scenario, we might need to handle certificates.
	// For GoodWe, it often uses a specific or no-verification setup depending on the model.
	// We'll use a standard config for now.
	config := &dtls.Config{
		CipherSuites: []dtls.CipherSuiteID{
			dtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			dtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		// In many IoT cases, we might need InsecureSkipVerify if they use self-signed certs.
		InsecureSkipVerify: true, 
	}

	conn, err := dtls.Dial(addr, config)
	if err != nil {
		return fmt.Errorf("dtls dial failed: %w", err)
	}

	s.dtlsConn = conn
	s.dtlsPort = port
	return nil
}

// readModbusRegister performs a single Modbus register read over the DTLS connection.
func (s *service) readModbusRegister(ctx context.Context, reg uint16) (uint16, error) {
	if s.dtlsConn == nil {
		return 0, errors.New("no DTLS connection established")
	}

	// Modbus TCP-style ADU over DTLS:
	// Transaction ID (2), Protocol ID (2), Length (2), Unit ID (1), Function Code (1), Register (2), Count (2)
	// We'll use a simplified version for the example.
	request := make([]byte, 12)
	binary.BigEndian.PutUint16(request[0:2], 0x0001) // Transaction ID
	binary.BigEndian.PutUint16(request[2:4], 0x0000) // Protocol ID
	binary.BigEndian.PutUint16(request[4:6], 0x0006) // Length
	request[6] = 0x01                               // Unit ID
	request[7] = 0x03                               // Function Code (Read Holding Registers)
	binary.BigEndian.PutUint16(request[8:10], reg)  // Register Address
	binary.BigEndian.PutUint16(request[10:12], 0x0001) // Quantity

	// Write with context support
	_, err := s.dtlsConn.Write(request)
	if err != nil {
		return 0, fmt.Errorf("modbus write error: %w", err)
	}

	// Read response
	// Expected: Trans(2), Prot(2), Len(2), Unit(1), Func(1), ByteCount(1), Data(N)
	// For 1 register (2 bytes), total = 2+2+2+1+1+1+2 = 11 bytes
	respBuf := make([]byte, 256)
	n, err := s.dtlsConn.Read(respBuf)
	if err != nil {
		return 0, fmt.Errorf("modbus read error: %w", err)
	}

	if n < 9 {
		return 0, fmt.Errorf("modbus response too short: %d bytes", n)
	}

	// Validate Function Code (should be 0x03 or 0x83 for error)
	funcCode := respBuf[7]
	if funcCode&0x80 != 0 {
		return 0, fmt.Errorf("modbus error response: code 0x%02X", funcCode)
	}

	// Data starts at index 9
	if n < 11 {
		return 0, fmt.Errorf("incomplete modbus data")
	}

	val := binary.BigEndian.Uint16(respBuf[9:11])
	return val, nil
}

func (s *service) close() error {
	if s.dtlsConn != nil {
		return s.dtlsConn.Close()
	}
	return nil
}
