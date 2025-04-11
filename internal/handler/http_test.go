package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/xmidt-org/caduceus/internal/metrics"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

/***********************************/
type HandlerSuite struct {
	suite.Suite
	handler                      *ServerHandler
	exampleRequestFunc           func(int, ...string) *http.Request
	fakeLatency                  time.Duration
	fakeEmptyRequests            *metrics.MockCounter
	fakeErrorRequests            *metrics.MockCounter
	fakeInvalidCount             *metrics.MockCounter
	fakeQueueDepth               *metrics.MockGauge
	fakeQueueLatency             *metrics.MockHistogram
	fakeIncomingContentTypeCount *metrics.MockCounter
	fakeModifiedWRPCount         metrics.CounterVec
}

func (suite *HandlerSuite) SetupTest() {
	date1 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 0, time.UTC)
	date2 := time.Date(2021, time.Month(2), 21, 1, 10, 30, 45, time.UTC)

	fakeTime := mockTime(date1, date2)

	logger := zap.NewExample()
	fakeEmptyRequests := new(metrics.MockCounter)
	fakeErrorRequests := new(metrics.MockCounter)
	fakeInvalidCount := new(metrics.MockCounter)
	fakeQueueDepth := new(metrics.MockGauge)
	fakeHist := new(metrics.MockHistogram)
	fakeModifiedWRPCount := new(metrics.MockCounter)
	fakeIncomingContentTypeCount := new(metrics.MockCounter)

	wrapper := new(MockWrapper)
	wrapper.On("Queue", mock.AnythingOfType("*wrp.Message"))
	m := metrics.ServerHandlerMetrics{
		ErrorRequests:        fakeErrorRequests,
		EmptyRequests:        fakeEmptyRequests,
		InvalidCount:         fakeInvalidCount,
		IncomingQueueDepth:   fakeQueueDepth,
		IncomingQueueLatency: fakeHist,
	}
	handler, err := New(wrapper, logger, m, 1, 0)
	suite.Require().NoError(err)
	handler.now = fakeTime

	suite.handler = handler
	suite.fakeLatency = date2.Sub(date1)
	suite.fakeEmptyRequests = fakeEmptyRequests
	suite.fakeErrorRequests = fakeErrorRequests
	suite.fakeInvalidCount = fakeInvalidCount
	suite.fakeQueueDepth = fakeQueueDepth
	suite.fakeQueueLatency = fakeHist
	suite.fakeIncomingContentTypeCount = fakeIncomingContentTypeCount
	suite.fakeModifiedWRPCount = fakeModifiedWRPCount

	suite.exampleRequestFunc = func(msgType int, list ...string) *http.Request {
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

}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerSuite))
}

func (suite *HandlerSuite) TestServerHandlerPass() {

	w := httptest.NewRecorder()
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: "bob"}

	suite.fakeQueueLatency.On("With", histogramFunctionCall).Return().Once()
	suite.fakeQueueLatency.On("Observe", suite.fakeLatency.Seconds()).Return().Once()
	suite.fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)
	suite.handler.ServeHTTP(w, suite.exampleRequestFunc(4))
	resp := w.Result()
	defer resp.Body.Close()

	suite.Equal(http.StatusAccepted, resp.StatusCode)
	suite.NotNil(suite.T(), resp.Body)
	suite.fakeQueueLatency.AssertExpectations(suite.T())

}

func (suite *HandlerSuite) TestServerHandlerBadRequest() {
	w := httptest.NewRecorder()
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	suite.fakeQueueLatency.On("With", histogramFunctionCall).Return().Once()
	suite.fakeQueueLatency.On("Observe", suite.fakeLatency.Seconds()).Return().Once()
	suite.fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)
	suite.fakeInvalidCount.On("Add", mock.AnythingOfType("float64")).Return().Once()

	suite.handler.ServeHTTP(w, suite.exampleRequestFunc(1))
	resp := w.Result()

	suite.Equal(http.StatusBadRequest, resp.StatusCode)
	suite.fakeQueueLatency.AssertExpectations(suite.T())
	suite.fakeQueueDepth.AssertExpectations(suite.T())
}

