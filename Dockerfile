## SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
## SPDX-License-Identifier: Apache-2.0
FROM docker.io/library/golang:1.19-alpine as builder

WORKDIR /src

RUN apk add --no-cache --no-progress \
    ca-certificates \
    curl

# Download spruce here to eliminate the need for curl in the final image
RUN mkdir -p /go/bin && \
    curl -L -o /go/bin/spruce https://github.com/geofffranks/spruce/releases/download/v1.29.0/spruce-linux-amd64 && \
    chmod +x /go/bin/spruce

COPY . .

##########################
# Build the final image.
##########################

FROM alpine:latest

# Copy over the standard things you'd expect.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt  /etc/ssl/certs/
COPY caduceus /
COPY .release/docker/entrypoint.sh  /

# Copy over spruce and the spruce template file used to make the actual configuration file.
COPY .release/docker/caduceus_spruce.yaml  /tmp/caduceus_spruce.yaml
COPY --from=builder /go/bin/spruce        /bin/

# Include compliance details about the container and what it contains.
COPY Dockerfile /
COPY NOTICE     /
COPY LICENSE    /

# Make the location for the configuration file that will be used.
RUN     mkdir /etc/caduceus/ \
    &&  touch /etc/caduceus/caduceus.yaml \
    &&  chmod 666 /etc/caduceus/caduceus.yaml

USER nobody

ENTRYPOINT ["/entrypoint.sh"]

EXPOSE 6000
EXPOSE 6001
EXPOSE 6002
EXPOSE 6003

CMD ["/caduceus"]
