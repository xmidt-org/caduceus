package main

import (
	"container/ring"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

type ClientMock struct {
}

func (c *ClientMock) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{}, nil
}

type WebhookI interface {
	CheckMsg(*wrp.Message) error
	getUrls() *ring.Ring
}
type WebhookV1 struct {
	events  []*regexp.Regexp
	matcher []*regexp.Regexp
	urls    *ring.Ring
	CommonWebhook
}

type CommonWebhook struct {
	mutex  sync.RWMutex
	logger *zap.Logger
}

// Update applies user configurable values for the outbound sender when a
// webhook is registered
func (w1 *WebhookV1) Update(l ListenerV1) error {

	//TODO: don't believe the logger for webhook is being set anywhere just yet
	w1.logger = w1.logger.With(zap.String("webhook.address", l.Registration.Address))

	if l.Registration.FailureURL != "" {
		_, err := url.ParseRequestURI(l.Registration.FailureURL)
		return err
	}

	var events []*regexp.Regexp
	for _, event := range l.Registration.Events {
		var re *regexp.Regexp
		re, err := regexp.Compile(event)
		if err != nil {
			return err
		}
		events = append(events, re)
	}

	if len(events) < 1 {
		return errors.New("events must not be empty")
	}

	var matcher []*regexp.Regexp
	for _, item := range l.Registration.Matcher.DeviceID {
		if item == ".*" {
			// Match everything - skip the filtering
			matcher = []*regexp.Regexp{}
			break
		}

		var re *regexp.Regexp
		re, err := regexp.Compile(item)
		if err != nil {
			return fmt.Errorf("invalid matcher item: '%s'", item)
		}
		matcher = append(matcher, re)
	}

	// Validate the various urls
	urlCount := len(l.Registration.Config.AlternativeURLs)
	for i := 0; i < urlCount; i++ {
		_, err := url.Parse(l.Registration.Config.AlternativeURLs[i])
		if err != nil {
			w1.logger.Error("failed to update url", zap.Any("url", l.Registration.Config.AlternativeURLs[i]), zap.Error(err))
			return err
		}
	}

	// write/update sink sender
	w1.mutex.Lock()
	defer w1.mutex.Unlock()

	w1.events = events

	//TODO: need to figure out how to set this

	// if matcher list is empty set it nil for Queue() logic
	w1.matcher = nil
	if 0 < len(matcher) {
		w1.matcher = matcher
	}

	if urlCount == 0 {
		w1.urls = ring.New(1)
		w1.urls.Value = l.Registration.Config.ReceiverURL
	} else {
		ring := ring.New(urlCount)
		for i := 0; i < urlCount; i++ {
			ring.Value = l.Registration.Config.AlternativeURLs[i]
			ring = ring.Next()
		}
		w1.urls = ring
	}

	// Randomize where we start so all the instances don't synchronize
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	offset := rand.Intn(w1.urls.Len())
	for 0 < offset {
		w1.urls = w1.urls.Next()
		offset--
	}

	return nil

}

func (w1 *WebhookV1) CheckMsg(msg *wrp.Message) (err error) {
	w1.mutex.RLock()
	events := w1.events
	matcher := w1.matcher
	w1.mutex.RUnlock()

	var (
		matchEvent  bool
		matchDevice = true
	)
	for _, eventRegex := range events {
		if eventRegex.MatchString(strings.TrimPrefix(msg.Destination, "event:")) {
			matchEvent = true
			break
		}
	}
	if !matchEvent {
		w1.logger.Debug("destination regex doesn't match", zap.String("event.dest", msg.Destination))
		//TODO: return an error here?
		return
	}

	if matcher != nil {
		matchDevice = false
		for _, deviceRegex := range matcher {
			if deviceRegex.MatchString(msg.Source) {
				matchDevice = true
				break
			}
		}
	}

	if !matchDevice {
		w1.logger.Debug("device regex doesn't match", zap.String("event.source", msg.Source))
		//TODO: return an error here?
		return
	}
	return
}

func (w1 *WebhookV1) getUrls() (urls *ring.Ring) {
	urls = w1.urls
	// Move to the next URL to try 1st the next time.
	// This is okay because we run a single dispatcher and it's the
	// only one updating this field.
	w1.urls = w1.urls.Next()
	return
}

type WebhookV2 struct {
	//nolint:staticcheck
	placeholder string
	CommonWebhook
}

func (w2 *WebhookV2) Update(l ListenerV2) error {

	return nil
}

func (w2 *WebhookV2) CheckMsg(msg *wrp.Message) error {
	return nil
}

func (w2 *WebhookV2) getUrls() *ring.Ring {
	return nil
}
