// SPDX-FileCopyrightText: 2024 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package sink

// func TestNewSink(t *testing.T) {
// 	tests := []struct {
// 		description string
// 		config      Config
// 		listener    ancla.Register
// 		expected    Sink
// 	}{
// 		{
// 			description: "RegistryV1 - success",
// 			listener: &ancla.RegistryV1{
// 				Registration: webhook.RegistrationV1{
// 					Config: webhook.DeliveryConfig{
// 						ReceiverURL: "www.example.com",
// 					},
// 				},
// 			},
// 			config: Config{
// 				DeliveryInterval: 5 * time.Minute,
// 				DeliveryRetries:  3,
// 			},
// 			expected: &WebhookV1{
// 				id:               "www.example.com",
// 				deliveryInterval: 5 * time.Minute,
// 				deliveryRetries:  3,
// 				logger:           logger,
// 			},
// 		},
// 		{
// 			description: "default case",
// 			listener:    &ancla.RegistryV1{},
// 			expected:    nil,
// 		},
// 	}

// 	for _, tc := range tests {
// 		t.Run(tc.description, func(t *testing.T) {
// 			sink := NewSink(tc.config, logger, tc.listener)
// 			assert.Equal(t, tc.expected, sink)
// 		})
// 	}
// }

// func TestNewSink(t *testing.T) {
// 	tests := []struct {
// 		description string
// 		config      Config
// 		listener    ancla.Register
// 		expected    Sink
// 	}{
// 		{
// 			description: "RegistryV1 - success",
// 			listener: &ancla.RegistryV1{
// 				Registration: webhook.RegistrationV1{
// 					Config: webhook.DeliveryConfig{
// 						ReceiverURL: "www.example.com",
// 					},
// 				},
// 			},
// 			config: Config{
// 				DeliveryInterval: 5 * time.Minute,
// 				DeliveryRetries:  3,
// 			},
// 			expected: &WebhookV1{
// 				id:               "www.example.com",
// 				deliveryInterval: 5 * time.Minute,
// 				deliveryRetries:  3,
// 				logger:           logger,
// 			},
// 		},
// 		{
// 			description: "default case",
// 			listener:    &ancla.RegistryV1{},
// 			expected:    &WebhookV1{logger: logger},
// 		},
// 	}

// 	for _, tc := range tests {
// 		t.Run(tc.description, func(t *testing.T) {
// 			sink := NewSink(tc.config, logger, tc.listener)
// 			assert.Equal(t, tc.expected, sink)
// 		})
// 	}
// }

// func TestUpdateSink(t *testing.T) {
// 	listener := &ancla.RegistryV1{
// 		Registration: webhook.RegistrationV1{
// 			Config: webhook.DeliveryConfig{
// 				ReceiverURL: "www.example.com",
// 			},
// 		},
// 	}
// 	expected := &WebhookV1{
// 		id: "www.example.com",
// 	}
// 	err := expected.Update(listener)
// 	assert.NoError(t, err)
// }

// func TestSend(t *testing.T) {
// 	tests := []struct {
// 		description   string
// 		urls          *ring.Ring
// 		secret        string
// 		acceptType    string
// 		msg           *wrp.Message
// 		expectedError error
// 		webhook       WebhookV1
// 	}{
// 		{
// 			description: "success",
// 			secret:      "test_secret",
// 			acceptType:  "application/msgpack",
// 			msg: &wrp.Message{
// 				Type:        wrp.SimpleEventMessageType,
// 				Source:      "mac:00deadbeef00",
// 				Destination: "mac:112233445566",
// 				ContentType: "application/json",
// 				Accept:      "json",
// 				Payload:     []byte("here is a lovely little payload that the device understands"),
// 				PartnerIDs:  []string{"hello", "world"},
// 				QualityOfService: 99,
// 			},
// 		},
// 	}
// }
