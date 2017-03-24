package main

import (
	"errors"
	"github.com/Comcast/webpa-common/logging"
	whl "github.com/Comcast/webpa-common/webhooklisteners"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SenderWrapperFactory configures the SenderWrapper for creation
type SenderWrapperFactory struct {
	// The number of workers to assign to each OutboundSender created.
	NumWorkersPerSender int

	// The queue size to assign to each OutboundSender created.
	QueueSizePerSender int

	// The cut off time to assign to each OutboundSender created.
	CutOffPeriod time.Duration

	// The amount of time to let expired OutboundSenders linger before
	// shutting them down and cleaning up the resources associated with them.
	Linger time.Duration

	// The logger implementation to share with OutboundSenders.
	Logger logging.Logger

	// The http client to share with OutboundSenders.
	Client *http.Client
}

// SenderWrapper contains no external parameters.
type SenderWrapper struct {
	client              *http.Client
	numWorkersPerSender int
	queueSizePerSender  int
	cutOffPeriod        time.Duration
	linger              time.Duration
	logger              logging.Logger
	mutex               sync.RWMutex
	senders             map[string]*OutboundSender
	wg                  sync.WaitGroup
	shutdown            chan struct{}
}

// New produces a new SenderWrapper based on the factory configuration.
func (swf SenderWrapperFactory) New() (sw *SenderWrapper, err error) {
	sw = &SenderWrapper{
		client:              swf.Client,
		numWorkersPerSender: swf.NumWorkersPerSender,
		queueSizePerSender:  swf.QueueSizePerSender,
		cutOffPeriod:        swf.CutOffPeriod,
		linger:              swf.Linger,
		logger:              swf.Logger}

	if swf.Linger <= 0 {
		err = errors.New("Linger must be positive.")
		sw = nil
		return
	}

	sw.senders = make(map[string]*OutboundSender)
	sw.shutdown = make(chan struct{})

	sw.wg.Add(1)
	go undertaker(sw)

	return
}

// Update is called when we get changes to our webhook listeners with either
// additions, or updates.  This code takes care of building new OutboundSenders
// and maintaining the existing OutboundSenders.
func (sw *SenderWrapper) Update(list []whl.WebHookListener) {
	// We'll like need this, so let's get one ready
	osf := OutboundSenderFactory{
		Client:       sw.client,
		CutOffPeriod: sw.cutOffPeriod,
		NumWorkers:   sw.numWorkersPerSender,
		QueueSize:    sw.queueSizePerSender,
		Logger:       sw.logger,
	}

	ids := make([]struct {
		Listener whl.WebHookListener
		ID       string
	}, len(list))

	for i, v := range list {
		ids[i].Listener = v
		ids[i].ID = v.ID()
	}

	sw.mutex.Lock()
	for _, inValue := range ids {
		sender, ok := sw.senders[inValue.ID]
		if true == ok {
			sender.Extend(inValue.Listener.Until)
		} else {
			osf.Listener = inValue.Listener
			obs, err := osf.New()
			if nil == err {
				sw.senders[inValue.ID] = obs
			}
		}
	}
	sw.mutex.Unlock()
}

// Queue is used to send all the possible outbound senders a request.  This
// function performs the fan-out and filtering to multiple possible endpoints.
func (sw *SenderWrapper) Queue(req CaduceusRequest) {
	switch req.ContentType {

	case "application/json":
		if url, err := url.Parse(req.TargetURL); nil == err {
			elements := strings.Split(url.Path, "/")
			if 6 < len(elements) &&
				"" == elements[0] &&
				"api" == elements[1] &&
				"v2" == elements[2] &&
				"notification" == elements[3] &&
				"device" == elements[4] &&
				// Skip the device id
				"event" == elements[6] {

				eventType := strings.SplitN(url.Path, "/event/", 2)[1]
				deviceID := elements[5]
				transID := "12345"
				sw.mutex.RLock()
				for _, v := range sw.senders {
					v.QueueJSON(req, eventType, deviceID, transID)
				}
				sw.mutex.RUnlock()
			}
		}

	case "application/wrp":
		sw.mutex.RLock()
		for _, v := range sw.senders {
			v.QueueWrp(req)
		}
		sw.mutex.RUnlock()
	}
}

// Shutdown closes down the delivery mechanisms and cleans up the underlying
// OutboundSenders either gently (waiting for delivery queues to empty) or not
// (dropping enqueued messages)
func (sw *SenderWrapper) Shutdown(gentle bool) {
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
func undertaker(sw *SenderWrapper) {
	defer sw.wg.Done()
	// Collecting unused OutboundSenders isn't a huge priority, so do it
	// slowly.
	ticker := time.NewTicker(2 * sw.linger)
	for {
		select {
		case <-ticker.C:
			threshold := time.Now().Add(-1 * sw.linger)
			deadList := make(map[string]*OutboundSender)

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
