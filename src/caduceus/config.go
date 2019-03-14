package main

import "time"

// Below is the struct we're using to contain the data from a provided config file
// TODO: Try to figure out how to make bucket ranges configurable
type CaduceusConfig struct {
	AuthHeader       []string
	NumWorkerThreads int
	JobQueueSize     int
	Sender           SenderConfig
	JWTValidators    []JWTValidator
}

type SenderConfig struct {
	NumWorkersPerSender   int
	QueueSizePerSender    int
	CutOffPeriod          time.Duration
	Linger                time.Duration
	ClientTimeout         time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	DeliveryRetries       int
	DeliveryInterval      time.Duration
}
