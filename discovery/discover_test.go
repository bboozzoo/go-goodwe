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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProbeResponse(t *testing.T) {
	tests := []struct {
		name        string
		resp        string
		wantErr     bool
		errContains string
		want        *probeResult
	}{
		{
			name:        "busy response",
			resp:        "dongle@sn,dtls_port:8899@busy,ExxxxETUxxxx",
			wantErr:     true,
			errContains: "inverter busy",
		},
		{
			name:        "busy response without dtls prefix",
			resp:        "inverter@busy",
			wantErr:     true,
			errContains: "inverter busy",
		},
		{
			name: "dtls response with port 8899",
			resp: "dongle@sn,dtls_port:8899,ExxxxETUxxxx",
			want: &probeResult{
				Serial:    "ExxxxETUxxxx",
				Transport: "dtls",
				DTLSPort:  8899,
			},
		},
		{
			name: "dtls response with non-default port",
			resp: "dongle@sn,dtls_port:9000,ExxxxETUxxxx",
			want: &probeResult{
				Serial:    "ExxxxETUxxxx",
				Transport: "dtls",
				DTLSPort:  9000,
			},
		},
		{
			name: "tcp response (ccm format)",
			resp: "ccm@sn,ccm@sn,ExxxxETUxxxx",
			want: &probeResult{
				Serial:    "ExxxxETUxxxx",
				Transport: "tcp",
			},
		},
		{
			name:        "empty response yields no serial error",
			resp:        "",
			wantErr:     true,
			errContains: "could not extract serial",
		},
		{
			name:        "no serial in tcp response",
			resp:        ",",
			wantErr:     true,
			errContains: "could not extract serial",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseProbeResponse(tc.resp)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.True(t, strings.Contains(err.Error(), tc.errContains),
						"expected error %q to contain %q", err.Error(), tc.errContains)
				}
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tc.want.Serial, got.Serial)
			assert.Equal(t, tc.want.Transport, got.Transport)
			assert.Equal(t, tc.want.DTLSPort, got.DTLSPort)
		})
	}
}
