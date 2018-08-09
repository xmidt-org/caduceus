# Dockerized Caduceus
Docker containers make life super easy. The steps below are to mainly help with
testings.


./build
docker run -itd -p 6000:6000 -p 6001:6001 -p 6002:6002 -v /etc/caduceus:temp/ --name caduceus caduceus:local

## Installation
- [Docker](https://www.docker.com/) (duh)
  - `brew install docker`

</br>

## Running
### Build the docker image
```bash
cd $workDir # aka the clone of this project
export GOPATH=`pwd`
cd src/
glide install --strip-vendor
cd ../.docker/
./build.sh
```
This `build.sh` script will build the binary and docker image

### Run the image
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

## Complete
Once you are done kill all the docker containers.
</br>
`docker rm $(docker stop $(docker ps -aq))`
