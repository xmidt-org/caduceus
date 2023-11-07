package metrics

import (
	"github.com/go-kit/kit/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/touchstone"
	"go.uber.org/fx"
)

const (
	ReasonLabel = "reason"
	UrlLabel    = "url"
	EventLabel  = "event"
	CodeLabel   = "code"

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

type CaduceusMetricsRegistry interface {
	NewCounter(name string) metrics.Counter
	NewGauge(name string) metrics.Gauge
	NewHistogram(name string, buckets int) metrics.Histogram
}

func ProvideMetrics() fx.Option {
	return fx.Options(
		touchstone.Gauge(
			prometheus.GaugeOpts{
				Name: IncomingQueueDepth,
				Help: "The depth of the queue behind the incoming handlers.",
			},
		),
		touchstone.Counter(
			prometheus.CounterOpts{
				Name: ErrorRequestBodyCounter,
				Help: "Count of the number of errors encountered reading the body.",
			},
		),
		touchstone.Counter(
			prometheus.CounterOpts{
				Name: EmptyRequestBodyCounter,
				Help: "Count of the number of times the request is an empty body.",
			},
		),
		touchstone.Counter(
			prometheus.CounterOpts{
				Name: DropsDueToInvalidPayload,
				Help: "Dropped messages due to invalid payloads.",
			},
		),
		touchstone.CounterVec(
			prometheus.CounterOpts{
				Name: ModifiedWRPCounter,
				Help: "Number of times a WRP was modified by Caduceus",
			}, ReasonLabel,
		),
		touchstone.CounterVec(
			prometheus.CounterOpts{
				Name: DeliveryRetryCounter,
				Help: "Number of delivery retries made",
			}, UrlLabel, EventLabel,
		),
		touchstone.GaugeVec(
			prometheus.GaugeOpts{
				Name: DeliveryRetryMaxGauge,
				Help: "Maximum number of delivery retries attempted",
			}, UrlLabel,
		),
		touchstone.CounterVec(
			prometheus.CounterOpts{
				Name: DeliveryCounter,
				Help: "Count of delivered messages to a url with a status code",
			}, UrlLabel, CodeLabel, EventLabel,
		),
		touchstone.CounterVec(
			prometheus.CounterOpts{
				Name: SlowConsumerDroppedMsgCounter,
				Help: "Count of dropped messages due to a slow consumer",
			}, UrlLabel, ReasonLabel,
		),
		touchstone.CounterVec(
			prometheus.CounterOpts{
				Name: SlowConsumerCounter,
				Help: "Count of the number of times a consumer has been deemed too slow and is cut off.",
			}, UrlLabel,
		),
		touchstone.GaugeVec(
			prometheus.GaugeOpts{
				Name: OutgoingQueueDepth,
				Help: "The depth of the queue per outgoing url.",
			}, UrlLabel,
		),
		touchstone.CounterVec(
			prometheus.CounterOpts{
				Name: IncomingEventTypeCounter,
				Help: "Incoming count of events by event type",
			}, EventLabel,
		),
		touchstone.CounterVec(
			prometheus.CounterOpts{
				Name: DropsDueToPanic,
				Help: "The outgoing message delivery pipeline panicked.",
			}, UrlLabel,
		),
		touchstone.GaugeVec(
			prometheus.GaugeOpts{
				Name: ConsumerRenewalTimeGauge,
				Help: "Time when the consumer data was updated.",
			}, UrlLabel,
		),
		touchstone.GaugeVec(
			prometheus.GaugeOpts{
				Name: ConsumerDeliverUntilGauge,
				Help: "Time when the consumer's registration expires and events will be dropped.",
			}, UrlLabel,
		),
		touchstone.GaugeVec(
			prometheus.GaugeOpts{
				Name: ConsumerDeliveryWorkersGauge,
				Help: "The number of active delivery workers for a particular customer.",
			}, UrlLabel,
		),
		touchstone.GaugeVec(
			prometheus.GaugeOpts{
				Name: ConsumerMaxDeliveryWorkersGauge,
				Help: "The maximum number of delivery workers available for a particular customer.",
			}, UrlLabel,
		),
		touchstone.HistogramVec(
			prometheus.HistogramOpts{
				Name:    QueryDurationHistogram,
				Help:    "A histogram of latencies for queries.",
				Buckets: []float64{0.0625, 0.125, .25, .5, 1, 5, 10, 20, 40, 80, 160},
			}, UrlLabel, CodeLabel,
		),
		touchstone.HistogramVec(
			prometheus.HistogramOpts{
				Name:    IncomingQueueLatencyHistogram,
				Help:    "A histogram of latencies for the incoming queue.",
				Buckets: []float64{0.0625, 0.125, .25, .5, 1, 5, 10, 20, 40, 80, 160},
			}, EventLabel,
		),
	)
}

type Measures struct {
	fx.In
	IncomingQueueDepth              prometheus.Gauge        `name:"incoming_queue_depth"`
	ErrorRequestBodyCounter         *prometheus.Counter     `name:"error_request_body_count"`
	EmptyRequestBodyCounter         *prometheus.Counter     `name:"empty_request_body_count"`
	DropsDueToInvalidPayload        *prometheus.Counter     `name:"drops_due_to_invalid_payload"`
	ModifiedWRPCounter              *prometheus.CounterVec  `name:"modified_wrp_count"`
	DeliveryRetryCounter            *prometheus.CounterVec  `name:"delivery_retry_count"`
	DeliveryRetryMaxGauge           *prometheus.CounterVec  `name:"delivery_retry_max"`
	DeliveryCounter                 *prometheus.CounterVec  `name:"delivery_count"`
	SlowConsumerDroppedMsgCounter   *prometheus.CounterVec  `name:"slow_consumer_dropped_message_count"`
	SlowConsumerCounter             *prometheus.CounterVec  `name:"slow_consumer_cut_off_count"`
	OutgoingQueueDepth              prometheus.Gauge        `name:"outgoing_queue_depths"`
	IncomingEventTypeCounter        *prometheus.CounterVec  `name:"incoming_event_type_count"`
	DropsDueToPanic                 *prometheus.CounterVec  `name:"drops_due_to_panic"`
	ConsumerRenewalTimeGauge        prometheus.GaugeVec     `name:"consumer_renewal_time"`
	ConsumerDeliverUntilGauge       prometheus.GaugeVec     `name:"consumer_deliver_until"`
	ConsumerDeliveryWorkersGauge    prometheus.GaugeVec     `name:"consumer_delivery_workers"`
	ConsumerMaxDeliveryWorkersGauge prometheus.GaugeVec     `name:"consumer_delivery_workers_max"`
	QueryDurationHistogram          prometheus.HistogramVec `name:"query_duration_histogram_seconds"`
	IncomingQueueLatencyHistogram   prometheus.HistogramVec `name:"IncomingQueueLatencyHistogram"`
}

func NewMetricWrapperMeasures(m CaduceusMetricsRegistry) metrics.Histogram {
	return m.NewHistogram(QueryDurationHistogram, 11)
}
