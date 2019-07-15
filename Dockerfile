FROM golang:alpine as builder
MAINTAINER Jack Murdock <jack_murdock@comcast.com>

ENV build_gopath /go/src/caduceus

# fetch needed software
RUN apk add --update --repository https://dl-3.alpinelinux.org/alpine/edge/testing/ git curl
RUN curl https://glide.sh/get | sh

COPY src/ ${build_gopath}/src

WORKDIR ${build_gopath}/src
ENV GOPATH ${build_gopath} 

# fetch golang dependencies
RUN glide -q install --strip-vendor

# build the binary
WORKDIR ${build_gopath}/src/caduceus
RUN go build -o caduceus_linux_amd64 caduceus

# prep the actual image
FROM alpine:latest
RUN apk --no-cache add ca-certificates
EXPOSE 6100 6101 6102
RUN mkdir -p /etc/caduceus
VOLUME /etc/caduceus
WORKDIR /root/
COPY --from=builder /go/src/caduceus/src/caduceus/caduceus_linux_amd64 .
ENTRYPOINT ["./caduceus_linux_amd64"]
