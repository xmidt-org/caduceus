// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"github.com/go-kit/kit/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"

	// nolint:staticcheck
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
	emptyContentTypeReason = "empty_content_type"
	emptyUUIDReason        = "empty_uuid"
	bothEmptyReason        = "empty_uuid_and_content_type"
	networkError           = "network_err"
	unknownEventType       = "unknown"
)

const (
	ReasonLabel = "reason"
	UrlLabel    = "url"
	EventLabel  = "event"
	CodeLabel   = "code"
)

func CreateOutbounderMetrics(m CaduceusMetricsRegistry, c *CaduceusOutboundSender) {
	c.deliveryCounter = m.NewCounter(DeliveryCounter)
	c.deliveryRetryCounter = m.NewCounter(DeliveryRetryCounter)
	c.deliveryRetryMaxGauge = m.NewGauge(DeliveryRetryMaxGauge).With("url", c.id)
	c.cutOffCounter = m.NewCounter(SlowConsumerCounter).With("url", c.id)
	c.droppedQueueFullCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", "queue_full")
	c.droppedExpiredCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", "expired")
	c.droppedExpiredBeforeQueueCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", "expired_before_queueing")

	c.droppedCutoffCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", "cut_off")
	c.droppedInvalidConfig = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", "invalid_config")
	c.droppedNetworkErrCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", networkError)
	c.droppedPanic = m.NewCounter(DropsDueToPanic).With("url", c.id)
	c.queueDepthGauge = m.NewGauge(OutgoingQueueDepth).With("url", c.id)
	c.renewalTimeGauge = m.NewGauge(ConsumerRenewalTimeGauge).With("url", c.id)
	c.deliverUntilGauge = m.NewGauge(ConsumerDeliverUntilGauge).With("url", c.id)
	c.dropUntilGauge = m.NewGauge(ConsumerDropUntilGauge).With("url", c.id)
	c.currentWorkersGauge = m.NewGauge(ConsumerDeliveryWorkersGauge).With("url", c.id)
	c.maxWorkersGauge = m.NewGauge(ConsumerMaxDeliveryWorkersGauge).With("url", c.id)
}

func NewMetricWrapperMeasures(m CaduceusMetricsRegistry) metrics.Histogram {
	return m.NewHistogram(QueryDurationHistogram, 11)
}

// TODO: do these need to be annonated/broken into groups based on where the metrics are being used/called
func ProvideMetrics() fx.Option {
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
