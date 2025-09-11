// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/wrp-go/v3"
)

type StreamOutboundSenderSuite struct {
	suite.Suite
	streamOutboundSender *streamOutboundSender
	mockDispatcher       *mockDispatcher
}

func TestStreamOutboundSenderSuite(t *testing.T) {
	suite.Run(t, new(StreamOutboundSenderSuite))
}

func (suite *StreamOutboundSenderSuite) SetupTest() {
	trans := &transport{}
	obs, err := simpleFactorySetup(trans, time.Second, nil, true).New()
	mockDispatcher := new(mockDispatcher)

	suite.NotNil(obs)
	suite.Nil(err)
	suite.streamOutboundSender = obs.(*streamOutboundSender)
	suite.streamOutboundSender.obs.dispatcher = mockDispatcher
	suite.mockDispatcher = mockDispatcher
}

func (suite *StreamOutboundSenderSuite) TestRetiredSince() {
	suite.Greater(suite.streamOutboundSender.RetiredSince(), time.Now().UTC())
}

func (suite *StreamOutboundSenderSuite) TestShutdown() {
	suite.streamOutboundSender.Shutdown(true)

	time.Sleep(100 * time.Millisecond)

	_, ok := <-suite.streamOutboundSender.obs.queue.Load().(chan *wrp.Message)
	suite.False(ok)
}

func (suite *StreamOutboundSenderSuite) TestQueue() {
	suite.mockDispatcher.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	req := simpleRequestWithPartnerIDs()
	req.Destination = "event:iot"
	suite.streamOutboundSender.Queue(req)
	time.Sleep(100 * time.Millisecond)
	suite.mockDispatcher.AssertCalled(suite.T(), "Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func (suite *StreamOutboundSenderSuite) TestUpdate() {
	suite.streamOutboundSender.Update(ancla.InternalWebhook{})
}
