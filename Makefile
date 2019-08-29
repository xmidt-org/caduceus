DEFAULT: build

GO           ?= go
GOFMT        ?= $(GO)fmt
APP          := caduceus
FIRST_GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
BINARY    := $(FIRST_GOPATH)/bin/$(APP)

PROGVER = $(shell grep 'applicationVersion.*= ' main.go | awk '{print $$3}' | sed -e 's/\"//g')

.PHONY: go-mod-vendor
go-mod-vendor:
	GO111MODULE=on $(GO) mod vendor

.PHONY: build
build: go-mod-vendor
	$(GO) build -o $(APP)

rpm:
	mkdir -p ./.ignore/SOURCES
	tar -czf ./.ignore/SOURCES/$(APP)-$(PROGVER).tar.gz --transform 's/^\./$(APP)-$(PROGVER)/' --exclude ./.git --exclude ./.ignore --exclude ./conf --exclude ./deploy --exclude ./vendor --exclude ./vendor .
	cp conf/$(APP).service ./.ignore/SOURCES
	cp $(APP).yaml  ./.ignore/SOURCES
	cp LICENSE ./.ignore/SOURCES
	cp NOTICE ./.ignore/SOURCES
	cp CHANGELOG.md ./.ignore/SOURCES
	rpmbuild --define "_topdir $(CURDIR)/.ignore" \
    		--define "_version $(PROGVER)" \
    		--define "_release 1" \
    		-ba deploy/packaging/$(APP).spec

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
	sed -i "s/$(PROGVER)/$(RUN_ARGS)/g" main.go


.PHONY: install
install: go-mod-vendor
	echo $(GO) build -o $(BINARY) $(PROGVER)

.PHONY: release-artifacts
release-artifacts: go-mod-vendor
	GOOS=darwin GOARCH=amd64 $(GO) build -o ./OPATH/$(APP)-$(PROGVER).darwin-amd64
	GOOS=linux  GOARCH=amd64 $(GO) build -o ./OPATH/$(APP)-$(PROGVER).linux-amd64

.PHONY: docker
docker:
	docker build -f ./deploy/Dockerfile -t $(APP):$(PROGVER) .

# build docker without running modules
.PHONY: local-docker
local-docker:
	GOOS=linux  GOARCH=amd64 $(GO) build -o $(APP)_linux_amd64
	docker build -f ./deploy/Dockerfile.local -t $(APP):local .

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
	rm -rf ./$(APP) ./OPATH ./coverage.txt ./vendor
