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

	mutex sync.RWMutex
}

type MatcherV2 struct {
	matcher map[string]*regexp.Regexp
	logger  *zap.Logger

	mutex sync.RWMutex
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
	case *ancla.RegistryV2:
		m := &MatcherV2{}
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
	m1.logger = m1.logger.With(zap.String("webhook.address", l.Registration.Address))
	if l.Registration.FailureURL != "" {
		_, err := url.ParseRequestURI(l.Registration.FailureURL)
		if err != nil {
			return err
		}
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

	// write/update sink sender
	m1.mutex.Lock()
	defer m1.mutex.Unlock()

	m1.events = events

	// if matcher list is empty set it nil for Queue() logic
	m1.matcher = nil
	if len(matcher) > 0 {
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

// Update applies user configurable values for the outbound sender when a
// webhook is registered
func (m2 *MatcherV2) update(l ancla.RegistryV2) error {

	m2.logger = m2.logger.With(zap.String("webhook.address", l.Registration.Address))

	if l.Registration.FailureURL != "" {
		_, err := url.ParseRequestURI(l.Registration.FailureURL)
		if err != nil {
			return err
		}
	}

	//TODO: should we be checking that the l.Registration.Matcher.Field is a field in the wrp.Message?
	matcher := make(map[string]*regexp.Regexp)
	for _, item := range l.Registration.Matcher {
		if item.Regex == ".*" {
			// Match everything - skip the filtering
			matcher[item.Field] = &regexp.Regexp{}
			break
		}

		var re *regexp.Regexp
		re, err := regexp.Compile(item.Regex)
		if err != nil {
			return fmt.Errorf("invalid matcher item: '%s'", item.Regex)
		}
		matcher[item.Field] = re
	}

	// write/update sink sender
	m2.mutex.Lock()
	defer m2.mutex.Unlock()

	// if matcher list is empty set it nil for Queue() logic
	m2.matcher = nil
	if len(matcher) > 0 {
		m2.matcher = matcher
	}

	return nil

}

func (m2 *MatcherV2) IsMatch(msg *wrp.Message) bool {
	m2.mutex.RLock()
	matcher := m2.matcher
	m2.mutex.RUnlock()

	var (
		matchDevice = false
	)
	for field, deviceRegex := range matcher {
		if deviceRegex.MatchString(field) || deviceRegex.MatchString(strings.TrimPrefix(field, "event:")) {
			matchDevice = true
			break
		}
	}

	if !matchDevice {
		m2.logger.Debug("device regex doesn't match", zap.String("event.source", msg.Source))
		return false
	}
	return true
}
