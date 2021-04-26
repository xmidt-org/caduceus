package main

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/kit/metrics/provider"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/xmidt-org/candlelight"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/secure"
	"github.com/xmidt-org/webpa-common/secure/handler"
)

func TestNewPrimaryHandler(t *testing.T) {
	var (
		l                  = logging.New(nil)
		viper              = viper.New()
		sw                 = &ServerHandler{}
		expectedAuthHeader = []string{"Basic xxxxxxx"}
	)

	viper.Set("authHeader", expectedAuthHeader)
	if _, err := NewPrimaryHandler(l, viper, sw, nil, provider.NewDiscardProvider(), candlelight.TraceConfig{}); err != nil {
		t.Fatalf("NewPrimaryHandler failed: %v", err)
	}

}

func TestGetValidator(t *testing.T) {
	assert := assert.New(t)

	fakeViper := viper.New()

	t.Run("TestAuthHeaderNotSet", func(t *testing.T) {
		validator, err := getValidator(fakeViper)

		assert.Nil(err)

		validators := validator.(secure.Validators)
		assert.Equal(0, len(validators))
	})

	t.Run("TestAuthHeaderSet", func(t *testing.T) {
		expectedAuthHeader := []string{"Basic xxxxxxx"}
		fakeViper.Set("authHeader", expectedAuthHeader)

		validator, err := getValidator(fakeViper)

		assert.Nil(err)

		validators := validator.(secure.Validators)
		assert.Equal(1, len(validators))

		exactMatchValidator := validators[0].(secure.ExactMatchValidator)
		assert.Equal(expectedAuthHeader[0], string(exactMatchValidator))
	})
}

/*
Simply tests that no bad requests make it to the caduceus listener.
*/
func TestMuxServerConfig(t *testing.T) {
	assert := assert.New(t)

	logger := logging.DefaultLogger()
	fakeHandler := new(mockHandler)
	fakeHandler.On("HandleRequest", mock.AnythingOfType("int"),
		mock.AnythingOfType("*wrp.Message")).Return().Once()

	fakeEmptyRequests := new(mockCounter)
	fakeErrorRequests := new(mockCounter)
	fakeInvalidCount := new(mockCounter)

	fakeQueueDepth := new(mockGauge)
	fakeQueueDepth.On("Add", mock.AnythingOfType("float64")).Return().Times(2)

	serverWrapper := &ServerHandler{
		Logger:                   logger,
		caduceusHandler:          fakeHandler,
		errorRequests:            fakeErrorRequests,
		emptyRequests:            fakeEmptyRequests,
		incomingQueueDepthMetric: fakeQueueDepth,
		invalidCount:             fakeInvalidCount,
	}

	authHandler := handler.AuthorizationHandler{Validator: nil}
	caduceusHandler := alice.New(authHandler.Decorate)

	router := configServerRouter(mux.NewRouter(), caduceusHandler, serverWrapper, nil, provider.NewDiscardProvider())

	t.Run("TestMuxResponseCorrectMSP", func(t *testing.T) {
		req := exampleRequest("1234", "application/msgpack", "/api/v3/notify")

		req.Header.Set("Content-Type", "application/msgpack")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		resp := w.Result()
		if nil != resp.Body {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
		assert.Equal(http.StatusAccepted, resp.StatusCode)
	})
}
