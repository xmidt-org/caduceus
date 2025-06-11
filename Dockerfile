## SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
## SPDX-License-Identifier: Apache-2.0

##########################
# Build an image to download spruce and the ca-certificates.
##########################

FROM docker.io/library/golang:1.23.2-alpine AS builder
WORKDIR /src

RUN apk update && \
    apk add --no-cache --no-progress \
    ca-certificates \
    curl

# Download spruce here to eliminate the need for curl in the final image
RUN mkdir -p /go/bin
RUN curl -Lo /go/bin/spruce https://github.com/geofffranks/spruce/releases/download/v1.31.1/spruce-linux-$(go env GOARCH)
RUN chmod +x /go/bin/spruce
# Error out if spruce download fails by checking the version
RUN /go/bin/spruce --version

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
