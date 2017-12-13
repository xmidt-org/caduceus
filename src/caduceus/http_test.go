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
	"encoding/json"
	"errors"
	"github.com/Comcast/webpa-common/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io/ioutil"
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

	fakeHealth := new(mockHealthTracker)
	fakeHealth.On("IncrementBucket", mock.AnythingOfType("int")).Return().Once()

	requestSuccessful := func(func(workerID int)) error {
		fakeHandler.HandleRequest(0, CaduceusRequest{})
		return nil
	}

	fakeEmptyRequests := new(mockCounter)
	fakeEmptyRequests.On("Add", mock.AnythingOfType("float64")).Return().Times(0)

	serverWrapper := &ServerHandler{
		Logger:          logger,
		caduceusHandler: fakeHandler,
		caduceusHealth:  fakeHealth,
		emptyRequests:   fakeEmptyRequests,
		doJob:           requestSuccessful,
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
		fakeHealth.AssertExpectations(t)
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

func TestProfilerHandler(t *testing.T) {
	assert := assert.New(t)

	var testData []interface{}
	testData = append(testData, "passed")

	logger := logging.DefaultLogger()
	fakeProfiler := new(mockServerProfiler)
	fakeProfiler.On("Report").Return(testData).Once()

	testProfilerWrapper := ProfileHandler{
		profilerData: fakeProfiler,
		Logger:       logger,
	}

	req := httptest.NewRequest("GET", "localhost:8080", nil)
	req.Header.Set("Content-Type", "application/json")

	t.Run("TestServeHTTPHappyPath", func(t *testing.T) {
		w := httptest.NewRecorder()
		testProfilerWrapper.ServeHTTP(w, req)
		resp := w.Result()

		var testResults []interface{}
		dec := json.NewDecoder(resp.Body)
		err := dec.Decode(&testResults)
		assert.Nil(err)

		assert.Equal(http.StatusOK, resp.StatusCode)
		assert.Equal(1, len(testResults))
		assert.Equal("passed", testResults[0].(string))
		fakeProfiler.AssertExpectations(t)
	})

	t.Run("TestServeHTTPSadPath", func(t *testing.T) {
		innocentList := make([]interface{}, 1)
		innocentList[0] = make(chan int) //channels cannot be marshaled

		badProfiler := new(mockServerProfiler)
		badProfiler.On("Report").Return(innocentList).Once()
		testProfilerWrapper.profilerData = badProfiler

		w := httptest.NewRecorder()
		testProfilerWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusInternalServerError, resp.StatusCode)
		badProfiler.AssertExpectations(t)
	})

	t.Run("TestServeHTTPSadNoProblemPath", func(t *testing.T) {
		badProfiler := new(mockServerProfiler)

		badProfiler.On("Report").Return(nil).Once()
		testProfilerWrapper.profilerData = badProfiler

		w := httptest.NewRecorder()
		testProfilerWrapper.ServeHTTP(w, req)
		resp := w.Result()

		responseBody, err := ioutil.ReadAll(resp.Body)

		assert.Nil(err)
		assert.Equal(http.StatusOK, resp.StatusCode)
		assert.Equal("[]\n", string(responseBody))
		badProfiler.AssertExpectations(t)
	})
}
