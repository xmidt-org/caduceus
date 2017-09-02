package main

import (
	"os"
	"testing"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/secure/handler"
	"github.com/stretchr/testify/mock"
	"net/http/httptest"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/stretchr/testify/assert"
	"net/http"
	"errors"
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
		mock.AnythingOfType("CaduceusRequest")).Return().Once()

	fakeHealth := new(mockHealthTracker)
	fakeHealth.On("IncrementBucket", mock.AnythingOfType("int")).Return().Once()

	forceTimeOut := func(func(workerID int)) error {
		return errors.New("time out!")
	}

	serverWrapper := &ServerHandler{
		Logger:          logger,
		caduceusHandler: fakeHandler,
		caduceusHealth:  fakeHealth,
		doJob:           forceTimeOut,
	}

	authHandler := handler.AuthorizationHandler{Validator:nil}
	caduceusHandler := alice.New(authHandler.Decorate)
	router := configServerRouter(mux.NewRouter(), caduceusHandler, serverWrapper)

	req := httptest.NewRequest("POST", "/api/v3/notify", nil)
	req.Header.Set("Content-Type", "application/json")

	t.Run("TestMuxResponseCorrectJSON", func(t *testing.T) {
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusRequestTimeout, resp.StatusCode)
	})

	t.Run("TestMuxResponseCorrectMSP", func(t *testing.T) {
		req.Header.Set("Content-Type", "application/msgpack")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusRequestTimeout, resp.StatusCode)
	})


	t.Run("TestMuxResponseManyHeaders", func(t *testing.T) {
		req.Header.Add("Content-Type", "too/many/headers")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusNotFound, resp.StatusCode)
	})

	t.Run("TestServeHTTPNoContentType", func(t *testing.T) {
		req.Header.Del("Content-Type")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusNotFound, resp.StatusCode)
	})

	t.Run("TestServeHTTPBadContentType", func(t *testing.T) {
		req.Header.Set("Content-Type", "something/unsupported")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		resp := w.Result()

		assert.Equal(http.StatusNotFound, resp.StatusCode)
	})
}