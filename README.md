# caduceus
(pronounced "kuh-doo-see-uhs")

[![Build Status](https://travis-ci.com/xmidt-org/caduceus.svg?branch=master)](https://travis-ci.com/xmidt-org/caduceus)
[![codecov.io](http://codecov.io/github/xmidt-org/caduceus/coverage.svg?branch=master)](http://codecov.io/github/xmidt-org/caduceus?branch=master)
[![Code Climate](https://codeclimate.com/github/xmidt-org/caduceus/badges/gpa.svg)](https://codeclimate.com/github/xmidt-org/caduceus)
[![Issue Count](https://codeclimate.com/github/xmidt-org/caduceus/badges/issue_count.svg)](https://codeclimate.com/github/xmidt-org/caduceus)
[![Go Report Card](https://goreportcard.com/badge/github.com/xmidt-org/caduceus)](https://goreportcard.com/report/github.com/xmidt-org/caduceus)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/xmidt-org/caduceus/blob/master/LICENSE)
[![GitHub release](https://img.shields.io/github/release/xmidt-org/caduceus.svg)](CHANGELOG.md)

## Summary
The [XMiDT](https://xmidt.io/) server for delivering events from talaria to the
registered consumer. Refer the [overview docs](https://xmidt.io/docs/introduction/overview/)
for more information on how caduceus fits into the overall picture.

## Details
Caduceus has one function: to deliver events to a consumer.
To enable this caduceus has three endpoints: 1) receive events, 2) register webhooks,
and 3) get webhooks.

#### Notify - `api/v3/notify` endpoint
The notify endpoint will accept a `msgpack` encoding of a [WRP Messages](https://github.com/xmidt-org/wrp-c/wiki/Web-Routing-Protocol).
If a webhook is registered and matches the device regex and event regex the event
will be sent to the corresponding url. To register a webhook refer to
the [webhook registration section](#notify)

#### Webhook - `/hook` endpoint
To register a webhook, the consumer must send an http POST request to caduceus
including the http url for receiving the events and a list of regex filters to get events.
The following is an example request. Note: this is not a valid json since I added comments.
```
{
  "config" : {
    # The url to push events to.
    "url" : "http://localhost:8080/webhook",

    # The content type event.
    # (Optional) defaults to msgpack.
    "content_type" : "application/json",

    # The secret used for SHA1 HMAC.
    # Used to validate the received payload.
    # (Optional) defaults to no sha1 hmac.
    "secret" : "secret",

    # On failure how many times to retry the url for the event.
    # (Optional) defaults to the configured value on the server
    "max_retry_count": 3,

    # Alternative urls to send requests too. A list of server urls to use in
    # round robin for sending events.
    # (Optional) defaults to no alternative urls.
    "alt_urls" : [
      "http://localhost:8080/webhook"
    ]
  },

  # The URL to notify when we cut off a client due to overflow.
  # (Optional) defaults no notification of error.
  "failure_url" : "http://localhost:8080/webhook-failure",

  # The list of regular expressions to match event type against.
  "events" : [
    ".*"
  ],

  # matcher type contains values to match against the metadata.
  # currently only device_id is supported.
  # (Optional) defaults to all devices.
  "matcher" : {
    "device_id": [
      ".*"
    ]
  }
}
```

#### Get Webhooks - `/hooks` endpoint
To speed up caduceus start up time and testing the registration of webhooks the
`/hooks` endpoint was created. This is a simple `GET` requests which will return
all the webhooks and their configuration.

## Usage
Once everything is up and running you can start sending requests. Bellow are
a few examples.

#### Create a hook
```bash
curl -X POST \
  http://localhost:6000/hook \
  -H 'Authorization: Basic YXV0aEhlYWRlcg==' \
  -H 'Content-Type: application/json' \
  -d '{
  "config": {
		"url" : "http://127.0.0.1:9090/webhook2",
		"content_type" : "application/json"
  },
  "events": [
    ".*"
  ],
  "duration": 120,
  "address": "127.0.0.1"
}'
```

#### Send an Event
```bash
curl -X POST \
  http://localhost:6000/api/v3/notify \
  -H 'Authorization: Basic YXV0aEhlYWRlcg==' \
  -H 'Content-Type: application/msgpack' \
  --data "@msg.bin"
```
where `msg.bin` is a msgpack encoded json

#### List Hooks
```bash
curl -X GET \
  http://localhost:6000/hooks \
  -H 'Authorization: Basic YXV0aEhlYWRlcg=='
```

#### Get Health
```bash
curl http://localhost:6001/health
```

#### GET some [pprof](https://golang.org/pkg/net/http/pprof/) stats
```bash
curl http://localhost:6002/debug/pprof/mutex
```


## Build

### Source

In order to build from the source, you need a working Go environment with
version 1.11 or greater. Find more information on the [Go website](https://golang.org/doc/install).

You can directly use `go get` to put the caduceus binary into your `GOPATH`:
```bash
GO111MODULE=on go get github.com/xmidt-org/caduceus
```

You can also clone the repository yourself and build using make:

```bash
mkdir -p $GOPATH/src/github.com/xmidt-org
cd $GOPATH/src/github.com/xmidt-org
git clone git@github.com:xmidt-org/caduceus.git
cd caduceus
make build
```

### Makefile

The Makefile has the following options you may find helpful:
* `make build`: builds the caduceus binary
* `make rpm`: builds an rpm containing caduceus
* `make docker`: builds a docker image for caduceus, making sure to get all
   dependencies
* `make local-docker`: builds a docker image for caduceus with the assumption
   that the dependencies can be found already
* `make test`: runs unit tests with coverage for caduceus
* `make clean`: deletes previously-built binaries and object files

### Docker

The docker image can be built either with the Makefile or by running a docker
command.  Either option requires first getting the source code.

See [Makefile](#Makefile) on specifics of how to build the image that way.

For running a command, either you can run `docker build` after getting all
dependencies, or make the command fetch the dependencies.  If you don't want to
get the dependencies, run the following command:
```bash
docker build -t caduceus:local -f deploy/Dockerfile .
```
If you want to get the dependencies then build, run the following commands:
```bash
GO111MODULE=on go mod vendor
docker build -t caduceus:local -f deploy/Dockerfile.local .
```

For either command, if you want the tag to be a version instead of `local`,
then replace `local` in the `docker build` command.

### Kubernetes

WIP. TODO: add info

## Deploy

For deploying a XMiDT cluster refer to [getting started](https://xmidt.io/docs/operating/getting_started/).

For running locally, ensure you have the binary [built](#Source).  If it's in
your `GOPATH`, run:
```
caduceus
```
If the binary is in your current folder, run:
```
./caduceus
```

## Contributing

Refer to [CONTRIBUTING.md](CONTRIBUTING.md).

Refer to [CONTRIBUTING.md](CONTRIBUTING.md).
