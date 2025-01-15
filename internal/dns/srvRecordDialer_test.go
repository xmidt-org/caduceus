package dns

import (
	// "net"
	"context"
	"errors"
	"math/rand"
	"net"
	"net/http"
	"strconv"
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
func TestGetAddrByWeight(t *testing.T) {
	tests := []struct {
		name          string
		srvs          []*net.SRV
		expectedIndex int
		expectedError error
	}{
		{
			name: "single srv record",
			srvs: []*net.SRV{
				{
					Target:   "example.com",
					Port:     443,
					Priority: 1,
					Weight:   100,
				},
			},
			expectedIndex: 0,
			expectedError: nil,
		},
		{
			name: "failed to choose SRV record",
			srvs: []*net.SRV{
				{
					Target:   "example1.com",
					Port:     443,
					Priority: 1,
					Weight:   0,
				},
				{
					Target:   "example2.com",
					Port:     443,
					Priority: 1,
					Weight:   0,
				},
			},
			expectedIndex: -1, // index can vary due to randomness
			expectedError: errors.New("failed to choose an SRV record by weight"),
		},
		{
			name:          "no srv records",
			srvs:          []*net.SRV{},
			expectedIndex: -1,
			expectedError: errors.New("no SRV records available"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, index, err := getAddrByWeight(tt.srvs)
			if tt.expectedError != nil {
				assert.Nil(t, addr)
				assert.Equal(t, tt.expectedIndex, index)
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, addr)

			}
		})
	}
}

func TestWeighting(t *testing.T) {
	srvs := mockSRVs(t)
	counts := make(map[int]int)

	for i:= 0; i < 1000; i++{
		srv, _, err := getAddrByWeight(srvs)
		assert.NoError(t, err)
		counts[int(srv.Weight)]++
	}
	verifyWeightCounts(t, counts, srvs)
}

func mockSRVs(t *testing.T) []*net.SRV {
	t.Helper()
	n := 10
	srvs := make([]*net.SRV, 0, n)

	nums := rand.Perm(n)

	for _, num := range nums {
		i := strconv.Itoa(num)
		t := "example" + i + ".com"
		srv := &net.SRV{
			Target:   t,
			Port:     443,
			Priority: 0,
			Weight:   uint16(num),
		}
		srvs = append(srvs, srv)
	}
	return srvs
}

func verifyWeightCounts(t *testing.T, counts map[int]int, weights []*net.SRV) {
	t.Helper()

	if zero := counts[0]; zero != 0 {
		t.Error("Weight of 0 resulted in a nonzero result: ", zero)
	}

	// Test that higher weighted choices were chosen more often than their lower
	// weighted peers.
	for i, c := range weights[0 : len(weights)-1] {
		next := weights[i+1]
		cw, nw := c.Weight, next.Weight
		if !(counts[int(cw)] < counts[int(nw)]) {
			t.Error("Value not lesser", cw, nw, counts[int(cw)], counts[int(nw)])
		}
	}
}
