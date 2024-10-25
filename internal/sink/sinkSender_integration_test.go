// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package sink

import (
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/IBM/sarama"
	m "github.com/IBM/sarama/mocks"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/webhook-schema"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap/zaptest"
)

func TestKafkaSink(t *testing.T) {
	logger := zaptest.NewLogger(t)
	require := require.New(t)

	producer := NewMockProducer(t)
	consumer := NewMockConsumer(t)

	config := Config{
		NumWorkersPerSender: 5000,
		CutOffPeriod:        10,
		Linger:              180,
		IsTest:              true,
	}
	mac, err := generateMacAddress()
	canonicalMac := net.HardwareAddr(mac).String()
	now := time.Now()

	listener := &ancla.RegistryV2{
		PartnerIds: []string{"comcast"},
		Registration: webhook.RegistrationV2{
			ContactInfo: webhook.ContactInfo{
				Name:  "Maura Test",
				Phone: "012-345-6789",
				Email: "integration@test.com",
			},
			CanonicalName: "integration_test",
			Address:       canonicalMac,
			Kafkas: []webhook.Kafka{
				{
					Accept:           "application/json",
					BootstrapServers: []string{"localhost:9092"},
				},
			},
			Hash: webhook.FieldRegex{
				Field: "Destination",
				Regex: "[a-z0-9]",
			},
			FailureURL: "https://qa-basin.xmidt.comcast.net:443/api/v1/failure_is_an_option",
			Matcher: []webhook.FieldRegex{
				{
					Field: "DeviceId",
					Regex: "[a-z0-9]",
				},
			},
			Expires: now.Add(1 * 24 * time.Hour),
		},
	}
	sender := &sender{
		id:                "integration_test",
		listener:          listener,
		queueSize:         5,
		cutOffPeriod:      10,
		deliveryRetries:   3,
		deliveryInterval:  time.Duration(15) * time.Second,
		maxWorkers:        5000,
		logger:            logger,
		customPIDs:        []string{"comcast"},
		disablePartnerIDs: true,
		config: config,
	}
	sender.Update(listener)

	now = now.Add(15 * time.Second)

	payload := fmt.Sprintf("{\"ts\":\"%s\"}", now.UTC().Format(time.RFC3339Nano))
	ksession, err := ksuid.NewRandomWithTime(now)
	require.NoError(err)
	sessionID := ksession.String()

	msg := &wrp.Message{
		Type:       wrp.SimpleEventMessageType,
		Source:     fmt.Sprintf("mac:%x", mac),
		PartnerIDs: []string{"comcast"},
		SessionID:  sessionID,
		Metadata: map[string]string{
			"/hw-model":              "QA1682",
			"/hw-last-reboot-reason": "unknown",
			"/fw-name":               "2.364s2",
			"/random-value":          "random",
			"/last-reboot-reason":    "unknown",
		},
		Payload: []byte(payload),
	}

	sender.Queue(msg)
	time.Sleep(time.Duration(5) * time.Second)
	sender.sink.Send("", "application/wrp", msg)

}

func NewMockProducer(t *testing.T) *m.SyncProducer {
	return m.NewSyncProducer(t, sarama.NewConfig())
}
func NewMockConsumer(t *testing.T) *m.Consumer {
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	//TODO: this is basic set up for now - will need to add more options to config
	//once we know what we are allowing users to send

	return m.NewConsumer(t, config)

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
