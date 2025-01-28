# Connecting Kafka

## Summary
Kafka is an event streaming service which we will be sending events to via caduceus. In order to send events we need to connect Caduceus to both Kafka and tr1d1um. First we will set up on Kafka environment which will include a topic and broker(s). In production we are not concerned with the consumer, but for testing environments we will also set up a consumer to see the messages being sent via Caduceus. Once Kafka is up and running we can run tr1d1um and start registering webhooks. After a webhook is registered we will send a wrp message to Caduceus which will place the message on a queue and then be handled by the producer to send to the Kafka topic we created earlier. 


## Requirements
Go (https://go.dev/doc/install)
Java
Kafka (https://www.apache.org/dyn/closer.cgi?path=/kafka/3.8.0/kafka_2.13-3.8.0.tgz)
Docker (https://docs.docker.com/get-started/get-docker/)
Caduceus (https://github.com/xmidt-org/caduceus/tree/denopink/feat/rewrite)
Tr1d1um (https://github.com/xmidt-org/tr1d1um/tree/feat/caduceus-updates)
API Platform (i.e Insomnia) or curl

## Pre-Setup - Kafka
After downloading Kafka run `tar -xzf kafka_2.13-3.8.0.tgz` to unzip the file 

## Seting up and Running - Caduceus
```
authHeader: ["$BASIC_AUTH_TOKEN"]

webhook:
  # JWTParserType establishes which parser type will be used by the JWT token
  # acquirer used by Argus. Options include 'simple' and 'raw'.
  # Simple: parser assumes token payloads have the following structure: https://github.com/xmidt-org/bascule/blob/c011b128d6b95fa8358228535c63d1945347adaa/acquire/bearer.go#L77
  # Raw: parser assumes all of the token payload == JWT token
  # (Optional). Defaults to 'simple'.
  # jwtParserType: "raw"
  basicClientConfig:
    # listen is the subsection that configures the listening feature of the argus client
    # (Optional)
    # listen:
    #   # pullInterval provides how often the current webhooks list gets refreshed.
    #   pullInterval: 5s

    # bucket is the partition name where webhooks will be stored.
    bucket: "webhooks"

    # address is Argus' network location.
    address: "$ARGUS_CD_URL"

    # auth the authentication method for argus.
    auth:
      # basic configures basic authentication for argus.
      # Must be of form: 'Basic xyz=='
      basic: "Basic $BASIC_AUTH_URL"
  
     # jwt configures jwt style authentication for argus.
      jwt:
       # requestHeaders are added to the request for the token.
       # (Optional)
        requestHeaders: 
          "X-Client-Id": "$CLIENT_ID"
          "X-Client-Secret": "$CLIENT_SECRET"
  
       # authURL is the URL to access the token.
        authUrl: "$GET_AUTH_URL"
  
       # timeout is how long the request to get the token will take before
       # timing out.
        timeout: "1m"
  
       # buffer is the length of time before a token expires to get a new token.
        buffer: "2m"
```
In `caduceus.yaml` add use the same `$BASIC_AUTH_TOKEN` and add to `authHeader`

In the terminal `cd cmd` then run `go run .` or `go run . -d` to see the logs

## Setting up and Running - Tr1d1um
```
 #JWTParserType should remain commented out
  # JWTParserType: "raw"
  BasicClientConfig:
    # listen is the subsection that configures the listening feature of the argus client
    # (Optional)
    listen:
      # pullInterval provides how often the current webhooks list gets refreshed.
      pullInterval: 5s

    # bucket is the partition name where webhooks will be stored.
    bucket: "webhooks"

    # address is Argus' network location.
    address: "$ARGUS_CD_URL"

    # auth the authentication method for argus.
    auth:
      # basic configures basic authentication for argus.
      # Must be of form: 'Basic xyz=='
      basic: "Basic $BASIC_AUTH_URL"
  #
  #    # jwt configures jwt style authentication for argus.
      JWT:
       # requestHeaders are added to the request for the token.
       # (Optional)
        requestHeaders:
          "X-Client-Id": "$CLIENT_ID"
          "X-Client-Secret": "$CLIENT_SECRET"
  
       # authURL is the URL to access the token.
        authURL: "$SAT_AUTH_URL"
  
       # timeout is how long the request to get the token will take before
       # timing out.
        timeout: "1m"
  
       # buffer is the length of time before a token expires to get a new token.
        buffer: "2m"
jwtValidator:
  config:
    resolve:
      # Template is a URI template used to fetch keys.  This template may
      # use a single parameter named keyID, e.g. http://keys.com/{keyID}.
      # This field is required and has no default.
      template: "$SAT_KEYID_URL"
    refresh:
      sources:
        # URI is the location where keys are served.  By default, clortho supports
        # file://, http://, and https:// URIs, as well as standard file system paths
        # such as /etc/foo/bar.jwk.
        #
        # This field is required and has no default.
        - uri: "$SAT_SIGN_KEYS_URL"
authx:
  inbound:
    # basic is a list of Basic Auth credentials intended to be used for local testing purposes
    # WARNING! Be sure to remove this from your production config
    basic: ["$BASIC_AUTH"]
```

In terminal run `go run .`

### Setting up and Running Kafka - Single Node
1. From your terminal cd to where your kafka_2.13-3.8.0 folder is located
2. `docker pull apache/kafka:3.8.0` (make sure docker desktop is running)
3. `docker run -p 9092:9092 apache/kafka:3.8.0`
4. create a topic `bin/kafka-topics.sh --create --topic quickstart-events --bootstrap-server localhost:9092`
5.read the events `bin/kafka-console-consumer.sh --topic quickstart-events --from-beginning --bootstrap-server localhost:9092` 

** `quickstart-events` is currently being hardcoded - need to discuss with owen but will probably add webhook-schema **

#### Sample Request - Tr1d1um: `/hook`
```
curl --request POST \
  --url http://localhost:6100/api/v3/hook \
  --header 'Authorization: Bearer $BEARERTOKEN' \
  --data '{
	"registered_from_address": "10.166.207.91:55406",
	"contact_info": {
		"name": "maura fortino",
		"phone": "1234567890",
		"email": "maurafortino@testemail.com"
	},
	"canonical_name": "test_kafka",
	"kafkas": [
		{
			"accept": "application/json",
			"bootstrap_servers": [
				"localhost:9092"
			]
		}
	],
	"failure_url": "https://qa-basin.xmidt.comcast.net:443/api/v1/failure_is_an_option",
	"matcher": [
		{
			"field": "DeviceId",
			"regex": "[a-z0-9]"
		}
	],
	"expires": "$EXPIRATIONDATE"
}'
```
#### Sample Request - Caduceus: `/notify`
```curl --request POST \
  --url http://localhost:443/api/v4/notify \
  --header 'Authorization: Bearer `$BEARER_TOKEN`' \
  --header 'Content-Type: application/msgpack' \
  --header 'User-Agent: insomnia/9.3.3' \
  --data `$WRPMsgPack`
  ```

  #### Sample response - Kafka terminal
  *In the Kafka terminal the consumer should be running in the background after running `bin/kafka-console-consumer.sh --topic quickstart-events --from-beginning --bootstrap-server localhost:9092`*
  ** shsould see the msg pack come through **
  


### Setting up and Running Kafka - Mutli Node
For multi node you'll need two terminals for caduceus (one to run caduceus and one to run the docker-compose file), one terminal for tr1d1um, and one terminal for each node (broker) in the docker-compose file
1. In terminal from the root directory of caduceus run `IMAGE=apache/kafka:latest docker compose -f docker-compose.yml up`
2. In another terminal cd into kafka_2.13-3.8.0 folder(repeat this step for each node (broker) you have)
3. create a topic `bin/kafka-topics.sh --create --topic quickstart-events --bootstrap-server localhost:29092` (only needs to be done in one terminal)
4.read the events `bin/kafka-console-consumer.sh --topic quickstart-events --from-beginning --bootstrap-server localhost:29092` (repeat this step in the other two terminals replacing localhost:29092 with localhost:39092 and localhost:49092)

#### Sample Request - Tr1d1um: `/hook`
```
curl --request POST \
  --url http://localhost:6100/api/v3/hook \
  --header 'Authorization: Bearer $BEARERTOKEN' \
  --data '{
	"registered_from_address": "10.166.207.91:55406",
	"contact_info": {
		"name": "maura fortino",
		"phone": "1234567890",
		"email": "maurafortino@testemail.com"
	},
	"canonical_name": "test_kafka",
	"kafkas": [
		{
			"accept": "application/json",
			"bootstrap_servers": [
				"localhost:29092",
                "localhost:39092",
                "localhost:49092"
			]
		}
	],
	"failure_url": "https://qa-basin.xmidt.comcast.net:443/api/v1/failure_is_an_option",
	"matcher": [
		{
			"field": "DeviceId",
			"regex": "[a-z0-9]"
		}
	],
	"expires": "$EXPIRATIONDATE"
}'
```

#### Sample Request - Caduceus: `/notify`
```curl --request POST \
  --url http://localhost:443/api/v4/notify \
  --header 'Authorization: Bearer $BEARTOKEN' \
  --header 'Content-Type: application/msgpack' \
  --header 'User-Agent: insomnia/9.3.3' \
  --data `$WRPMsgPack`
  ```

#### Sample Repsonse - Kafka
  *In the Kafka terminal the consumer should be running in the background after running `bin/kafka-console-consumer.sh --topic quickstart-events --from-beginning --bootstrap-server localhost:9092`*
  ** shsould see the msg pack come through **
  