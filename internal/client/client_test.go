// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package client

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
)

type MockClient struct {
	mock.Mock
}

func (m *MockClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	switch t := args.Get(0).(type) {
	case *http.Response:
		return t, args.Error(1)
	}
	return &http.Response{}, args.Error(1)
}

func TestNopClient_Do(t *testing.T) {
	// Create a mock request
	req, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	mockobj := new(MockClient)
	mockobj.On("Do", req).Return(&http.Response{
		StatusCode: 200}, nil)

	// Create a NopClient instance
	nopClient := NopClient(mockobj)

	// Call the Do method
	resp, err := nopClient.Do(req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Validate the response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

func TestDoerFunc_Do(t *testing.T) {
	// Create a mock request
	req, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a DoerFunc instance
	doerFunc := DoerFunc(func(req *http.Request) (*http.Response, error) {
		// Create a mock response
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       nil,
			Header:     make(http.Header),
		}
		return resp, nil
	})

	// Call the Do method
	resp, err := doerFunc.Do(req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Validate the response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}
