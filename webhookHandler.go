package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/webhook"
	"github.com/xmidt-org/webpa-common/webhookStore"
	"io/ioutil"
	"net/http"
)

type Registry struct {
	hookStore *webhookStore.InMem
	config    RegistryConfig
}

type RegistryConfig struct {
	Logger       log.Logger
	Listener     webhookStore.ListenerFunc
	InMemConfig  webhookStore.InMemConfig
	ConsulConfig webhookStore.ConsulConfig
}

func NewRegistry(config RegistryConfig) *Registry {
	if config.ConsulConfig.Client == nil {
		logging.Error(config.Logger).Log(logging.MessageKey(), "consul client must be defined")
		return nil
	}

	hookStorage := webhookStore.CreateInMemStore(
		config.InMemConfig,
		webhookStore.WithLogger(config.Logger),
		webhookStore.WithStorageListener(
			func(options ...webhookStore.Option) webhookStore.Pusher { return webhookStore.CreateConsulStore(config.ConsulConfig, options...) },
			webhookStore.WithLogger(config.Logger)),
		webhookStore.WithListener(config.Listener),
	)

	return &Registry{
		config:    config,
		hookStore: hookStorage,
	}
}

// jsonResponse is an internal convenience function to write a json response
func jsonResponse(rw http.ResponseWriter, code int, msg string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(code)
	rw.Write([]byte(fmt.Sprintf(`{"message":"%s"}`, msg)))
}

// get is an api call to return all the registered listeners
func (r *Registry) GetRegistry(rw http.ResponseWriter, req *http.Request) {
	items, err := r.hookStore.GetWebhook()
	if err != nil {
		jsonResponse(rw, http.StatusInternalServerError, err.Error())
	}

	if msg, err := json.Marshal(items); err != nil {
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

	err = r.hookStore.Push(*w)
	if err != nil {
		jsonResponse(rw, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(rw, http.StatusOK, "Success")
}
