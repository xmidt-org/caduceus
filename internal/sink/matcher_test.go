// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package sink

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/webhook-schema"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

var (
	logger  = zap.NewNop()
	matcher = &MatcherV1{
		events:  []*regexp.Regexp{regexp.MustCompile("iot")},
		matcher: []*regexp.Regexp{regexp.MustCompile("mac:112233445566")},
		logger:  logger,
	}
)

func TestIsMatch(t *testing.T) {
	tests := []struct {
		description string
		matcher     Matcher
		msg         *wrp.Message
		shouldMatch bool
	}{
		{
			description: "MatcherV1 - matching event & matcher",
			matcher:     matcher,
			msg: &wrp.Message{
				Destination: "event: iot",
				Source:      "mac:112233445566",
			},
			shouldMatch: true,
		},
		{
			description: "MatcherV1 - mismatch event",
			matcher:     matcher,
			msg: &wrp.Message{
				Destination: "event: test",
				Source:      "mac:112233445566",
			},
			shouldMatch: false,
		},
		{
			description: "MatcherV1 - mismatch matcher",
			matcher:     matcher,
			msg: &wrp.Message{
				Destination: "event: iot",
				Source:      "mac:00deadbeef00",
			},
			shouldMatch: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			if tc.shouldMatch {
				assert.True(t, tc.matcher.IsMatch(tc.msg))
			} else {
				assert.False(t, tc.matcher.IsMatch(tc.msg))
			}
		})
	}
}

// func TestMatcherV1_GetUrls(t *testing.T) {

// 	tests := []struct {
// 		description  string
// 		matcher      Matcher
// 		urlsExpected bool
// 		expectedUrls *ring.Ring
// 	}{
// 		{
// 			description: "no urls",
// 			matcher: &MatcherV1{
// 				urls: &ring.Ring{},
// 			},
// 			urlsExpected: false,
// 			expectedUrls: &ring.Ring{},
// 		},
// 	}

//		for _, tc := range tests {
//			t.Run(tc.description, func(t *testing.T) {
//				assert.Equal(t, tc.expectedUrls, tc.matcher.getUrls())
//			})
//		}
//	}
func TestUpdate_MatcherV1(t *testing.T) {
	tests := []struct {
		description string
		matcher     *MatcherV1
		registry    ancla.RegistryV1
		expectedErr error
	}{
		{
			description: "success - with device id",
			matcher:     matcher,
			registry: ancla.RegistryV1{
				Registration: webhook.RegistrationV1{
					Events: []string{"iot"},
					Matcher: webhook.MetadataMatcherConfig{
						DeviceID: []string{"mac:112233445566"},
					},
					Config: webhook.DeliveryConfig{
						ReceiverURL:     "www.example.com",
						AlternativeURLs: []string{"www.example2.com"},
					},
				},
			},
		},
		{
			description: "success - with .* device id",
			matcher:     matcher,
			registry: ancla.RegistryV1{
				Registration: webhook.RegistrationV1{
					Events: []string{"iot"},
					Matcher: webhook.MetadataMatcherConfig{
						DeviceID: []string{"mac:112233445566", ".*"},
					},
					Config: webhook.DeliveryConfig{
						ReceiverURL:     "www.example.com",
						AlternativeURLs: []string{"www.example2.com"},
					},
				},
			},
		},
		{
			description: "failing failureURL",
			matcher:     matcher,
			registry: ancla.RegistryV1{
				Registration: webhook.RegistrationV1{
					FailureURL: "localhost.io",
				},
			},
			expectedErr: fmt.Errorf("invalid URI for request"),
		},
		{
			description: "missing events",
			matcher:     matcher,
			registry: ancla.RegistryV1{
				Registration: webhook.RegistrationV1{
					Events: []string{},
				},
			},
			expectedErr: fmt.Errorf("events must not be empty"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			err := tc.matcher.update(tc.registry)
			if tc.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.expectedErr.Error())
			}
		})
	}
}
func TestNewMatcher(t *testing.T) {

	tests := []struct {
		description string
		registry    ancla.Register
		expectedErr error
	}{
		{
			description: "RegistryV1 - success",
			registry: &ancla.RegistryV1{
				PartnerIDs: []string{"comcast"},
				Registration: webhook.RegistrationV1{
					Address: "www.example.com",
					Events:  []string{"event1", "event2"},
				},
			},
		},
		{
			description: "RegistryV1 - fail",
			registry: &ancla.RegistryV1{
				PartnerIDs: []string{"comcast"},
				Registration: webhook.RegistrationV1{
					Address: "www.example.com",
				},
			},
			expectedErr: fmt.Errorf("events must not be empty"),
		},
		{
			description: "Invalid listener",
			registry:    &ancla.RegistryV1{},
			expectedErr: fmt.Errorf("invalid listener"),
		},
	}

	//TODO: want to update these tests to be a little more robust
	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			matcher, err := NewMatcher(tc.registry, logger)
			if tc.expectedErr == nil {
				assert.NoError(t, err)
				assert.NotNil(t, matcher)
			} else if tc.expectedErr != nil {
				assert.Error(t, err)
			}

		})
	}
}

func TestClientMock_Do(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &ClientMock{}
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)

	// Test case 1: Successful request
	resp, err := client.Do(req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	// Test case 2: Error in request
	req.URL.Scheme = "invalid"
	resp, err = client.Do(req)
	assert.Error(t, err)
	assert.Nil(t, resp)
}
