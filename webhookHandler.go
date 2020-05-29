package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/xmidt-org/argus/webhookclient"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/webhook"
	"io/ioutil"
	"net/http"
	"time"
)

type Registry struct {
	hookStore *webhookclient.Client
	config    RegistryConfig
}

type RegistryConfig struct {
	Logger      log.Logger
	Listener    webhookclient.ListenerFunc
	ArgusConfig webhookclient.ClientConfig
}

func NewRegistry(config RegistryConfig, listener webhookclient.Listener) (*Registry, error) {
	argus, err := webhookclient.CreateClient(config.ArgusConfig, webhookclient.WithLogger(config.Logger))
	if err != nil {
		return nil, err
	}
	if listener != nil {
		argus.SetListener(listener)
	}

	return &Registry{
		config:    config,
		hookStore: argus,
	}, nil
}

// jsonResponse is an internal convenience function to write a json response
func jsonResponse(rw http.ResponseWriter, code int, msg string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(code)
	rw.Write([]byte(fmt.Sprintf(`{"message":"%s"}`, msg)))
}

// get is an api call to return all the registered listeners
func (r *Registry) GetRegistry(rw http.ResponseWriter, req *http.Request) {
	logging.Info(r.config.Logger).Log(logging.MessageKey(), "get registry")
	items, err := r.hookStore.GetWebhook("")
	if err != nil {
		jsonResponse(rw, http.StatusInternalServerError, err.Error())
	}
	data := []struct {
		URL         string   `json:"url"`
		ContentType string   `json:"content_type"`
		FailureURL  string   `json:"failure_url"`
		Events      []string `json:"events"`
		Matcher     struct {
			DeviceId []string `json:"device_id"`
		} `json:"matcher,omitempty"`
		Until            time.Time `json:"until"`
		LastRegistration string    `json:"registered_from_address"`
	}{}
	for _, hook := range items {
		data = append(data, struct {
			URL         string   `json:"url"`
			ContentType string   `json:"content_type"`
			FailureURL  string   `json:"failure_url"`
			Events      []string `json:"events"`
			Matcher     struct {
				DeviceId []string `json:"device_id"`
			} `json:"matcher,omitempty"`
			Until time.Time `json:"until"`

			LastRegistration string `json:"registered_from_address"`
		}{URL: hook.Config.URL, ContentType: hook.Config.ContentType, FailureURL: hook.FailureURL, Events: hook.Events, Matcher: hook.Matcher, Until: hook.Until, LastRegistration: hook.Address})
	}

	if msg, err := json.Marshal(data); err != nil {
		jsonResponse(rw, http.StatusInternalServerError, err.Error())
	} else {
		rw.Header().Set("Content-Type", "application/json")
		rw.Write(msg)
	}
}

// update is an api call to processes a listenener registration for adding and updating
func (r *Registry) UpdateRegistry(rw http.ResponseWriter, req *http.Request) {
	payload, err := ioutil.ReadAll(req.Body)
	req.Body.Close()

	w, err := webhook.NewW(payload, req.RemoteAddr)
	if err != nil {
		jsonResponse(rw, http.StatusBadRequest, err.Error())
		return
	}

	err = r.hookStore.Push(*w, "")
	if err != nil {
		jsonResponse(rw, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(rw, http.StatusOK, "Success")
}
