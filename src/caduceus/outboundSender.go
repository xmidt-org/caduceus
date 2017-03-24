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
	whl "github.com/Comcast/webpa-common/webhooklisteners"
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
	Text         string              `json:"text"`
	Original     whl.WebHookListener `json:"webhook_registration"`
	CutOffPeriod string              `json:"cut_off_period"`
	QueueSize    int                 `json:"queue_size"`
	Workers      int                 `json:"worker_count"`
}

// OutboundSenderFactory is a configurable factory for OutboundSender objects.
type OutboundSenderFactory struct {
	// The WebHookListener to service
	Listener whl.WebHookListener

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

// OutboundSender is the outbound sender object.
type OutboundSender struct {
	listener     whl.WebHookListener
	deliverUntil time.Time
	dropUntil    time.Time
	client       *http.Client
	secret       []byte
	events       []*regexp.Regexp
	matcher      map[string][]*regexp.Regexp
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
func (osf OutboundSenderFactory) New() (obs *OutboundSender, err error) {
	if _, err = url.ParseRequestURI(osf.Listener.URL); nil != err {
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

	obs = &OutboundSender{
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

	if "" != osf.Listener.Secret {
		obs.secret = []byte(osf.Listener.Secret)
	}

	if "" != osf.Listener.FailureURL {
		if _, err = url.ParseRequestURI(osf.Listener.FailureURL); nil != err {
			obs = nil
			return
		}
	}

	obs.profiler, err = osf.ProfilerFactory.New()
	if err != nil {
		obs = nil
		return
	}

	// Give us some head room so that we don't block when we get near the
	// completely full point.
	obs.queue = make(chan outboundRequest, osf.QueueSize)

	// Create the event regex objects
	for _, event := range osf.Listener.Events {
		var re *regexp.Regexp
		if re, err = regexp.Compile(event); nil != err {
			obs = nil
			return
		}

		obs.events = append(obs.events, re)
	}
	if nil == obs.events {
		err = errors.New("Events must not be empty.")
		obs = nil
		return
	}

	// Create the matcher regex objects
	if nil != osf.Listener.Matchers {
		obs.matcher = make(map[string][]*regexp.Regexp)
		for key, value := range osf.Listener.Matchers {
			var list []*regexp.Regexp
			for _, item := range value {
				if ".*" == item {
					// Match everything - skip the filtering
					obs.matcher = nil
					break
				}
				var re *regexp.Regexp
				if re, err = regexp.Compile(item); nil != err {
					err = fmt.Errorf("Invalid matcher item: Matcher['%s'] = '%s'", value, item)
					obs = nil
					return
				}
				list = append(list, re)
			}

			if nil == obs.matcher {
				break
			}

			obs.matcher[key] = list
		}
	}

	obs.wg.Add(osf.NumWorkers)
	for i := 0; i < osf.NumWorkers; i++ {
		go obs.worker(i)
	}

	return
}

// Extend extends the time the OutboundSender will deliver messages to the
// specified time.  The new delivery cutoff time must be after the previously
// set delivery cutoff time.
func (obs *OutboundSender) Extend(until time.Time) {
	obs.mutex.Lock()
	if until.After(obs.deliverUntil) {
		obs.deliverUntil = until
	}
	obs.mutex.Unlock()
}

// Shutdown causes the OutboundSender to stop it's activities either gently or
// abruptly based on the gentle parameter.  If gentle is false, all queued
// messages will be dropped without an attempt to send made.
func (obs *OutboundSender) Shutdown(gentle bool) {
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

// RetiredSince returns the time the OutboundSender retired (which could be in
// the future).
func (obs *OutboundSender) RetiredSince() time.Time {
	obs.mutex.RLock()
	deliverUntil := obs.deliverUntil
	obs.mutex.RUnlock()

	return deliverUntil
}

// QueueWrp is given a request to evaluate and optionally enqueue in the list
// of messages to deliver.  The request is checked to see if it matches the
// criteria before being accepted or silently dropped.
func (obs *OutboundSender) QueueWrp(req CaduceusRequest) {
	// TODO Not supported yet
	obs.logger.Error("QueueWrp() not supported yet.")
}

// QueueJSON is given a request to evaluate and optionally enqueue in the list
// of messages to deliver.  The request is checked to see if it matches the
// criteria before being accepted or silently dropped.
func (obs *OutboundSender) QueueJSON(req CaduceusRequest,
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
					for _, deviceRegex := range obs.matcher["device_id"] {
						if deviceRegex.MatchString(deviceID) {
							matchDevice = true
							break
						}
					}
				}
				if matchDevice {
					if len(obs.queue) < obs.queueSize {
						outboundReq := outboundRequest{req: req,
							event:       eventType,
							transID:     transID,
							deviceID:    deviceID,
							contentType: "application/json",
						}
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
func (obs *OutboundSender) worker(id int) {
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
			req, err := http.NewRequest("POST", obs.listener.URL, payload)
			if nil != err {
				// Report drop
				obs.logger.Error("http.NewRequest(\"POST\", '%s', payload) failed: %s", obs.listener.URL, err)
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
				resp, err := obs.client.Do(req)
				if nil != err {
					// Report failure
				} else {
					if (200 <= resp.StatusCode) && (resp.StatusCode <= 204) {
						// Report success
						obs.profiler.Send(work.req)
					} else {
						// Report partial success
						obs.profiler.Send(work.req)
					}
				}
			}

		} else {
			// Report drop
		}
	}
}

// queueOverflow handles the logic of what to do when a queue overflows
func (obs *OutboundSender) queueOverflow() {
	obs.mutex.Lock()
	obs.dropUntil = time.Now().Add(obs.cutOffPeriod)
	obs.mutex.Unlock()

	// Send a "you've been cut off" warning message
	if "" != obs.listener.FailureURL {
		msg, err := json.Marshal(obs.failureMsg)

		if nil != err {
			obs.logger.Error("json.Marshal( %v ) failed: %s", obs.failureMsg, err)
		} else {

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
				obs.logger.Error("Unable to send cut-off notification (%s) err: %s", obs.listener.FailureURL, err)
			} else {
				if nil == resp {
					// Failure
					obs.logger.Error("Unable to send cut-off notification (%s) nil response", obs.listener.FailureURL)
				} else {
					// Success
					obs.logger.Error("Able to send cut-off notification (%s) status: %s", obs.listener.FailureURL, resp.Status)
				}
			}
		}
	} else {
		obs.logger.Error("No cut-off notification URL specified.")
	}
}
