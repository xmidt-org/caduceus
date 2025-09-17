// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const testEvent = "event:test"

type StreamDispatcherSuite struct {
	suite.Suite
	obs                    *streamOutboundSender
	caduceusOutboundSender *CaduceusOutboundSender
	dispatcher             *StreamDispatcher
	mockStreamSender       *mockStreamSender
	mockMetrics            *OutboundSenderMetrics
}

func TestStreamDispatcherSuite(t *testing.T) {
	suite.Run(t, new(StreamDispatcherSuite))
}

func (suite *StreamDispatcherSuite) SetupTest() {
	mockStreamSender := new(mockStreamSender)
	trans := &transport{}
	obs, err := simpleFactorySetup(trans, time.Second, nil, true).New()
	suite.NoError(err)
	dispatcher, err := NewStreamDispatcher(obs.(*streamOutboundSender).obs) // TODO - this is an awkward pattern
	suite.NoError(err)
	suite.mockStreamSender = mockStreamSender
	suite.dispatcher = dispatcher.(*StreamDispatcher)
	suite.dispatcher.sender = mockStreamSender
	suite.obs = obs.(*streamOutboundSender)
	suite.caduceusOutboundSender = suite.obs.obs
	mockMetrics := getMockMetrics()
	suite.caduceusOutboundSender.metrics = mockMetrics
	suite.mockMetrics = &mockMetrics
}

func getMockMetrics() OutboundSenderMetrics {
	fakeDC := new(mockCounter)
	fakeDC.On("With", mock.Anything).Return(fakeDC)
	fakeDC.On("Add", mock.Anything).Return()

	// test slow metric
	fakeSlow := new(mockCounter)
	fakeSlow.On("Add", mock.Anything).Return()
	fakeSlow.On("With", mock.Anything).Return(fakeSlow)

	// test dropped metric
	fakeDroppedSlow := new(mockCounter)
	fakeDroppedSlow.On("Add", mock.Anything).Return()
	fakeDroppedSlow.On("With", mock.Anything).Return(fakeDroppedSlow)

	// IncomingContentType cases
	fakeContentType := new(mockCounter)
	fakeContentType.On("Add", mock.Anything).Return()
	fakeContentType.On("With", mock.Anything).Return(fakeContentType)

	// QueueDepth case
	fakeQdepth := new(mockGauge)
	fakeQdepth.On("Add", mock.Anything).Return()
	fakeQdepth.On("Set", mock.Anything).Return()
	fakeQdepth.On("With", mock.Anything).Return(fakeQdepth)

	// Fake Latency
	fakeLatency := new(mockHistogram)
	fakeLatency.On("Observe", mock.Anything).Return()

	return OutboundSenderMetrics{
		queryLatency:          fakeLatency,
		deliveryCounter:       fakeDC,
		deliveryRetryCounter:  fakeDC,
		droppedMessage:        fakeDroppedSlow,
		cutOffCounter:         fakeSlow,
		queueDepthGauge:       fakeQdepth,
		renewalTimeGauge:      fakeQdepth,
		deliverUntilGauge:     fakeQdepth,
		dropUntilGauge:        fakeQdepth,
		maxWorkersGauge:       fakeQdepth,
		currentWorkersGauge:   fakeQdepth,
		deliveryRetryMaxGauge: fakeQdepth,
	}
}

func (suite *StreamDispatcherSuite) TestSend() {
	args := suite.mockStreamSender.On("OnEvent", mock.Anything, mock.Anything).Return(0, nil)
	msg := simpleRequestWithPartnerIDs()
	msg.Destination = "testEvent"

	suite.obs.obs.workers.Acquire()
	suite.dispatcher.Send(suite.obs.obs.urls, "", "", msg)

	args.Parent.AssertCalled(suite.T(), "OnEvent", mock.Anything, mock.Anything)
}

func (suite *StreamDispatcherSuite) TestSendError() {
	fakeDC := suite.mockMetrics.deliveryCounter.(*mockCounter)
	metricArgs := fakeDC.On("With", mock.Anything).Return(fakeDC)

	args := suite.mockStreamSender.On("OnEvent", mock.Anything, mock.Anything).Return(0, errors.New("network_err"))
	msg := simpleRequestWithPartnerIDs()
	msg.Destination = testEvent

	suite.obs.obs.workers.Acquire()
	suite.dispatcher.Send(suite.obs.obs.urls, "", "", msg)

	args.Parent.AssertCalled(suite.T(), "OnEvent", mock.Anything, mock.Anything)
	labels := metricArgs.Parent.Calls[0].Arguments[0].(prometheus.Labels)
	suite.Equal("message_dropped", labels[codeLabel])
	suite.Equal("send_error", labels[reasonLabel])
}

func (suite *StreamDispatcherSuite) TestSomeFailedRecordsError() {
	fakeDC := suite.mockMetrics.deliveryCounter.(*mockCounter)
	metricArgs := fakeDC.On("With", mock.Anything).Return(fakeDC)

	args := suite.mockStreamSender.On("OnEvent", mock.Anything, mock.Anything).Return(1, nil)
	msg := simpleRequestWithPartnerIDs()
	msg.Destination = "testEvent"

	suite.obs.obs.workers.Acquire()
	suite.dispatcher.Send(suite.obs.obs.urls, "", "", msg)

	args.Parent.AssertCalled(suite.T(), "OnEvent", mock.Anything, mock.Anything)
	labels := metricArgs.Parent.Calls[0].Arguments[0].(prometheus.Labels)
	suite.Equal("message_dropped", labels[codeLabel])
	suite.Equal("some_records_failed", labels[reasonLabel])
}

func (suite *StreamDispatcherSuite) TestQueueOverflow() {
	fakeSlow := suite.mockMetrics.cutOffCounter.(*mockCounter)
	metricArgs := fakeSlow.On("With", mock.Anything).Return(fakeSlow)

	suite.dispatcher.QueueOverflow()
	metricArgs.Parent.AssertCalled(suite.T(), "Add", 1.0)
}
