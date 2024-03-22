package main

import (
	"fmt"
	"time"

	"github.com/xmidt-org/webpa-common/v2/semaphore"
	"github.com/xmidt-org/wrp-go/v3"
)

type SinkSenderV2 struct {
	placeholder string
	CommonSink
}

func (s *SinkSenderV2) Update(l Listener) error {
	//TODO: unmarshal r
	if _, ok := l.(*ListenerV2); !ok {
		return fmt.Errorf("Invalid Listener for Sink Sender V2")
	}
	return nil
}

func (s *SinkSenderV2) Shutdown(b bool) {

}

func (s *SinkSenderV2) Queue(msg *wrp.Message) {

}

func (s *SinkSenderV2) RetiredSince() time.Time {
	return time.Now()
}
func (s *SinkSenderV2) SetCommonSink(cs CommonSink) {
	s.id = cs.id
	s.maxWorkers = cs.maxWorkers
	s.deliveryRetries = cs.deliveryRetries
	s.logger = cs.logger
	s.queue = cs.queue
	s.queueSize = cs.queueSize
	s.cutOffPeriod = cs.cutOffPeriod
	s.workers = semaphore.New(cs.maxWorkers)
	s.deliveryInterval = cs.deliveryInterval
	s.wg.Add(1)
}

func (s *SinkSenderV2) SetMetrics(sm SinkMetrics) {
	s.SinkMetrics = sm
}

func (s *SinkSenderV2) SetFailureMessage(fm FailureMessage) {
	s.failureMsg = fm
}
