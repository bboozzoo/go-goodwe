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

package discovery

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bboozzoo/go-goodwe"
	"github.com/bboozzoo/go-goodwe/et"
)

// probeResult holds the parsed probe response information.
type probeResult struct {
	Serial string
	// Transport type: "dtls" or "tcp"
	Transport string
	// DTLS port (only valid when Transport is "dtls")
	DTLSPort int
}

// probe sends the WIFIKIT UDP packet and returns the raw response string.
func probe(ctx context.Context, ip string) (string, error) {
	probeMsg := []byte("WIFIKIT-214028-READ")

	slog.Debug("Sending UDP probe", "ip", ip, "payload", hex.EncodeToString(probeMsg))

	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:%d", ip, 48899), 2*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to dial UDP: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Warn("Failed to close UDP probe connection", "error", err)
		}
	}()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	_ = conn.SetDeadline(deadline)

	_, err = conn.Write(probeMsg)
	if err != nil {
		return "", fmt.Errorf("failed to write probe: %w", err)
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("failed to read probe response: %w", err)
	}

	response := string(buf[:n])
	slog.Debug("Received UDP probe response", "payload", hex.EncodeToString(buf[:n]))
	return response, nil
}

// parseProbeResponse extracts serial and transport information from the probe response.
// Supports two formats:
//
//	DTLS: "dongle@sn,dtls_port:8899,<serial>" or "dongle@sn,dtls_port:8899@busy,<serial>"
//	TCP:  "ccm@sn,ccm@sn,<serial>"
func parseProbeResponse(resp string) (*probeResult, error) {
	if strings.Contains(resp, "@busy") {
		return nil, fmt.Errorf("inverter busy, try again later: %s", resp)
	}

	// Check for DTLS response
	re := regexp.MustCompile(`dtls_port:(\d+)`)
	if matches := re.FindStringSubmatch(resp); len(matches) >= 2 {
		port, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("invalid dtls_port: %w", err)
		}

		parts := strings.Split(resp, ",")
		serial := extractSerial(parts)

		return &probeResult{
			Serial:    serial,
			Transport: "dtls",
			DTLSPort:  port,
		}, nil
	}

	// TCP response: "ccm@sn,ccm@sn,<serial>"
	parts := strings.Split(resp, ",")
	serial := extractSerial(parts)
	if serial == "" {
		return nil, fmt.Errorf("could not extract serial from response: %s", resp)
	}

	return &probeResult{
		Serial:    serial,
		Transport: "tcp",
	}, nil
}

// extractSerial extracts the serial number from comma-separated probe response parts.
func extractSerial(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	lastPart := parts[len(parts)-1]
	if len(lastPart) >= 12 {
		return lastPart
	}
	return lastPart
}

// isETModel checks if the serial number matches known ET inverter model tags.
func isETModel(serial string) bool {
	tags := []string{
		// Platform 205
		"ETU", "ETL", "ETR", "BHN", "EHU", "BHU", "EHR", "BTU",
		// Platform 745 LV
		"ESN", "EBN", "EMN", "SPN", "ERN", "ESC", "HLB", "HMB", "HBB", "EOA",
		// Platform 745 HV
		"ETT", "HTA", "HUB", "AEB", "SPB", "CUB", "EUB", "HEB", "ERB",
		"BTT", "ETF", "ARB", "URB", "EBR", "NAH",
		// Platform 753
		"AES", "HHI", "ABP", "EHB", "HSB", "HUA", "CUA",
		// Qianhai
		"ETC", "BTC", "BTN",
		// ET series with battery
		"KET", "KEU",
	}
	for _, tag := range tags {
		if strings.Contains(serial, tag) {
			return true
		}
	}
	return false
}

// Discover probes the inverter at the given IP address, detects its type and
// transport protocol, and returns a fully configured Inverter ready for use.
//
// Returns goodwe.ErrUnsupported if the inverter is not a supported model.
func Discover(ctx context.Context, ip string) (goodwe.Inverter, error) {
	resp, err := probe(ctx, ip)
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}

	pr, err := parseProbeResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}

	if !isETModel(pr.Serial) {
		return nil, fmt.Errorf("%w: serial=%s: not a recognized ET series inverter",
			goodwe.ErrUnsupported, pr.Serial)
	}

	var transport et.Transport
	switch pr.Transport {
	case "dtls":
		transport = et.NewDTLSTransport(ip, pr.DTLSPort)
	case "tcp":
		transport = et.NewTCPTransport(ip, 502)
	default:
		return nil, fmt.Errorf("%w: serial=%s: unknown transport %q",
			goodwe.ErrUnsupported, pr.Serial, pr.Transport)
	}

	inv := et.NewWithTransport(pr.Serial, transport)
	return inv, nil
}
