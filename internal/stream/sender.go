// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: LicenseRef-COMCAST

package stream

import (
	"context"
	"encoding/json"
	"time"

	"github.com/xmidt-org/caduceus/internal/kinesis"
	"github.com/xmidt-org/retry"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

const schemaVersion = "1.1"
const retries = 3

type StreamEventSender struct {
	kc            kinesis.KinesisClientAPI
	url           string
	schemaVersion string
	logger        *zap.Logger
}

type EventSender interface {
	OnEvent(event []*wrp.Message) (int, error)
	GetUrl() string
}

var kPutRunner, _ = retry.NewRunner[int](
	retry.WithPolicyFactory[int](retry.Config{
		Interval:   10 * time.Millisecond,
		MaxRetries: retries,
	}),
)

func New(url string, version string, kc kinesis.KinesisClientAPI, logger *zap.Logger) (EventSender, error) {
	if schemaVersion == "" {
		version = schemaVersion
	}
	return &StreamEventSender{
		kc:            kc,
		url:           url,
		schemaVersion: version,
		logger:        logger,
	}, nil
}

func (s *StreamEventSender) GetUrl() string {
	return s.url
}

// TODO - add a queue and a channel instead
func (s *StreamEventSender) OnEvent(msgs []*wrp.Message) (int, error) {
	items := []kinesis.Item{}
	for _, m := range msgs {
		data, err := json.Marshal(m)
		if err != nil {
			s.logger.Error("error marshaling statusEvent", zap.Any("event", m), zap.Error(err))
			continue
		}
		items = append(items, kinesis.Item{Item: data, PartitionKey: m.TransactionUUID})
	}

	attempts := 0
	failedRecordCount, err := kPutRunner.Run(
		context.Background(),
		func(_ context.Context) (int, error) {
			attempts++
			failedRecordCount, err := s.kc.PutRecords(items, s.url)
			if err != nil {
				s.logger.Error("kinesis.PutRecords error", zap.Int("attempt", attempts), zap.Error(err))
			}

			return failedRecordCount, err
		},
	)

	return failedRecordCount, err
}
