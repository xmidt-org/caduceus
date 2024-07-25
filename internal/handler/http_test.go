// // SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// // SPDX-License-Identifier: Apache-2.0
package handler_test

// import (
// 	"bytes"
// 	"io"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"
// 	"testing/iotest"
// 	"time"

// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/mock"
// 	"github.com/xmidt-org/caduceus/internal/handler"
// 	"github.com/xmidt-org/caduceus/internal/metrics"
// 	"github.com/xmidt-org/caduceus/internal/mocks"
// 	"go.uber.org/zap/zaptest"

// 	"github.com/xmidt-org/wrp-go/v3"
// )

// func exampleRequest(msgType int, list ...string) *http.Request {
// 	var buffer bytes.Buffer

// 	trans := "1234"
// 	ct := wrp.MimeTypeMsgpack
// 	url := "localhost:8080"

// 	for i := range list {
// 		switch {
// 		case i == 0:
// 			trans = list[i]
// 		case i == 1:
// 			ct = list[i]
// 		case i == 2:
// 			url = list[i]
// 		}

// 	}
// 	wrp.NewEncoder(&buffer, wrp.Msgpack).Encode(
// 		&wrp.Message{
// 			Type:            wrp.MessageType(msgType),
// 			Source:          "mac:112233445566/lmlite",
// 			TransactionUUID: trans,
// 			ContentType:     ct,
// 			Destination:     "event:bob/magic/dog",
// 			Payload:         []byte("Hello, world."),
// 		})

// 	r := bytes.NewReader(buffer.Bytes())
// 	req := httptest.NewRequest("POST", url, r)
// 	req.Header.Set("Content-Type", wrp.MimeTypeMsgpack)

// 	return req
// }

// func TestServerHandler(t *testing.T) {
// 	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
// 	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

// 	tcs := []struct {
// 		desc                  string
// 		expectedResponse      int
// 		request               *http.Request
// 		throwStatusBadRequest bool
// 		expectedEventType     string
// 		startTime             time.Time
// 		endTime               time.Time
// 	}{
// 		{
// 			desc:              "TestServeHTTPHappyPath",
// 			expectedResponse:  http.StatusAccepted,
// 			request:           exampleRequest(4),
// 			expectedEventType: "bob",
// 			startTime:         date1,
// 			endTime:           date2,
// 		},
// 		{
// 			desc:                  "TestServeHTTPInvalidMessageType",
// 			expectedResponse:      http.StatusBadRequest,
// 			request:               exampleRequest(1),
// 			throwStatusBadRequest: true,
// 			expectedEventType:     metrics.UnknownEventType,
// 			startTime:             date1,
// 			endTime:               date2,
// 		},
// 	}

// 	for _, tc := range tcs {
// 		assert := assert.New(t)
// 		logger := zaptest.NewLogger(t)

// 		fakeHandler := new(mocks.Handler)
// 		if !tc.throwStatusBadRequest {
// 			fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
// 				mock.AnythingOfType("*wrp.Message")).Return().Times(1)
// 		}

// 		fakeEmptyRequests := new(mocks.Counter)
// 		fakeErrorRequests := new(mocks.Counter)
// 		fakeInvalidCount := new(mocks.Counter)
// 		fakeQueueDepth := new(mocks.Gauge)
// 		fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)
// 		if tc.throwStatusBadRequest {
// 			fakeInvalidCount.On("Add", mock.AnythingOfType("float64")).Return().Once()
// 		}

// 		fakeTime := mocks.Time(tc.startTime, tc.endTime)
// 		fakeHist := new(mocks.Histogram)
// 		histogramFunctionCall := []string{metrics.EventLabel, tc.expectedEventType}
// 		fakeLatency := date2.Sub(date1)
// 		fakeHist.On("With", histogramFunctionCall).Return().Once()
// 		fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()
// 		fakeTel := &handler.Telemetry{
// 			ErrorRequests:            fakeErrorRequests,
// 			EmptyRequests:            fakeEmptyRequests,
// 			InvalidCount:             fakeInvalidCount,
// 			IncomingQueueDepthMetric: fakeQueueDepth,
// 			IncomingQueueLatency:     fakeHist,
// 		}

// 		fakeHandler.SinkWrapper = new(mocks.Wrapper)
// 		fakeHandler.Logger = logger
// 		fakeHandler.Telemetry = fakeTel
// 		fakeHandler.MaxOutstanding = 1
// 		fakeHandler.Now = fakeTime

// 		t.Run(tc.desc, func(t *testing.T) {
// 			w := httptest.NewRecorder()

// 			fakeHandler.ServeHTTP(w, tc.request)
// 			resp := w.Result()

