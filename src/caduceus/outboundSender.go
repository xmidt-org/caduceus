package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"
)

type OutboundRequest struct {
	req         CaduceusRequest
	event       string
	transId     string
	deviceId    string
	contentType string
}

type OutboundSenderFactory struct {
	Url         string
	ContentType string
	Client      *http.Client
	Secret      string
	Until       time.Time
	Events      []string
	Matchers    map[string][]string
	NumWorkers  int
	QueueSize   int
}

type OutboundSender struct {
	url          string
	contentType  string
	deliverUntil time.Time
	dropUntil    time.Time
	client       *http.Client
	secret       []byte
	events       []*regexp.Regexp
	matcher      map[string][]*regexp.Regexp
	queueSize    int
	queue        chan OutboundRequest
	profiler     ServerRing
	wg           sync.WaitGroup
}

func (osf OutboundSenderFactory) New() (obs *OutboundSender, err error) {
	if _, err = url.ParseRequestURI(osf.Url); nil != err {
		return
	}

	if nil == osf.Client {
		err = errors.New("nil http.Client")
		return
	}

	obs = &OutboundSender{
		url:          osf.Url,
		contentType:  osf.ContentType,
		client:       osf.Client,
		deliverUntil: osf.Until,
		queueSize:    osf.QueueSize,
	}

	if "" != osf.Secret {
		obs.secret = []byte(osf.Secret)
	}

	// Give us some head room so that we don't block when we get near the
	// completely full point.
	obs.queue = make(chan OutboundRequest, osf.QueueSize+10)

	// Create the event regex objects
	for _, event := range osf.Events {
		var re *regexp.Regexp
		if re, err = regexp.Compile(event); nil != err {
			obs = nil
			return
		}

		obs.events = append(obs.events, re)
	}

	// Create the matcher regex objects
	if nil != osf.Matchers {
		obs.matcher = make(map[string][]*regexp.Regexp)
		for key, value := range osf.Matchers {
			var list []*regexp.Regexp
			for _, item := range value {
				if ".*" == item {
					// Match everything - skip the filtering
					obs.matcher = nil
					break
				}
				var re *regexp.Regexp
				if re, err = regexp.Compile(item); nil != err {
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

	obs.profiler = NewCaduceusRing(100)

	obs.wg.Add(osf.NumWorkers)
	for i := 0; i < osf.NumWorkers; i++ {
		go obs.run(i)
	}

	return
}

func (obs *OutboundSender) Extend(until time.Time) {
	if until.After(obs.deliverUntil) {
		obs.deliverUntil = until
	}
}

func (obs *OutboundSender) Shutdown(gentle bool) {
	close(obs.queue)
	if false == gentle {
		obs.deliverUntil = time.Time{}
	}
	obs.wg.Wait()
}

func (obs *OutboundSender) QueueWrp(req CaduceusRequest) {
	// TODO Not supported yet
}

func (obs *OutboundSender) QueueJson(req CaduceusRequest,
	eventType, deviceId, transId string) {

	for _, eventRegex := range obs.events {
		if eventRegex.MatchString(eventType) {
			matchDevice := (nil == obs.matcher)
			if nil != obs.matcher {
				for _, deviceRegex := range obs.matcher["device_id"] {
					if deviceRegex.MatchString(deviceId) {
						matchDevice = true
						break
					}
				}
			}
			if matchDevice {
				if len(obs.queue) < obs.queueSize {
					or := OutboundRequest{req: req,
						event:       eventType,
						transId:     transId,
						deviceId:    deviceId,
						contentType: "application/json",
					}
					obs.queue <- or
				} else {
					obs.queueOverflow()
				}
			}
		}
	}
}

func (obs *OutboundSender) run(id int) {
	defer obs.wg.Done()

	// Make a local copy of the hmac
	var h hash.Hash

	// Create the base sha1 hash object for each thread
	if nil != obs.secret {
		h = hmac.New(sha1.New, obs.secret)
	}

	for work := range obs.queue {
		now := time.Now()
		if now.Before(obs.deliverUntil) && now.After(obs.dropUntil) {
			payload := bytes.NewReader(work.req.Payload)
			req, err := http.NewRequest("POST", obs.url, payload)
			if nil != err {
				// ??? Should never happen
			}
			req.Header.Set("Content-Type", work.contentType)
			req.Header.Set("X-Webpa-Event", work.event)
			req.Header.Set("X-Webpa-Transaction-Id", work.transId)
			req.Header.Set("X-Webpa-Device-Id", work.deviceId)

			if nil != h {
				h.Reset()
				h.Write(work.req.Payload)
				sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil)))
				req.Header.Set("X-Webpa-Signature", sig)
			}

			// Send it
			resp, err := obs.client.Do(req)
			if (nil != err) || (nil == resp) {
				// Report failure
			} else {
				// Report result
				obs.profiler.Add(work.req)
			}

		} else {
			// Report drop
		}
	}
}

func (obs *OutboundSender) queueOverflow() {
	// TODO Not supported yet
}
