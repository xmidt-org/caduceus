// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/xmidt-org/caduceus/internal/metrics"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
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
			expectedEventType:     metrics.UnknownEventType,
			startTime:             date1,
			endTime:               date2,
		},
	}

	for _, tc := range tcs {
		assert := assert.New(t)

		logger := zap.NewExample()
		fakeHandler := new(mockHandler)
		if !tc.throwStatusBadRequest {
			fakeHandler.On("handleRequest",
				mock.AnythingOfType("*wrp.Message")).Return().Times(1)
		}

		fakeEmptyRequests := new(metrics.MockCounter)
		fakeErrorRequests := new(metrics.MockCounter)
		fakeInvalidCount := new(metrics.MockCounter)
		fakeQueueDepth := new(metrics.MockGauge)
		fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)
		if tc.throwStatusBadRequest {
			fakeInvalidCount.On("Add", mock.AnythingOfType("float64")).Return().Once()
		}

		fakeTime := mockTime(tc.startTime, tc.endTime)
		fakeHist := new(metrics.MockHistogram)
		histogramFunctionCall := prometheus.Labels{metrics.EventLabel: tc.expectedEventType}
		fakeLatency := date2.Sub(date1)
		fakeHist.On("With", histogramFunctionCall).Return().Once()
		fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

		serverWrapper := &ServerHandler{
			logger:          logger,
			caduceusHandler: fakeHandler,
			metrics: metrics.ServerHandlerMetrics{
				ErrorRequests:        fakeErrorRequests,
				EmptyRequests:        fakeEmptyRequests,
				InvalidCount:         fakeInvalidCount,
				IncomingQueueDepth:   fakeQueueDepth,
				IncomingQueueLatency: fakeHist,
			},
			maxOutstanding: 1,
			now:            fakeTime,
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

	logger := zap.NewExample()
	fakeHandler := new(mockHandler)
	fakeHandler.On("handleRequest",
		mock.AnythingOfType("*wrp.Message")).Return().Once()

	fakeEmptyRequests := new(metrics.MockCounter)
	fakeErrorRequests := new(metrics.MockCounter)
	fakeInvalidCount := new(metrics.MockCounter)
	fakeQueueDepth := new(metrics.MockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)

	fakeIncomingContentTypeCount := new(metrics.MockCounter)
	fakeIncomingContentTypeCount.On("With", prometheus.Labels{"content_type": wrp.MimeTypeMsgpack}).Return(fakeIncomingContentTypeCount)
	fakeIncomingContentTypeCount.On("With", prometheus.Labels{"content_type": ""}).Return(fakeIncomingContentTypeCount)
	fakeIncomingContentTypeCount.On("Add", 1.0).Return()

	fakeModifiedWRPCount := new(metrics.MockCounter)
	fakeModifiedWRPCount.On("With", prometheus.Labels{metrics.ReasonLabel: metrics.BothEmptyReason}).Return(fakeIncomingContentTypeCount).Once()
	fakeModifiedWRPCount.On("Add", 1.0).Return().Once()

	fakeHist := new(metrics.MockHistogram)
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: "bob"}
	fakeLatency := date2.Sub(date1)
	fakeHist.On("With", histogramFunctionCall).Return().Once()
	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

	serverWrapper := &ServerHandler{
		logger:          logger,
		caduceusHandler: fakeHandler,
		metrics: metrics.ServerHandlerMetrics{
			ErrorRequests:        fakeErrorRequests,
			EmptyRequests:        fakeEmptyRequests,
			InvalidCount:         fakeInvalidCount,
			ModifiedWRPCount:     fakeModifiedWRPCount,
			IncomingQueueDepth:   fakeQueueDepth,
			IncomingQueueLatency: fakeHist,
		},
		maxOutstanding: 1,
		now:            mockTime(date1, date2),
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

	logger := zap.NewExample()
	fakeHandler := new(mockHandler)
	fakeHandler.On("handleRequest",
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Times(2)

	fakeQueueDepth := new(metrics.MockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	fakeHist := new(metrics.MockHistogram)
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	fakeLatency := date2.Sub(date1)
	fakeHist.On("With", histogramFunctionCall).Return().Once()
	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

	serverWrapper := &ServerHandler{
		logger:          logger,
		caduceusHandler: fakeHandler,
		metrics: metrics.ServerHandlerMetrics{
			IncomingQueueDepth:   fakeQueueDepth,
			IncomingQueueLatency: fakeHist,
		},
		maxOutstanding: 1,
		now:            mockTime(date1, date2),
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

	logger := zap.NewExample()
	fakeHandler := new(mockHandler)
	fakeHandler.On("handleRequest",
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Times(2)

	fakeEmptyRequests := new(metrics.MockCounter)
	fakeEmptyRequests.On("Add", mock.AnythingOfType("float64")).Return().Once()
	fakeQueueDepth := new(metrics.MockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	fakeHist := new(metrics.MockHistogram)
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	fakeLatency := date2.Sub(date1)
	fakeHist.On("With", histogramFunctionCall).Return().Once()
	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

	serverWrapper := &ServerHandler{
		logger:          logger,
		caduceusHandler: fakeHandler,
		metrics: metrics.ServerHandlerMetrics{
			EmptyRequests:        fakeEmptyRequests,
			IncomingQueueDepth:   fakeQueueDepth,
			IncomingQueueLatency: fakeHist,
		},
		maxOutstanding: 1,
		now:            mockTime(date1, date2),
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

	logger := zap.NewExample()
	fakeHandler := new(mockHandler)
	fakeHandler.On("handleRequest",
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Once()

	fakeErrorRequests := new(metrics.MockCounter)
	fakeErrorRequests.On("Add", mock.AnythingOfType("float64")).Return().Once()
	fakeQueueDepth := new(metrics.MockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	fakeHist := new(metrics.MockHistogram)
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	fakeLatency := date2.Sub(date1)
	fakeHist.On("With", histogramFunctionCall).Return().Once()
	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

	serverWrapper := &ServerHandler{
		logger:          logger,
		caduceusHandler: fakeHandler,
		metrics: metrics.ServerHandlerMetrics{
			ErrorRequests:        fakeErrorRequests,
			IncomingQueueDepth:   fakeQueueDepth,
			IncomingQueueLatency: fakeHist,
		},
		maxOutstanding: 1,
		now:            mockTime(date1, date2),
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

	logger := zap.NewExample()
	fakeHandler := new(mockHandler)
	fakeHandler.On("handleRequest",
		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Once()

	fakeQueueDepth := new(metrics.MockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	fakeInvalidCount := new(metrics.MockCounter)
	fakeInvalidCount.On("Add", mock.AnythingOfType("float64")).Return().Once()

	fakeHist := new(metrics.MockHistogram)
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	fakeLatency := date2.Sub(date1)
	fakeHist.On("With", histogramFunctionCall).Return().Once()
	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

	serverWrapper := &ServerHandler{
		logger:          logger,
		caduceusHandler: fakeHandler,
		metrics: metrics.ServerHandlerMetrics{
			InvalidCount:         fakeInvalidCount,
			IncomingQueueDepth:   fakeQueueDepth,
			IncomingQueueLatency: fakeHist},
		maxOutstanding: 1,
		now:            mockTime(date1, date2),
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

	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	fakeLatency := date2.Sub(date1)

	assert := assert.New(t)

	logger := zap.NewExample()
	fakeHandler := new(mockHandler)

	fakeQueueDepth := new(metrics.MockGauge)

	serverWrapper := &ServerHandler{
		logger:          logger,
		caduceusHandler: fakeHandler,
		metrics: metrics.ServerHandlerMetrics{
			IncomingQueueDepth: fakeQueueDepth,
		},
		maxOutstanding: 1,
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
			fakeHist := new(metrics.MockHistogram)
			serverWrapper.metrics.IncomingQueueLatency = fakeHist
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
