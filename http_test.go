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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/wrp-go/v2"
)

func exampleRequest(list ...string) *http.Request {
	var buffer bytes.Buffer

	trans := "1234"
	ct := "application/msgpack"
	url := "localhost:8080"

	for i := range list {
		switch {
		case 0 == i:
			trans = list[i]
		case 1 == i:
			ct = list[i]
		case 2 == i:
			url = list[i]
		}

	}
	wrp.NewEncoder(&buffer, wrp.Msgpack).Encode(
		&wrp.Message{
			Source:          "mac:112233445566/lmlite",
			TransactionUUID: trans,
			ContentType:     ct,
			Destination:     "event:bob/magic/dog",
			Payload:         []byte("Hello, world."),
		})

	r := bytes.NewReader(buffer.Bytes())
	req := httptest.NewRequest("POST", url, r)
	req.Header.Set("Content-Type", "application/msgpack")

	return req
}

func TestServerHandler(t *testing.T) {
	fmt.Print("TestingeServerHandler")

	assert := assert.New(t)

	logger := logging.DefaultLogger()
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).Return().Once()

	fakeEmptyRequests := new(mockCounter)
	fakeErrorRequests := new(mockCounter)
	fakeInvalidCount := new(mockCounter)
	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		errorRequests:            fakeErrorRequests,
		emptyRequests:            fakeEmptyRequests,
		invalidCount:             fakeInvalidCount,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
	}

	t.Run("TestServeHTTPHappyPath", func(t *testing.T) {
		w := httptest.NewRecorder()

		serverWrapper.ServeHTTP(w, exampleRequest())
		resp := w.Result()

		assert.Equal(http.StatusAccepted, resp.StatusCode)
		if nil != resp.Body {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
		fakeHandler.AssertExpectations(t)
	})
}

func TestServerHandlerFixWrp(t *testing.T) {
	fmt.Printf("TestServerHandlerFixWrp")

	assert := assert.New(t)

	logger := logging.DefaultLogger()
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).Return().Once()

	fakeEmptyRequests := new(mockCounter)
	fakeErrorRequests := new(mockCounter)
	fakeInvalidCount := new(mockCounter)
	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)

	fakeIncomingContentTypeCount := new(mockCounter)
	fakeIncomingContentTypeCount.On("With", []string{"content_type", "application/msgpack"}).Return(fakeIncomingContentTypeCount)
	fakeIncomingContentTypeCount.On("With", []string{"content_type", ""}).Return(fakeIncomingContentTypeCount)
	fakeIncomingContentTypeCount.On("Add", 1.0).Return()

	fakeModifiedWRPCount := new(mockCounter)
	fakeModifiedWRPCount.On("With", []string{"reason", bothEmptyReason}).Return(fakeIncomingContentTypeCount).Once()
	fakeModifiedWRPCount.On("Add", 1.0).Return().Once()

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		errorRequests:            fakeErrorRequests,
		emptyRequests:            fakeEmptyRequests,
		invalidCount:             fakeInvalidCount,
		modifiedWRPCount:         fakeModifiedWRPCount,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
	}

	t.Run("TestServeHTTPHappyPath", func(t *testing.T) {
		w := httptest.NewRecorder()

		serverWrapper.ServeHTTP(w, exampleRequest("", ""))
		resp := w.Result()

		assert.Equal(http.StatusAccepted, resp.StatusCode)
		if nil != resp.Body {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
		fakeHandler.AssertExpectations(t)
	})
}

func TestServerHandlerFull(t *testing.T) {
	fmt.Printf("TestServerHandlerFull")

	assert := assert.New(t)

	logger := logging.DefaultLogger()
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Times(2)

	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
	}

	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
		w := httptest.NewRecorder()

		/* Act like we have 1 in flight */
		serverWrapper.incomingQueueDepth = 1

		/* Make the call that goes over the limit */
		serverWrapper.ServeHTTP(w, exampleRequest())
		resp := w.Result()

		assert.Equal(http.StatusServiceUnavailable, resp.StatusCode)
		if nil != resp.Body {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
	})
}

func TestServerEmptyPayload(t *testing.T) {
	fmt.Printf("TestServerEmptyPayLoad")

	assert := assert.New(t)

	var buffer bytes.Buffer
	r := bytes.NewReader(buffer.Bytes())
	req := httptest.NewRequest("POST", "localhost:8080", r)
	req.Header.Set("Content-Type", "application/msgpack")

	logger := logging.DefaultLogger()
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Times(2)

	fakeEmptyRequests := new(mockCounter)
	fakeEmptyRequests.On("Add", mock.AnythingOfType("float64")).Return().Once()
	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		emptyRequests:            fakeEmptyRequests,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
	}

	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
		w := httptest.NewRecorder()

		/* Make the call that goes over the limit */
		serverWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusBadRequest, resp.StatusCode)
		if nil != resp.Body {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
	})
}

func TestServerUnableToReadBody(t *testing.T) {
	fmt.Printf("TestServerUnableToReadBody")

	assert := assert.New(t)

	var buffer bytes.Buffer
	r := iotest.TimeoutReader(bytes.NewReader(buffer.Bytes()))

	_, _ = r.Read(nil)
	req := httptest.NewRequest("POST", "localhost:8080", r)
	req.Header.Set("Content-Type", "application/msgpack")

	logger := logging.DefaultLogger()
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Once()

	fakeErrorRequests := new(mockCounter)
	fakeErrorRequests.On("Add", mock.AnythingOfType("float64")).Return().Once()
	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		errorRequests:            fakeErrorRequests,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
	}

	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
		w := httptest.NewRecorder()

		/* Make the call that goes over the limit */
		serverWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusBadRequest, resp.StatusCode)
		if nil != resp.Body {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
	})
}

func TestServerInvalidBody(t *testing.T) {
	fmt.Printf("TestServerInvalidBody")

	assert := assert.New(t)

	r := bytes.NewReader([]byte("Invalid payload."))

	_, _ = r.Read(nil)
	req := httptest.NewRequest("POST", "localhost:8080", r)
	req.Header.Set("Content-Type", "application/msgpack")

	logger := logging.DefaultLogger()
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Once()

	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	fakeInvalidCount := new(mockCounter)
	fakeInvalidCount.On("Add", mock.AnythingOfType("float64")).Return().Once()

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		invalidCount:             fakeInvalidCount,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
	}

	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
		w := httptest.NewRecorder()

		/* Make the call that goes over the limit */
		serverWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusBadRequest, resp.StatusCode)
		if nil != resp.Body {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
	})
}
