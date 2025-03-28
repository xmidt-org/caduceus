// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	errNilHistogram = errors.New("histogram cannot be nil")
)

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

// DoerFunc implements HTTPClient
type doerFunc func(*http.Request) (*http.Response, error)

func (d doerFunc) Do(req *http.Request) (*http.Response, error) {
	return d(req)
}

type metricWrapper struct {
	now          func() time.Time
	queryLatency prometheus.ObserverVec
}

func newMetricWrapper(now func() time.Time, queryLatency prometheus.ObserverVec) (*metricWrapper, error) {
	if now == nil {
		now = time.Now
	}
	if queryLatency == nil {
		return nil, errNilHistogram
	}
	return &metricWrapper{
		now:          now,
		queryLatency: queryLatency,
	}, nil
}

func (m *metricWrapper) roundTripper(next httpClient) httpClient {
	return doerFunc(func(req *http.Request) (*http.Response, error) {
		startTime := m.now()
		resp, err := next.Do(req)
		endTime := m.now()

		code := unknown
		reason := noErrReason
		if err != nil {
			reason = getDoErrReason(err)
			if resp != nil {
				code = strconv.Itoa(resp.StatusCode)
			}
		} else {
			code = strconv.Itoa(resp.StatusCode)
		}

		// find time difference, add to metric
		m.queryLatency.With(prometheus.Labels{urlLabel: req.URL.String(), reasonLabel: reason, codeLabel: code}).Observe(endTime.Sub(startTime).Seconds())

		return resp, err
	})
}

func getDoErrReason(err error) string {
	var d *net.DNSError
	if err == nil {
		return noErrReason
	} else if errors.Is(err, context.DeadlineExceeded) {
		return deadlineExceededReason
	} else if errors.Is(err, context.Canceled) {
		return contextCanceledReason
	} else if errors.Is(err, &net.AddrError{}) {
		return addressErrReason
	} else if errors.Is(err, &net.ParseError{}) {
		return parseAddrErrReason
	} else if errors.Is(err, net.InvalidAddrError("")) {
		return invalidAddrReason
	} else if errors.As(err, &d) {
		if d.IsNotFound {
			return hostNotFoundReason
		}
		return dnsErrReason
	} else if errors.Is(err, net.ErrClosed) {
		return connClosedReason
	} else if errors.Is(err, &net.OpError{}) {
		return opErrReason
	} else if errors.Is(err, net.UnknownNetworkError("")) {
		return networkErrReason
	}

	// nolint: errorlint
	if err, ok := err.(*url.Error); ok {
		if strings.TrimSpace(strings.ToLower(err.Unwrap().Error())) == "eof" {
			return connectionUnexpectedlyClosedEOFReason
		}
	}

	return unknown
}
