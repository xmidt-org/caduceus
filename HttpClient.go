package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-kit/kit/metrics"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// DoerFunc implements HTTPClient
type DoerFunc func(*http.Request) (*http.Response, error)

func (d DoerFunc) Do(req *http.Request) (*http.Response, error) {
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
		return nil, errors.New("histogram cannot be nil")
	}
	return &metricWrapper{
		now:          now,
		queryLatency: queryLatency,
	}, nil
}

func (m *metricWrapper) roundTripper(next HTTPClient) HTTPClient {
	return DoerFunc(func(req *http.Request) (*http.Response, error) {
		startTime := m.now()
		resp, err := next.Do(req)
		endTime := m.now()

		// find time difference, add to metric
		var latency = endTime.Sub(startTime)
		m.queryLatency.Observe(latency.Seconds())

		return resp, err
	})
}
