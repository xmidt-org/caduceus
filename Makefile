DEFAULT: build

GO           ?= go
GOFMT        ?= $(GO)fmt
APP          := caduceus
DOCKER_ORG   := xmidt
FIRST_GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
BINARY    	 := $(FIRST_GOPATH)/bin/$(APP)

PROGVER = $(shell git describe --tags `git rev-list --tags --max-count=1` | tail -1 | sed 's/v\(.*\)/\1/')
BUILDTIME = $(shell date -u '+%Y-%m-%d %H:%M:%S')
GITCOMMIT = $(shell git rev-parse --short HEAD)

.PHONY: go-mod-vendor
go-mod-vendor:
	GO111MODULE=on $(GO) mod vendor

.PHONY: build
build: go-mod-vendor
	$(GO) build -o $(APP) -ldflags "-X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(PROGVER)"

.PHONY: version
version:
	@echo $(PROGVER)

# If the first argument is "update-version"...
ifeq (update-version,$(firstword $(MAKECMDGOALS)))
  # use the rest as arguments for "update-version"
  RUN_ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
  # ...and turn them into do-nothing targets
  $(eval $(RUN_ARGS):;@:)
endif

.PHONY: update-version
update-version:
	@echo "Update Version $(PROGVER) to $(RUN_ARGS)"
	git tag v$(RUN_ARGS)


.PHONY: install
install: go-mod-vendor
	$(GO) install -ldflags "-X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(PROGVER)"

.PHONY: release-artifacts
release-artifacts: go-mod-vendor
	mkdir -p ./.ignore
	GOOS=darwin GOARCH=amd64 $(GO) build -o ./.ignore/$(APP)-$(PROGVER).darwin-amd64 -ldflags "-X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(PROGVER)"
	GOOS=linux  GOARCH=amd64 $(GO) build -o ./.ignore/$(APP)-$(PROGVER).linux-amd64 -ldflags "-X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(PROGVER)"

.PHONY: docker
docker:
	docker build \
		--build-arg VERSION=$(PROGVER) \
		--build-arg GITCOMMIT=$(GITCOMMIT) \
		--build-arg BUILDTIME='$(BUILDTIME)' \
		-f ./deploy/Dockerfile -t $(DOCKER_ORG)/$(APP):$(PROGVER) .

.PHONY: local-docker
local-docker:
	docker build \
		--build-arg VERSION=$(PROGVER)+local \
		--build-arg GITCOMMIT=$(GITCOMMIT) \
		--build-arg BUILDTIME='$(BUILDTIME)' \
		-f ./deploy/Dockerfile.local -t $(DOCKER_ORG)/$(APP):local .

.PHONY: style
style:
	! $(GOFMT) -d $$(find . -path ./vendor -prune -o -name '*.go' -print) | grep '^'

.PHONY: test
test: go-mod-vendor
	GO111MODULE=on $(GO) test -v -race  -coverprofile=cover.out ./...

.PHONY: test-cover
test-cover: test
	$(GO) tool cover -html=cover.out

.PHONY: codecov
codecov: test
	curl -s https://codecov.io/bash | bash

.PHONEY: it
it:
	./it.sh

.PHONY: clean
clean:
	rm -rf ./$(APP) ./.ignore ./coverage.txt ./vendor
