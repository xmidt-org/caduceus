package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-kit/kit/metrics"
)

var (
    errNilHistogram = errors.New("histogram cannot be nil")
)

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

func noHTTPClient(next httpClient) httpClient {
	return next
}

// DoerFunc implements HTTPClient
type doerFunc func(*http.Request) (*http.Response, error)

func (d doerFunc) Do(req *http.Request) (*http.Response, error) {
	return d(req)
}

type metricWrapper struct {
	now          func() time.Time
	queryLatency metrics.Histogram
}

func newMetricWrapper(now func() time.Time, queryLatency metrics.Histogram) (*metricWrapper, error) {
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
		code := networkError

		if err == nil {
			code = resp.Status
		}

		// find time difference, add to metric
		var latency = endTime.Sub(startTime)
		m.queryLatency.With("code", code).Observe(latency.Seconds())

		return resp, err
	})
}
