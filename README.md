# caduceus

[![Build Status](https://travis-ci.org/Comcast/caduceus.svg?branch=master)](https://travis-ci.org/Comcast/caduceus) 
[![codecov.io](http://codecov.io/github/Comcast/caduceus/coverage.svg?branch=master)](http://codecov.io/github/Comcast/caduceus?branch=master)
[![Code Climate](https://codeclimate.com/github/Comcast/caduceus/badges/gpa.svg)](https://codeclimate.com/github/Comcast/caduceus)
[![Issue Count](https://codeclimate.com/github/Comcast/caduceus/badges/issue_count.svg)](https://codeclimate.com/github/Comcast/caduceus)
[![Go Report Card](https://goreportcard.com/badge/github.com/Comcast/caduceus)](https://goreportcard.com/report/github.com/Comcast/caduceus)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/Comcast/caduceus/blob/master/LICENSE)
[![GitHub release](https://img.shields.io/github/release/Comcast/caduceus.svg)](CHANGELOG.md)

The Xmidt server for delivering events written in Go.

# How to Install

## Centos 6

1. Import the public GPG key (replace `0.0.1-65` with the release you want)

```
rpm --import https://github.com/Comcast/caduceus/releases/download/0.0.1-65/RPM-GPG-KEY-comcast-xmidt
```

2. Install the rpm with yum (so it installs any/all dependencies for you)

```
yum install https://github.com/Comcast/caduceus/releases/download/0.0.1-65/caduceus-0.0.1-65.el6.x86_64.rpm
```

## Dockerized Caduceus
Docker containers make life super easy.

You will need SNS(or mock SNS) in order for Caduceus to work correctly.

### Installation
- [Docker](https://www.docker.com/) (duh)
  - `brew install docker`

</br>

### Running
#### Build the docker image
```bash
docker build -t caduceus:local .
```
This `build.sh` script will build the binary and docker image

#### Run the image
```bash
docker run -itd -p 6000:6000 -p 6001:6001 -p 6002:6002 -v `pwd`/temp/:/etc/caduceus --name caduceus caduceus:local
```
this assumes you have a folder `temp` in your directory with a configuration file name caduceus aka `caduceus.yaml` with
a port configuration like
```yaml
  primary:
    address: ":6000"
  health:
    address: ":6001"
  pprof:
    address: ":6002"
```

You will need some other configuration to get everything running. Refer to [example-caduceus.yaml](example-caduceus.yaml)
 for an example configuration


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