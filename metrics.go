// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"github.com/go-kit/kit/metrics"
	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
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
	unknownEventType       = "unknown"

	// metric labels

	codeLabel   = "code"
	urlLabel    = "url"
	eventLabel  = "event"
	reasonLabel = "reason"

	// metric label values
	// dropped messages reasons
	genericDoReason              = "do_error"
	deadlineExceededReason       = "context_deadline_exceeded"
	contextCanceledReason        = "context_canceled"
	addressErrReason             = "address_error"
	parseAddrErrReason           = "parse_address_error"
	invalidAddrReason            = "invalid_address"
	dnsErrReason                 = "dns_error"
	hostNotFoundReason           = "host_not_found"
	connClosedReason             = "connection_closed"
	opErrReason                  = "op_error"
	networkErrReason             = "unknown_network_err"
	updateRequestURLFailedReason = "update_request_url_failed"
	noErrReason                  = "no_err"

	// dropped message codes
	messageDroppedCode = "message_dropped"
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
			Name:       ModifiedWRPCounter,
			Help:       "Number of times a WRP was modified by Caduceus",
			Type:       "counter",
			LabelNames: []string{reasonLabel},
		},
		{
			Name:       DeliveryRetryCounter,
			Help:       "Number of delivery retries made",
			Type:       "counter",
			LabelNames: []string{urlLabel, eventLabel},
		},
		{
			Name:       DeliveryRetryMaxGauge,
			Help:       "Maximum number of delivery retries attempted",
			Type:       "gauge",
			LabelNames: []string{urlLabel},
		},
		{
			Name:       DeliveryCounter,
			Help:       "Count of delivered messages to a url with a status code",
			Type:       "counter",
			LabelNames: []string{urlLabel, reasonLabel, codeLabel, eventLabel},
		},
		{
			Name:       SlowConsumerDroppedMsgCounter,
			Help:       "Count of dropped messages due to a slow consumer",
			Type:       "counter",
			LabelNames: []string{urlLabel, reasonLabel},
		},
		{
			Name:       SlowConsumerCounter,
			Help:       "Count of the number of times a consumer has been deemed too slow and is cut off.",
			Type:       "counter",
			LabelNames: []string{urlLabel},
		},
		{
			Name:       OutgoingQueueDepth,
			Help:       "The depth of the queue per outgoing url.",
			Type:       "gauge",
			LabelNames: []string{urlLabel},
		},
		{
			Name:       IncomingEventTypeCounter,
			Help:       "Incoming count of events by event type",
			Type:       "counter",
			LabelNames: []string{eventLabel},
		},
		{
			Name:       DropsDueToPanic,
			Help:       "The outgoing message delivery pipeline panicked.",
			Type:       "counter",
			LabelNames: []string{urlLabel},
		},
		{
			Name:       ConsumerRenewalTimeGauge,
			Help:       "Time when the consumer data was updated.",
			Type:       "gauge",
			LabelNames: []string{urlLabel},
		},
		{
			Name:       ConsumerDeliverUntilGauge,
			Help:       "Time when the consumer's registration expires and events will be dropped.",
			Type:       "gauge",
			LabelNames: []string{urlLabel},
		},
		{
			Name:       ConsumerDropUntilGauge,
			Help:       "The time after which events going to a customer will be delivered.",
			Type:       "gauge",
			LabelNames: []string{urlLabel},
		},
		{
			Name:       ConsumerDeliveryWorkersGauge,
			Help:       "The number of active delivery workers for a particular customer.",
			Type:       "gauge",
			LabelNames: []string{urlLabel},
		},
		{
			Name:       ConsumerMaxDeliveryWorkersGauge,
			Help:       "The maximum number of delivery workers available for a particular customer.",
			Type:       "gauge",
			LabelNames: []string{urlLabel},
		},
		{
			Name:       QueryDurationHistogram,
			Help:       "A histogram of latencies for queries.",
			Type:       "histogram",
			LabelNames: []string{urlLabel, reasonLabel, codeLabel},
			Buckets:    []float64{0.0625, 0.125, .25, .5, 1, 5, 10, 20, 40, 80, 160},
		},
		{
			Name:       IncomingQueueLatencyHistogram,
			Help:       "A histogram of latencies for the incoming queue.",
			Type:       "histogram",
			LabelNames: []string{eventLabel},
			Buckets:    []float64{0.0625, 0.125, .25, .5, 1, 5, 10, 20, 40, 80, 160},
		},
	}
}

func CreateOutbounderMetrics(m CaduceusMetricsRegistry, c *CaduceusOutboundSender) {
	c.deliveryCounter = m.NewCounter(DeliveryCounter)
	c.deliveryRetryCounter = m.NewCounter(DeliveryRetryCounter)
	c.deliveryRetryMaxGauge = m.NewGauge(DeliveryRetryMaxGauge).With(urlLabel, c.id)
	c.cutOffCounter = m.NewCounter(SlowConsumerCounter).With(urlLabel, c.id)
	c.droppedQueueFullCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With(urlLabel, c.id, reasonLabel, "queue_full")
	c.droppedExpiredCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With(urlLabel, c.id, reasonLabel, "expired")
	c.droppedExpiredBeforeQueueCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With(urlLabel, c.id, reasonLabel, "expired_before_queueing")

	c.droppedCutoffCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With(urlLabel, c.id, reasonLabel, "cut_off")
	c.droppedInvalidConfig = m.NewCounter(SlowConsumerDroppedMsgCounter).With(urlLabel, c.id, reasonLabel, "invalid_config")
	c.droppedMessage = m.NewCounter(SlowConsumerDroppedMsgCounter)
	c.droppedPanic = m.NewCounter(DropsDueToPanic).With(urlLabel, c.id)
	c.queueDepthGauge = m.NewGauge(OutgoingQueueDepth).With(urlLabel, c.id)
	c.renewalTimeGauge = m.NewGauge(ConsumerRenewalTimeGauge).With(urlLabel, c.id)
	c.deliverUntilGauge = m.NewGauge(ConsumerDeliverUntilGauge).With(urlLabel, c.id)
	c.dropUntilGauge = m.NewGauge(ConsumerDropUntilGauge).With(urlLabel, c.id)
	c.currentWorkersGauge = m.NewGauge(ConsumerDeliveryWorkersGauge).With(urlLabel, c.id)
	c.maxWorkersGauge = m.NewGauge(ConsumerMaxDeliveryWorkersGauge).With(urlLabel, c.id)
}

func NewMetricWrapperMeasures(m CaduceusMetricsRegistry) metrics.Histogram {
	return m.NewHistogram(QueryDurationHistogram, 11)
}
