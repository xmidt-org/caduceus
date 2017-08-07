package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/webhook"
	"hash"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"
)

// failureText is human readable text for the failure message
const failureText = `Unfortunately, your endpoint is not able to keep up with the ` +
	`traffic being sent to it.  Due to this circumstance, all notification traffic ` +
	`is being cut off and dropped for a period of time.  Please increase your ` +
	`capacity to handle notifications, or reduce the number of notifications ` +
	`you have requested.`

// outboundRequest stores the outgoing request and assorted data that has been
// collected so far (to save on processing the information again).
type outboundRequest struct {
	req         CaduceusRequest
	event       string
	transID     string
	deviceID    string
	contentType string
}

// FailureMessage is a helper that lets us easily create a json struct to send
// when we have to cut and endpoint off.
type FailureMessage struct {
	Text         string    `json:"text"`
	Original     webhook.W `json:"webhook_registration"`
	CutOffPeriod string    `json:"cut_off_period"`
	QueueSize    int       `json:"queue_size"`
	Workers      int       `json:"worker_count"`
}

// OutboundSenderFactory is a configurable factory for OutboundSender objects.
type OutboundSenderFactory struct {
	// The WebHookListener to service
	Listener webhook.W

	// The http client to use for requests.
	Client *http.Client

	// The number of delivery workers to create and use.
	NumWorkers int

	// The queue depth to buffer events before we declare overflow, shut
	// off the message delivery, and basically put the endpoint in "timeout."
	QueueSize int

	// The amount of time to cut off the consumer if they don't keep up.
	// Must be greater then 0 seconds
	CutOffPeriod time.Duration

	// The factory that we'll use to make new ServerProfilers on a per
	// outboundSender basis
	ProfilerFactory ServerProfilerFactory

	// The logger to use.
	Logger logging.Logger
}

type OutboundSender interface {
	Extend(time.Time)
	Shutdown(bool)
	RetiredSince() time.Time
	QueueJSON(CaduceusRequest, string, string, string)
	QueueWrp(CaduceusRequest, map[string]string, string, string, string)
}

// CaduceusOutboundSender is the outbound sender object.
type CaduceusOutboundSender struct {
	listener     webhook.W
	deliverUntil time.Time
	dropUntil    time.Time
	client       *http.Client
	secret       []byte
	events       []*regexp.Regexp
	matcher      []*regexp.Regexp
	queueSize    int
	queue        chan outboundRequest
	profiler     ServerProfiler
	wg           sync.WaitGroup
	cutOffPeriod time.Duration
	failureMsg   FailureMessage
	logger       logging.Logger
	mutex        sync.RWMutex
}

// New creates a new OutboundSender object from the factory, or returns an error.
func (osf OutboundSenderFactory) New() (obs OutboundSender, err error) {
	if _, err = url.ParseRequestURI(osf.Listener.Config.URL); nil != err {
		return
	}

	if nil == osf.Client {
		err = errors.New("nil http.Client")
		return
	}

	if 0 == osf.CutOffPeriod.Nanoseconds() {
		err = errors.New("Invalid CutOffPeriod")
		return
	}

	if nil == osf.Logger {
		err = errors.New("Logger required")
		return
	}

	caduceusOutboundSender := &CaduceusOutboundSender{
		listener:     osf.Listener,
		client:       osf.Client,
		queueSize:    osf.QueueSize,
		cutOffPeriod: osf.CutOffPeriod,
		deliverUntil: osf.Listener.Until,
		logger:       osf.Logger,
		failureMsg: FailureMessage{
			Original:     osf.Listener,
			Text:         failureText,
			CutOffPeriod: osf.CutOffPeriod.String(),
			QueueSize:    osf.QueueSize,
			Workers:      osf.NumWorkers,
		},
	}

	// Don't share the secret with others when there is an error.
	caduceusOutboundSender.failureMsg.Original.Config.Secret = "XxxxxX"

	if "" != osf.Listener.Config.Secret {
		caduceusOutboundSender.secret = []byte(osf.Listener.Config.Secret)
	}

	if "" != osf.Listener.FailureURL {
		if _, err = url.ParseRequestURI(osf.Listener.FailureURL); nil != err {
			return
		}
	}

	caduceusOutboundSender.profiler, err = osf.ProfilerFactory.New(osf.Listener.Config.URL)
	if err != nil {
		return
	}

	// Give us some head room so that we don't block when we get near the
	// completely full point.
	caduceusOutboundSender.queue = make(chan outboundRequest, osf.QueueSize)

	// Create the event regex objects
	for _, event := range osf.Listener.Events {
		var re *regexp.Regexp
		if re, err = regexp.Compile(event); nil != err {
			return
		}

		caduceusOutboundSender.events = append(caduceusOutboundSender.events, re)
	}
	if nil == caduceusOutboundSender.events {
		err = errors.New("Events must not be empty.")
		return
	}

	// Create the matcher regex objects
	for _, item := range osf.Listener.Matcher.DeviceId {
		if ".*" == item {
			// Match everything - skip the filtering
			caduceusOutboundSender.matcher = nil
			break
		}

		var re *regexp.Regexp
		if re, err = regexp.Compile(item); nil != err {
			err = fmt.Errorf("Invalid matcher item: '%s'", item)
			return
		}
		caduceusOutboundSender.matcher = append(caduceusOutboundSender.matcher, re)
	}

	caduceusOutboundSender.wg.Add(osf.NumWorkers)
	for i := 0; i < osf.NumWorkers; i++ {
		go caduceusOutboundSender.worker(i)
	}

	obs = caduceusOutboundSender
	return
}

