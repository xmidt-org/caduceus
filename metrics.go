// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/xmidt-org/ancla"
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
	unknownEventType       = "unknown"

	// metric labels

	codeLabel   = "code"
	urlLabel    = "url"
	eventLabel  = "event"
	reasonLabel = "reason"

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
	noErrReason                           = "no_err"

	// dropped message codes
	messageDroppedCode = "message_dropped"
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
	errorRequests        prometheus.Counter
	emptyRequests        prometheus.Counter
	invalidCount         prometheus.Counter
	incomingQueueDepth   prometheus.Gauge
	modifiedWRPCount     CounterVec
	incomingQueueLatency prometheus.ObserverVec
}

type SenderWrapperMetrics struct {
	eventType CounterVec
}

type OutboundSenderMetrics struct {
	queryLatency          prometheus.ObserverVec
	deliveryCounter       CounterVec
	deliveryRetryCounter  CounterVec
	droppedMessage        CounterVec
	cutOffCounter         CounterVec
	queueDepthGauge       GaugeVec
	renewalTimeGauge      GaugeVec
	deliverUntilGauge     GaugeVec
	dropUntilGauge        GaugeVec
	maxWorkersGauge       GaugeVec
	currentWorkersGauge   GaugeVec
	deliveryRetryMaxGauge GaugeVec
}

