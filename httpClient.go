// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	errNilHistogram = errors.New("histogram cannot be nil")
)

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

func nopHTTPClient(next httpClient) httpClient {
	return next
}

// DoerFunc implements HTTPClient
type doerFunc func(*http.Request) (*http.Response, error)

func (d doerFunc) Do(req *http.Request) (*http.Response, error) {
	return d(req)
}

type metricWrapper struct {
	now          func() time.Time
	queryLatency prometheus.HistogramVec
	id           string
}

func newMetricWrapper(now func() time.Time, queryLatency prometheus.HistogramVec, id string) (*metricWrapper, error) {
	if now == nil {
		now = time.Now
	}
	if queryLatency.MetricVec == nil {
		return nil, errNilHistogram
	}
	return &metricWrapper{
		now:          now,
		queryLatency: queryLatency,
		id:           id,
	}, nil
}

func (m *metricWrapper) roundTripper(next httpClient) httpClient {
	return doerFunc(func(req *http.Request) (*http.Response, error) {
		startTime := m.now()
		resp, err := next.Do(req)
		endTime := m.now()
		code := networkError

		if err == nil {
			code = strconv.Itoa(resp.StatusCode)
		}

		// find time difference, add to metric
		var latency = endTime.Sub(startTime)
		m.queryLatency.With(prometheus.Labels{UrlLabel: m.id, CodeLabel: code}).Observe(latency.Seconds())

		return resp, err
	})
}
