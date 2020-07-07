package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/xmidt-org/argus/chrysom"
	"github.com/xmidt-org/argus/model"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/webhook"
	"io/ioutil"
	"net/http/httptest"
	"testing"
	"time"
)

type inMemPusher map[string]model.Item

func (i inMemPusher) Push(item model.Item, owner string) (string, error) {
	i[item.Identifier] = item
	return item.Identifier, nil
}

func (i inMemPusher) Remove(id string, owner string) (model.Item, error) {
	if item, ok := i[id]; ok {
		delete(i, id)
		return item, nil
	}
	return model.Item{}, fmt.Errorf("no item with id: %s", id)
}

func (i inMemPusher) Stop(context context.Context) {
	// do nothing
}

func TestWebhookHandler(t *testing.T) {

	registry := Registry{
		hookStore: inMemPusher{},
		config: RegistryConfig{
			Logger:   logging.NewTestLogger(nil, t),
			Listener: nil,
			Config: chrysom.ClientConfig{
				DefaultTTL: 5,
			},
		},
	}

	type testStruct struct {
		title              string
		hook               webhook.W
		expectedStatusCode int
	}

	testData := []testStruct{
		{
			title:              "empty webhook",
			hook:               webhook.W{},
			expectedStatusCode: 400,
		},
		{
			title: "good webhook",
			hook: webhook.W{
				Config: struct {
					URL             string   `json:"url"`
					ContentType     string   `json:"content_type"`
					Secret          string   `json:"secret,omitempty"`
					AlternativeURLs []string `json:"alt_urls,omitempty"`
				}{
					URL:             "http://localhost:8080/evetns",
					ContentType:     "application/json",
					Secret:          "noice",
					AlternativeURLs: nil,
				},
				FailureURL: "",
				Events:     []string{".*"},
				Matcher: struct {
					DeviceId []string `json:"device_id"`
				}{},
				Duration: 0,
				Until:    time.Time{},
				Address:  "",
			},
			expectedStatusCode: 200,
		},
	}

	for _, tc := range testData {
		t.Run(tc.title, func(t *testing.T) {
			assert := assert.New(t)
			status, body := testRegistryWithRequest(registry, tc.hook)
			assert.Equal(tc.expectedStatusCode, status)
			assert.NotEmpty(body)
			registry.config.Logger.Log("body", string(body))
		})
	}

}

func testRegistryWithRequest(registry Registry, w webhook.W) (int, []byte) {
	response := httptest.NewRecorder()
	payload, _ := json.Marshal(w)
	request := httptest.NewRequest("POST", "/hook", bytes.NewBuffer(payload))

	registry.UpdateRegistry(response, request)
	result := response.Result()

	data, _ := ioutil.ReadAll(result.Body)
	return result.StatusCode, data

}
