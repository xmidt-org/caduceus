// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/xmidt-org/webpa-common/v2/adapter"
	"github.com/xmidt-org/wrp-go/v3"
)

func exampleRequest(msgType int, list ...string) *http.Request {
	var buffer bytes.Buffer

	trans := "1234"
	ct := wrp.MimeTypeMsgpack
	url := "localhost:8080"

	for i := range list {
		switch {
		case i == 0:
			trans = list[i]
		case i == 1:
			ct = list[i]
		case i == 2:
			url = list[i]
		}

	}
	wrp.NewEncoder(&buffer, wrp.Msgpack).Encode(
		&wrp.Message{
			Type:            wrp.MessageType(msgType),
			Source:          "mac:112233445566/lmlite",
			TransactionUUID: trans,
			ContentType:     ct,
			Destination:     "event:bob/magic/dog",
			Payload:         []byte("Hello, world."),
		})

	r := bytes.NewReader(buffer.Bytes())
	req := httptest.NewRequest("POST", url, r)
	req.Header.Set("Content-Type", wrp.MimeTypeMsgpack)

	return req
}

func TestServerHandler(t *testing.T) {
	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

	tcs := []struct {
		desc                  string
		expectedResponse      int
		request               *http.Request
		throwStatusBadRequest bool
		expectedEventType     string
		startTime             time.Time
		endTime               time.Time
	}{
		{
			desc:              "TestServeHTTPHappyPath",
			expectedResponse:  http.StatusAccepted,
			request:           exampleRequest(4),
			expectedEventType: "bob",
			startTime:         date1,
			endTime:           date2,
		},
		{
			desc:                  "TestServeHTTPInvalidMessageType",
			expectedResponse:      http.StatusBadRequest,
			request:               exampleRequest(1),
			throwStatusBadRequest: true,
			expectedEventType:     unknownEventType,
			startTime:             date1,
			endTime:               date2,
		},
	}

	for _, tc := range tcs {
		assert := assert.New(t)

		logger := adapter.DefaultLogger().Logger
		fakeHandler := new(mockHandler)
		if !tc.throwStatusBadRequest {
			fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
				mock.AnythingOfType("*wrp.Message")).Return().Times(1)
		}

		fakeEmptyRequests := new(mockCounter)
		fakeErrorRequests := new(mockCounter)
		fakeInvalidCount := new(mockCounter)
		fakeQueueDepth := new(mockGauge)
		fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)
		if tc.throwStatusBadRequest {
			fakeInvalidCount.On("Add", mock.AnythingOfType("float64")).Return().Once()
		}

		fakeTime := mockTime(tc.startTime, tc.endTime)
		fakeHist := new(mockHistogram)
		histogramFunctionCall := []string{eventLabel, tc.expectedEventType}
		fakeLatency := date2.Sub(date1)
		fakeHist.On("With", histogramFunctionCall).Return().Once()
		fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

		serverWrapper := &ServerHandler{
			Logger:                   logger,
			caduceusHandler:          fakeHandler,
			errorRequests:            fakeErrorRequests,
			emptyRequests:            fakeEmptyRequests,
			invalidCount:             fakeInvalidCount,
			incomingQueueDepthMetric: fakeQueueDepth,
			maxOutstanding:           1,
			incomingQueueLatency:     fakeHist,
			now:                      fakeTime,
		}
		t.Run(tc.desc, func(t *testing.T) {
			w := httptest.NewRecorder()

			serverWrapper.ServeHTTP(w, tc.request)
			resp := w.Result()

			assert.Equal(tc.expectedResponse, resp.StatusCode)
			if nil != resp.Body {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			fakeHandler.AssertExpectations(t)
			fakeHist.AssertExpectations(t)
		})
	}
}

