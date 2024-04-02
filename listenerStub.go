// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"time"

	webhook "github.com/xmidt-org/webhook-schema"
)

// This is a stub for the webhook and kafka listeners. This will be removed once the webhook-schema configuration is approved
type Listener interface {
	GetId() string
	GetPartnerIds() []string
	GetUntil() time.Time
}
type ListenerV1 struct {
	PartnerIds   []string
	Registration RegistrationV1
}

type ListenerV2 struct {
	PartnerIds   []string
	Registration RegistrationV2
}

// Webhook is a substructure with data related to event delivery.
type Webhook struct {
	// Accept is the encoding type of outgoing events. The following encoding types are supported, otherwise
	// a 406 response code is returned: application/octet-stream, application/json, application/jsonl, application/msgpack.
	// Note: An `Accept` of application/octet-stream or application/json will result in a single response for batch sizes of 0 or 1
	// and batch sizes greater than 1 will result in a multipart response. An `Accept` of application/jsonl or application/msgpack
	// will always result in a single response with a list of batched events for any batch size.
	Accept string `json:"accept"`

	// AcceptEncoding is the content type of outgoing events. The following content types are supported, otherwise
	// a 406 response code is returned: gzip.
	AcceptEncoding string `json:"accept_encoding"`

	// Secret is the string value.
	// (Optional, set to "" to disable behavior).
	Secret string `json:"secret,omitempty"`

	// SecretHash is the hash algorithm to be used. Only sha256 HMAC and sha512 HMAC are supported.
	// (Optional).
	// The Default value is the largest sha HMAC supported, sha512 HMAC.
	SecretHash string `json:"secret_hash"`

	// If true, response will use the device content-type and wrp payload as its body
	// Otherwise, response will Accecpt as the content-type and wrp message as its body
	// Default: False (the entire wrp message is sent)
	PayloadOnly bool `json:"payload_only"`

	// ReceiverUrls is the list of receiver urls that will be used where as if the first url fails,
	// then the second url would be used and so on.
	// Note: either `ReceiverURLs` or `DNSSrvRecord` must be used but not both.
	ReceiverURLs []string `json:"receiver_urls"`

	// DNSSrvRecord is the substructure for configuration related to load balancing.
	// Note: either `ReceiverURLs` or `DNSSrvRecord` must be used but not both.
	DNSSrvRecord struct {
		// FQDNs is a list of FQDNs pointing to dns srv records
		FQDNs []string `json:"fqdns"`

		// LoadBalancingScheme is the scheme to use for load balancing. Either the
		// srv record attribute `weight` or `priortiy` can be used.
		LoadBalancingScheme string `json:"load_balancing_scheme"`
	} `json:"dns_srv_record"`
}

// Kafka is a substructure with data related to event delivery.
type Kafka struct {
	// Accept is content type value to set WRP messages to (unless already specified in the WRP).
	Accept string `json:"accept"`

	// BootstrapServers is a list of kafka broker addresses.
	BootstrapServers []string `json:"bootstrap_servers"`

	// TODO: figure out which kafka configuration substructures we want to expose to users (to be set by users)
	// going to be based on https://pkg.go.dev/github.com/IBM/sarama#Config
	// this substructures also includes auth related secrets, noted `MaxOpenRequests` will be excluded since it's already exposed
	KafkaProducer struct{} `json:"kafka_producer"`
}

type BatchHint struct {
	// MaxLingerDuration is the maximum delay for batching if MaxMesasges has not been reached.
	// Default value will set no maximum value.
	MaxLingerDuration time.Duration `json:"max_linger_duration"`
	// MaxMesasges is the maximum number of events that will be sent in a single batch.
	// Default value will set no maximum value.
	MaxMesasges int `json:"max_messages"`
}

// FieldRegex is a substructure with data related to regular expressions.
type FieldRegex struct {
	// Field is the wrp field to be used for regex.
	// All wrp field can be used, refer to the schema for examples.
	Field string `json:"field"`

	// FieldRegex is the regular expression to match `Field` against to.
	Regex string `json:"regex"`
}

type ContactInfo struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
}

