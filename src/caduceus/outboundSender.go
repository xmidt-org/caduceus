package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/http"
	URL "net/url"
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

type OutboundSender struct {
	url          string
	contentType  string
	deliverUntil int64
	dropUntil    int64
	client       *http.Client
	queue        chan OutboundRequest
	maxItems     int
	hmac         hash.Hash
	events       []*regexp.Regexp
	matcher      []*regexp.Regexp
	workerWg     sync.WaitGroup
}

func NewOutboundSender(url, contentType string, secret []byte,
	client *http.Client, events, deviceIdMatchers []string,
	workerCount, queueSize, maxItems int) (obs *OutboundSender, err error) {

	if _, err = URL.Parse(url); nil != err {
		return
	}

	if nil == client {
		err = errors.New("nil http.Client")
		return
	}

	if workerCount < 1 {
		err = errors.New("not enough workers")
		return
	}

	if queueSize < 1 {
		err = errors.New("not a large enough queueSize")
		return
	}

	var h hash.Hash
	if nil != secret {
		h = hmac.New(sha1.New, obs.Secret)
	}

	var eventRegexp []*regexp
	for _, event := range events {
		if re, e := regexp.Compile(event); nil != e {
			err = e
			return
		}

		eventRegexp = append(eventRegexp, re)
	}

	var matchRegexp []*regexp
	matchAll := (nil == deviceIdMatchers)
	for _, matcher := range deviceIdMatchers {
		if ".*" == matcher {
			matchAll = true
		}
		if re, e := regexp.Compile(matcher); nil != e {
			err = e
			return
		}

		matchRegexp = append(matchRegexp, re)
	}
	if true == matchAll {
		matchRegexp = nil
	}

	obs = &OutboundSender{
		url:         url,
		contentType: contentType,
		hmac:        h,
		client:      client,
		events:      regexpSlice,
		matcher:     matchRegexp,
		maxItems:    maxItems,
	}

	obs.queue = make(chan OutboundRequest, queueSize)
	obs.workerWg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go obs.run()
	}

	return
}

func (obs *OutboundSender) Extend(until int64) {
	obs.deliverUntil = until
}

func (obs *OutboundSender) Shutdown() {
	obs.deliverUntil = 0
	obs.queue.close()
}

func (obs *OutboundSender) QueueWrp(req CaduceusRequest) {
	// TODO Not supported yet
}

func (obs *OutboundSender) QueueJson(req CaduceusRequest,
	eventType, deviceId, transId string) {

	for _, eventRegex := range obs.events {
		if eventRegex.MatchString(eventType) {
			matchDevice := (nil == obs.matcher)
			for _, deviceRegex := range obs.matcher {
				if deviceRegex.MatchString(eventType) {
					matchDevice = true
					break
				}
			}
			if matchDevice {
				if len(obs.queue) < obs.maxItems {
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

func (obs *OutboundSender) run() {
	// Make a local copy of the hmac
	hmac := obs.hmac

	for work := range obs.queue {
		now := time.Time.Now().Unix()
		if now < obs.deliverUntil && obs.dropUntil < now {
			req, err := http.NewRequest("POST", obs.url, payload)
			if nil != err {
				// ??? Should never happen
			}
			req.Header.Set("Content-Type", work.contentType)
			req.Header.Set("X-Webpa-Event", work.event)
			req.Header.Set("X-Webpa-Transaction-Id", work.transId)
			req.Header.Set("X-Webpa-Device-Id", work.deviceId)

			if nil != hmac {
				hmac.Reset()
				hmac.Write(payload)
				sig := fmt.Sprintf("sha1=%s", hex.EncodeToString(hmac.Sum(nil)))
				req.Header.Set("X-Webpa-Signature", sig)
			}

			// Send it
			resp, err := obs.client.Do(req)
			if (nil != err) || (nil == resp) {
				// Report failure
			} else {
				// Report result
			}

		} else {
			// Report drop
		}
	}
	obs.workerWg.Done()
}

func (obs *OutboundSender) queueOverflow() {
	// TODO Not supported yet
}
