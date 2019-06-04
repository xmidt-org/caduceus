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

	"github.com/Comcast/webpa-common/webhook"
	"github.com/Comcast/wrp-go/wrp"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
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

	ContentTypeCounter metrics.Counter

	// The metrics counter for dropped messages due to invalid payloads
	DroppedMsgCounter metrics.Counter

	EventType metrics.Counter

	// The logger implementation to share with OutboundSenders.
	Logger log.Logger

	// The http client Do() function to share with OutboundSenders.
	Sender func(*http.Request) (*http.Response, error)
}

type SenderWrapper interface {
	Update([]webhook.W)
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
func (sw *CaduceusSenderWrapper) Update(list []webhook.W) {
	// We'll like need this, so let's get one ready
	osf := OutboundSenderFactory{
		Sender:           sw.sender,
		CutOffPeriod:     sw.cutOffPeriod,
		NumWorkers:       sw.numWorkersPerSender,
		QueueSize:        sw.queueSizePerSender,
		MetricsRegistry:  sw.metricsRegistry,
		DeliveryRetries:  sw.deliveryRetries,
		DeliveryInterval: sw.deliveryInterval,
		Logger:           sw.logger,
	}

	ids := make([]struct {
		Listener webhook.W
		ID       string
	}, len(list))

	for i, v := range list {
		ids[i].Listener = v
		ids[i].ID = v.ID()
	}

	sw.mutex.Lock()
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
	sw.mutex.Unlock()
}

// Queue is used to send all the possible outbound senders a request.  This
// function performs the fan-out and filtering to multiple possible endpoints.
func (sw *CaduceusSenderWrapper) Queue(msg *wrp.Message) {
	sw.mutex.RLock()

	sw.eventType.With("event", msg.FindEventStringSubMatch())

	for _, v := range sw.senders {
		v.Queue(msg)
	}
	sw.mutex.RUnlock()
}

// Shutdown closes down the delivery mechanisms and cleans up the underlying
// OutboundSenders either gently (waiting for delivery queues to empty) or not
// (dropping enqueued messages)
func (sw *CaduceusSenderWrapper) Shutdown(gentle bool) {
	sw.mutex.Lock()
	for k, v := range sw.senders {
		v.Shutdown(gentle)
		delete(sw.senders, k)
	}
	sw.mutex.Unlock()
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
			deadList := make(map[string]OutboundSender)

			// Actually shutting these down could take longer then we
			// want to lock the mutex, so just remove them from the active
			// list & shut them down afterwards.
			sw.mutex.Lock()
			for k, v := range sw.senders {
				retired := v.RetiredSince()
				if threshold.After(retired) {
					deadList[k] = v
					delete(sw.senders, k)
				}
			}
			sw.mutex.Unlock()

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
