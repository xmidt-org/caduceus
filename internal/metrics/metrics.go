// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package metrics

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"

	"github.com/xmidt-org/touchstone"
)

const (
	ErrorRequestBodyCounter         = "error_request_body_count"
	EmptyRequestBodyCounter         = "empty_request_body_count"
	ModifiedWRPCounter              = "modified_wrp_count"
	DeliveryCounter                 = "delivery_count"
	DeliveryRetryCounter            = "delivery_retry_count"
	DeliveryRetryMaxGauge           = "delivery_retry_max"
	SlowConsumerDroppedMsgCounter   = "slow_consumer_dropped_message_count"
	SlowConsumerCounter             = "slow_consumer_cut_off_count"
	IncomingQueueDepth              = "incoming_queue_depth"
	IncomingEventTypeCounter        = "incoming_event_type_count"
	DropsDueToInvalidPayload        = "drops_due_to_invalid_payload"
	OutgoingQueueDepth              = "outgoing_queue_depths"
	DropsDueToPanic                 = "drops_due_to_panic"
	ConsumerRenewalTimeGauge        = "consumer_renewal_time"
	ConsumerDeliverUntilGauge       = "consumer_deliver_until"
	ConsumerDropUntilGauge          = "consumer_drop_until"
	ConsumerDeliveryWorkersGauge    = "consumer_delivery_workers"
	ConsumerMaxDeliveryWorkersGauge = "consumer_delivery_workers_max"
	QueryDurationHistogram          = "query_duration_histogram_seconds"
	IncomingQueueLatencyHistogram   = "incoming_queue_latency_histogram_seconds"
)

const (
	NoErrReason = "no_err"
	// TODO remove the do_error case
	GenericDoReason        = "do_error"
	EmptyContentTypeReason = "empty_content_type"
	EmptyUUIDReason        = "empty_uuid"
	BothEmptyReason        = "empty_uuid_and_content_type"
	// TODO revisit the network_err case
	NetworkError     = "network_err"
	UnknownEventType = "unknown"
	// metric labels

	CodeLabel   = "code"
	UrlLabel    = "url"
	EventLabel  = "event"
	ReasonLabel = "reason"

	// metric label values
	// dropped messages reasons
	unknown                               = "unknown"
	deadlineExceededReason                = "context_deadline_exceeded"
	contextCanceledReason                 = "context_canceled"
	addressErrReason                      = "address_error"
	parseAddrErrReason                    = "parse_address_error"
	invalidAddrReason                     = "invalid_address"
	dnsErrReason                          = "dns_error"
	hostNotFoundReason                    = "host_not_found"
	connClosedReason                      = "connection_closed"
	opErrReason                           = "op_error"
	networkErrReason                      = "unknown_network_err"
	updateRequestURLFailedReason          = "update_request_url_failed"
	connectionUnexpectedlyClosedEOFReason = "connection_unexpectedly_closed_eof"

	// dropped message codes
	MessageDroppedCode = "message_dropped"
)

type CounterVec interface {
	prometheus.Collector
	CurryWith(labels prometheus.Labels) (*prometheus.CounterVec, error)
	GetMetricWith(labels prometheus.Labels) (prometheus.Counter, error)
	GetMetricWithLabelValues(lvs ...string) (prometheus.Counter, error)
	MustCurryWith(labels prometheus.Labels) *prometheus.CounterVec
	With(labels prometheus.Labels) prometheus.Counter
	WithLabelValues(lvs ...string) prometheus.Counter
}

type GaugeVec interface {
	prometheus.Collector
	CurryWith(labels prometheus.Labels) (*prometheus.GaugeVec, error)
	GetMetricWith(labels prometheus.Labels) (prometheus.Gauge, error)
	GetMetricWithLabelValues(lvs ...string) (prometheus.Gauge, error)
	MustCurryWith(labels prometheus.Labels) *prometheus.GaugeVec
	With(labels prometheus.Labels) prometheus.Gauge
	WithLabelValues(lvs ...string) prometheus.Gauge
}

type ServerHandlerMetrics struct {
	fx.In

	ErrorRequests        prometheus.Counter     `name:"error_request_body_count"`
	EmptyRequests        prometheus.Counter     `name:"empty_request_body_count"`
	InvalidCount         prometheus.Counter     `name:"drops_due_to_invalid_payload"`
	IncomingQueueDepth   prometheus.Gauge       `name:"incoming_queue_depth"`
	ModifiedWRPCount     CounterVec             `name:"modified_wrp_count"`
	IncomingQueueLatency prometheus.ObserverVec `name:"incoming_queue_latency_histogram_seconds"`
}

type SenderWrapperMetrics struct {
	EventType CounterVec `name:"incoming_event_type_count"`
}

