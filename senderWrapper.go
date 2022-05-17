/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
package main

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/wrp-go/v3"
)

// SenderWrapperFactory configures the CaduceusSenderWrapper for creation
type SenderWrapperFactory struct {
	// The number of workers to assign to each OutboundSender created.
	NumWorkersPerSender int

	// The queue size to assign to each OutboundSender created.
	QueueSizePerSender int

	// The cut off time to assign to each OutboundSender created.
	CutOffPeriod time.Duration

	// Number of delivery retries before giving up
	DeliveryRetries int

	// Time in between delivery retries
	DeliveryInterval time.Duration

	// The amount of time to let expired OutboundSenders linger before
	// shutting them down and cleaning up the resources associated with them.
	Linger time.Duration

	// Metrics registry.
	MetricsRegistry CaduceusMetricsRegistry

	// The metrics counter for dropped messages due to invalid payloads
	DroppedMsgCounter metrics.Counter

	EventType metrics.Counter

	// The logger implementation to share with OutboundSenders.
	Logger log.Logger

	// The http client Do() function to share with OutboundSenders.
	Sender func(*http.Request) (*http.Response, error)

	// CustomPIDs is a custom list of allowed PartnerIDs that will be used if a message
	// has no partner IDs.
	CustomPIDs []string

	// DisablePartnerIDs dictates whether or not to enforce the partner ID check.
	DisablePartnerIDs bool
}

type SenderWrapper interface {
	Update([]ancla.InternalWebhook)
	Queue(*wrp.Message)
	Shutdown(bool)
}

// CaduceusSenderWrapper contains no external parameters.
type CaduceusSenderWrapper struct {
	sender              func(*http.Request) (*http.Response, error)
	numWorkersPerSender int
	queueSizePerSender  int
	deliveryRetries     int
	deliveryInterval    time.Duration
	cutOffPeriod        time.Duration
	linger              time.Duration
	logger              log.Logger
	mutex               sync.RWMutex
	senders             map[string]OutboundSender
	metricsRegistry     CaduceusMetricsRegistry
	eventType           metrics.Counter
	wg                  sync.WaitGroup
	shutdown            chan struct{}
	customPIDs          []string
	disablePartnerIDs   bool
}

// New produces a new SenderWrapper implemented by CaduceusSenderWrapper
// based on the factory configuration.
func (swf SenderWrapperFactory) New() (sw SenderWrapper, err error) {
	caduceusSenderWrapper := &CaduceusSenderWrapper{
		sender:              swf.Sender,
		numWorkersPerSender: swf.NumWorkersPerSender,
		queueSizePerSender:  swf.QueueSizePerSender,
		deliveryRetries:     swf.DeliveryRetries,
		deliveryInterval:    swf.DeliveryInterval,
		cutOffPeriod:        swf.CutOffPeriod,
		linger:              swf.Linger,
		logger:              swf.Logger,
		metricsRegistry:     swf.MetricsRegistry,
		customPIDs:          swf.CustomPIDs,
		disablePartnerIDs:   swf.DisablePartnerIDs,
	}

	if swf.Linger <= 0 {
		err = errors.New("Linger must be positive.")
		sw = nil
		return
	}

	caduceusSenderWrapper.eventType = swf.MetricsRegistry.NewCounter(IncomingEventTypeCounter)

	caduceusSenderWrapper.senders = make(map[string]OutboundSender)
	caduceusSenderWrapper.shutdown = make(chan struct{})

	caduceusSenderWrapper.wg.Add(1)
	go undertaker(caduceusSenderWrapper)

	sw = caduceusSenderWrapper
	return
}

// Update is called when we get changes to our webhook listeners with either
// additions, or updates.  This code takes care of building new OutboundSenders
// and maintaining the existing OutboundSenders.
func (sw *CaduceusSenderWrapper) Update(list []ancla.InternalWebhook) {
	// We'll like need this, so let's get one ready
	osf := OutboundSenderFactory{
		Sender:            sw.sender,
		CutOffPeriod:      sw.cutOffPeriod,
		NumWorkers:        sw.numWorkersPerSender,
		QueueSize:         sw.queueSizePerSender,
		MetricsRegistry:   sw.metricsRegistry,
		DeliveryRetries:   sw.deliveryRetries,
		DeliveryInterval:  sw.deliveryInterval,
		Logger:            sw.logger,
		CustomPIDs:        sw.customPIDs,
		DisablePartnerIDs: sw.disablePartnerIDs,
	}

	ids := make([]struct {
		Listener ancla.InternalWebhook
		ID       string
	}, len(list))

	for i, v := range list {
		ids[i].Listener = v
		ids[i].ID = v.Webhook.Config.URL
	}

	sw.mutex.Lock()
	defer sw.mutex.Unlock()

	for _, inValue := range ids {
		sender, ok := sw.senders[inValue.ID]
		if !ok {
			osf.Listener = inValue.Listener
			obs, err := osf.New()
			if nil == err {
				sw.senders[inValue.ID] = obs
			}
			continue
		}
		sender.Update(inValue.Listener)
	}
}

// Queue is used to send all the possible outbound senders a request.  This
// function performs the fan-out and filtering to multiple possible endpoints.
func (sw *CaduceusSenderWrapper) Queue(msg *wrp.Message) {
	sw.mutex.RLock()
	defer sw.mutex.RUnlock()

	sw.eventType.With("event", msg.FindEventStringSubMatch()).Add(1)

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
	defer sw.mutex.RUnlock()
	for k, v := range sw.senders {
		retired := v.RetiredSince()
		if threshold.After(retired) {
			deadList[k] = v
			delete(sw.senders, k)
		}
	}
	return deadList
}
