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

	chooser := NewPriorityChooser(dialer.srvs)
	dialer = NewSRVRecordDialer(WithChooser(chooser))
	conn, err := dialer.DialContext(context.Background(), "", "")

	suite.NotNil(conn)
	suite.NoError(err)
}
