// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package dns

import (
	// "net"
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"
)

func TestNewSRVRecordDialer(t *testing.T) {
	tests := []struct {
		name          string
		fqdns         []string
		sortBy        string
		resolver      Resolver
		expectedError bool
	}{
		{
			name:          "empty fqdn",
			expectedError: false,
		},
		{
			name:   "valid fqdn - priority sort",
			fqdns:  []string{"example.org."},
			sortBy: "priority",
			resolver: &mockdns.Resolver{
				Zones: map[string]mockdns.Zone{
					"_._.example.org.": {
						SRV: []net.SRV{
							{
								Target:   "valid.example.com",
								Port:     443,
								Priority: 1,
								Weight:   50,
							},
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name:   "invalid fqdn",
			fqdns:  []string{"invalid.example.org"},
			sortBy: "priority",
			resolver: &mockdns.Resolver{
				Zones: map[string]mockdns.Zone{
					"_._.example.org.": {
						SRV: []net.SRV{
							{
								Target:   "valid.example.com",
								Port:     443,
								Priority: 1,
								Weight:   50,
							},
						},
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialer, err := NewSRVRecordDialer(tt.fqdns, tt.sortBy, tt.resolver)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, dialer)
			}
		})
	}
}
func TestDialContext(t *testing.T) {
	tests := []struct {
		name          string
		fqdns         []string
		sortBy        string
		resolver      Resolver
		expectedError bool
	}{
		{
			name:   "valid fqdn - weight sort",
			fqdns:  []string{"example.org."},
			sortBy: "weight",
			resolver: &mockdns.Resolver{
				Zones: map[string]mockdns.Zone{
					"_._.example.org.": {
						SRV: []net.SRV{
							{
								Target:   "example.com",
								Port:     443,
								Priority: 1,
								Weight:   100,
							},
							{
								Target:   "valid2.example.com",
								Port:     443,
								Priority: 1,
								Weight:   50,
							},
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name:   "valid fqdn - priority sort",
			fqdns:  []string{"example.org."},
			sortBy: "priority",
			resolver: &mockdns.Resolver{
				Zones: map[string]mockdns.Zone{
					"_._.example.org.": {
						SRV: []net.SRV{
							{
								Target:   "example.com",
								Port:     443,
								Priority: 1,
								Weight:   50,
							},
							{
								Target:   "valid2.example.com",
								Port:     443,
								Priority: 2,
								Weight:   50,
							},
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name:   "invalid fqdn",
			fqdns:  []string{"invalid.example.org"},
			sortBy: "priority",
			resolver: &mockdns.Resolver{
				Zones: map[string]mockdns.Zone{
					"_._.example.org.": {
						SRV: []net.SRV{
							{
								Target:   "valid.example.com",
								Port:     443,
								Priority: 1,
								Weight:   50,
							},
						},
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialer, err := NewSRVRecordDialer(tt.fqdns, tt.sortBy, tt.resolver)
			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, dialer)

			srvDialer, ok := dialer.(*http.Transport)
			assert.True(t, ok)

			conn, err := srvDialer.DialContext(context.Background(), "", "")
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, conn)
			}
		})
	}
}
