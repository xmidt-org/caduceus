package main

import "net/http"

// Client is the interface used to requests messages to the desired location.
// The Client can be either and HTTP Client or a Kafka Producer.
type Client interface {
	Do(*http.Request) (*http.Response, error)
}
