/**
 * Copyright 2022 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package main

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRoundTripper(t *testing.T) {

	tests := []struct {
		description      string
		startTime        time.Time
		endTime          time.Time
		expectedResponse int
		request          *http.Request
		expectedErr      error
	}{
		{
			description:      "Success",
			startTime:        time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC),
			endTime:          time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC),
			expectedResponse: http.StatusOK,
			request:          exampleRequest(1),
			expectedErr:      nil,
		},
		{
			description:      "503 Service Unavailable",
			startTime:        time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC),
			endTime:          time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC),
			expectedResponse: http.StatusServiceUnavailable,
			request:          exampleRequest(1),
			expectedErr:      nil,
		},
	}

	for _, tc := range tests {

		t.Run(tc.description, func(t *testing.T) {

			fakeTime := mockTime(tc.startTime, tc.endTime)
			fakeHandler := new(mockHandler)
			fakeHist := new(mockHistogram)
			fakeHist.On("With", mock.AnythingOfType("[]string")).Return().Times(2)
			fakeHist.On("Observe", mock.AnythingOfType("float64")).Return().Times(2)

			// Create a roundtripper with mock time and mock histogram
			m, err := newMetricWrapper(fakeTime, fakeHist)
			require.NoError(t, err)
			require.NotNil(t, m)

			// Create an http response
			expected := http.Response{
				StatusCode: tc.expectedResponse,
			}

			client := doerFunc(func(*http.Request) (*http.Response, error) {
				return &expected, tc.expectedErr
			})

			c := m.roundTripper(client)
			resp, err := c.Do(tc.request)

			// Check response
			assert.Equal(t, resp.StatusCode, tc.expectedResponse)

			// Check Error
			if tc.expectedErr != nil {
				assert.ErrorIs(t, tc.expectedErr, err)
			} else {
				assert.NoError(t, err)
			}

			// Check the histogram
			fakeHandler.AssertExpectations(t)

		})
	}

}

func TestNewMetricWrapper(t *testing.T) {

	tests := []struct {
		description   string
		expectedErr   error
		fakeTime      func() time.Time
		fakeHistogram metrics.Histogram
	}{
		{
			description:   "Success",
			expectedErr:   nil,
			fakeTime:      time.Now,
			fakeHistogram: &mockHistogram{},
		},
		{
			description:   "Nil Histogram",
			expectedErr:   errors.New("histogram cannot be nil"),
			fakeTime:      time.Now,
			fakeHistogram: nil,
		},
		{
			description:   "Nil Time",
			expectedErr:   nil,
			fakeTime:      nil,
			fakeHistogram: &mockHistogram{},
		},
	}

	for _, tc := range tests {

		t.Run(tc.description, func(t *testing.T) {

			// Make function call
			mw, err := newMetricWrapper(tc.fakeTime, tc.fakeHistogram)

			// Check histogram, nil should send error
			if tc.fakeHistogram == nil {
				assert.Equal(t, tc.expectedErr, err)
			}

			// Check time, nil should not error
			if tc.expectedErr == nil {
				require.NoError(t, err)
				require.NotNil(t, mw)
			}

		})
	}

}