func (suite *HandlerSuite) TestServerHandlerFull() {
	suite.handler.incomingQueueDepth = 1
	suite.fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	suite.fakeQueueLatency.On("With", histogramFunctionCall).Return().Once()
	suite.fakeQueueLatency.On("Observe", suite.fakeLatency.Seconds()).Return().Once()

	w := httptest.NewRecorder()
	suite.handler.ServeHTTP(w, suite.exampleRequestFunc(4))
	resp := w.Result()
	defer resp.Body.Close()

	suite.Equal(http.StatusServiceUnavailable, resp.StatusCode)
	assert.NotNil(suite.T(), resp.Body)
	suite.fakeQueueLatency.AssertExpectations(suite.T())

}

func (suite *HandlerSuite) TestServerEmptyPaylod() {
	var buffer bytes.Buffer
	r := bytes.NewReader(buffer.Bytes())
	req := httptest.NewRequest("POST", "localhost:8080", r)
	req.Header.Set("Content-Type", wrp.MimeTypeMsgpack)

	suite.fakeEmptyRequests.On("Add", mock.AnythingOfType("float64")).Return().Once()
	suite.fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	suite.fakeQueueLatency.On("With", histogramFunctionCall).Return().Once()
	suite.fakeQueueLatency.On("Observe", suite.fakeLatency.Seconds()).Return().Once()

	w := httptest.NewRecorder()
	suite.handler.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	suite.Equal(http.StatusBadRequest, resp.StatusCode)
	suite.NotNil(suite.T(), resp.Body)

	suite.fakeQueueLatency.AssertExpectations(suite.T())

}

func (suite *HandlerSuite) TestServerUnableToReadBody() {
	var buffer bytes.Buffer
	r := iotest.TimeoutReader(bytes.NewReader(buffer.Bytes()))
	_, _ = r.Read(nil)
	req := httptest.NewRequest("POST", "localhost:8080", r)
	req.Header.Set("Content-Type", wrp.MimeTypeMsgpack)

	suite.fakeErrorRequests.On("Add", mock.AnythingOfType("float64")).Return().Once()
	suite.fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)

	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	suite.fakeQueueLatency.On("With", histogramFunctionCall).Return().Once()
	suite.fakeQueueLatency.On("Observe", suite.fakeLatency.Seconds()).Return().Once()

	w := httptest.NewRecorder()
	suite.handler.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	suite.Equal(http.StatusBadRequest, resp.StatusCode)
	suite.NotNil(suite.T(), resp.Body)

	suite.fakeQueueLatency.AssertExpectations(suite.T())

}

func (suite *HandlerSuite) TestServerInvalidBody() {
	r := bytes.NewReader([]byte("Invalid payload."))

	_, _ = r.Read(nil)
	req := httptest.NewRequest("POST", "localhost:8080", r)
	req.Header.Set("Content-Type", wrp.MimeTypeMsgpack)

	suite.fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(4)
	suite.fakeInvalidCount.On("Add", mock.AnythingOfType("float64")).Return().Once()
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	suite.fakeQueueLatency.On("With", histogramFunctionCall).Return().Once()
	suite.fakeQueueLatency.On("Observe", suite.fakeLatency.Seconds()).Return().Once()

	w := httptest.NewRecorder()
	suite.handler.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	suite.Equal(http.StatusBadRequest, resp.StatusCode)
	suite.NotNil(suite.T(), resp.Body)

	suite.fakeQueueLatency.AssertExpectations(suite.T())

}

