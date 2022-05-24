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
	errTest := errors.New("test error")
	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

	tests := []struct {
		description      string
		startTime        time.Time
		endTime          time.Time
		expectedResponse string
		request          *http.Request
		expectedErr      error
	}{
		{
			description:      "Success",
			startTime:        date1,
			endTime:          date2,
			expectedResponse: "200 OK",
			request:          exampleRequest(1),
			expectedErr:      nil,
		},
		{
			description:      "503 Service Unavailable",
			startTime:        date1,
			endTime:          date2,
			expectedResponse: "503 Service Unavailable",
			request:          exampleRequest(1),
			expectedErr:      nil,
		},
		{
			description:      "Network Error",
			startTime:        date1,
			endTime:          date2,
			expectedResponse: "network_err",
			request:          exampleRequest(1),
			expectedErr:      errTest,
		},
	}

	for _, tc := range tests {

		t.Run(tc.description, func(t *testing.T) {

			fakeTime := mockTime(tc.startTime, tc.endTime)
			fakeHandler := new(mockHandler)
			fakeHist := new(mockHistogram)
			histogramFunctionCall := []string{"code", tc.expectedResponse}
			fakeHist.On("With", histogramFunctionCall).Return().Once()
			fakeHist.On("Observe", mock.AnythingOfType("float64")).Return().Once()

			// Create a roundtripper with mock time and mock histogram
			m, err := newMetricWrapper(fakeTime, fakeHist)
			require.NoError(t, err)
			require.NotNil(t, m)

			// Create an http response
			expected := http.Response{
				Status: tc.expectedResponse,
			}

			client := doerFunc(func(*http.Request) (*http.Response, error) {
				return &expected, tc.expectedErr
			})

			c := m.roundTripper(client)
			resp, err := c.Do(tc.request)

			// Check Error
			if tc.expectedErr != nil {
				assert.ErrorIs(t, tc.expectedErr, err)
			} else {
				assert.NoError(t, err)
			}

			// Check response
			assert.Equal(t, resp.Status, tc.expectedResponse)

			// Check the histogram and expectations
			fakeHandler.AssertExpectations(t)
			fakeHist.AssertExpectations(t)

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
			expectedErr:   errNilHistogram,
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

			if tc.expectedErr == nil {
				// Check for no errors
				assert.NoError(t, err)
				require.NotNil(t, mw)

				// Check that the time and histogram aren't nil
				assert.NotNil(t, mw.now)
				assert.NotNil(t, mw.queryLatency)
				return
			}

			// with error checks
			assert.Nil(t, mw)
			assert.ErrorIs(t, err, tc.expectedErr)

		})
	}

}
