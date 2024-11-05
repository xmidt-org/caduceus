// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package sink

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/caduceus/internal/client"
	"github.com/xmidt-org/caduceus/internal/metrics"

	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/wrp-go/v3"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// WrapperIn configures the Wrapper for creation
type WrapperIn struct {
	fx.In

	Tracing candlelight.Tracing
	Config  Config
	// Metrics   metrics.MetricsIn
	EventType *prometheus.CounterVec `name:"incoming_event_type_count"`
	Logger    *zap.Logger
	metrics.Metrics
}

// SinkWrapper interface is needed for unit testing.
type Wrapper interface {
	Update([]ancla.Register)
	Queue(*wrp.Message)
	Shutdown(bool)
}

// Wrapper contains the configuration that will be shared with each outbound sender. It contains no external parameters.
type wrapper struct {
	// The amount of time to let expired SinkSenders linger before
	// shutting them down and cleaning up the resources associated with them.
	linger time.Duration

	// The logger implementation to share with sinkSenders.
	logger *zap.Logger

	//the configuration needed for eash sinkSender
	config Config

	mutex            sync.RWMutex
	senders          map[string]*Sender
	eventType        *prometheus.CounterVec
	wg               sync.WaitGroup
	shutdown         chan struct{}
	metrics          metrics.Metrics
	client           client.Client                     //TODO: keeping here for now - but might move to SinkSender in a later PR
	clientMiddleware func(client.Client) client.Client //TODO: keeping here for now - but might move to SinkSender in a later PR

}

func Provide() fx.Option {
	return fx.Provide(
		func(in WrapperIn) (Wrapper, error) {
			w, err := NewWrapper(in)
			return w, err
		},
	)
}

func NewWrapper(in WrapperIn) (wr Wrapper, err error) {
	w := &wrapper{
		linger:    in.Config.Linger,
		logger:    in.Logger,
		eventType: in.EventType,
		config:    in.Config,
		metrics:   in.Metrics,
	}

	tr := newRoundTripper(in.Config, in.Tracing)
	w.client = client.DoerFunc((&http.Client{
		Transport: tr,
		Timeout:   in.Config.ClientTimeout,
	}).Do)

	if in.Config.Linger <= 0 {
		linger := fmt.Sprintf("linger not positive: %v", in.Config.Linger)
		err = errors.New(linger)
		w = nil
		return
	}
	w.senders = make(map[string]*Sender)
	w.shutdown = make(chan struct{})

	w.wg.Add(1)
	go undertaker(w)
	wr = w

	return
}

// no longer being initialized at start up - needs to be initialized by the creation of the outbound sender
func newRoundTripper(config Config, tracing candlelight.Tracing) (tr http.RoundTripper) {
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

// Update is called when we get changes to our webhook listeners with either
// additions, or updates.  This code takes care of building new OutboundSenders
// and maintaining the existing OutboundSenders.
func (w *wrapper) Update(list []ancla.Register) {

	ids := make([]struct {
		Listener ancla.Register
		ID       string
	}, len(list))

	for i, v := range list {
		ids[i].Listener = v
		ids[i].ID = v.GetId()
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	for _, inValue := range ids {
		sender, ok := w.senders[inValue.ID]
		if !ok {
			var ss *Sender
			var err error

			listener := inValue.Listener
			metricWrapper, err := client.NewMetricWrapper(time.Now, w.metrics.QueryLatency, inValue.ID)

			if err != nil {
				continue
			}

			ss, err = NewSender(w, listener)
			w.clientMiddleware = metricWrapper.RoundTripper

			if err == nil {
				w.senders[inValue.ID] = ss
			}

			continue
		}
		sender.Update(inValue.Listener) //commenting out until argus/ancla fix
	}
}

// Queue is used to send all the possible outbound senders a request.  This
// function performs the fan-out and filtering to multiple possible endpoints.
func (w *wrapper) Queue(msg *wrp.Message) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	w.eventType.With(prometheus.Labels{metrics.EventLabel: msg.FindEventStringSubMatch()}).Add(1)

	for _, v := range w.senders {
		v.Queue(msg)
	}
}

// Shutdown closes down the delivery mechanisms and cleans up the underlying
// OutboundSenders either gently (waiting for delivery queues to empty) or not
// (dropping enqueued messages)
func (w *wrapper) Shutdown(gentle bool) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	for k, v := range w.senders {
		v.Shutdown(gentle)
		delete(w.senders, k)
	}
	close(w.shutdown)
}

// undertaker looks at the OutboundSenders periodically and prunes the ones
// that have been retired for too long, freeing up resources.
func undertaker(w *wrapper) {
	defer w.wg.Done()
	// Collecting unused OutboundSenders isn't a huge priority, so do it
	// slowly.
	ticker := time.NewTicker(2 * w.linger)
	for {
		select {
		case <-ticker.C:
			threshold := time.Now().Add(-1 * w.linger)

			// Actually shutting these down could take longer then we
			// want to lock the mutex, so just remove them from the active
			// list & shut them down afterwards.
			deadList, err := createDeadlist(w, threshold)
			if err != nil {
				break
			}

			// Shut them down
			for _, v := range deadList {
				v.Shutdown(false)
			}
		case <-w.shutdown:
			ticker.Stop()
			return
		}
	}
}

func createDeadlist(w *wrapper, threshold time.Time) (map[string]*Sender, error) {
	if w == nil || threshold.IsZero() {
		return nil, nil
	}

	deadList := make(map[string]*Sender)
	w.mutex.Lock()
	defer w.mutex.Unlock()
	for k, v := range w.senders {
		retired, err := v.RetiredSince()
		if err != nil {
			return nil, fmt.Errorf("failed to get retirement time for sender %s: %w", k, err)
		}
		if threshold.After(retired) {
			deadList[k] = v
			delete(w.senders, k)
		}
	}
	return deadList, nil
}
