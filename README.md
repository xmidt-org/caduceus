# caduceus
(pronounced "kuh-doo-see-us")

[![Build Status](https://github.com/xmidt-org/caduceus/actions/workflows/ci.yml/badge.svg)](https://github.com/xmidt-org/caduceus/actions/workflows/ci.yml)
[![Dependency Updateer](https://github.com/xmidt-org/caduceus/actions/workflows/updater.yml/badge.svg)](https://github.com/xmidt-org/caduceus/actions/workflows/updater.yml)
[![codecov.io](http://codecov.io/github/xmidt-org/caduceus/coverage.svg?branch=main)](http://codecov.io/github/xmidt-org/caduceus?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/xmidt-org/caduceus)](https://goreportcard.com/report/github.com/xmidt-org/caduceus)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=xmidt-org_caduceus&metric=alert_status)](https://sonarcloud.io/dashboard?id=xmidt-org_caduceus)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/xmidt-org/caduceus/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/release/xmidt-org/caduceus.svg)](CHANGELOG.md)

## Summary
The [XMiDT](https://xmidt.io/) server for delivering events from talaria to the
registered consumer. Refer the [overview docs](https://xmidt.io/docs/introduction/overview/)
for more information on how caduceus fits into the overall picture.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Details](#details)
- [Usage](#usage)
- [Build](#build)
- [Deploy](#deploy)
- [Contributing](#contributing)

## Code of Conduct

This project and everyone participating in it are governed by the [XMiDT Code Of Conduct](https://xmidt.io/code_of_conduct/). 
By participating, you agree to this Code.

## Details
Caduceus has one function: to deliver events to a consumer.
To enable this, caduceus has three endpoints: 1) receive events, 2) register webhooks,
and 3) get webhooks.

#### Notify - `api/v3/notify` endpoint
The notify endpoint will accept a `msgpack` encoding of a [WRP Message](https://github.com/xmidt-org/wrp-c/wiki/Web-Routing-Protocol).
If a webhook is registered and matches the device regex and event regex, the event
will be sent to the webhook's registered url. To register a webhook, refer to
the [webhook registration section](#webhook---hook-endpoint)

#### Webhook - `/hook` endpoint
To register a webhook and get events, the consumer must send an http POST request to caduceus
that includes the http url for receiving the events and a list of regex filters.
The following is an example request. Note: this is not a valid json because of the added comments.
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
  # Warning: This is a very expensive webhook registration. This is not recommended for production.
  # This registration will be sent all events.
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
* `make docker`: fetches all dependencies from source and builds a caduceus 
   docker image
* `make local-docker`: vendors dependencies and builds a caduceus docker image
   (recommended for local testing)
* `make test`: runs unit tests with coverage for caduceus
* `make clean`: deletes previously-built binaries and object files

### RPM

First have a local clone of the source and go into the root directory of the 
repository.  Then use rpkg to build the rpm:
```bash
rpkg srpm --spec <repo location>/<spec file location in repo>
rpkg -C <repo location>/.config/rpkg.conf sources --outdir <repo location>'
```

### Docker

The docker image can be built either with the Makefile or by running a docker
command.  Either option requires first getting the source code.

See [Makefile](#Makefile) on specifics of how to build the image that way.

If you'd like to build it without make, follow these instructions based on your use case:

- Local testing
```bash
go mod vendor
docker build -t caduceus:local -f deploy/Dockerfile .
```
This allows you to test local changes to a dependency. For example, you can build 
a caduceus image with the changes to an upcoming changes to [webpa-common](https://github.com/xmidt-org/webpa-common) by using the [replace](https://golang.org/ref/mod#go) directive in your go.mod file like so:
```
replace github.com/xmidt-org/webpa-common v1.10.2 => ../webpa-common
```
**Note:** if you omit `go mod vendor`, your build will fail as the path `../webpa-common` does not exist on the builder container.

- Building a specific version
```bash
git checkout v0.4.0
docker build -t caduceus:v0.4.0 -f deploy/Dockerfile .
```

**Additional Info:** If you'd like to stand up a XMiDT docker-compose cluster, read [this](https://github.com/xmidt-org/xmidt/blob/master/deploy/docker-compose/README.md).

### Kubernetes

A helm chart can be used to deploy caduceus to kubernetes
```
helm install xmidt-caduceus deploy/helm/caduceus
```

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