// 			assert.Equal(tc.expectedResponse, resp.StatusCode)
// 			if nil != resp.Body {
// 				io.Copy(io.Discard, resp.Body)
// 				resp.Body.Close()
// 			}
// 			fakeHandler.AssertExpectations(t)
// 			fakeHist.AssertExpectations(t)
// 		})
// 	}
// }

// func TestServerHandlerFixWrp(t *testing.T) {
// 	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
// 	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

// 	assert := assert.New(t)
// 	logger := zaptest.NewLogger(t)

// 	fakeHandler := new(mocks.Handler)
// 	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
// 		mock.AnythingOfType("*wrp.Message")).Return().Once()

// 	fakeEmptyRequests := new(mocks.Counter)
// 	fakeErrorRequests := new(mocks.Counter)
// 	fakeInvalidCount := new(mocks.Counter)
// 	fakeQueueDepth := new(mocks.Gauge)
// 	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)

// 	fakeIncomingContentTypeCount := new(mocks.Counter)
// 	fakeIncomingContentTypeCount.On("With", []string{"content_type", wrp.MimeTypeMsgpack}).Return(fakeIncomingContentTypeCount)
// 	fakeIncomingContentTypeCount.On("With", []string{"content_type", ""}).Return(fakeIncomingContentTypeCount)
// 	fakeIncomingContentTypeCount.On("Add", 1.0).Return()

// 	fakeModifiedWRPCount := new(mocks.Counter)
// 	fakeModifiedWRPCount.On("With", []string{metrics.ReasonLabel, metrics.BothEmptyReason}).Return(fakeIncomingContentTypeCount).Once()
// 	fakeModifiedWRPCount.On("Add", 1.0).Return().Once()

// 	fakeHist := new(mocks.Histogram)
// 	histogramFunctionCall := []string{metrics.EventLabel, "bob"}
// 	fakeLatency := date2.Sub(date1)
// 	fakeHist.On("With", histogramFunctionCall).Return().Once()
// 	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()
// 	fakeTel := &handler.Telemetry{
// 		ErrorRequests:            fakeErrorRequests,
// 		EmptyRequests:            fakeEmptyRequests,
// 		InvalidCount:             fakeInvalidCount,
// 		IncomingQueueDepthMetric: fakeQueueDepth,
// 		ModifiedWRPCount:         fakeModifiedWRPCount,
// 		IncomingQueueLatency:     fakeHist,
// 	}
// 	fakeHandler.SinkWrapper = new(mocks.Wrapper)
// 	fakeHandler.Logger = logger
// 	fakeHandler.Telemetry = fakeTel
// 	fakeHandler.MaxOutstanding = 1
// 	fakeHandler.Now = mocks.Time(date1, date2)

// 	t.Run("TestServeHTTPHappyPath", func(t *testing.T) {
// 		w := httptest.NewRecorder()

// 		fakeHandler.ServeHTTP(w, exampleRequest(4, "", ""))
// 		resp := w.Result()

// 		assert.Equal(http.StatusAccepted, resp.StatusCode)
// 		if nil != resp.Body {
// 			io.Copy(io.Discard, resp.Body)
// 			resp.Body.Close()
// 		}
// 		fakeHandler.AssertExpectations(t)
// 		fakeHist.AssertExpectations(t)
// 	})
// }

// func TestServerHandlerFull(t *testing.T) {
// 	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
// 	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

// 	assert := assert.New(t)
// 	logger := zaptest.NewLogger(t)

// 	fakeHandler := new(mocks.Handler)
// 	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
// 		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Times(2)

// 	fakeQueueDepth := new(mocks.Gauge)
// 	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

// 	fakeHist := new(mocks.Histogram)
// 	histogramFunctionCall := []string{metrics.EventLabel, metrics.UnknownEventType}
// 	fakeLatency := date2.Sub(date1)
// 	fakeHist.On("With", histogramFunctionCall).Return().Once()
// 	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()
// 	fakeTel := &handler.Telemetry{
// 		IncomingQueueDepthMetric: fakeQueueDepth,
// 		IncomingQueueLatency:     fakeHist,
// 	}

// 	fakeHandler.SinkWrapper = new(mocks.Wrapper)
// 	fakeHandler.Logger = logger
// 	fakeHandler.Telemetry = fakeTel
// 	fakeHandler.MaxOutstanding = 1
// 	fakeHandler.Now = mocks.Time(date1, date2)

// 	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
// 		w := httptest.NewRecorder()

// 		/* Act like we have 1 in flight */
// 		fakeHandler.IncomingQueueDepth = 1

