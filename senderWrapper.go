// SPDX-FileCopyrightText: 2021 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/tls"
	"errors"
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

// SenderWrapperFactory configures the CaduceusSenderWrapper for creation
type CaduceusSenderWrapperIn struct {
	fx.In

	Tracing      candlelight.Tracing
	SenderConfig SenderConfig
	Metrics      SenderMetricsIn
	Logger       *zap.Logger
}

type CaduceusSenderWrapperOut struct {
	fx.Out
	CaduceusSenderWrapper *CaduceusSenderWrapper
}
type SenderMetricsIn struct {
	fx.In
	QueryLatency prometheus.HistogramVec `name:"query_duration_histogram_seconds"`
	EventType    prometheus.CounterVec   `name:"incoming_event_type_count"`
}
type SenderWrapper interface {
	// Update([]ancla.InternalWebhook)
	Queue(*wrp.Message)
	Shutdown(bool)
}

// CaduceusSenderWrapper contains no external parameters.
type CaduceusSenderWrapper struct {
	// The http client Do() function to share with OutboundSenders.
	sender              httpClient
	// The number of workers to assign to each OutboundSender created.
	numWorkersPerSender int

	// The queue size to assign to each OutboundSender created.
	queueSizePerSender  int

	// Number of delivery retries before giving up
	deliveryRetries     int

	// Time in between delivery retries
	deliveryInterval    time.Duration

	// The cut off time to assign to each OutboundSender created.
	cutOffPeriod        time.Duration

	// The amount of time to let expired OutboundSenders linger before
	// shutting them down and cleaning up the resources associated with them.
	linger              time.Duration

	// The logger implementation to share with OutboundSenders.
	logger              *zap.Logger

	mutex               *sync.RWMutex
	senders             map[string]OutboundSender
	eventType           prometheus.CounterVec
	queryLatency        prometheus.HistogramVec
	wg                  sync.WaitGroup
	shutdown            chan struct{}

	// CustomPIDs is a custom list of allowed PartnerIDs that will be used if a message
	// has no partner IDs.
	customPIDs          []string
	
	// DisablePartnerIDs dictates whether or not to enforce the partner ID check.
	disablePartnerIDs   bool
}

var SenderWrapperModule = fx.Module("caduceusSenderWrapper",
	fx.Provide(
		func(in CaduceusSenderWrapperIn) http.RoundTripper {
			return NewRoundTripper(in.SenderConfig, in.Tracing)
		},
	),
	fx.Provide(
		func(tr http.RoundTripper, in CaduceusSenderWrapperIn) (CaduceusSenderWrapperOut, error) {
			csw, err := NewSenderWrapper(tr, in)
			return CaduceusSenderWrapperOut{
				CaduceusSenderWrapper: csw,
			}, err
		},
	),
)

// New produces a new CaduceusSenderWrapper
// based on the SenderConfig
func NewSenderWrapper(tr http.RoundTripper, in CaduceusSenderWrapperIn) (csw *CaduceusSenderWrapper, err error) {
	csw = &CaduceusSenderWrapper{
		numWorkersPerSender: in.SenderConfig.NumWorkersPerSender,
		queueSizePerSender:  in.SenderConfig.QueueSizePerSender,
		deliveryRetries:     in.SenderConfig.DeliveryRetries,
		deliveryInterval:    in.SenderConfig.DeliveryInterval,
		cutOffPeriod:        in.SenderConfig.CutOffPeriod,
		linger:              in.SenderConfig.Linger,
		logger:              in.Logger,
		customPIDs:          in.SenderConfig.CustomPIDs,
		disablePartnerIDs:   in.SenderConfig.DisablePartnerIDs,
		eventType:           in.Metrics.EventType,
		queryLatency:        in.Metrics.QueryLatency,
	}
	csw.sender = doerFunc((&http.Client{
		Transport: tr,
		Timeout:   in.SenderConfig.ClientTimeout,
	}).Do)

	if in.SenderConfig.Linger <= 0 {
		err = errors.New("Linger must be positive.")
		csw = nil
		return
	}

	csw.senders = make(map[string]OutboundSender)
	csw.shutdown = make(chan struct{})

	csw.wg.Add(1)
	go undertaker(csw)

	return
}

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

//Commenting out while until ancla/argus dependency issue is fixed.
// Update is called when we get changes to our webhook listeners with either
// additions, or updates.  This code takes care of building new OutboundSenders
// and maintaining the existing OutboundSenders.
// func (sw *CaduceusSenderWrapper) Update(list []ancla.InternalWebhook) {
// 	// We'll like need this, so let's get one ready
// 	osf := OutboundSenderFactory{
// 		Sender:            sw.sender,
// 		CutOffPeriod:      sw.cutOffPeriod,
// 		NumWorkers:        sw.numWorkersPerSender,
// 		QueueSize:         sw.queueSizePerSender,
// 		MetricsRegistry:   sw.metricsRegistry,
// 		DeliveryRetries:   sw.deliveryRetries,
// 		DeliveryInterval:  sw.deliveryInterval,
// 		Logger:            sw.logger,
// 		CustomPIDs:        sw.customPIDs,
// 		DisablePartnerIDs: sw.disablePartnerIDs,
// 		QueryLatency:      sw.queryLatency,
// 	}

// 	ids := make([]struct {
// 		Listener ancla.InternalWebhook
// 		ID       string
// 	}, len(list))

// 	for i, v := range list {
// 		ids[i].Listener = v
// 		ids[i].ID = v.Webhook.Config.URL
// 	}

// 	sw.mutex.Lock()
// 	defer sw.mutex.Unlock()

// 	for _, inValue := range ids {
// 		sender, ok := sw.senders[inValue.ID]
// 		if !ok {
// 			osf.Listener = inValue.Listener
// 			metricWrapper, err := newMetricWrapper(time.Now, osf.QueryLatency.With("url", inValue.ID))

// 			if err != nil {
// 				continue
// 			}
// 			osf.ClientMiddleware = metricWrapper.roundTripper
// 			obs, err := osf.New()
// 			if nil == err {
// 				sw.senders[inValue.ID] = obs
// 			}
// 			continue
// 		}
// 		sender.Update(inValue.Listener)
// 	}
// }

// Queue is used to send all the possible outbound senders a request.  This
// function performs the fan-out and filtering to multiple possible endpoints.
func (sw *CaduceusSenderWrapper) Queue(msg *wrp.Message) {
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
func (sw *CaduceusSenderWrapper) Shutdown(gentle bool) {
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
func undertaker(sw *CaduceusSenderWrapper) {
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

func createDeadlist(sw *CaduceusSenderWrapper, threshold time.Time) map[string]OutboundSender {
	if sw == nil || threshold.IsZero() {
		return nil
	}

	deadList := make(map[string]OutboundSender)
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
