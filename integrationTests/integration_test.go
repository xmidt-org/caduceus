// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package integrationtests

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/wrp-go/v3"
)

type tr1d1umCall struct {
	path   string
	mux    *chi.Mux
	server *http.Server
}

// testFlags returns "" to run no tests, "all" to run all tests, "broken" to
// run only broken tests, and "working" to run only working tests.
func testFlags() string {
	env := os.Getenv("INTEGRATION_TESTS_RUN")
	env = strings.ToLower(env)
	env = strings.TrimSpace(env)

	switch env {
	case "all":
	case "broken":
	case "":
	default:
		return "working"
	}

	return env
}

func generateMacAddress() ([]byte, error) {
	buf := make([]byte, 6)
	_, err := rand.Read(buf)
	if err != nil {
		return []byte(""), err
	}
	// Set the local bit
	buf[0] |= 2
	return buf, nil
}

func runIt(t *testing.T, tc aTest) {
	assert := assert.New(t)
	require := require.New(t)

	switch testFlags() {
	case "":
		t.Skip("skipping integration test")
	case "all":
	case "broken":
		if !tc.broken {
			t.Skip("skipping non-broken integration test")
		}
	default: // Including working
		if tc.broken {
			t.Skip("skipping broken integration test")
		}
	}

	// To avoid running tests in parallel, set `test.parallel` to `1`
	t.Parallel()

	mac, err := generateMacAddress()
	require.NoError(err)
	if !tc.eventTime.IsZero() {
		now = tc.eventTime
	}
	require.NoError(err)

	req, err := http.NewRequest("POST", "http://localhost:6100/api/v3/hook", nil)
	// Create a recorder to record the response
	rr := httptest.NewRecorder()

	// Create a handler and serve the request
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		payload, err := ioutil.ReadAll(r.Body)
		if err != nil {
			//TODO: add metric?
			fmt.Print(err)
		}
		if len(payload) == 0 {
			//TODO: add metric?
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Empty payload.\n"))
		}
		decoder := wrp.NewDecoderBytes(payload, wrp.Msgpack)
		msg := new(wrp.Message)
		err = decoder.Decode(msg)
		if err != nil || msg.MessageType() != 4 {
			// return a 400
			w.WriteHeader(http.StatusBadRequest)
			if err != nil {
				w.Write([]byte("Invalid payload format.\n"))
			} else {
				w.Write([]byte("Invalid MessageType.\n"))
			}
			return
		}

	})
	handler.ServeHTTP(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check the response body
	expected := "Hello, World!"
	if rr.Body.String() != expected {
		t.Errorf("handler returned wrong body: got %v want %v",
			rr.Body.String(), expected)
	}
	if err != nil {
		t.Fatal(err)
	}

}
