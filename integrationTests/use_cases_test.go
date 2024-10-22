// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package integrationtests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/caduceus/internal/sink"
)

type aTest struct {
	broken            bool
	eventTime         time.Time
	numOutputExpected int
	list              []item
	senders           func([]sink.Sender)
}

type item struct {
	origin   string
	dest      string
	partition int32
	id        string
	offset    int64
	delay     time.Duration
	partnerid []string
}

func TestNewSenders(t *testing.T) {
	// assert := assert.New(t)
	require := require.New(t)
	runIt(t, aTest{
		numOutputExpected: 3,
		senders: func(senders []sink.Sender) {
			require.Equal(3, len(senders))
		},
	})
}