type OutboundSenderMetrics_ struct {
	fx.In

	QueryLatency              prometheus.ObserverVec `name:"query_duration_histogram_seconds"`
	DeliveryCounter           *prometheus.CounterVec `name:"delivery_count"`
	DeliveryRetryCounter      *prometheus.CounterVec `name:"delivery_retry_count"`
	DroppedMessage            *prometheus.CounterVec `name:"dropped_message_count"`
	CutOffCounter             *prometheus.CounterVec `name:"slow_consumer_cut_off_count"`
	QueueDepthGauge           *prometheus.GaugeVec   `name:"queue_depth_gauge"`
	ConsumerRenewalTimeGauge  *prometheus.GaugeVec   `name:"consumer_renewal_time"`
	ConsumerDeliverUntilGauge *prometheus.GaugeVec   `name:"consumer_deliver_until"`
	ConsumerDropUntilGauge    *prometheus.GaugeVec   `name:"consumer_drop_until"`
	MaxWorkersGauge           *prometheus.GaugeVec   `name:"max_workers_gauge"`
	CurrentWorkersGauge       *prometheus.GaugeVec   `name:"current_workers_gauge"`
	DeliveryRetryMaxGauge     *prometheus.GaugeVec   `name:"delivery_retry_max"`
}

// Metrics provides be used to set up the metrics for each sink.
type Metrics struct {
	fx.In

	QueryLatency                    prometheus.ObserverVec `name:"query_duration_histogram_seconds"`
	DeliveryCounter                 *prometheus.CounterVec `name:"delivery_count"`
	DeliveryRetryCounter            *prometheus.CounterVec `name:"delivery_retry_count"`
	DeliveryRetryMaxGauge           *prometheus.GaugeVec   `name:"delivery_retry_max"`
	CutOffCounter                   *prometheus.CounterVec `name:"slow_consumer_cut_off_count"`
	SlowConsumerDroppedMsgCounter   *prometheus.CounterVec `name:"slow_consumer_dropped_message_count"`
	DropsDueToPanic                 *prometheus.CounterVec `name:"drops_due_to_panic"`
	ConsumerDeliverUntilGauge       *prometheus.GaugeVec   `name:"consumer_deliver_until"`
	ConsumerDropUntilGauge          *prometheus.GaugeVec   `name:"consumer_drop_until"`
	ConsumerDeliveryWorkersGauge    *prometheus.GaugeVec   `name:"consumer_delivery_workers"`
	ConsumerMaxDeliveryWorkersGauge *prometheus.GaugeVec   `name:"consumer_delivery_workers_max"`
	OutgoingQueueDepth              *prometheus.GaugeVec   `name:"outgoing_queue_depths"`
	ConsumerRenewalTimeGauge        *prometheus.GaugeVec   `name:"consumer_renewal_time"`
}

type OutboundSenderMetrics struct {
	fx.In

	QueryLatency          prometheus.ObserverVec `name:"query_duration_histogram_seconds"`
	DeliveryCounter       CounterVec             `name:"delivery_count"`
	DeliveryRetryCounter  CounterVec             `name:"delivery_retry_count"`
	DroppedMessage        CounterVec             `name:"slow_consumer_dropped_message_count"`
	CutOffCounter         CounterVec             `name:"slow_consumer_cut_off_count"`
	QueueDepthGauge       GaugeVec               `name:"outgoing_queue_depths"`
	RenewalTimeGauge      GaugeVec               `name:"consumer_renewal_time"`
	DeliverUntilGauge     GaugeVec               `name:"consumer_deliver_until"`
	DropUntilGauge        GaugeVec               `name:"consumer_drop_until"`
	MaxWorkersGauge       GaugeVec               `name:"consumer_delivery_workers_max"`
	CurrentWorkersGauge   GaugeVec               `name:"consumer_delivery_workers"`
	DeliveryRetryMaxGauge GaugeVec               `name:"delivery_retry_max"`
}

