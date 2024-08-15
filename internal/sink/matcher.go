// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package sink

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/caduceus/internal/metrics"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

type ClientMock struct {
}

func (c *ClientMock) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{}, nil
}

// move to subpackage and change to Interface
type Matcher interface {
	IsMatch(*wrp.Message) bool
}

type MatcherV1 struct {
	events  []*regexp.Regexp
	matcher []*regexp.Regexp
	logger  *zap.Logger
	mutex   sync.RWMutex
}

// TODO: need to add matching logic for RegistryV2 & MatcherV2
func NewMatcher(l ancla.Register, logger *zap.Logger) (Matcher, error) {
	switch v := l.(type) {
	case *ancla.RegistryV1:
		m := &MatcherV1{}
		m.logger = logger
		if err := m.update(*v); err != nil {
			return nil, err
		}
		return m, nil
	default:
		return nil, fmt.Errorf("invalid listener")
	}
}

// Update applies user configurable values for the outbound sender when a
// webhook is registered
func (m1 *MatcherV1) update(l ancla.RegistryV1) error {

	//TODO: don't believe the logger for webhook is being set anywhere just yet
	m1.logger = m1.logger.With(zap.String("webhook.address", l.Registration.Address))

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
			m1.logger.Error("failed to update url", zap.Any(metrics.UrlLabel, l.Registration.Config.AlternativeURLs[i]), zap.Error(err))
			return err
		}
	}

	// write/update sink sender
	m1.mutex.Lock()
	defer m1.mutex.Unlock()

	m1.events = events

	// if matcher list is empty set it nil for Queue() logic
	m1.matcher = nil
	if 0 < len(matcher) {
		m1.matcher = matcher
	}

	return nil

}

func (m1 *MatcherV1) IsMatch(msg *wrp.Message) bool {
	m1.mutex.RLock()
	events := m1.events
	matcher := m1.matcher
	m1.mutex.RUnlock()

	var (
		matchEvent  = false
		matchDevice = false
	)
	for _, eventRegex := range events {
		if eventRegex.MatchString(strings.TrimPrefix(msg.Destination, "event:")) {
			matchEvent = true
			break
		}
	}
	if !matchEvent {
		m1.logger.Debug("destination regex doesn't match", zap.String("event.dest", msg.Destination))
		return false
	}

	for _, deviceRegex := range matcher {
		if deviceRegex.MatchString(msg.Source) || deviceRegex.MatchString(strings.TrimPrefix(msg.Destination, "event:")) {
			matchDevice = true
			break
		}
	}

	if !matchDevice {
		m1.logger.Debug("device regex doesn't match", zap.String("event.source", msg.Source))
		return false
	}
	return true
}
