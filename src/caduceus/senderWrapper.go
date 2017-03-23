package main

import (
	"fmt"
	"github.com/Comcast/webpa-common/logging"
	"net/http"
	"sync"
	"time"
)

// WebpaWebHookListener is the structure that represents the Webhook listener
// data we share.
type WebpaWebHookListener struct {
	URL         string
	ContentType string
	Secret      string
	Events      []string
	Matchers    map[string][]string
	Duration    time.Duration
	Until       time.Time
	Address     string
}

// ID creates the canonical string identifing a WebpaWebhookListener
func (w *WebpaWebHookListener) ID() string {

	events := ""
	comma := ""
	for _, v := range w.Events {
		events += comma + v
		comma = ","
	}

	matcher := ""
	if 0 < len(w.Matchers) {
		comma = ""
		for k, mVal := range w.Matchers {
			for _, v := range mVal {
				matcher += comma + k + "-" + v
				comma = ","
			}
		}
	} else {
		matcher = "none"
	}

	return fmt.Sprintf("%s|%s|%s|%s", w.URL, w.Secret, events, matcher)
}

// SenderWrapperFactory configures the SenderWrapper for creation
type SenderWrapperFactory struct {
	// The number of workers to assign to each OutboundSender created.
	NumWorkersPerSender int

	// The queue size to assign to each OutboundSender created.
	QueueSizePerSender int

	// The cut off time to assign to each OutboundSender created.
	CutOffPeriod time.Duration

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
	logger              logging.Logger
	mutex               sync.RWMutex
	senders             map[string]*OutboundSender
}

// New produces a new SenderWrapper based on the factory configuration.
func (swf SenderWrapperFactory) New() (sw *SenderWrapper, err error) {
	sw = &SenderWrapper{
		client:              swf.Client,
		numWorkersPerSender: swf.NumWorkersPerSender,
		queueSizePerSender:  swf.QueueSizePerSender,
		cutOffPeriod:        swf.CutOffPeriod,
		logger:              swf.Logger}

	return
}

// Update is called when we get changes to our webhook listeners with either
// additions, or updates.  This code takes care of building new OutboundSenders
// and maintaining the existing OutboundSenders.
func (sw *SenderWrapper) Update(list []WebpaWebHookListener) {
	// We'll like need this, so let's get one ready
	osf := OutboundSenderFactory{
		Client:       sw.client,
		CutOffPeriod: sw.cutOffPeriod,
		NumWorkers:   sw.numWorkersPerSender,
		QueueSize:    sw.queueSizePerSender,
		Logger:       sw.logger,
	}

	ids := make([]struct {
		Listener WebpaWebHookListener
		ID       string
	}, len(list))

	for i, v := range list {
		ids[i].Listener = v
		ids[i].ID = v.ID()
	}

	now := time.Now()
	sw.mutex.Lock()
	for _, inValue := range ids {
		sender, ok := sw.senders[inValue.ID]
		if true == ok {
			sender.Extend(now.Add(inValue.Listener.Duration))
		} else {
			osf.URL = inValue.Listener.URL
			osf.ContentType = inValue.Listener.ContentType
			osf.Secret = inValue.Listener.Secret
			osf.Events = inValue.Listener.Events
			osf.Matchers = inValue.Listener.Matchers
			osf.Until = now.Add(inValue.Listener.Duration)
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
		sw.mutex.RLock()
		eventType := "foo"
		deviceID := "mac:123"
		transID := "12345"
		for _, v := range sw.senders {
			v.QueueJSON(req, eventType, deviceID, transID)
		}
		sw.mutex.RUnlock()

	case "application/wrp":
		sw.mutex.RLock()
		for _, v := range sw.senders {
			v.QueueWrp(req)
		}
		sw.mutex.RUnlock()
	}
}
