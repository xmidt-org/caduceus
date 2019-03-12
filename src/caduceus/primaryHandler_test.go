package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/secure"
	"github.com/Comcast/webpa-common/secure/handler"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewPrimaryHandler(t *testing.T) {
	var (
		l     = logging.New(nil)
		viper = viper.New()
		sw    = &ServerHandler{}
	)

	if _, err := NewPrimaryHandler(l, viper, sw); err != nil {
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

	router := configServerRouter(mux.NewRouter(), caduceusHandler, serverWrapper)

	t.Run("TestMuxResponseCorrectMSP", func(t *testing.T) {
		req := exampleRequest("1234", "application/msgpack", "/api/v3/notify")

		req.Header.Set("Content-Type", "application/msgpack")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusAccepted, resp.StatusCode)
	})
}
