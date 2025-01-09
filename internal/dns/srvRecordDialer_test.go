package dns

import (
	// "net"
	"net"
	"testing"

	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"
)

func TestNewSRVRecordDialer(t *testing.T) {
	tests := []struct {
		name              string
		fqdns             []string
		sortBy            string
		resolver          Resolver
		expectedError     bool
	}{
		{
			name: "empty fqdn",
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
			name: "invalid fqdn",
			fqdns: []string{"invalid.example.org"},
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
		{
			name: "unknown sortBy",
			fqdns:  []string{"example.org."},
			sortBy: "none",
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
		{
			name:   "valid fqdn - weight sort",
			fqdns:  []string{"example.org."},
			sortBy: "weight",
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

