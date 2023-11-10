## SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
## SPDX-License-Identifier: Apache-2.0
.PHONY: default build test style docker binaries clean


DOCKER       ?= docker
APP          := caduceus
DOCKER_ORG   := ghcr.io/xmidt-org

VERSION ?= $(shell git describe --tag --always --dirty)
PROGVER ?= $(shell git describe --tags `git rev-list --tags --max-count=1` | tail -1 | sed 's/v\(.*\)/\1/')
BUILDTIME = $(shell date -u '+%c')
GITCOMMIT = $(shell git rev-parse --short HEAD)
GOBUILDFLAGS = -a -ldflags "-w -s -X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(VERSION)" -o $(APP)

default: build

generate:
	go generate ./...
	go install ./...

test:
	go test -v -race  -coverprofile=coverage.txt ./...
	go test -v -race  -json ./... > report.json

style:
	! gofmt -d $$(find . -path ./vendor -prune -o -name '*.go' -print) | grep '^'

check:
	golangci-lint run -n | tee errors.txt

build:
	CGO_ENABLED=0 go build $(GOBUILDFLAGS)

release: build
	upx $(APP)

docker:
	-$(DOCKER) rmi "$(DOCKER_ORG)/$(APP):$(VERSION)"
	-$(DOCKER) rmi "$(DOCKER_ORG)/$(APP):latest"
	$(DOCKER) build -t "$(DOCKER_ORG)/$(APP):$(VERSION)" -t "$(DOCKER_ORG)/$(APP):latest" .

binaries: generate
	mkdir -p ./.ignore
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o ./.ignore/$(APP)-$(PROGVER).darwin-amd64 -ldflags "-X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(VERSION)"
	CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -o ./.ignore/$(APP)-$(PROGVER).linux-amd64 -ldflags "-X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(VERSION)"

	upx ./.ignore/$(APP)-$(PROGVER).darwin-amd64
	upx ./.ignore/$(APP)-$(PROGVER).linux-amd64

clean:
	-rm -r .ignore/ $(APP) errors.txt report.json coverage.txt


