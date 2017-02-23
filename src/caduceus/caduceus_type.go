package main

import (
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/logging/golog"
	"github.com/Comcast/webpa-common/server"
)

// Below is the struct we're using to contain the data from a provided config file
type Configuration struct {
	AuthHeader    string              `json:"auth_header"`
	LoggerFactory golog.LoggerFactory `json:"log"`
	server.Configuration
}

// Below is the struct that will implement our ServeHTTP method
type ServerHandler struct {
	logger logging.Logger
}
