// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import "net/http"

// Client is the interface used to requests messages to the desired location.
// The Client can be either and HTTP Client or a Kafka Producer.
type Client interface {
	Do(*http.Request) (*http.Response, error)
}

func nopClient(next Client) Client {
	return next
}

// DoerFunc implements Client
type doerFunc func(*http.Request) (*http.Response, error)

func (d doerFunc) Do(req *http.Request) (*http.Response, error) {
	return d(req)
}
