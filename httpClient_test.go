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
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRoundTripper(t *testing.T) {

	tests := []struct {
		description      string
		startTime        time.Time
		endTime          time.Time
		expectedResponse int
		request          *http.Request
	}{
		{
			description:      "Success",
			startTime:        time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC),
			endTime:          time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC),
			expectedResponse: http.StatusOK,
			request:          exampleRequest(1),
		},
	}

	for _, tc := range tests {

		fakeTime := mockTime(tc.startTime, tc.endTime)
		fakeHandler := new(mockHandler)
		fakeHist := new(mockHistogram)
		fakeHist.On("With", mock.AnythingOfType("[]string")).Return().Times(2)
		fakeHist.On("Observe", mock.AnythingOfType("float64")).Return().Times(2)

		t.Run(tc.description, func(t *testing.T) {

			// Create a roundtripper with mock time and mock histogram
			m, err := newMetricWrapper(fakeTime, fakeHist)

			// Create an http response
			expected := http.Response{
				StatusCode: tc.expectedResponse,
			}

			client := &mockHttpClient{
				MockDo: func(*http.Request) (*http.Response, error) {
					return &expected, nil
				},
			}

			c := m.roundTripper(client)
			resp, err := c.Do(tc.request)

			// Check response
			assert.Equal(t, resp.StatusCode, tc.expectedResponse)

			// Check Error
			assert.Equal(t, err, nil)

			// Check the histogram
			fakeHandler.AssertExpectations(t)

		})
	}

}
