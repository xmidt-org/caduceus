// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package kinesis

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"go.uber.org/zap"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const SomeStream = "some-stream"

var logger = zap.NewExample()

var shardId = "shardId"
var sequenceNumber = "123"

var putRecordOutput = kinesis.PutRecordOutput{
	EncryptionType: types.EncryptionTypeNone,
	ShardId:        &shardId,
	SequenceNumber: &sequenceNumber,
}

type mockKinesis struct{ mock.Mock }

func newMockKinesis() *mockKinesis { return &mockKinesis{} }

func (m *mockKinesis) PutRecords(ctx context.Context, input *kinesis.PutRecordsInput, optFuns ...func(*kinesis.Options)) (*kinesis.PutRecordsOutput, error) {
	args := m.Called(ctx, input, optFuns)
	return args.Get(0).(*kinesis.PutRecordsOutput), args.Error(1)
}

func (m *mockKinesis) PutRecord(ctx context.Context, input *kinesis.PutRecordInput, optFuns ...func(*kinesis.Options)) (*kinesis.PutRecordOutput, error) {
	m.Called(ctx, input, optFuns)
	return &putRecordOutput, nil
}

func (m *mockKinesis) GetRecords(ctx context.Context, input *kinesis.GetRecordsInput, optFuns ...func(*kinesis.Options)) (*kinesis.GetRecordsOutput, error) {
	m.Called(ctx, input, optFuns)

	return nil, nil
}

func (m *mockKinesis) DescribeStream(ctx context.Context, input *kinesis.DescribeStreamInput, optFuns ...func(*kinesis.Options)) (*kinesis.DescribeStreamOutput, error) {
	m.Called(ctx, input, optFuns)

	return nil, nil
}

func (m *mockKinesis) GetShardIterator(ctx context.Context, input *kinesis.GetShardIteratorInput, optFuns ...func(*kinesis.Options)) (*kinesis.GetShardIteratorOutput, error) {
	m.Called(ctx, input, optFuns)

	return nil, nil
}

func TestNewNoAwsRole(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "accessKey")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secretKey")

	cfg := Config{
		Region:   "na",
		Endpoint: "http://localhost",
	}

	kc, err := New(&cfg, logger)
	kclient := kc.(*KinesisClient)

	assert.Nil(t, err)
	assert.NotNil(t, kclient.svc)
}

type KinesisSuite struct {
	suite.Suite
	kc          KinesisClient
	mockKinesis *mockKinesis
}

func TestKinesisSuite(t *testing.T) {
	suite.Run(t, new(KinesisSuite))
}

func (suite *KinesisSuite) SetupTest() {
	m := newMockKinesis()
	kc := KinesisClient{
		logger: logger,
		svc:    m,
		config: &Config{
			Region:   "na",
			Endpoint: "http://localhost",
		},
	}
	suite.kc = kc
	suite.mockKinesis = m
}

func (suite *KinesisSuite) TestPutRecords() {
	items := []Item{{PartitionKey: "mac1", Item: []byte("test1")}, {PartitionKey: "mac2", Item: []byte("test2")}}
	stream := SomeStream

	failedRecordInputCount := int32(10)
	putRecordsOutput := &kinesis.PutRecordsOutput{
		EncryptionType:    types.EncryptionTypeNone,
		FailedRecordCount: &failedRecordInputCount,
	}

	suite.mockKinesis.On("PutRecords", mock.Anything, mock.Anything, mock.Anything).Return(putRecordsOutput, nil)

	failedRecordCount, err := suite.kc.PutRecords(items, stream)
	suite.T().Log(err)
	suite.Nil(err)
	suite.Equal(10, failedRecordCount)
}

func (suite *KinesisSuite) TestPutRecordsError() {
	items := []Item{{PartitionKey: "mac1", Item: []byte("test1")}, {PartitionKey: "mac2", Item: []byte("test2")}}
	stream := SomeStream

	failedRecordInputCount := int32(0)
	putRecordsOutput := &kinesis.PutRecordsOutput{
		EncryptionType:    types.EncryptionTypeNone,
		FailedRecordCount: &failedRecordInputCount,
	}

	suite.mockKinesis.On("PutRecords", mock.Anything, mock.Anything, mock.Anything).Return(putRecordsOutput, errors.New("some db error"))

	failedRecordCount, err := suite.kc.PutRecords(items, stream)
	suite.T().Log(err)
	suite.NotNil(err)
	suite.Equal(0, failedRecordCount)
}

func (suite *KinesisSuite) TestPutRecordsErrorNilOutput() {
	items := []Item{{PartitionKey: "mac1", Item: []byte("test1")}, {PartitionKey: "mac2", Item: []byte("test2")}}
	stream := SomeStream

	putRecordsOutput := (*kinesis.PutRecordsOutput)(nil)

	suite.mockKinesis.On("PutRecords", mock.Anything, mock.Anything, mock.Anything).Return(putRecordsOutput, errors.New("some db error"))

	failedRecordCount, err := suite.kc.PutRecords(items, stream)
	suite.T().Log(err)
	suite.NotNil(err)
	suite.Equal(0, failedRecordCount)
}

func (suite *KinesisSuite) TestPutRecord() {
	event := []byte("test")
	stream := SomeStream
	partitionKey := "some-key"

	suite.mockKinesis.On("PutRecord", mock.Anything, mock.Anything, mock.Anything).Return(&putRecordOutput)

	_, err := suite.kc.PutRecord(event, stream, partitionKey)
	suite.T().Log(err)
	suite.Nil(err)
}