func (suite *HandlerSuite) TestHandlerNoContentType() {

	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	suite.handler.metrics.IncomingQueueLatency = suite.fakeQueueLatency
	suite.fakeQueueLatency.On("With", histogramFunctionCall).Return().Once()
	suite.fakeQueueLatency.On("Observe", suite.fakeLatency.Seconds()).Return().Once()
	w := httptest.NewRecorder()
	req := suite.exampleRequestFunc(4)
	req.Header.Del("Content-Type")

	suite.handler.ServeHTTP(w, req)
	resp := w.Result()
	suite.Equal(http.StatusUnsupportedMediaType, resp.StatusCode)
}

func (suite *HandlerSuite) TestHandlerUnSupportedContentType() {
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	suite.handler.metrics.IncomingQueueLatency = suite.fakeQueueLatency
	suite.fakeQueueLatency.On("With", histogramFunctionCall).Return().Once()
	suite.fakeQueueLatency.On("Observe", suite.fakeLatency.Seconds()).Return().Once()
	w := httptest.NewRecorder()
	req := suite.exampleRequestFunc(4)
	req.Header.Del("Content-Type")

	req.Header.Set("Content-Type", "application/json")
	suite.handler.ServeHTTP(w, req)
	resp := w.Result()
	suite.Equal(http.StatusUnsupportedMediaType, resp.StatusCode)
}

func (suite *HandlerSuite) TestHandlerMultipleContentTypes() {
	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: metrics.UnknownEventType}
	suite.handler.metrics.IncomingQueueLatency = suite.fakeQueueLatency
	suite.fakeQueueLatency.On("With", histogramFunctionCall).Return().Once()
	suite.fakeQueueLatency.On("Observe", suite.fakeLatency.Seconds()).Return().Once()
	w := httptest.NewRecorder()
	req := suite.exampleRequestFunc(4)
	req.Header.Del("Content-Type")

	req.Header.Add("Content-Type", "application/msgpack")
	req.Header.Add("Content-Type", "application/msgpack")
	req.Header.Add("Content-Type", "application/msgpack")

	suite.handler.ServeHTTP(w, req)
	resp := w.Result()
	suite.Equal(http.StatusUnsupportedMediaType, resp.StatusCode)

}

// func (suite *HandlerSuite) TestServerHandlerFixWrp() {
// 	suite.fakeIncomingContentTypeCount.On("With", prometheus.Labels{"content_type": wrp.MimeTypeMsgpack}).Return(suite.fakeIncomingContentTypeCount)
// 	suite.fakeIncomingContentTypeCount.On("With", prometheus.Labels{"content_type": ""}).Return(suite.fakeIncomingContentTypeCount)
// 	suite.fakeIncomingContentTypeCount.On("Add", 1.0).Return()

// 	suite.fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)

// 	suite.fakeModifiedWRPCount.On("With", prometheus.Labels{metrics.ReasonLabel: metrics.BothEmptyReason}).Return(suite.fakeIncomingContentTypeCount).Once()
// 	suite.fakeModifiedWRPCount.On("Add", 1.0).Return().Once()

// 	histogramFunctionCall := prometheus.Labels{metrics.EventLabel: "bob"}
// 	suite.fakeQueueLatency.On("With", histogramFunctionCall).Return().Once()
// 	suite.fakeQueueLatency.On("Observe", suite.fakeLatency.Seconds()).Return().Once()

// 	w := httptest.NewRecorder()
// 	suite.handler.ServeHTTP(w, suite.exampleRequestFunc(4, "", ""))
// 	resp := w.Result()
// 	defer resp.Body.Close()

// 	suite.Equal(http.StatusAccepted, resp.StatusCode)
// 	assert.NotNil(suite.T(), resp.Body)

// 	suite.fakeQueueLatency.AssertExpectations(suite.T())
// 	suite.fakeQueueDepth.AssertExpectations(suite.T())
// 	suite.fakeModifiedWRPCount.AssertExpectations(suite.T())
// 	suite.fakeIncomingContentTypeCount.AssertExpectations(suite.T())
// }
