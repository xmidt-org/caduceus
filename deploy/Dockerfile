FROM docker.io/library/golang:1.14-alpine as builder

MAINTAINER Jack Murdock <jack_murdock@comcast.com>

WORKDIR /go/src/github.com/xmidt-org/caduceus

ARG VERSION=unknown
ARG GITCOMMIT=unknown
ARG BUILDTIME=unknown

ADD https://github.com/geofffranks/spruce/releases/download/v1.25.2/spruce-linux-amd64 /usr/local/bin/spruce
RUN chmod +x /usr/local/bin/spruce

RUN apk add --no-cache --no-progress \
    ca-certificates \
    make \
    git \
    openssh \
    gcc \
    libc-dev \
    upx

COPY . .
RUN make build

FROM alpine:latest

COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/src/github.com/xmidt-org/caduceus/caduceus.yaml /caduceus.yaml
COPY --from=builder /go/src/github.com/xmidt-org/caduceus/caduceus /caduceus
COPY --from=builder /go/src/github.com/xmidt-org/caduceus/deploy/Dockerfile /go/src/github.com/xmidt-org/caduceus/NOTICE /go/src/github.com/xmidt-org/caduceus/LICENSE /go/src/github.com/xmidt-org/caduceus/CHANGELOG.md /
COPY --from=builder /go/src/github.com/xmidt-org/caduceus/deploy/packaging/entrypoint.sh /entrypoint.sh
COPY --from=builder /go/src/github.com/xmidt-org/caduceus/deploy/packaging/caduceus_spruce.yaml /tmp/caduceus_spruce.yaml
COPY --from=builder /usr/local/bin/spruce /spruce

RUN mkdir /etc/caduceus/ && touch /etc/caduceus/caduceus.yaml && chmod 666 /etc/caduceus/caduceus.yaml

USER nobody

ENTRYPOINT ["/entrypoint.sh"]

EXPOSE 6000
EXPOSE 6001
EXPOSE 6002
EXPOSE 6003

CMD ["/caduceus"]