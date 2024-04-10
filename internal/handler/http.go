// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package handler

import (
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/xmidt-org/caduceus/internal/metrics"
	"github.com/xmidt-org/caduceus/internal/sink"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/wrp-go/v3"
)

type ServerHandlerIn struct {
	fx.In
	SinkWrapper *sink.SinkWrapper
	Logger      *zap.Logger
	Telemetry   *HandlerTelemetry
}

type ServerHandlerOut struct {
	fx.Out
	Handler *ServerHandler
}

// Below is the struct that will implement our ServeHTTP method
type ServerHandler struct {
	caduceusHandler    RequestHandler
	telemetry          *HandlerTelemetry
	incomingQueueDepth int64
	maxOutstanding     int64
	now                func() time.Time
}
type HandlerTelemetryIn struct {
	fx.In
	ErrorRequests            prometheus.Counter     `name:"error_request_body_count"`
	EmptyRequests            prometheus.Counter     `name:"empty_request_body_count"`
	InvalidCount             prometheus.Counter     `name:"drops_due_to_invalid_payload"`
	IncomingQueueDepthMetric prometheus.Gauge       `name:"incoming_queue_depth"`
	ModifiedWRPCount         *prometheus.CounterVec `name:"modified_wrp_count"`
	IncomingQueueLatency     prometheus.ObserverVec `name:"incoming_queue_latency_histogram_seconds"`
}
type HandlerTelemetry struct {
	errorRequests            prometheus.Counter
	emptyRequests            prometheus.Counter
	invalidCount             prometheus.Counter
	incomingQueueDepthMetric prometheus.Gauge
	modifiedWRPCount         *prometheus.CounterVec
	incomingQueueLatency     prometheus.ObserverVec
}

func (sh *ServerHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	eventType := metrics.UnknownEventType
	logger := sallust.Get(request.Context())
	// find time difference, add to metric after function finishes
	defer func(s time.Time) {
		sh.recordQueueLatencyToHistogram(s, eventType)
	}(sh.now())

	logger.Info("Receiving incoming request...")

	if len(request.Header["Content-Type"]) != 1 || request.Header["Content-Type"][0] != "application/msgpack" {
		//return a 415
		response.WriteHeader(http.StatusUnsupportedMediaType)
		response.Write([]byte("Invalid Content-Type header(s). Expected application/msgpack. \n"))
		logger.Debug("Invalid Content-Type header(s). Expected application/msgpack. \n")
		return
	}

	outstanding := atomic.AddInt64(&sh.incomingQueueDepth, 1)
	defer atomic.AddInt64(&sh.incomingQueueDepth, -1)

	if 0 < sh.maxOutstanding && sh.maxOutstanding < outstanding {
		// return a 503
		response.WriteHeader(http.StatusServiceUnavailable)
		response.Write([]byte("Incoming queue is full.\n"))
		logger.Debug("Incoming queue is full.\n")
		return
	}

	sh.telemetry.incomingQueueDepthMetric.Add(1.0)
	defer sh.telemetry.incomingQueueDepthMetric.Add(-1.0)

	payload, err := io.ReadAll(request.Body)
	if err != nil {
		sh.telemetry.errorRequests.Add(1.0)
		logger.Error("Unable to retrieve the request body.", zap.Error(err))
		response.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(payload) == 0 {
		sh.telemetry.emptyRequests.Add(1.0)
		logger.Error("Empty payload.")
		response.WriteHeader(http.StatusBadRequest)
		response.Write([]byte("Empty payload.\n"))
		return
	}

	decoder := wrp.NewDecoderBytes(payload, wrp.Msgpack)
	msg := new(wrp.Message)

	err = decoder.Decode(msg)
	if err != nil || msg.MessageType() != 4 {
		// return a 400
		sh.telemetry.invalidCount.Add(1.0)
		response.WriteHeader(http.StatusBadRequest)
		if err != nil {
			response.Write([]byte("Invalid payload format.\n"))
			logger.Debug("Invalid payload format.")
		} else {
			response.Write([]byte("Invalid MessageType.\n"))
			logger.Debug("Invalid MessageType.")
		}
		return
	}

	err = wrp.UTF8(msg)
	if err != nil {
		// return a 400
		sh.telemetry.invalidCount.Add(1.0)
		response.WriteHeader(http.StatusBadRequest)
		response.Write([]byte("Strings must be UTF-8.\n"))
		logger.Debug("Strings must be UTF-8.")
		return
	}
	eventType = msg.FindEventStringSubMatch()

	sh.caduceusHandler.HandleRequest(0, sh.fixWrp(msg))

	// return a 202
	response.WriteHeader(http.StatusAccepted)
	response.Write([]byte("Request placed on to queue.\n"))

	logger.Debug("event passed to senders.", zap.Any("event", msg))
}

func (sh *ServerHandler) recordQueueLatencyToHistogram(startTime time.Time, eventType string) {
	endTime := sh.now()
	sh.telemetry.incomingQueueLatency.With(prometheus.Labels{"event": eventType}).Observe(endTime.Sub(startTime).Seconds())
}

func (sh *ServerHandler) fixWrp(msg *wrp.Message) *wrp.Message {
	// "Fix" the WRP if needed.
	var reason string

	// Default to "application/json" if there is no content type, otherwise
	// use the one the source specified.
	if msg.ContentType == "" {
		msg.ContentType = wrp.MimeTypeJson
		reason = metrics.EmptyContentTypeReason
	}

	// Ensure there is a transaction id even if we make one up
	if msg.TransactionUUID == "" {
		msg.TransactionUUID = uuid.NewV4().String()
		if reason == "" {
			reason = metrics.EmptyUUIDReason
		} else {
			reason = metrics.BothEmptyReason
		}
	}

	if reason != "" {
		sh.telemetry.modifiedWRPCount.With(prometheus.Labels{"reason": reason}).Add(1.0)
	}

	return msg
}

func ProvideHandler() fx.Option {
	return fx.Provide(
		func(in HandlerTelemetryIn) *HandlerTelemetry {
			return &HandlerTelemetry{
				errorRequests:            in.ErrorRequests,
				emptyRequests:            in.EmptyRequests,
				invalidCount:             in.InvalidCount,
				incomingQueueDepthMetric: in.IncomingQueueDepthMetric,
				modifiedWRPCount:         in.ModifiedWRPCount,
				incomingQueueLatency:     in.IncomingQueueLatency,
			}
		},
		func(in ServerHandlerIn) (ServerHandlerOut, error) {
			//Hard coding maxOutstanding and incomingQueueDepth for now
			handler, err := New(in.SinkWrapper, in.Logger, in.Telemetry, 0.0, 0.0)
			return ServerHandlerOut{
				Handler: handler,
			}, err
		},
	)
}
func New(sw *sink.SinkWrapper, log *zap.Logger, t *HandlerTelemetry, maxOutstanding, incomingQueueDepth int64) (*ServerHandler, error) {
	return &ServerHandler{
		caduceusHandler: &CaduceusHandler{
			wrapper: sw,
			Logger:  log,
		},
		telemetry:          t,
		maxOutstanding:     maxOutstanding,
		incomingQueueDepth: incomingQueueDepth,
		now:                time.Now,
	}, nil
}
