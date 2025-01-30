// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package dns

import (
	"context"
	"net"
	"testing"

	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/suite"
)

type DnsSuite struct {
	suite.Suite

	resolver Resolver
	fqdns    []string
	// rt       http.RoundTripper
}

func newMockResolver() *mockdns.Resolver {
	return &mockdns.Resolver{
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
	}
}

func (suite *DnsSuite) SetupSuite() {
	suite.resolver = newMockResolver()
	suite.fqdns = append(suite.fqdns, "example.org.", "invalid.example.org")
}

func TestDnsSuite(t *testing.T) {
	suite.Run(t, new(DnsSuite))
}

func (suite *DnsSuite) TestDialContext() {

	dialer := NewSRVRecordDialer(WithFQDNS(suite.fqdns), WithResolver(suite.resolver))
	suite.NotNil(dialer.resolver)
	suite.NotEmpty(dialer.fqdns)

	transport, err := NewCustomTransport(WithCustomDialer(dialer))
	suite.NotEmpty(dialer.srvs)
	suite.Error(err)
	suite.NotNil(transport)

	weightedDialer := dialer
	wChooser := NewWeightChooser(dialer.srvs)
	weightedDialer = NewSRVRecordDialer(WithChooser(wChooser))
	conn, err := weightedDialer.DialContext(context.Background(), "", "")

	suite.NotNil(conn)
	suite.NoError(err)

	conn = nil
	err = nil
	priorityDialer := dialer 
	pChooser := NewPriorityChooser(dialer.srvs)
	priorityDialer = NewSRVRecordDialer(WithChooser(pChooser))
	conn, err = priorityDialer.DialContext(context.Background(), "","")

	suite.NotNil(conn)
	suite.NoError(err)
}

// func TestNewSRVRecordDialer(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		fqdns    []string
// 		sortBy   string
// 		resolver Resolver
// 	}{
// 		{
// 			name: "empty fqdn",
// 		},
// 		{
// 			name:   "valid fqdn - priority sort",
// 			fqdns:  []string{"example.org."},
// 			sortBy: "priority",
// 			resolver: &mockdns.Resolver{
// 				Zones: map[string]mockdns.Zone{
// 					"_._.example.org.": {
// 						SRV: []net.SRV{
// 							{
// 								Target:   "valid.example.com",
// 								Port:     443,
// 								Priority: 1,
// 								Weight:   50,
// 							},
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name:   "invalid fqdn",
// 			fqdns:  []string{"invalid.example.org"},
// 			sortBy: "priority",
// 			resolver: &mockdns.Resolver{
// 				Zones: map[string]mockdns.Zone{
// 					"_._.example.org.": {
// 						SRV: []net.SRV{
// 							{
// 								Target:   "valid.example.com",
// 								Port:     443,
// 								Priority: 1,
// 								Weight:   50,
// 							},
// 						},
// 					},
// 				},
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			dialer := NewSRVRecordDialer(WithResolver(tt.resolver), WithFQDNS(tt.fqdns))
// 			assert.NotNil(t, dialer)

// 		})
// 	}
// }

// func TestDialContext(t *testing.T) {
// 	tests := []struct {
// 		name          string
// 		fqdns         []string
// 		chooser       Chooser
// 		resolver      Resolver
// 		expectedError bool
// 	}{
// 		{
// 			name:  "valid fqdn - weight sort",
// 			fqdns: []string{"example.org."},
// 			chooser: &WeightChooser{
// 				srvs: []*net.SRV{
// 					{
// 						Target:   "example.com",
// 						Port:     443,
// 						Priority: 1,
// 						Weight:   100,
// 					},
// 					{
// 						Target:   "valid2.example.com",
// 						Port:     443,
// 						Priority: 1,
// 						Weight:   50,
// 					},
// 				},
// 			},
// 			resolver: &mockdns.Resolver{
// 				Zones: map[string]mockdns.Zone{
// 					"_._.example.org.": {
// 						SRV: []net.SRV{
// 							{
// 								Target:   "example.com",
// 								Port:     443,
// 								Priority: 1,
// 								Weight:   100,
// 							},
// 							{
// 								Target:   "valid2.example.com",
// 								Port:     443,
// 								Priority: 1,
// 								Weight:   50,
// 							},
// 						},
// 					},
// 				},
// 			},
// 			expectedError: false,
// 		},
// 		{
// 			name:  "valid fqdn - priority sort",
// 			fqdns: []string{"example.org."},
// 			chooser: &PriorityChooser{
// 				srvs: []*net.SRV{
// 					{
// 						Target:   "example.com",
// 						Port:     443,
// 						Priority: 1,
// 						Weight:   50,
// 					},
// 					{
// 						Target:   "valid2.example.com",
// 						Port:     443,
// 						Priority: 2,
// 						Weight:   50,
// 					},
// 				},
// 			},
// 			resolver: &mockdns.Resolver{
// 				Zones: map[string]mockdns.Zone{
// 					"_._.example.org.": {
// 						SRV: []net.SRV{
// 							{
// 								Target:   "example.com",
// 								Port:     443,
// 								Priority: 1,
// 								Weight:   50,
// 							},
// 							{
// 								Target:   "valid2.example.com",
// 								Port:     443,
// 								Priority: 2,
// 								Weight:   50,
// 							},
// 						},
// 					},
// 				},
// 			},
// 			expectedError: false,
// 		},
// 		{
// 			name:  "invalid fqdn",
// 			fqdns: []string{"invalid.example.org"},
// 			chooser: &PriorityChooser{
// 				srvs: []*net.SRV{
// 					{
// 						Target:   "valid.example.com",
// 						Port:     443,
// 						Priority: 1,
// 						Weight:   50,
// 					},
// 				},
// 			},
// 			resolver: &mockdns.Resolver{
// 				Zones: map[string]mockdns.Zone{
// 					"_._.example.org.": {
// 						SRV: []net.SRV{
// 							{
// 								Target:   "valid.example.com",
// 								Port:     443,
// 								Priority: 1,
// 								Weight:   50,
// 							},
// 						},
// 					},
// 				},
// 			},
// 			expectedError: true,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			dialer, err := NewRoundTripper(WithFQDNS(tt.fqdns), WithChooser(tt.chooser), WithResolver(tt.resolver))
// 			if tt.expectedError {
// 				assert.Error(t, err)
// 				return
// 			}
// 			assert.NoError(t, err)
// 			assert.NotNil(t, dialer)

// 			srvDialer, ok := dialer.(*http.Transport)
// 			assert.True(t, ok)

// 			conn, err := srvDialer.DialContext(context.Background(), "", "")
// 			if tt.expectedError {
// 				assert.Error(t, err)
// 			} else {
// 				assert.NoError(t, err)
// 				assert.NotNil(t, conn)
// 			}
// 		})
// 	}
// }