func TestServerHandlerFixWrp(t *testing.T) {
	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

	assert := assert.New(t)

	logger := adapter.DefaultLogger().Logger
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).Return().Once()

	fakeEmptyRequests := new(mockCounter)
	fakeErrorRequests := new(mockCounter)
	fakeInvalidCount := new(mockCounter)
	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)

	fakeIncomingContentTypeCount := new(mockCounter)
	fakeIncomingContentTypeCount.On("With", []string{"content_type", wrp.MimeTypeMsgpack}).Return(fakeIncomingContentTypeCount)
	fakeIncomingContentTypeCount.On("With", []string{"content_type", ""}).Return(fakeIncomingContentTypeCount)
	fakeIncomingContentTypeCount.On("Add", 1.0).Return()

	fakeModifiedWRPCount := new(mockCounter)
	fakeModifiedWRPCount.On("With", []string{reasonLabel, bothEmptyReason}).Return(fakeIncomingContentTypeCount).Once()
	fakeModifiedWRPCount.On("Add", 1.0).Return().Once()

	fakeHist := new(mockHistogram)
	histogramFunctionCall := []string{eventLabel, "bob"}
	fakeLatency := date2.Sub(date1)
	fakeHist.On("With", histogramFunctionCall).Return().Once()
	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		errorRequests:            fakeErrorRequests,
		emptyRequests:            fakeEmptyRequests,
		invalidCount:             fakeInvalidCount,
		modifiedWRPCount:         fakeModifiedWRPCount,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
		incomingQueueLatency:     fakeHist,
		now:                      mockTime(date1, date2),
	}

	t.Run("TestServeHTTPHappyPath", func(t *testing.T) {
		w := httptest.NewRecorder()

		serverWrapper.ServeHTTP(w, exampleRequest(4, "", ""))
		resp := w.Result()

		assert.Equal(http.StatusAccepted, resp.StatusCode)
		if nil != resp.Body {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		fakeHandler.AssertExpectations(t)
		fakeHist.AssertExpectations(t)
	})
}

func TestServerHandlerFull(t *testing.T) {
	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

	assert := assert.New(t)

	logger := adapter.DefaultLogger().Logger
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Times(2)

	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	fakeHist := new(mockHistogram)
	histogramFunctionCall := []string{eventLabel, unknownEventType}
	fakeLatency := date2.Sub(date1)
	fakeHist.On("With", histogramFunctionCall).Return().Once()
	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
		incomingQueueLatency:     fakeHist,
		now:                      mockTime(date1, date2),
	}

	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
		w := httptest.NewRecorder()

		/* Act like we have 1 in flight */
		serverWrapper.incomingQueueDepth = 1

		/* Make the call that goes over the limit */
		serverWrapper.ServeHTTP(w, exampleRequest(4))
		resp := w.Result()

		assert.Equal(http.StatusServiceUnavailable, resp.StatusCode)
		if nil != resp.Body {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		fakeHist.AssertExpectations(t)
	})
}

func TestServerEmptyPayload(t *testing.T) {
	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

	assert := assert.New(t)

	var buffer bytes.Buffer
	r := bytes.NewReader(buffer.Bytes())
	req := httptest.NewRequest("POST", "localhost:8080", r)
	req.Header.Set("Content-Type", wrp.MimeTypeMsgpack)

	logger := adapter.DefaultLogger().Logger
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Times(2)

	fakeEmptyRequests := new(mockCounter)
	fakeEmptyRequests.On("Add", mock.AnythingOfType("float64")).Return().Once()
	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	fakeHist := new(mockHistogram)
	histogramFunctionCall := []string{eventLabel, unknownEventType}
	fakeLatency := date2.Sub(date1)
	fakeHist.On("With", histogramFunctionCall).Return().Once()
	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		emptyRequests:            fakeEmptyRequests,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
		incomingQueueLatency:     fakeHist,
		now:                      mockTime(date1, date2),
	}

	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
		w := httptest.NewRecorder()

		/* Make the call that goes over the limit */
		serverWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusBadRequest, resp.StatusCode)
		if nil != resp.Body {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		fakeHist.AssertExpectations(t)
	})
}

