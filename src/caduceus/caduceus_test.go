/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
package main

import (
	"net/http"
	"net/http/httptest"
	"os"
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

func TestMain(m *testing.M) {
	os.Exit(m.Run())
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
