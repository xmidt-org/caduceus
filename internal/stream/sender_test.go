// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: LicenseRef-COMCAST

package stream

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"go.uber.org/zap"

	"github.com/segmentio/ksuid"
	"github.com/xmidt-org/wrp-go/v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	mykinesis "github.com/xmidt-org/caduceus/internal/kinesis"
)

type mockKinesisClient struct{ mock.Mock }

func newMockKinesisClient() *mockKinesisClient { return &mockKinesisClient{} }

func (m *mockKinesisClient) PutRecord(event []byte, stream string, partitionKey string) (*kinesis.PutRecordOutput, error) {
	args := m.Called(event, stream, partitionKey)
	pio, _ := args.Get(0).(*kinesis.PutRecordOutput)
	return pio, args.Error(1)
}

func (m *mockKinesisClient) PutRecords(items []mykinesis.Item, stream string) (int, error) {
	args := m.Called(items, stream)
	return args.Get(0).(int), args.Error(1)
}

func (m *mockKinesisClient) GetRecords(stream string) (*kinesis.GetRecordsOutput, error) {
	args := m.Called(stream)
	pio, _ := args.Get(0).(*kinesis.GetRecordsOutput)
	return pio, args.Error(1)
}

func TestOnEvent(t *testing.T) {
	assert := assert.New(t)

	uuid := ksuid.New()

	e := &wrp.Message{
		TransactionUUID: uuid.String(),
	}

	var kc = newMockKinesisClient()

	sender, err := New("", "", kc, zap.NewExample())
	assert.Nil(err)

	mockCall := kc.On("PutRecords", mock.Anything, mock.Anything).Return(0, nil)

	events := []*wrp.Message{e}
	failedRecordCount, err := sender.OnEvent(events)
	assert.Equal(0, failedRecordCount)
	assert.Nil(err)

	items := mockCall.Parent.Calls[0].Arguments.Get(0).([]mykinesis.Item)
	var msg = wrp.Message{}
	err = json.Unmarshal(items[0].Item, &msg)
	assert.NoError(err)

	assert.Equal(uuid.String(), items[0].PartitionKey)
}
