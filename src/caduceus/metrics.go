package main

import (
	"github.com/Comcast/webpa-common/xmetrics"
)

const (
	EmptyRequestBodyCounter = "empty_request_body_count"
)

func Metrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		xmetrics.Metric{
			Name: EmptyRequestBodyCounter,
			Type: "counter",
		},
	}
}
