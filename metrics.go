package main

import (
	"github.com/xmidt-org/webpa-common/xmetrics"
)

const (
	ErrorRequestBodyCounter         = "error_request_body_count"
	EmptyRequestBodyCounter         = "empty_request_body_count"
	DeliveryCounter                 = "delivery_count"
	DeliveryRetryCounter            = "delivery_retry_count"
	DeliveryRetryMaxGauge           = "delivery_retry_max"
	SlowConsumerDroppedMsgCounter   = "slow_consumer_dropped_message_count"
	SlowConsumerCounter             = "slow_consumer_cut_off_count"
	IncomingQueueDepth              = "incoming_queue_depth"
	IncomingContentTypeCounter      = "incoming_content_type_count"
	IncomingEventTypeCounter        = "incoming_event_type_count"
	DropsDueToInvalidPayload        = "drops_due_to_invalid_payload"
	OutgoingQueueDepth              = "outgoing_queue_depths"
	DropsDueToPanic                 = "drops_due_to_panic"
	ConsumerRenewalTimeGauge        = "consumer_renewal_time"
	ConsumerDeliverUntilGauge       = "consumer_deliver_until"
	ConsumerDropUntilGauge          = "consumer_drop_until"
	ConsumerDeliveryWorkersGauge    = "consumer_delivery_workers"
	ConsumerMaxDeliveryWorkersGauge = "consumer_delivery_workers_max"
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
			LabelNames: []string{"url", "event"},
		},
		{
			Name:       DeliveryRetryMaxGauge,
			Help:       "Maximum number of delivery retries attempted",
			Type:       "gauge",
			LabelNames: []string{"url"},
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
			Name:       IncomingEventTypeCounter,
			Help:       "Incoming count of events by event type",
			Type:       "counter",
			LabelNames: []string{"event"},
		},
		{
			Name:       DropsDueToPanic,
			Help:       "The outgoing message delivery pipeline panicked.",
			Type:       "counter",
			LabelNames: []string{"url"},
		},
		{
			Name:       ConsumerRenewalTimeGauge,
			Help:       "Time when the consumer data was updated.",
			Type:       "gauge",
			LabelNames: []string{"url"},
		},
		{
			Name:       ConsumerDeliverUntilGauge,
			Help:       "Time when the consumer's registration expires and events will be dropped.",
			Type:       "gauge",
			LabelNames: []string{"url"},
		},
		{
			Name:       ConsumerDropUntilGauge,
			Help:       "The time after which events going to a customer will be delivered.",
			Type:       "gauge",
			LabelNames: []string{"url"},
		},
		{
			Name:       ConsumerDeliveryWorkersGauge,
			Help:       "The number of active delivery workers for a particular customer.",
			Type:       "gauge",
			LabelNames: []string{"url"},
		},
		{
			Name:       ConsumerMaxDeliveryWorkersGauge,
			Help:       "The maximum number of delivery workers available for a particular customer.",
			Type:       "gauge",
			LabelNames: []string{"url"},
		},
	}
}

func CreateOutbounderMetrics(m CaduceusMetricsRegistry, c *CaduceusOutboundSender) {
	c.deliveryCounter = m.NewCounter(DeliveryCounter)
	c.deliveryRetryCounter = m.NewCounter(DeliveryRetryCounter)
	c.deliveryRetryMaxGauge = m.NewGauge(DeliveryRetryMaxGauge)
	c.cutOffCounter = m.NewCounter(SlowConsumerCounter).With("url", c.id)
	c.droppedQueueFullCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", "queue_full")
	c.droppedExpiredCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", "expired")
	c.droppedExpiredBeforeQueueCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", "expired_before_queueing")

	c.droppedCutoffCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", "cut_off")
	c.droppedInvalidConfig = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", "invalid_config")
	c.droppedNetworkErrCounter = m.NewCounter(SlowConsumerDroppedMsgCounter).With("url", c.id, "reason", "network_err")
	c.droppedPanic = m.NewCounter(DropsDueToPanic).With("url", c.id)
	c.queueDepthGauge = m.NewGauge(OutgoingQueueDepth).With("url", c.id)
	c.contentTypeCounter = m.NewCounter(IncomingContentTypeCounter)
	c.renewalTimeGauge = m.NewGauge(ConsumerRenewalTimeGauge).With("url", c.id)
	c.deliverUntilGauge = m.NewGauge(ConsumerDeliverUntilGauge).With("url", c.id)
	c.dropUntilGauge = m.NewGauge(ConsumerDropUntilGauge).With("url", c.id)
	c.currentWorkersGauge = m.NewGauge(ConsumerDeliveryWorkersGauge).With("url", c.id)
	c.maxWorkersGauge = m.NewGauge(ConsumerMaxDeliveryWorkersGauge).With("url", c.id)
}
