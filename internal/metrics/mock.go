package metrics

import (
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/mock"
)

// metric mocks

type MockCollector struct {
}

func (m *MockCollector) Describe(chan<- *prometheus.Desc) {
}
func (m *MockCollector) Collect(chan<- prometheus.Metric) {
}

type MockMetric struct {
}

func (m *MockMetric) Desc() *prometheus.Desc {
	return &prometheus.Desc{}
}

func (m *MockMetric) Write(*dto.Metric) error {
	return nil
}

// mockCounter provides the mock implementation of the metrics.Counter object
type MockCounter struct {
	MockCollector
	MockMetric
	mock.Mock
}

func (m *MockCounter) Add(delta float64) {
	m.Called(delta)
}

func (m *MockCounter) Inc() { m.Called() }

func (m *MockCounter) With(labels prometheus.Labels) prometheus.Counter {
	for _, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}

	m.Called(labels)
	return m
}

func (m *MockCounter) CurryWith(labels prometheus.Labels) (*prometheus.CounterVec, error) {
	m.Called(labels)
	labelnames := []string{}
	for l, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
		labelnames = append(labelnames, l)
	}

	return prometheus.NewCounterVec(prometheus.CounterOpts{}, labelnames).CurryWith(labels)
}

func (m *MockCounter) GetMetricWith(labels prometheus.Labels) (prometheus.Counter, error) {
	m.Called(labels)
	return m, nil
}

func (m *MockCounter) GetMetricWithLabelValues(lvs ...string) (prometheus.Counter, error) {
	m.Called(lvs)
	return m, nil
}

func (m *MockCounter) MustCurryWith(labels prometheus.Labels) *prometheus.CounterVec {
	m.Called(labels)
	labelnames := []string{}
	for l, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
		labelnames = append(labelnames, l)
	}

	return prometheus.NewCounterVec(prometheus.CounterOpts{}, labelnames).MustCurryWith(labels)
}

func (m *MockCounter) WithLabelValues(lvs ...string) prometheus.Counter {
	m.Called(lvs)
	return m
}

// mockGauge provides the mock implementation of the metrics.Counter object
type MockGauge struct {
	MockCollector
	MockMetric
	mock.Mock
}

func (m *MockGauge) CurryWith(labels prometheus.Labels) (*prometheus.GaugeVec, error) {
	m.Called(labels)
	labelnames := []string{}
	for l, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
		labelnames = append(labelnames, l)
	}

	return prometheus.NewGaugeVec(prometheus.GaugeOpts{}, labelnames).CurryWith(labels)
}

func (m *MockGauge) MustCurryWith(labels prometheus.Labels) *prometheus.GaugeVec {
	m.Called(labels)
	labelnames := []string{}
	for l, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
		labelnames = append(labelnames, l)
	}

	return prometheus.NewGaugeVec(prometheus.GaugeOpts{}, labelnames).MustCurryWith(labels)
}

func (m *MockGauge) GetMetricWith(labels prometheus.Labels) (prometheus.Gauge, error) {
	m.Called(labels)
	return m, nil
}

func (m *MockGauge) GetMetricWithLabelValues(lvs ...string) (prometheus.Gauge, error) {
	m.Called(lvs)
	return m, nil
}

func (m *MockGauge) WithLabelValues(lvs ...string) prometheus.Gauge {
	m.Called(lvs)
	return m
}

func (m *MockGauge) With(labels prometheus.Labels) prometheus.Gauge {
	for _, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}

	m.Called(labels)
	return m
}

func (m *MockGauge) Add(delta float64) {
	m.Called(delta)
}

func (m *MockGauge) Set(value float64) {
	m.Called(value)
}

func (m *MockGauge) Inc() { m.Called() }

func (m *MockGauge) Dec() { m.Called() }

func (m *MockGauge) Sub(val float64) {
	m.Called(val)
}

func (m *MockGauge) SetToCurrentTime() { m.Called() }

// mockHistogram provides the mock implementation of the metrics.Histogram object
type MockHistogram struct {
	MockCollector
	mock.Mock
}

func (m *MockHistogram) Observe(value float64) {
	m.Called(value)
}

func (m *MockHistogram) With(labels prometheus.Labels) prometheus.Observer {
	for _, v := range labels {
		if !utf8.ValidString(v) {
			panic("not UTF-8")
		}
	}

	m.Called(labels)
	return m
}

func (m *MockHistogram) CurryWith(labels prometheus.Labels) (prometheus.ObserverVec, error) {
	m.Called(labels)
	return m, nil
}

func (m *MockHistogram) GetMetricWith(labels prometheus.Labels) (prometheus.Observer, error) {
	m.Called(labels)
	return m, nil
}

func (m *MockHistogram) GetMetricWithLabelValues(lvs ...string) (prometheus.Observer, error) {
	m.Called(lvs)
	return m, nil
}

func (m *MockHistogram) MustCurryWith(labels prometheus.Labels) prometheus.ObserverVec {
	m.Called(labels)
	return m
}

func (m *MockHistogram) WithLabelValues(lvs ...string) prometheus.Observer {
	m.Called(lvs)
	return m
}