func TestServerUnableToReadBody(t *testing.T) {
	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

	assert := assert.New(t)

	var buffer bytes.Buffer
	r := iotest.TimeoutReader(bytes.NewReader(buffer.Bytes()))

	_, _ = r.Read(nil)
	req := httptest.NewRequest("POST", "localhost:8080", r)
	req.Header.Set("Content-Type", wrp.MimeTypeMsgpack)

	logger := adapter.DefaultLogger().Logger
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Once()

	fakeErrorRequests := new(mockCounter)
	fakeErrorRequests.On("Add", mock.AnythingOfType("float64")).Return().Once()
	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	fakeHist := new(mockHistogram)
	histogramFunctionCall := []string{eventLabel, unknownEventType}
	fakeLatency := date2.Sub(date1)
	fakeHist.On("With", histogramFunctionCall).Return().Once()
	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		errorRequests:            fakeErrorRequests,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
		incomingQueueLatency:     fakeHist,
		now:                      mockTime(date1, date2),
	}

	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
		w := httptest.NewRecorder()

		/* Make the call that goes over the limit */
		serverWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusBadRequest, resp.StatusCode)
		if nil != resp.Body {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})
	fakeHist.AssertExpectations(t)
}

func TestServerInvalidBody(t *testing.T) {
	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

	assert := assert.New(t)

	r := bytes.NewReader([]byte("Invalid payload."))

	_, _ = r.Read(nil)
	req := httptest.NewRequest("POST", "localhost:8080", r)
	req.Header.Set("Content-Type", wrp.MimeTypeMsgpack)

	logger := adapter.DefaultLogger().Logger
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Once()

	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	fakeInvalidCount := new(mockCounter)
	fakeInvalidCount.On("Add", mock.AnythingOfType("float64")).Return().Once()

	fakeHist := new(mockHistogram)
	histogramFunctionCall := []string{eventLabel, unknownEventType}
	fakeLatency := date2.Sub(date1)
	fakeHist.On("With", histogramFunctionCall).Return().Once()
	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		invalidCount:             fakeInvalidCount,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
		incomingQueueLatency:     fakeHist,
		now:                      mockTime(date1, date2),
	}

	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
		w := httptest.NewRecorder()

		/* Make the call that goes over the limit */
		serverWrapper.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusBadRequest, resp.StatusCode)
		if nil != resp.Body {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})
	fakeHist.AssertExpectations(t)
}

func TestHandlerUnsupportedMediaType(t *testing.T) {
	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

	histogramFunctionCall := []string{eventLabel, unknownEventType}
	fakeLatency := date2.Sub(date1)

	assert := assert.New(t)

	logger := adapter.DefaultLogger().Logger
	fakeHandler := new(mockHandler)

	fakeQueueDepth := new(mockGauge)

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		incomingQueueDepthMetric: fakeQueueDepth,
		maxOutstanding:           1,
	}
	testCases := []struct {
		name    string
		headers []string
	}{
		{
			name: "No Content Type Header",
		}, {
			name:    "Wrong Content Type Header",
			headers: []string{"application/json"},
		}, {
			name:    "Multiple Content Type Headers",
			headers: []string{"application/msgpack", "application/msgpack", "application/msgpack"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fakeHist := new(mockHistogram)
			serverWrapper.incomingQueueLatency = fakeHist
			serverWrapper.now = mockTime(date1, date2)
			fakeHist.On("With", histogramFunctionCall).Return().Once()
			fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

			w := httptest.NewRecorder()
			req := exampleRequest(4)
			req.Header.Del("Content-Type")
			for _, h := range testCase.headers {
				req.Header.Add("Content-Type", h)
			}
			serverWrapper.ServeHTTP(w, req)
			resp := w.Result()

			assert.Equal(http.StatusUnsupportedMediaType, resp.StatusCode)
			if nil != resp.Body {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		})
	}

}
