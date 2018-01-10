/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
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
	"github.com/Comcast/webpa-common/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerHandler(t *testing.T) {
	assert := assert.New(t)

	logger := logging.DefaultLogger()
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("CaduceusRequest")).Return().Once()

	requestSuccessful := func(func(workerID int)) error {
		fakeHandler.HandleRequest(0, CaduceusRequest{})
		return nil
	}

	fakeEmptyRequests := new(mockCounter)
	fakeEmptyRequests.On("Add", mock.AnythingOfType("float64")).Return().Times(0)

	fakeErrorRequests := new(mockCounter)
	fakeErrorRequests.On("Add", mock.AnythingOfType("float64")).Return().Times(0)

	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)

	serverWrapper := &ServerHandler{
		Logger:             logger,
		caduceusHandler:    fakeHandler,
		errorRequests:      fakeErrorRequests,
		emptyRequests:      fakeEmptyRequests,
		incomingQueueDepth: fakeQueueDepth,
		doJob:              requestSuccessful,
	}

	req := httptest.NewRequest("POST", "localhost:8080", strings.NewReader("Test payload."))
	// todo: maybe will become useful later
	// badReq := httptest.NewRequest("GET", "localhost:8080", strings.NewReader("Test payload."))

	t.Run("TestServeHTTPHappyPath", func(t *testing.T) {
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		serverWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(202, resp.StatusCode)
		fakeHandler.AssertExpectations(t)
	})

	t.Run("TestServeHTTPFullQueue", func(t *testing.T) {
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		requestTimeout := func(func(workerID int)) error {
			return errors.New("Intentional time out")
		}
		serverWrapper.doJob = requestTimeout
		serverWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusRequestTimeout, resp.StatusCode)
	})
}
