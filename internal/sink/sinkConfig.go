package sink

import "time"

// Below is the struct we're using to contain the data from a provided config file
// TODO: Try to figure out how to make bucket ranges configurable
type SinkConfig struct {
	// The number of workers to assign to each SinkSender created.
	NumWorkersPerSender int

	// The queue size to assign to each SinkSender created.
	QueueSizePerSender int

	// The cut off time to assign to each SinkSender created.
	CutOffPeriod time.Duration

	// The amount of time to let expired SinkSenders linger before
	// shutting them down and cleaning up the resources associated with them.
	Linger time.Duration

	// Number of delivery retries before giving up
	DeliveryRetries int

	// Time in between delivery retries
	DeliveryInterval time.Duration

	// CustomPIDs is a custom list of allowed PartnerIDs that will be used if a message
	// has no partner IDs.
	CustomPIDs []string

	// DisablePartnerIDs dictates whether or not to enforce the partner ID check.
	DisablePartnerIDs bool

	// ClientTimeout specifies a time limit for requests made by the SinkSender's Client
	ClientTimeout time.Duration

	//DisableClientHostnameValidation used in HTTP Client creation
	DisableClientHostnameValidation bool

	// ResponseHeaderTimeout specifies the amount of time to wait for a server's response headers after fully
	// writing the request
	ResponseHeaderTimeout time.Duration

	//IdleConnTimeout is the maximum amount of time an idle
	// (keep-alive) connection will remain idle before closing
	// itself.
	IdleConnTimeout time.Duration
}