// Extend extends the time the CaduceusOutboundSender will deliver messages to the
// specified time.  The new delivery cutoff time must be after the previously
// set delivery cutoff time.
func (obs *CaduceusOutboundSender) Extend(until time.Time) {
	obs.mutex.Lock()
	if until.After(obs.deliverUntil) {
		obs.deliverUntil = until
	}
	obs.mutex.Unlock()
}

// Shutdown causes the CaduceusOutboundSender to stop it's activities either gently or
// abruptly based on the gentle parameter.  If gentle is false, all queued
// messages will be dropped without an attempt to send made.
func (obs *CaduceusOutboundSender) Shutdown(gentle bool) {
	close(obs.queue)
	obs.mutex.Lock()
	if false == gentle {
		obs.deliverUntil = time.Time{}
	}
	obs.mutex.Unlock()
	obs.wg.Wait()

	obs.mutex.Lock()
	obs.deliverUntil = time.Time{}
	obs.mutex.Unlock()
}

// RetiredSince returns the time the CaduceusOutboundSender retired (which could be in
// the future).
func (obs *CaduceusOutboundSender) RetiredSince() time.Time {
	obs.mutex.RLock()
	deliverUntil := obs.deliverUntil
	obs.mutex.RUnlock()

	return deliverUntil
}

// QueueJSON is given a request to evaluate and optionally enqueue in the list
// of messages to deliver.  The request is checked to see if it matches the
// criteria before being accepted or silently dropped.
func (obs *CaduceusOutboundSender) QueueJSON(req CaduceusRequest,
	eventType, deviceID, transID string) {

	obs.mutex.RLock()
	deliverUntil := obs.deliverUntil
	dropUntil := obs.dropUntil
	obs.mutex.RUnlock()

	now := time.Now()

	if now.Before(deliverUntil) && now.After(dropUntil) {
		for _, eventRegex := range obs.events {
			if eventRegex.MatchString(eventType) {
				matchDevice := (nil == obs.matcher)
				if nil != obs.matcher {
					for _, deviceRegex := range obs.matcher {
						if deviceRegex.MatchString(deviceID) {
							matchDevice = true
							break
						}
					}
				}
				if matchDevice {
					if len(obs.queue) < obs.queueSize {
						outboundReq := outboundRequest{
							req:         req,
							event:       eventType,
							transID:     transID,
							deviceID:    deviceID,
							contentType: "application/json",
						}
						outboundReq.req.Telemetry.TimeOutboundAccepted = time.Now()
						obs.queue <- outboundReq
					} else {
						obs.queueOverflow()
					}
				}
			}
		}
	} else {
		// Report drop
	}
}

