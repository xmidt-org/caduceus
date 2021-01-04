.PHONY: default build test style docker binaries clean


DOCKER       ?= docker
GO           ?= go
GOFMT        ?= $(GO)fmt
APP          := caduceus
DOCKER_ORG   := xmidt

VERSION ?= $(shell git describe --tag --always --dirty)
PROGVER ?= $(shell git describe --tags `git rev-list --tags --max-count=1` | tail -1 | sed 's/v\(.*\)/\1/')
BUILDTIME = $(shell date -u '+%c')
GITCOMMIT = $(shell git rev-parse --short HEAD)
GOBUILDFLAGS = -a -ldflags "-w -s -X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(VERSION)" -o $(APP)

default: build

generate:
	$(GO) generate ./...
	$(GO) install ./...

test:
	$(GO) test -v -race  -coverprofile=coverage.txt ./...
	$(GO) test -v -race  -json ./... > report.json

style:
	! $(GOFMT) -d $$(find . -path ./vendor -prune -o -name '*.go' -print) | grep '^'

check:
	golangci-lint run -n | tee errors.txt

build:
	CGO_ENABLED=0 $(GO) build $(GOBUILDFLAGS)

release: build
	upx $(APP)

docker:
	-$(DOCKER) rmi "$(DOCKER_ORG)/$(APP):$(VERSION)"
	-$(DOCKER) rmi "$(DOCKER_ORG)/$(APP):latest"
	$(DOCKER) build -t "$(DOCKER_ORG)/$(APP):$(VERSION)" -t "$(DOCKER_ORG)/$(APP):latest" .

binaries: generate
	mkdir -p ./.ignore
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -o ./.ignore/$(APP)-$(PROGVER).darwin-amd64 -ldflags "-X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(VERSION)"
	CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 $(GO) build -o ./.ignore/$(APP)-$(PROGVER).linux-amd64 -ldflags "-X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(VERSION)"

	upx ./.ignore/$(APP)-$(PROGVER).darwin-amd64
	upx ./.ignore/$(APP)-$(PROGVER).linux-amd64

clean:
	-rm -r .ignore/ $(APP) errors.txt report.json coverage.txt


