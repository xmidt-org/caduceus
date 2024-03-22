// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/wrp-go/v3"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// WrapperIn configures the Wrapper for creation
type SinkWrapperIn struct {
	fx.In

	Tracing    candlelight.Tracing
	SinkConfig SinkConfig
	Metrics    Metrics
	EventType  *prometheus.CounterVec
	Logger     *zap.Logger
}

// SinkWrapper interface is needed for unit testing.
type Wrapper interface {
	// Update([]ancla.InternalWebhook)
	Queue(*wrp.Message)
	Shutdown(bool)
}

// Wrapper contains the configuration that will be shared with each outbound sender. It contains no external parameters.
type SinkWrapper struct {
	// The amount of time to let expired SinkSenders linger before
	// shutting them down and cleaning up the resources associated with them.
	linger time.Duration

	// The logger implementation to share with sinkSenders.
	logger *zap.Logger

	//the configuration needed for eash sinkSender
	config SinkConfig

	mutex            *sync.RWMutex
	senders          map[string]Sender
	eventType        *prometheus.CounterVec
	wg               sync.WaitGroup
	shutdown         chan struct{}
	metrics          Metrics
	client           Client              //TODO: keeping here for now - but might move to SinkSender in a later PR
	clientMiddleware func(Client) Client //TODO: keeping here for now - but might move to SinkSender in a later PR

}

func ProvideWrapper() fx.Option {
	return fx.Provide(
		func(in MetricsIn) Metrics {
			senderMetrics := Metrics{
				DeliveryCounter:                 in.DeliveryCounter,
				DeliveryRetryCounter:            in.DeliveryRetryCounter,
				DeliveryRetryMaxGauge:           in.DeliveryRetryMaxGauge,
				CutOffCounter:                   in.CutOffCounter,
				SlowConsumerDroppedMsgCounter:   in.SlowConsumerDroppedMsgCounter,
				DropsDueToPanic:                 in.DropsDueToPanic,
				ConsumerDeliverUntilGauge:       in.ConsumerDeliverUntilGauge,
				ConsumerDropUntilGauge:          in.ConsumerDropUntilGauge,
				ConsumerDeliveryWorkersGauge:    in.ConsumerDeliveryWorkersGauge,
				ConsumerMaxDeliveryWorkersGauge: in.ConsumerMaxDeliveryWorkersGauge,
				OutgoingQueueDepth:              in.OutgoingQueueDepth,
				ConsumerRenewalTimeGauge:        in.ConsumerRenewalTimeGauge,
				QueryLatency:                    in.QueryLatency,
			}
			return senderMetrics
		},
		func(in SinkWrapperIn) (*SinkWrapper, error) {
			csw, err := NewSinkWrapper(in)
			return csw, err
		},
	)
}

func NewSinkWrapper(in SinkWrapperIn) (sw *SinkWrapper, err error) {
	sw = &SinkWrapper{
		linger:    in.SinkConfig.Linger,
		logger:    in.Logger,
		eventType: in.EventType,
		config:    in.SinkConfig,
		metrics:   in.Metrics,
	}

	if in.SinkConfig.Linger <= 0 {
		linger := fmt.Sprintf("linger not positive: %v", in.SinkConfig.Linger)
		err = errors.New(linger)
		sw = nil
		return
	}
	sw.senders = make(map[string]Sender)
	sw.shutdown = make(chan struct{})

	sw.wg.Add(1)
	go undertaker(sw)

	return
}

// no longer being initialized at start up - needs to be initialized by the creation of the outbound sender
func NewRoundTripper(config SinkConfig, tracing candlelight.Tracing) (tr http.RoundTripper) {
	tr = &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: config.DisableClientHostnameValidation},
		MaxIdleConnsPerHost:   config.NumWorkersPerSender,
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
		IdleConnTimeout:       config.IdleConnTimeout,
	}

	tr = otelhttp.NewTransport(tr,
		otelhttp.WithPropagators(tracing.Propagator()),
		otelhttp.WithTracerProvider(tracing.TracerProvider()),
	)
	return
}

// Commenting out while until ancla/argus dependency issue is fixed.
// Update is called when we get changes to our webhook listeners with either
// additions, or updates.  This code takes care of building new OutboundSenders
// and maintaining the existing OutboundSenders.
func (sw *SinkWrapper) Update(list []Listener) {

	ids := make([]struct {
		Listener Listener
		ID       string
	}, len(list))

	for i, v := range list {
		ids[i].Listener = v
		ids[i].ID = v.GetId()
	}

	sw.mutex.Lock()
	defer sw.mutex.Unlock()

	for _, inValue := range ids {
		sender, ok := sw.senders[inValue.ID]
		if !ok {
			var ss Sender
			var err error

			listener := inValue.Listener
			metricWrapper, err := newMetricWrapper(time.Now, sw.metrics.QueryLatency, inValue.ID)

			if err != nil {
				continue
			}

			ss, err = NewSinkSender(sw, listener)
			sw.clientMiddleware = metricWrapper.roundTripper

			// {
			// 	ss, err = newSinkSender(sw, r1)
			// }

			if err == nil {
				sw.senders[inValue.ID] = ss
			}
			continue
		}
		fmt.Println(sender)
		// sender.Update(inValue.Listener) //commenting out until argus/ancla fix
	}
}

// Queue is used to send all the possible outbound senders a request.  This
// function performs the fan-out and filtering to multiple possible endpoints.
func (sw *SinkWrapper) Queue(msg *wrp.Message) {
	sw.mutex.RLock()
	defer sw.mutex.RUnlock()

	sw.eventType.With(prometheus.Labels{"event": msg.FindEventStringSubMatch()}).Add(1)

	for _, v := range sw.senders {
		v.Queue(msg)
	}
}

// Shutdown closes down the delivery mechanisms and cleans up the underlying
// OutboundSenders either gently (waiting for delivery queues to empty) or not
// (dropping enqueued messages)
func (sw *SinkWrapper) Shutdown(gentle bool) {
	sw.mutex.Lock()
	defer sw.mutex.Unlock()
	for k, v := range sw.senders {
		v.Shutdown(gentle)
		delete(sw.senders, k)
	}
	close(sw.shutdown)
}

// undertaker looks at the OutboundSenders periodically and prunes the ones
// that have been retired for too long, freeing up resources.
func undertaker(sw *SinkWrapper) {
	defer sw.wg.Done()
	// Collecting unused OutboundSenders isn't a huge priority, so do it
	// slowly.
	ticker := time.NewTicker(2 * sw.linger)
	for {
		select {
		case <-ticker.C:
			threshold := time.Now().Add(-1 * sw.linger)

			// Actually shutting these down could take longer then we
			// want to lock the mutex, so just remove them from the active
			// list & shut them down afterwards.
			deadList := createDeadlist(sw, threshold)

			// Shut them down
			for _, v := range deadList {
				v.Shutdown(false)
			}
		case <-sw.shutdown:
			ticker.Stop()
			return
		}
	}
}

func createDeadlist(sw *SinkWrapper, threshold time.Time) map[string]Sender {
	if sw == nil || threshold.IsZero() {
		return nil
	}

	deadList := make(map[string]Sender)
	sw.mutex.Lock()
	defer sw.mutex.Unlock()
	for k, v := range sw.senders {
		retired := v.RetiredSince()
		if threshold.After(retired) {
			deadList[k] = v
			delete(sw.senders, k)
		}
	}
	return deadList
}
