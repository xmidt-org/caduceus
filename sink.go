package main

import (
	"fmt"
	"time"

	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

type SinkI interface {
	Update(Listener) error
	Shutdown(bool)
	Queue(*wrp.Message)
}

type Sink struct {
}

func (s *Sink) Update(l Listener) (err error) {
	switch v := l.(type) {
	case *ListenerV1:
		m := &MatcherV1{}
		if err = m.Update(*v); err != nil {
			return
		}
		s.matcher = m
	default:
		err = fmt.Errorf("invalid listner")
	}

	s.renewalTimeGauge.Set(float64(time.Now().Unix()))

	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.deliverUntil = l.GetUntil()
	s.deliverUntilGauge.Set(float64(s.deliverUntil.Unix()))
	s.deliveryRetryMaxGauge.Set(float64(s.deliveryRetries))

	s.id = l.GetId()
	s.listener = l
	s.failureMessage.Original = l

	// Update this here in case we make this configurable later
	s.maxWorkersGauge.Set(float64(s.maxWorkers))

	return
}

// Shutdown causes the CaduceusOutboundSender to stop its activities either gently or
// abruptly based on the gentle parameter.  If gentle is false, all queued
// messages will be dropped without an attempt to send made.
func (s *Sink) Shutdown(gentle bool) {
	if !gentle {
		// need to close the channel we're going to replace, in case it doesn't
		// have any events in it.
		close(s.queue.Load().(chan *wrp.Message))
		s.Empty(s.droppedExpiredCounter)
	}
	close(s.queue.Load().(chan *wrp.Message))
	s.wg.Wait()

	s.mutex.Lock()
	s.deliverUntil = time.Time{}
	s.deliverUntilGauge.Set(float64(s.deliverUntil.Unix()))
	s.queueDepthGauge.Set(0) //just in case
	s.mutex.Unlock()
}

// Queue is given a request to evaluate and optionally enqueue in the list
// of messages to deliver.  The request is checked to see if it matches the
// criteria before being accepted or silently dropped.
// TODO: can pass in message along with webhook information
func (s *Sink) Queue(msg *wrp.Message) {
	s.mutex.RLock()
	deliverUntil := s.deliverUntil
	dropUntil := s.dropUntil
	s.mutex.RUnlock()

	now := time.Now()

	if !s.isValidTimeWindow(now, dropUntil, deliverUntil) {
		s.logger.Debug("invalid time window for event", zap.Any("now", now), zap.Any("dropUntil", dropUntil), zap.Any("deliverUntil", deliverUntil))
		return
	}

	if !s.disablePartnerIDs {
		if len(msg.PartnerIDs) == 0 {
			msg.PartnerIDs = s.customPIDs
		}
		if !overlaps(s.listener.GetPartnerIds(), msg.PartnerIDs) {
			s.logger.Debug("parter id check failed", zap.Strings("webhook.partnerIDs", s.listener.GetPartnerIds()), zap.Strings("event.partnerIDs", msg.PartnerIDs))
			return
		}
		if ok := s.matcher.IsMatch(msg); !ok {
			return
		}

	}

	//TODO: this code will be changing will take away queue and send to the sink interface (not webhook interface)
	select {
	case s.queue.Load().(chan *wrp.Message) <- msg:
		s.queueDepthGauge.Add(1.0)
		s.logger.Debug("event added to outbound queue", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
	default:
		s.logger.Debug("queue full. event dropped", zap.String("event.source", msg.Source), zap.String("event.destination", msg.Destination))
		s.queueOverflow()
		s.droppedQueueFullCounter.Add(1.0)
	}
}