// QueueWrp is given a request to evaluate and optionally enqueue in the list
// of messages to deliver.  The request is checked to see if it matches the
// criteria before being accepted or silently dropped.
func (obs *CaduceusOutboundSender) QueueWrp(req CaduceusRequest, metaData map[string]string,
	eventType, deviceID, transID string) {
	obs.mutex.RLock()
	deliverUntil := obs.deliverUntil
	dropUntil := obs.dropUntil
	obs.mutex.RUnlock()

	now := time.Now()

	if now.Before(deliverUntil) && now.After(dropUntil) {
		for _, eventRegex := range obs.events {
			if eventRegex.MatchString(eventType) {
				matchDevice := (nil == obs.matcher)
				if nil != obs.matcher {
					for _, deviceRegex := range obs.matcher {
						if deviceRegex.MatchString(deviceID) {
							matchDevice = true
							break
						}
					}
				}
				/*
					// if the device id matches then we want to look through all the metadata
					// and make sure that the obs metadata matches the metadata provided
					if matchDevice {
						for key, val := range metaData {
							if matchers, ok := obs.matcher[key]; ok {
								for _, deviceRegex := range matchers {
									matchDevice = false
									if deviceRegex.MatchString(val) {
										matchDevice = true
										break
									}
								}

								// metadata was provided but did not match our expectations,
								// so it is time to drop the message
								if !matchDevice {
									break
								}
							}
						}
					}
				*/
				if matchDevice {
					if len(obs.queue) < obs.queueSize {
						outboundReq := outboundRequest{req: req,
							event:       eventType,
							transID:     transID,
							deviceID:    deviceID,
							contentType: "application/wrp",
						}
						outboundReq.req.Telemetry.TimeOutboundAccepted = time.Now()
						obs.queue <- outboundReq
					} else {
						obs.queueOverflow()
					}
				}
			}
		}
	} else {
		// Report drop
	}
}

// worker is the routine that actually takes the queued messages and delivers
// them to the listeners outside webpa
func (obs *CaduceusOutboundSender) worker(id int) {
	defer obs.wg.Done()

	// Make a local copy of the hmac
	var h hash.Hash

	// Create the base sha1 hash object for each thread
	if nil != obs.secret {
		h = hmac.New(sha1.New, obs.secret)
	}

	for work := range obs.queue {
		obs.mutex.RLock()
		deliverUntil := obs.deliverUntil
		dropUntil := obs.dropUntil
		obs.mutex.RUnlock()

		now := time.Now()
		if now.Before(deliverUntil) && now.After(dropUntil) {
			payload := bytes.NewReader(work.req.Payload)
			req, err := http.NewRequest("POST", obs.listener.Config.URL, payload)
			if nil != err {
				// Report drop
				obs.logger.Error("http.NewRequest(\"POST\", '%s', payload) failed: %s", obs.listener.Config.URL, err)
			} else {
				req.Header.Set("Content-Type", work.contentType)
				req.Header.Set("X-Webpa-Event", work.event)
				req.Header.Set("X-Webpa-Transaction-Id", work.transID)
				req.Header.Set("X-Webpa-Device-Id", work.deviceID)

				if nil != h {
					h.Reset()
					h.Write(work.req.Payload)
					sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
					req.Header.Set("X-Webpa-Signature", sig)
				}

				// Send it
				work.req.Telemetry.TimeSent = time.Now()
				resp, err := obs.client.Do(req)
				work.req.Telemetry.TimeResponded = time.Now()
				if nil != err {
					// Report failure
				} else {
					if (200 <= resp.StatusCode) && (resp.StatusCode <= 204) {
						// Report success
						obs.profiler.Send(work.req.Telemetry)
					} else {
						// Report partial success
						obs.profiler.Send(work.req.Telemetry)
					}
				}
			}
		}
	}
}

// queueOverflow handles the logic of what to do when a queue overflows
func (obs *CaduceusOutboundSender) queueOverflow() {
	obs.mutex.Lock()
	obs.dropUntil = time.Now().Add(obs.cutOffPeriod)
	obs.mutex.Unlock()

	msg, err := json.Marshal(obs.failureMsg)
	if nil != err {
		obs.logger.Error("Cut-off notification json.Marshal( %v ) failed for %s, err: %s", obs.failureMsg, obs.listener.Config.URL, err)
	} else {
		obs.logger.Error("Cut-off notification for %s ( %s )", obs.listener.Config.URL, msg)

		// Send a "you've been cut off" warning message
		if "" != obs.listener.FailureURL {

			payload := bytes.NewReader(msg)
			req, err := http.NewRequest("POST", obs.listener.FailureURL, payload)
			req.Header.Set("Content-Type", "application/json")

			if nil != obs.secret {
				h := hmac.New(sha1.New, obs.secret)
				h.Write(msg)
				sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
				req.Header.Set("X-Webpa-Signature", sig)
			}

			resp, err := obs.client.Do(req)
			if nil != err {
				// Failure
				obs.logger.Error("Unable to send cut-off notification (%s) for %s, err: %s", obs.listener.FailureURL, obs.listener.Config.URL, err)
			} else {
				if nil == resp {
					// Failure
					obs.logger.Error("Unable to send cut-off notification (%s) nil response", obs.listener.FailureURL)
				} else {
					// Success
					obs.logger.Error("Able to send cut-off notification (%s) status: %s", obs.listener.FailureURL, resp.Status)
				}
			}
		} else {
			obs.logger.Error("No cut-off notification URL specified.")
		}
	}
}
