// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package metrics

import (
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
	EmptyContentTypeReason = "empty_content_type"
	EmptyUUIDReason        = "empty_uuid"
	BothEmptyReason        = "empty_uuid_and_content_type"
	NetworkError           = "network_err"
	UnknownEventType       = "unknown"
)

const (
	ReasonLabel = "reason"
	UrlLabel    = "url"
	EventLabel  = "event"
	CodeLabel   = "code"
)

// MetricsIn will be populated automatically by the ProvideMetrics function
// and then used to populate the Metrics struct
type MetricsIn struct {
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

// Metrics will be used to set up the metrics for each sink
type Metrics struct {
	DeliveryCounter                 *prometheus.CounterVec
	DeliveryRetryCounter            *prometheus.CounterVec
	DeliveryRetryMaxGauge           *prometheus.GaugeVec
	CutOffCounter                   *prometheus.CounterVec
	SlowConsumerDroppedMsgCounter   *prometheus.CounterVec
	DropsDueToPanic                 *prometheus.CounterVec
	ConsumerDeliverUntilGauge       *prometheus.GaugeVec
	ConsumerDropUntilGauge          *prometheus.GaugeVec
	ConsumerDeliveryWorkersGauge    *prometheus.GaugeVec
	ConsumerMaxDeliveryWorkersGauge *prometheus.GaugeVec
	OutgoingQueueDepth              *prometheus.GaugeVec
	ConsumerRenewalTimeGauge        *prometheus.GaugeVec
	QueryLatency                    prometheus.ObserverVec
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