func Metrics(tf *touchstone.Factory) (ServerHandlerMetrics, SenderWrapperMetrics, OutboundSenderMetrics, ancla.Measures, error) {
	var errs error

	errorRequests, err := tf.NewCounter(prometheus.CounterOpts{
		Name: ErrorRequestBodyCounter,
		Help: "Count of the number of errors encountered reading the body.",
	})
	errs = errors.Join(errs, err)
	emptyRequests, err := tf.NewCounter(prometheus.CounterOpts{
		Name: EmptyRequestBodyCounter,
		Help: "Count of the number of times the request is an empty body.",
	})
	errs = errors.Join(errs, err)
	invalidCount, err := tf.NewCounter(prometheus.CounterOpts{
		Name: DropsDueToInvalidPayload,
		Help: "Count of dropped messages dues to an invalid payload",
	})
	errs = errors.Join(errs, err)
	incomingQueueDepth, err := tf.NewGauge(prometheus.GaugeOpts{
		Name: IncomingQueueDepth,
		Help: "The depth of the queue behind the incoming handlers.",
	})
	errs = errors.Join(errs, err)
	modifiedWRPCount, err := tf.NewCounterVec(prometheus.CounterOpts{
		Name: ModifiedWRPCounter,
		Help: "Number of times a WRP was modified by Caduceus",
	}, reasonLabel)
	errs = errors.Join(errs, err)
	incomingQueueLatency, err := tf.NewHistogramVec(prometheus.HistogramOpts{
		Name:    IncomingQueueLatencyHistogram,
		Help:    "A histogram of latencies for the incoming queue.",
		Buckets: []float64{0.0625, 0.125, .25, .5, 1, 5, 10, 20, 40, 80, 160},
	}, eventLabel)
	errs = errors.Join(errs, err)

	sh := ServerHandlerMetrics{
		errorRequests:        errorRequests,
		emptyRequests:        emptyRequests,
		invalidCount:         invalidCount,
		incomingQueueDepth:   incomingQueueDepth,
		modifiedWRPCount:     modifiedWRPCount,
		incomingQueueLatency: incomingQueueLatency,
	}

	eventType, err := tf.NewCounterVec(prometheus.CounterOpts{
		Name: IncomingEventTypeCounter,
		Help: "Incoming count of events by event type",
	}, eventLabel)
	errs = errors.Join(errs, err)

	sw := SenderWrapperMetrics{
		eventType: eventType,
	}

	queryLatency, err := tf.NewHistogramVec(prometheus.HistogramOpts{
		Name:    QueryDurationHistogram,
		Help:    "A histogram of latencies for queries.",
		Buckets: []float64{0.0625, 0.125, .25, .5, 1, 5, 10, 20, 40, 80, 160},
	}, urlLabel, codeLabel, reasonLabel)
	errs = errors.Join(errs, err)
	deliveryCounter, err := tf.NewCounterVec(prometheus.CounterOpts{
		Name: DeliveryCounter,
		Help: "Count of delivered messages to a url with a status code",
	}, urlLabel, reasonLabel, eventLabel, codeLabel)
	errs = errors.Join(errs, err)
	deliveryRetryCounter, err := tf.NewCounterVec(prometheus.CounterOpts{
		Name: DeliveryRetryCounter,
		Help: "Number of delivery retries made",
	}, urlLabel, eventLabel)
	errs = errors.Join(errs, err)
	deliveryRetryMaxGauge, err := tf.NewGaugeVec(prometheus.GaugeOpts{
		Name: DeliveryRetryMaxGauge,
		Help: "Maximum number of delivery retries attempted",
	}, urlLabel)
	errs = errors.Join(errs, err)
	cutOffCounter, err := tf.NewCounterVec(prometheus.CounterOpts{
		Name: SlowConsumerCounter,
		Help: "Count of the number of times a consumer has been deemed too slow and is cut off.",
	}, urlLabel)
	errs = errors.Join(errs, err)
	droppedMessage, err := tf.NewCounterVec(prometheus.CounterOpts{
		Name: SlowConsumerDroppedMsgCounter,
		Help: "Count of dropped messages due to a slow consumer",
	}, urlLabel, reasonLabel)
	errs = errors.Join(errs, err)
	queueDepthGauge, err := tf.NewGaugeVec(prometheus.GaugeOpts{
		Name: OutgoingQueueDepth,
		Help: "The depth of the queue per outgoing url.",
	}, urlLabel)
	errs = errors.Join(errs, err)
	renewalTimeGauge, err := tf.NewGaugeVec(prometheus.GaugeOpts{
		Name: ConsumerRenewalTimeGauge,
		Help: "Time when the consumer data was updated.",
	}, urlLabel)
	errs = errors.Join(errs, err)
	deliverUntilGauge, err := tf.NewGaugeVec(prometheus.GaugeOpts{
		Name: ConsumerDeliverUntilGauge,
		Help: "Time when the consumer's registration expires and events will be dropped.",
	}, urlLabel)
	errs = errors.Join(errs, err)
	dropUntilGauge, err := tf.NewGaugeVec(prometheus.GaugeOpts{
		Name: ConsumerDropUntilGauge,
		Help: "The time after which events going to a customer will be delivered.",
	}, urlLabel)
	errs = errors.Join(errs, err)
	currentWorkersGauge, err := tf.NewGaugeVec(prometheus.GaugeOpts{
		Name: ConsumerDeliveryWorkersGauge,
		Help: "The number of active delivery workers for a particular customer.",
	}, urlLabel)
	errs = errors.Join(errs, err)
	maxWorkersGauge, err := tf.NewGaugeVec(prometheus.GaugeOpts{
		Name: ConsumerMaxDeliveryWorkersGauge,
		Help: "The maximum number of delivery workers available for a particular customer.",
	}, urlLabel)
	errs = errors.Join(errs, err)

	os := OutboundSenderMetrics{
		queryLatency:          queryLatency,
		deliveryCounter:       deliveryCounter,
		deliveryRetryCounter:  deliveryRetryCounter,
		droppedMessage:        droppedMessage,
		cutOffCounter:         cutOffCounter,
		queueDepthGauge:       queueDepthGauge,
		renewalTimeGauge:      renewalTimeGauge,
		deliverUntilGauge:     deliverUntilGauge,
		dropUntilGauge:        dropUntilGauge,
		maxWorkersGauge:       maxWorkersGauge,
		currentWorkersGauge:   currentWorkersGauge,
		deliveryRetryMaxGauge: deliveryRetryMaxGauge,
	}

	webhookListSizeGauge, err := tf.NewGauge(
		prometheus.GaugeOpts{
			Name: ancla.WebhookListSizeGaugeName,
			Help: ancla.WebhookListSizeGaugeHelp,
		})
	errs = errors.Join(errs, err)
	pollsTotalCounter, err := tf.NewCounterVec(
		prometheus.CounterOpts{
			Name: ancla.ChrysomPollsTotalCounterName,
			Help: ancla.ChrysomPollsTotalCounterHelp,
		},
		ancla.OutcomeLabel,
	)
	errs = errors.Join(errs, err)

	al := ancla.Measures{
		WebhookListSizeGaugeName:     webhookListSizeGauge,
		ChrysomPollsTotalCounterName: pollsTotalCounter,
	}

	return sh, sw, os, al, errs
}