// 		/* Make the call that goes over the limit */
// 		fakeHandler.ServeHTTP(w, exampleRequest(4))
// 		resp := w.Result()

// 		assert.Equal(http.StatusServiceUnavailable, resp.StatusCode)
// 		if nil != resp.Body {
// 			io.Copy(io.Discard, resp.Body)
// 			resp.Body.Close()
// 		}
// 		fakeHist.AssertExpectations(t)
// 	})
// }

// func TestServerEmptyPayload(t *testing.T) {
// 	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
// 	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

// 	assert := assert.New(t)
// 	logger := zaptest.NewLogger(t)

// 	var buffer bytes.Buffer
// 	r := bytes.NewReader(buffer.Bytes())
// 	req := httptest.NewRequest("POST", "localhost:8080", r)
// 	req.Header.Set("Content-Type", wrp.MimeTypeMsgpack)

// 	fakeHandler := new(mocks.Handler)
// 	fakeHandler.On("handleRequest", mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Times(2)

// 	fakeEmptyRequests := new(mocks.Counter)
// 	fakeEmptyRequests.On("Add", mock.AnythingOfType("float64")).Return().Once()
// 	fakeQueueDepth := new(mocks.Gauge)
// 	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

// 	fakeHist := new(mocks.Histogram)
// 	histogramFunctionCall := []string{metrics.EventLabel, metrics.UnknownEventType}
// 	fakeLatency := date2.Sub(date1)
// 	fakeHist.On("With", histogramFunctionCall).Return().Once()
// 	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()
// 	fakeTel := &handler.Telemetry{
// 		EmptyRequests:            fakeEmptyRequests,
// 		IncomingQueueDepthMetric: fakeQueueDepth,
// 		IncomingQueueLatency:     fakeHist,
// 	}

// 	fakeHandler.SinkWrapper = new(mocks.Wrapper)
// 	fakeHandler.Logger = logger
// 	fakeHandler.Telemetry = fakeTel
// 	fakeHandler.MaxOutstanding = 1
// 	fakeHandler.Now = mocks.Time(date1, date2)

// 	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
// 		w := httptest.NewRecorder()

// 		/* Make the call that goes over the limit */
// 		fakeHandler.ServeHTTP(w, req)
// 		resp := w.Result()

// 		assert.Equal(http.StatusBadRequest, resp.StatusCode)
// 		if nil != resp.Body {
// 			io.Copy(io.Discard, resp.Body)
// 			resp.Body.Close()
// 		}
// 		fakeHist.AssertExpectations(t)
// 	})
// }

// func TestServerUnableToReadBody(t *testing.T) {
// 	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
// 	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

// 	assert := assert.New(t)
// 	logger := zaptest.NewLogger(t)

// 	var buffer bytes.Buffer
// 	r := iotest.TimeoutReader(bytes.NewReader(buffer.Bytes()))

// 	_, _ = r.Read(nil)
// 	req := httptest.NewRequest("POST", "localhost:8080", r)
// 	req.Header.Set("Content-Type", wrp.MimeTypeMsgpack)

// 	fakeHandler := new(mocks.Handler)
// 	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
// 		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Once()

// 	fakeErrorRequests := new(mocks.Counter)
// 	fakeErrorRequests.On("Add", mock.AnythingOfType("float64")).Return().Once()
// 	fakeQueueDepth := new(mocks.Gauge)
// 	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

// 	fakeHist := new(mocks.Histogram)
// 	histogramFunctionCall := []string{metrics.EventLabel, metrics.UnknownEventType}
// 	fakeLatency := date2.Sub(date1)
// 	fakeHist.On("With", histogramFunctionCall).Return().Once()
// 	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()
// 	fakeTel := &handler.Telemetry{
// 		ErrorRequests:            fakeErrorRequests,
// 		IncomingQueueDepthMetric: fakeQueueDepth,
// 		IncomingQueueLatency:     fakeHist,
// 	}

// 	fakeHandler.SinkWrapper = new(mocks.Wrapper)
// 	fakeHandler.Logger = logger
// 	fakeHandler.Telemetry = fakeTel
// 	fakeHandler.MaxOutstanding = 1
// 	fakeHandler.Now = mocks.Time(date1, date2)

// 	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
// 		w := httptest.NewRecorder()

// 		/* Make the call that goes over the limit */
// 		fakeHandler.ServeHTTP(w, req)
// 		resp := w.Result()

// 		assert.Equal(http.StatusBadRequest, resp.StatusCode)
// 		if nil != resp.Body {
// 			io.Copy(io.Discard, resp.Body)
// 			resp.Body.Close()
// 		}
// 	})
// 	fakeHist.AssertExpectations(t)
// }

