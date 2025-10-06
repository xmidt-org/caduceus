// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package kinesis

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/xmidt-org/caduceus/internal/batch"
	"go.uber.org/zap"
)

const MaxPutRecordsBatchSize = 500

type KinesisClientAPI interface {
	PutRecord(event []byte, stream string, partitionKey string) (*kinesis.PutRecordOutput, error)
	PutRecords(items []Item, stream string) (int, error)
	GetRecords(stream string) (*kinesis.GetRecordsOutput, error)
}

type KinesisAPI interface {
	PutRecord(context.Context, *kinesis.PutRecordInput, ...func(*kinesis.Options)) (*kinesis.PutRecordOutput, error)
	PutRecords(context.Context, *kinesis.PutRecordsInput, ...func(*kinesis.Options)) (*kinesis.PutRecordsOutput, error)
	GetRecords(context.Context, *kinesis.GetRecordsInput, ...func(*kinesis.Options)) (*kinesis.GetRecordsOutput, error)
	GetShardIterator(context.Context, *kinesis.GetShardIteratorInput, ...func(*kinesis.Options)) (*kinesis.GetShardIteratorOutput, error)
	DescribeStream(context.Context, *kinesis.DescribeStreamInput, ...func(*kinesis.Options)) (*kinesis.DescribeStreamOutput, error)
}

type KinesisClient struct {
	svc           KinesisAPI
	logger        *zap.Logger
	config        *Config
	credsExpireAt time.Time
}

type Config struct {
	Region      string `validate:"empty=false"`
	Endpoint    string `validate:"empty=false"`
	Role        string
	Credentials Credentials
}

type Credentials struct {
	AccessKey    string
	SecretKey    string
	SessionToken string
}

type Item struct {
	PartitionKey string
	Item         []byte
}

func New(cfg *Config, logger *zap.Logger) (KinesisClientAPI, error) {
	k, credsExpireAt, err := getClient(cfg, logger)
	if err != nil {
		return nil, err
	}

	return &KinesisClient{
		svc:           k,
		logger:        logger,
		config:        cfg,
		credsExpireAt: credsExpireAt,
	}, nil
}

func getClient(cfg *Config, logger *zap.Logger) (KinesisAPI, time.Time, error) {
	ctx := context.Background()

	// try removing this, not sure why we need it
	if cfg.Role != "" &&
		(cfg.Credentials.AccessKey != "" ||
			cfg.Credentials.SecretKey != "" ||
			cfg.Credentials.SessionToken != "") {
		return nil, time.Time{}, fmt.Errorf("cannot specify both role and credentials")
	}

	var credsExpireAt time.Time
	var awsConfig aws.Config
	var err error

	logger.Debug("kinesis role is", zap.String("role", cfg.Role))

	if cfg.Role != "" { // we need to authenticate with aws and assume a cross-account role

		// config to retrieve sts credentials
		awsConfig, err = config.LoadDefaultConfig(
			ctx,
			config.WithRegion(cfg.Region),
			config.WithRetryMaxAttempts(3),
			//config.WithCredentialsChainVerboseErrors(true),
			config.WithHTTPClient(
				&http.Client{Timeout: 10 * time.Second},
			),
		)
		if err != nil {
			logger.Error("unable to load aws config for sts credentials", zap.String("role", cfg.Role), zap.Error(err))
			return nil, time.Time{}, err
		}

		// retrieve credentials for cross-account role
		client := sts.NewFromConfig(awsConfig)
		creds := stscreds.NewAssumeRoleProvider(client, cfg.Role)
		stsResponse, err := creds.Retrieve(ctx)
		if err != nil {
			logger.Error("unable to authenticate kinesis assumed role", zap.String("role", cfg.Role), zap.Error(err))
			return nil, time.Time{}, err
		}

		// create config for kinesis client with cross account credentials
		awsConfig, err = config.LoadDefaultConfig(
			ctx,
			config.WithRegion(cfg.Region),
			config.WithBaseEndpoint(cfg.Endpoint),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				stsResponse.AccessKeyID,
				stsResponse.SecretAccessKey,
				stsResponse.SessionToken),
			),
		)
		if err != nil {
			logger.Error("unable to create kinesis client config", zap.String("role", cfg.Role), zap.Error(err))
			return nil, time.Time{}, err
		}

		credsExpireAt = stsResponse.Expires
	} else {
		// create config to talk to local kinesis
		var err error
		awsConfig, err = config.LoadDefaultConfig(
			ctx,
			config.WithRegion(cfg.Region),
			config.WithBaseEndpoint(cfg.Endpoint),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				cfg.Credentials.AccessKey,
				cfg.Credentials.SecretKey,
				cfg.Credentials.SessionToken,
			),
			),
		)
		if err != nil {
			logger.Error("unable to create local kinesis client config", zap.String("role", cfg.Role), zap.Error(err))
			return nil, time.Time{}, err
		}

		// if cfg.Credentials.AccessKey != "" ||
		// 	cfg.Credentials.SecretKey != "" ||
		// 	cfg.Credentials.SessionToken != "" {

		// 	awsConfig = awsConfig.WithCredentials(
		// 		credentials.NewStaticCredentials(
		// 			cfg.Credentials.AccessKey,
		// 			cfg.Credentials.SecretKey,
		// 			cfg.Credentials.SessionToken,
		// 		))
		// }
	}

	k := kinesis.NewFromConfig(awsConfig)

	return k, credsExpireAt, nil
}

