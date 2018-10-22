package main

import (
	"net/http"
	"time"

	"github.com/Comcast/webpa-common/xhttp"
	"github.com/Comcast/webpa-common/xmetrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	ErrorRequestBodyCounter       = "error_request_body_count"
	EmptyRequestBodyCounter       = "empty_request_body_count"
	DeliveryCounter               = "delivery_count"
	DeliveryRetryCounter          = "delivery_retry_count"
	SlowConsumerDroppedMsgCounter = "slow_consumer_dropped_message_count"
	SlowConsumerCounter           = "slow_consumer_cut_off_count"
	IncomingQueueDepth            = "incoming_queue_depth"
	IncomingContentTypeCounter    = "incoming_content_type_count"
	DropsDueToInvalidPayload      = "drops_due_to_invalid_payload"
	OutgoingQueueDepth            = "outgoing_queue_depths"
	OutboundRequestDuration       = "outbound_request_duration"
)

func Metrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		{
			Name: IncomingQueueDepth,
			Help: "The depth of the queue behind the incoming handlers.",
			Type: "gauge",
		},
		{
			Name: ErrorRequestBodyCounter,
			Help: "Count of the number of errors encountered reading the body.",
			Type: "counter",
		},
		{
			Name: EmptyRequestBodyCounter,
			Help: "Count of the number of times the request is an empty body.",
			Type: "counter",
		},
		{
			Name: DropsDueToInvalidPayload,
			Help: "Dropped messages due to invalid payloads.",
			Type: "counter",
		},
		{
			Name:       IncomingContentTypeCounter,
			Help:       "Count of the content type processed.",
			Type:       "counter",
			LabelNames: []string{"content_type"},
		},
		{
			Name:       DeliveryRetryCounter,
			Help:       "Number of delivery retries made",
			Type:       "counter",
			LabelNames: []string{"url", "code", "event"},
		},
		{
			Name:       DeliveryCounter,
			Help:       "Count of delivered messages to a url with a status code",
			Type:       "counter",
			LabelNames: []string{"url", "code", "event"},
		},
		{
			Name:       SlowConsumerDroppedMsgCounter,
			Help:       "Count of dropped messages due to a slow consumer",
			Type:       "counter",
			LabelNames: []string{"url", "reason"},
		},
		{
			Name:       SlowConsumerCounter,
			Help:       "Count of the number of times a consumer has been deemed too slow and is cut off.",
			Type:       "counter",
			LabelNames: []string{"url"},
		},
		{
			Name:       OutgoingQueueDepth,
			Help:       "The depth of the queue per outgoing url.",
			Type:       "gauge",
			LabelNames: []string{"url"},
		},
		{
			Name:    OutboundRequestDuration,
			Help:    "The time for outbound request to get a response",
			Type:    "histogram",
			Buckets: []float64{0.010, 0.020, 0.050, 0.100, 0.200, 0.500, 1.00, 2.00, 5.00},
		},
	}
}

// OutboundMeasures holds different prometheus outbound measurements.
//
// Future prometheus measurements should be added here.
type OutboundMeasures struct {
	RequestDuration prometheus.Observer
}

// NewOutboundMeasuresFunc is used to create a OutboundMeasuresFunc type at the start of Sender Wrapper Factory
func NewOutboundMeasuresFunc(r CaduceusMetricsRegistry) OutboundMeasuresFunc {
	return func(r CaduceusMetricsRegistry) OutboundMeasures {
		return OutboundMeasures{RequestDuration: r.NewHistogramVec(OutboundRequestDuration).WithLabelValues()}
	}
}

// OutboundMeasuresFunc is the function signature for NewOutboundMeasures. It is a attribute of SenderWrapperFactory and
// is used to create new OutboundMeasure structs when the SenderWrapperFactory occurs.
type OutboundMeasuresFunc func(r CaduceusMetricsRegistry) OutboundMeasures

// NewOutboundMeasures creates a OutboundMeasures struct
func NewOutboundMeasures(r CaduceusMetricsRegistry) OutboundMeasures {
	return OutboundMeasures{
		RequestDuration: r.NewHistogramVec(OutboundRequestDuration).WithLabelValues(),
	}
}

// InstrumentOutboundDuration is used in NewOutboundRoundTripper to record the duration of
// a request's round trip.
func InstrumentOutboundDuration(obs prometheus.Observer, next func(request *http.Request) (*http.Response, error)) promhttp.RoundTripperFunc {
	return promhttp.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		start := time.Now()
		response, err := next(request)
		if err == nil {
			obs.Observe(time.Since(start).Seconds())
		}

		return response, err
	})
}

// OutboundTripperDecorator decorates a transport to record outboundrequest durations.
func NewOutboundRoundTripper(r xhttp.RetryOptions, obs *CaduceusOutboundSender) func(request *http.Request) (*http.Response, error) {
	return promhttp.RoundTripperFunc(xhttp.RetryTransactor(r, InstrumentOutboundDuration(obs.outboundMeasures.RequestDuration, obs.sender)))
}
