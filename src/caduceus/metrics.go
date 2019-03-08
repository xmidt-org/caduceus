package main

import (
	"github.com/Comcast/webpa-common/xmetrics"
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
	IncomingEventTypeCounter      = "incoming_event_type_count"
	DropsDueToInvalidPayload      = "drops_due_to_invalid_payload"
	OutgoingQueueDepth            = "outgoing_queue_depths"
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
			LabelNames: []string{"content"},
		},
		{
			Name:       DeliveryRetryCounter,
			Help:       "Number of delivery retries made",
			Type:       "counter",
			LabelNames: []string{"url", "event"},
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
	}
}