// RegistrationV2 is a special struct for unmarshaling sink information as part of a sink registration request.
type RegistrationV2 struct {
	// ContactInfo contains contact information used to reach the owner of the registration.
	// (Optional).
	ContactInfo ContactInfo `json:"contact_info,omitempty"`

	// CanonicalName is the canonical name of the registration request.
	// Reusing a CanonicalName will override the configurations set in that previous
	// registration request with the same CanonicalName.
	CanonicalName string `json:"canonical_name"`

	// Address is the subscription request origin HTTP Address.
	Address string `json:"registered_from_address"`

	// Webhooks contains data to inform how events are delivered to multiple urls.
	Webhooks []Webhook `json:"webhooks"`

	// Kafkas contains data to inform how events are delivered to multiple kafkas.
	Kafkas []Kafka `json:"kafkas"`

	// Hash is a substructure for configuration related to distributing events among sinks.
	// Note. Any failures due to a bad regex feild or regex expression will result in a silent failure.
	Hash FieldRegex `json:"hash"`

	// BatchHint is the substructure for configuration related to event batching.
	// (Optional, if omited then batches of singal events will be sent)
	// Default value will disable batch. All zeros will also disable batch.
	BatchHint BatchHint `json:"batch_hints"`

	// FailureURL is the URL used to notify subscribers when they've been cut off due to event overflow.
	// Optional, set to "" to disable notifications.
	FailureURL string `json:"failure_url"`

	// Matcher is the list of regular expressions to match incoming events against to.
	// Note. Any failures due to a bad regex feild or regex expression will result in a silent failure.
	Matcher []FieldRegex `json:"matcher,omitempty"`

	// Expires describes the time this subscription expires.
	// TODO: list of supported formats
	Expires time.Time `json:"expires"`
}

// Deprecated: This structure should only be used for backwards compatibility
// matching. Use RegistrationV2 instead.
// RegistrationV1 is a special struct for unmarshaling a webhook as part of a webhook registration request.
type RegistrationV1 struct {
	// Address is the subscription request origin HTTP Address.
	Address string `json:"registered_from_address"`

	// Config contains data to inform how events are delivered.
	Config DeliveryConfig `json:"config"`

	// FailureURL is the URL used to notify subscribers when they've been cut off due to event overflow.
	// Optional, set to "" to disable notifications.
	FailureURL string `json:"failure_url"`

	// Events is the list of regular expressions to match an event type against.
	Events []string `json:"events"`

	// Matcher type contains values to match against the metadata.
	Matcher MetadataMatcherConfig `json:"matcher,omitempty"`

	// Duration describes how long the subscription lasts once added.
	Duration webhook.CustomDuration `json:"duration"`

	// Until describes the time this subscription expires.
	Until time.Time `json:"until"`
}

// MetadataMatcherConfig is Webhook substructure with config to match event metadata.
type MetadataMatcherConfig struct {
	// DeviceID is the list of regular expressions to match device id type against.
	DeviceID []string `json:"device_id"`
}

// Deprecated: This substructure should only be used for backwards compatibility
// matching. Use Webhook instead.
// DeliveryConfig is a Webhook substructure with data related to event delivery.
type DeliveryConfig struct {
	// URL is the HTTP URL to deliver messages to.
	ReceiverURL string `json:"url"`

	// ContentType is content type value to set WRP messages to (unless already specified in the WRP).
	ContentType string `json:"content_type"`

	// Secret is the string value for the SHA1 HMAC.
	// (Optional, set to "" to disable behavior).
	Secret string `json:"secret,omitempty"`

	// AlternativeURLs is a list of explicit URLs that should be round robin through on failure cases to the main URL.
	AlternativeURLs []string `json:"alt_urls,omitempty"`
}

func (v1 *ListenerV1) GetId() string {
	return v1.Registration.Config.ReceiverURL
}
func (v1 *ListenerV1) GetPartnerIds() []string {
	return v1.PartnerIds
}

func (v1 *ListenerV1) GetUntil() time.Time {
	return v1.Registration.Until
}
func (v2 *ListenerV2) GetId() string {
	return v2.Registration.CanonicalName
}

func (v2 *ListenerV2) GetPartnerIds() []string {
	return v2.PartnerIds
}

func (v2 *ListenerV2) GetUntil() time.Time {
	return v2.Registration.Expires
}