// func TestServerInvalidBody(t *testing.T) {
// 	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
// 	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

// 	assert := assert.New(t)
// 	logger := zaptest.NewLogger(t)

// 	r := bytes.NewReader([]byte("Invalid payload."))

// 	_, _ = r.Read(nil)
// 	req := httptest.NewRequest("POST", "localhost:8080", r)
// 	req.Header.Set("Content-Type", wrp.MimeTypeMsgpack)

// 	fakeHandler := new(mocks.Handler)
// 	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
// 		mock.AnythingOfType("*wrp.Message")).WaitUntil(time.After(time.Second)).Once()

// 	fakeQueueDepth := new(mocks.Gauge)
// 	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

// 	fakeInvalidCount := new(mocks.Counter)
// 	fakeInvalidCount.On("Add", mock.AnythingOfType("float64")).Return().Once()

// 	fakeHist := new(mocks.Histogram)
// 	histogramFunctionCall := []string{metrics.EventLabel, metrics.UnknownEventType}
// 	fakeLatency := date2.Sub(date1)
// 	fakeHist.On("With", histogramFunctionCall).Return().Once()
// 	fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()
// 	fakeTel := &handler.Telemetry{
// 		InvalidCount:             fakeInvalidCount,
// 		IncomingQueueDepthMetric: fakeQueueDepth,
// 		IncomingQueueLatency:     fakeHist,
// 	}

// 	fakeHandler.SinkWrapper = new(mocks.Wrapper)
// 	fakeHandler.Logger = logger
// 	fakeHandler.Telemetry = fakeTel
// 	fakeHandler.MaxOutstanding = 1
// 	fakeHandler.Now = mocks.Time(date1, date2)

// 	t.Run("TestServeHTTPTooMany", func(t *testing.T) {
// 		w := httptest.NewRecorder()

// 		/* Make the call that goes over the limit */
// 		fakeHandler.ServeHTTP(w, req)
// 		resp := w.Result()

// 		assert.Equal(http.StatusBadRequest, resp.StatusCode)
// 		if nil != resp.Body {
// 			io.Copy(io.Discard, resp.Body)
// 			resp.Body.Close()
// 		}
// 	})
// 	fakeHist.AssertExpectations(t)
// }

// func TestHandlerUnsupportedMediaType(t *testing.T) {
// 	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
// 	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

// 	histogramFunctionCall := []string{metrics.EventLabel, metrics.UnknownEventType}
// 	fakeLatency := date2.Sub(date1)

// 	assert := assert.New(t)
// 	logger := zaptest.NewLogger(t)

// 	fakeHandler := new(mocks.Handler)

// 	fakeQueueDepth := new(mocks.Gauge)
// 	fakeTel := &handler.Telemetry{
// 		IncomingQueueDepthMetric: fakeQueueDepth,
// 	}

// 	fakeHandler.SinkWrapper = new(mocks.Wrapper)
// 	fakeHandler.Logger = logger
// 	fakeHandler.Telemetry = fakeTel
// 	fakeHandler.MaxOutstanding = 1
// 	fakeHandler.Now = mocks.Time(date1, date2)

// 	testCases := []struct {
// 		name    string
// 		headers []string
// 	}{
// 		{
// 			name: "No Content Type Header",
// 		}, {
// 			name:    "Wrong Content Type Header",
// 			headers: []string{"application/json"},
// 		}, {
// 			name:    "Multiple Content Type Headers",
// 			headers: []string{"application/msgpack", "application/msgpack", "application/msgpack"},
// 		},
// 	}

// 	for _, testCase := range testCases {
// 		t.Run(testCase.name, func(t *testing.T) {
// 			fakeHist := new(mocks.Histogram)
// 			fakeHandler.Telemetry.IncomingQueueLatency = fakeHist
// 			fakeHandler.Now = mocks.Time(date1, date2)
// 			fakeHist.On("With", histogramFunctionCall).Return().Once()
// 			fakeHist.On("Observe", fakeLatency.Seconds()).Return().Once()

// 			w := httptest.NewRecorder()
// 			req := exampleRequest(4)
// 			req.Header.Del("Content-Type")
// 			for _, h := range testCase.headers {
// 				req.Header.Add("Content-Type", h)
// 			}
// 			fakeHandler.ServeHTTP(w, req)
// 			resp := w.Result()

// 			assert.Equal(http.StatusUnsupportedMediaType, resp.StatusCode)
// 			if nil != resp.Body {
// 				io.Copy(io.Discard, resp.Body)
// 				resp.Body.Close()
// 			}
// 		})
// 	}

// }
