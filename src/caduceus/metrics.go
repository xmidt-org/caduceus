package main

import (
	"bytes"
	"io/ioutil"
	"net/http"

	"github.com/Comcast/webpa-common/xmetrics"
	"github.com/go-kit/kit/metrics/provider"
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

// TrackEmptyRequestBody increments the EmptyRequestBodyCounter anytime an empty request is received
func TrackEmptyRequestBody(provider provider.Provider) func(http.Handler) http.Handler {
	counter := provider.NewCounter(EmptyRequestBodyCounter)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			// don't trust the Content-Length header ...
			body, err := ioutil.ReadAll(request.Body)
			if err != nil {
				response.WriteHeader(http.StatusBadRequest)
				return
			}

			if len(body) == 0 {
				counter.Add(1.0)
			}

			request.Body = ioutil.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(response, request)
		})
	}
}