// TODO: do these need to be annonated/broken into groups based on where the metrics are being used/called
func Provide() fx.Option {
	return fx.Options(
		touchstone.Gauge(prometheus.GaugeOpts{
			Name: IncomingQueueDepth,
			Help: "The depth of the queue behind the incoming handlers.",
		}),
		touchstone.Counter(prometheus.CounterOpts{
			Name: ErrorRequestBodyCounter,
			Help: "Count of the number of errors encountered reading the body.",
		}),
		touchstone.Counter(prometheus.CounterOpts{
			Name: EmptyRequestBodyCounter,
			Help: "Count of the number of times the request is an empty body.",
		}),
		touchstone.CounterVec(prometheus.CounterOpts{
			Name: ModifiedWRPCounter,
			Help: "Number of times a WRP was modified by Caduceus",
		}, ReasonLabel),
		touchstone.CounterVec(prometheus.CounterOpts{
			Name: DeliveryRetryCounter,
			Help: "Number of delivery retries made",
		}, UrlLabel, EventLabel),
		touchstone.GaugeVec(prometheus.GaugeOpts{
			Name: DeliveryRetryMaxGauge,
			Help: "Maximum number of delivery retries attempted",
		}, UrlLabel),
		touchstone.CounterVec(prometheus.CounterOpts{
			Name: DeliveryCounter,
			Help: "Count of delivered messages to a url with a status code",
		}, UrlLabel, EventLabel, CodeLabel),
		touchstone.CounterVec(prometheus.CounterOpts{
			Name: SlowConsumerDroppedMsgCounter,
			Help: "Count of dropped messages due to a slow consumer",
		}, UrlLabel, ReasonLabel),
		touchstone.CounterVec(prometheus.CounterOpts{
			Name: SlowConsumerCounter,
			Help: "Count of the number of times a consumer has been deemed too slow and is cut off.",
		}, UrlLabel),
		touchstone.Counter(prometheus.CounterOpts{
			Name: DropsDueToInvalidPayload,
			Help: "Count of dropped messages dues to an invalid payload",
		}),
		touchstone.GaugeVec(prometheus.GaugeOpts{
			Name: OutgoingQueueDepth,
			Help: "The depth of the queue per outgoing url.",
		}, UrlLabel),
		touchstone.CounterVec(prometheus.CounterOpts{
			Name: IncomingEventTypeCounter,
			Help: "Incoming count of events by event type",
		}, EventLabel),
		touchstone.CounterVec(prometheus.CounterOpts{
			Name: DropsDueToPanic,
			Help: "The outgoing message delivery pipeline panicked.",
		}, UrlLabel),
		touchstone.GaugeVec(prometheus.GaugeOpts{
			Name: ConsumerRenewalTimeGauge,
			Help: "Time when the consumer data was updated.",
		}, UrlLabel),
		touchstone.GaugeVec(prometheus.GaugeOpts{
			Name: ConsumerDeliverUntilGauge,
			Help: "Time when the consumer's registration expires and events will be dropped.",
		}, UrlLabel),
		touchstone.GaugeVec(prometheus.GaugeOpts{
			Name: ConsumerDropUntilGauge,
			Help: "The time after which events going to a customer will be delivered.",
		}, UrlLabel),
		touchstone.GaugeVec(prometheus.GaugeOpts{
			Name: ConsumerDeliveryWorkersGauge,
			Help: "The number of active delivery workers for a particular customer.",
		}, UrlLabel),
		touchstone.GaugeVec(prometheus.GaugeOpts{
			Name: ConsumerMaxDeliveryWorkersGauge,
			Help: "The maximum number of delivery workers available for a particular customer.",
		}, UrlLabel),
		touchstone.HistogramVec(prometheus.HistogramOpts{
			Name:    QueryDurationHistogram,
			Help:    "A histogram of latencies for queries.",
			Buckets: []float64{0.0625, 0.125, .25, .5, 1, 5, 10, 20, 40, 80, 160},
		}, UrlLabel, CodeLabel),
		touchstone.HistogramVec(prometheus.HistogramOpts{
			Name:    IncomingQueueLatencyHistogram,
			Help:    "A histogram of latencies for the incoming queue.",
			Buckets: []float64{0.0625, 0.125, .25, .5, 1, 5, 10, 20, 40, 80, 160},
		}, EventLabel),
	)
}

func GetDoErrReason(err error) string {
	var d *net.DNSError
	if err == nil {
		return NoErrReason
	} else if errors.Is(err, context.DeadlineExceeded) {
		return deadlineExceededReason

	} else if errors.Is(err, context.Canceled) {
		return contextCanceledReason

	} else if errors.Is(err, &net.AddrError{}) {
		return addressErrReason

	} else if errors.Is(err, &net.ParseError{}) {
		return parseAddrErrReason

	} else if errors.Is(err, net.InvalidAddrError("")) {
		return invalidAddrReason

	} else if errors.As(err, &d) {
		if d.IsNotFound {
			return hostNotFoundReason
		}

		return dnsErrReason
	} else if errors.Is(err, net.ErrClosed) {
		return connClosedReason

	} else if errors.Is(err, &net.OpError{}) {
		return opErrReason

	} else if errors.Is(err, net.UnknownNetworkError("")) {
		return networkErrReason

	}
	// nolint: errorlint
	if err, ok := err.(*url.Error); ok {
		if strings.TrimSpace(strings.ToLower(err.Unwrap().Error())) == "eof" {
			return connectionUnexpectedlyClosedEOFReason
		}

	}

	return GenericDoReason
}
