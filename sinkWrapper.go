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

	Tracing        candlelight.Tracing
	SenderConfig   SenderConfig
	WrapperMetrics SinkWrapperMetrics
	SenderMetrics  SinkSenderMetrics
	Logger         *zap.Logger
}

type SinkWrapperMetrics struct {
	QueryLatency prometheus.ObserverVec
	EventType    *prometheus.CounterVec
}

// SinkWrapper interface is needed for unit testing.
type Wrapper interface {
	// Update([]ancla.InternalWebhook)
	Queue(*wrp.Message)
	Shutdown(bool)
}

// Wrapper contains the configuration that will be shared with each outbound sender. It contains no external parameters.
type SinkWrapper struct {
	// The amount of time to let expired OutboundSenders linger before
	// shutting them down and cleaning up the resources associated with them.
	linger time.Duration

	// The logger implementation to share with OutboundSenders.
	logger *zap.Logger

	mutex            *sync.RWMutex
	senders          map[string]Sender
	eventType        *prometheus.CounterVec
	queryLatency     prometheus.ObserverVec
	wg               sync.WaitGroup
	shutdown         chan struct{}
	config           SenderConfig
	metrics          SinkSenderMetrics
	client           Client              //should this be a part of wrapper or sender?
	listener         ListenerStub        //should this be a part of wrapper or sender?
	clientMiddleware func(Client) Client //should this be a part of wrapper or sender?
}

func ProvideWrapper() fx.Option {
	return fx.Provide(
		func(in SinkWrapperIn) (*SinkWrapper, error) {
			csw, err := NewSinkWrapper(in)
			return csw, err
		},
	)
}

func NewSinkWrapper(in SinkWrapperIn) (sw *SinkWrapper, err error) {
	sw = &SinkWrapper{
		linger:       in.SenderConfig.Linger,
		logger:       in.Logger,
		eventType:    in.WrapperMetrics.EventType,
		queryLatency: in.WrapperMetrics.QueryLatency,
		config:       in.SenderConfig,
		metrics:      in.SenderMetrics,
	}

	if in.SenderConfig.Linger <= 0 {
		linger := fmt.Sprintf("linger not positive: %v", in.SenderConfig.Linger)
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
func NewRoundTripper(config SenderConfig, tracing candlelight.Tracing) (tr http.RoundTripper) {
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
func (sw *SinkWrapper) Update(list []ListenerStub) {

	ids := make([]struct {
		Listener ListenerStub
		ID       string
	}, len(list))

	for i, v := range list {
		ids[i].Listener = v
		ids[i].ID = v.Webhook.Config.ReceiverURL
	}

	sw.mutex.Lock()
	defer sw.mutex.Unlock()

	for _, inValue := range ids {
		sender, ok := sw.senders[inValue.ID]
		if !ok {
			// osf.Sender = sw.sender
			sw.listener = inValue.Listener
			metricWrapper, err := newMetricWrapper(time.Now, sw.queryLatency, inValue.ID)

			if err != nil {
				continue
			}
			sw.clientMiddleware = metricWrapper.roundTripper
			ss, err := newSinkSender(sw)
			if nil == err {
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