func (kc *KinesisClient) PutRecord(event []byte, stream string, partitionKey string) (*kinesis.PutRecordOutput, error) {
	kc.logger.Debug("putting event to kinesis", zap.String("event", string(event)))

	kc.refreshClient()

	putOutput, err := kc.svc.PutRecord(context.Background(), &kinesis.PutRecordInput{
		Data:         event,
		StreamName:   &stream,
		PartitionKey: aws.String(partitionKey),
	})

	if err != nil {
		kc.logger.Error("PutRecord failed", zap.Error(err))
		return putOutput, err
	}

	kc.logger.Debug("Published Event", zap.Any("result", putOutput), zap.String("endpoint", kc.config.Endpoint))
	return putOutput, nil
}

func (kc *KinesisClient) PutRecords(items []Item, stream string) (int, error) {
	notDone := true
	start := 0
	failedRecordCount := int32(0)
	for notDone {
		batch, more := batch.GetBatch(start, MaxPutRecordsBatchSize, items)
		notDone = more
		start += MaxPutRecordsBatchSize

		kc.logger.Debug("putting events to kinesis", zap.Int("size", len(items)))

		records := []types.PutRecordsRequestEntry{}
		for _, item := range batch {
			entry := types.PutRecordsRequestEntry{
				Data:         item.Item,
				PartitionKey: aws.String(item.PartitionKey),
			}
			records = append(records, entry)
		}

		kc.refreshClient()
		putOutput, err := kc.svc.PutRecords(context.Background(), &kinesis.PutRecordsInput{
			Records:    records,
			StreamName: &stream,
		})

		if putOutput != nil {
			failedRecordCount += *putOutput.FailedRecordCount
		}

		if err != nil {
			kc.logger.Error("PutRecords failed", zap.Error(err))
			return int(failedRecordCount), err
		}
		kc.logger.Debug("Published Records", zap.Any("result", putOutput), zap.String("endpoint", kc.config.Endpoint))
	}

	return int(failedRecordCount), nil
}

// get all records - for integration testing, only, not used in production

func (kc *KinesisClient) GetRecords(stream string) (*kinesis.GetRecordsOutput, error) {
	kc.logger.Debug("getting records", zap.String("endpoint", kc.config.Endpoint))

	// get starting sequence number first, then shard iterator for that
	// retrieve iterator
	shardId := "shardId-000000000000"

	description, err := kc.svc.DescribeStream(context.Background(), &kinesis.DescribeStreamInput{
		StreamName: aws.String(stream),
	})

	if err != nil {
		kc.logger.Error("error describing stream", zap.Error(err))
		return nil, err
	}
	if description == nil {
		return nil, fmt.Errorf("description is nil")
	}
	if description.StreamDescription == nil {
		return nil, fmt.Errorf("stream description is nil")
	}
	if len(description.StreamDescription.Shards) == 0 {
		return nil, fmt.Errorf("no shards found")
	}
	if description.StreamDescription.Shards[0].SequenceNumberRange == nil {
		return nil, fmt.Errorf("sequence number range is nil")
	}
	if description.StreamDescription.Shards[0].SequenceNumberRange.StartingSequenceNumber == nil {
		return nil, fmt.Errorf("starting sequence number is nil")
	}
	sequenceNo := description.StreamDescription.Shards[0].SequenceNumberRange.StartingSequenceNumber

	iteratorOutput, err := kc.svc.GetShardIterator(context.Background(), &kinesis.GetShardIteratorInput{
		// Shard Id is provided when making put record(s) request.
		ShardId: &shardId,
		//ShardIteratorType: aws.String("TRIM_HORIZON"),
		ShardIteratorType:      types.ShardIteratorTypeAtSequenceNumber,
		StartingSequenceNumber: sequenceNo,
		// ShardIteratorType: aws.String("LATEST"),
		StreamName: &stream,
	})

	if err != nil {
		kc.logger.Error("error getting iterator output", zap.Error(err))
		return nil, err
	}

	// get records use shard iterator for making request
	records, err := kc.svc.GetRecords(context.Background(), &kinesis.GetRecordsInput{
		ShardIterator: iteratorOutput.ShardIterator,
		//StreamARN: &streamARN,
	})

	if err != nil {
		kc.logger.Error("error getting records", zap.Error(err))
		panic(err)
	}

	if err != nil {
		kc.logger.Error(fmt.Sprintf("Error: %v", err))
		return records, err
	}

	return records, nil
}

func (kc *KinesisClient) refreshClient() {
	if kc.config.Role != "" {
		if kc.credsExpireAt.Unix() <= time.Now().Add(3*time.Minute).Unix() {
			k, credsExpireAt, err := getClient(kc.config, kc.logger)
			if err != nil {
				kc.logger.Error("error refreshing kinesis client", zap.Error(err))
				return
			}

			// save expiration locally because asking for it is a drain on the metadata server
			kc.credsExpireAt = credsExpireAt
			kc.svc = k
		}
	}
}
