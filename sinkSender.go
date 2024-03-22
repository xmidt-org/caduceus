// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/webpa-common/v2/semaphore"
	"github.com/xmidt-org/wrp-go/v3"
)

// failureText is human readable text for the failure message
const failureText = `Unfortunately, your endpoint is not able to keep up with the ` +
	`traffic being sent to it.  Due to this circumstance, all notification traffic ` +
	`is being cut off and dropped for a period of time.  Please increase your ` +
	`capacity to handle notifications, or reduce the number of notifications ` +
	`you have requested.`

// FailureMessage is a helper that lets us easily create a json struct to send
// when we have to cut and endpoint off.
type FailureMessage struct {
	Text         string   `json:"text"`
	Original     Listener `json:"webhook_registration"` //TODO: remove listener stub once ancla/argus issues fixed
	CutOffPeriod string   `json:"cut_off_period"`
	QueueSize    int      `json:"queue_size"`
	Workers      int      `json:"worker_count"`
}

type Sender interface {
	Update(Listener) error
	Shutdown(bool)
	Queue(*wrp.Message)
	SetCommonSink(CommonSink)
	SetMetrics(SinkMetrics)
	SetFailureMessage(FailureMessage)
	RetiredSince() time.Time
	Dispatcher() //TODO: not sure if dispatcher will be called by both - if not need to move dispatcher to within LegacySink creation
}

type CommonSink struct {
	id                string
	mutex             sync.RWMutex
	listener          Listener
	deliverUntil      time.Time
	dropUntil         time.Time
	failureMsg        FailureMessage
	logger            *zap.Logger
	queue             atomic.Value
	wg                sync.WaitGroup
	deliveryInterval  time.Duration
	queueSize         int
	deliveryRetries   int
	maxWorkers        int
	cutOffPeriod      time.Duration
	disablePartnerIDs bool
	customPIDs        []string
	workers           semaphore.Interface
	client            Client
	clientMiddleware  func(Client) Client
	SinkMetrics
}

// SinkMetrics are the metrics added to sink with sink specific information (i.e. id)
type SinkMetrics struct {
	deliveryCounter                  prometheus.CounterVec
	deliveryRetryCounter             *prometheus.CounterVec
	droppedQueueFullCounter          prometheus.Counter
	droppedCutoffCounter             prometheus.Counter
	droppedExpiredCounter            prometheus.Counter
	droppedExpiredBeforeQueueCounter prometheus.Counter
	droppedNetworkErrCounter         prometheus.Counter
	droppedInvalidConfig             prometheus.Counter
	droppedPanic                     prometheus.Counter
	cutOffCounter                    prometheus.Counter
	queueDepthGauge                  prometheus.Gauge
	renewalTimeGauge                 prometheus.Gauge
	deliverUntilGauge                prometheus.Gauge
	dropUntilGauge                   prometheus.Gauge
	maxWorkersGauge                  prometheus.Gauge
	currentWorkersGauge              prometheus.Gauge
	deliveryRetryMaxGauge            prometheus.Gauge
}

func NewSinkSender(sw *SinkWrapper, l Listener) (s Sender, err error) {
	if l.GetVersion() == 1 {
		s = &LegacySinkSender{}
	} else if l.GetVersion() == 2 {
		// s = &SinkSenderV2
	}

	if sw.clientMiddleware == nil {
		sw.clientMiddleware = nopClient
	}
	if sw.client == nil {
		err = errors.New("nil Client")
		return
	}

	if sw.config.CutOffPeriod.Nanoseconds() == 0 {
		err = errors.New("invalid CutOffPeriod")
		return
	}

	if sw.logger == nil {
		err = errors.New("logger required")
		return
	}

	id := l.GetId()
	cs := CommonSink{
		id:                id,
		maxWorkers:        sw.config.NumWorkersPerSender,
		deliveryRetries:   sw.config.DeliveryRetries,
		logger:            sw.logger,
		queueSize:         sw.config.QueueSizePerSender,
		cutOffPeriod:      sw.config.CutOffPeriod,
		disablePartnerIDs: sw.config.DisablePartnerIDs,
		customPIDs:        sw.config.CustomPIDs,
		deliveryInterval:  sw.config.DeliveryInterval,
		//TODO: add client middleware
	}

	CreateFailureMessage(sw.config, l, s)
	CreateSinkMetrics(sw.metrics, id, s)
	s.SetCommonSink(cs)

	//TODO: need to figure out how to set this up
	// Don't share the secret with others when there is an error.
	// sinkSender.failureMsg.Original.Webhook.Config.Secret = "XxxxxX"

	if err = s.Update(l); nil != err {
		return
	}

	go s.Dispatcher()

	return
}

func overlaps(sl1 []string, sl2 []string) bool {
	for _, s1 := range sl1 {
		for _, s2 := range sl2 {
			if s1 == s2 {
				return true
			}
		}
	}
	return false
}

func CreateFailureMessage(sc SinkConfig, l Listener, s Sender) {
	fm := FailureMessage{
		Original:     l,
		Text:         failureText,
		CutOffPeriod: sc.CutOffPeriod.String(),
		QueueSize:    sc.QueueSizePerSender,
		Workers:      sc.NumWorkersPerSender,
	}

	s.SetFailureMessage(fm)
}

func CreateSinkMetrics(m Metrics, id string, s Sender) {
	sm := SinkMetrics{
		deliveryRetryCounter:             m.DeliveryRetryCounter,
		deliveryRetryMaxGauge:            m.DeliveryRetryMaxGauge.With(prometheus.Labels{UrlLabel: id}),
		cutOffCounter:                    m.CutOffCounter.With(prometheus.Labels{UrlLabel: id}),
		droppedQueueFullCounter:          m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: "queue_full"}),
		droppedExpiredCounter:            m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: "expired"}),
		droppedExpiredBeforeQueueCounter: m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: "expired_before_queueing"}),
		droppedCutoffCounter:             m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: "cut_off"}),
		droppedInvalidConfig:             m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: "invalid_config"}),
		droppedNetworkErrCounter:         m.SlowConsumerDroppedMsgCounter.With(prometheus.Labels{UrlLabel: id, ReasonLabel: networkError}),
		droppedPanic:                     m.DropsDueToPanic.With(prometheus.Labels{UrlLabel: id}),
		queueDepthGauge:                  m.OutgoingQueueDepth.With(prometheus.Labels{UrlLabel: id}),
		renewalTimeGauge:                 m.ConsumerRenewalTimeGauge.With(prometheus.Labels{UrlLabel: id}),
		deliverUntilGauge:                m.ConsumerDeliverUntilGauge.With(prometheus.Labels{UrlLabel: id}),
		dropUntilGauge:                   m.ConsumerDropUntilGauge.With(prometheus.Labels{UrlLabel: id}),
		currentWorkersGauge:              m.ConsumerDeliveryWorkersGauge.With(prometheus.Labels{UrlLabel: id}),
		maxWorkersGauge:                  m.ConsumerMaxDeliveryWorkersGauge.With(prometheus.Labels{UrlLabel: id}),
	}

	// update queue depth and current workers gauge to make sure they start at 0
	sm.queueDepthGauge.Set(0)
	sm.currentWorkersGauge.Set(0)

	s.SetMetrics(sm)
}
