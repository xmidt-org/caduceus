DEFAULT: build

GOPATH       := ${CURDIR}
GOFMT        ?= gofmt
APP          := caduceus
FIRST_GOPATH := $(firstword $(subst :, ,$(shell go env GOPATH)))
BINARY       := $(FIRST_GOPATH)/bin/$(APP)

PROGVER = $(shell grep 'applicationVersion.*= ' src/$(APP)/$(APP).go | awk '{print $$3}' | sed -e 's/\"//g')
RELEASE = 1

.PHONY: glide-install
glide-install:
	export GOPATH=$(GOPATH) && cd src && glide install --strip-vendor

.PHONY: build
build: glide-install
	export GOPATH=$(GOPATH) && cd src/$(APP) && go build

rpm:
	mkdir -p ./.ignore/SOURCES
	tar -czf ./.ignore/SOURCES/$(APP)-$(PROGVER).tar.gz --transform 's/^\./$(APP)-$(PROGVER)/' --exclude ./keys --exclude ./.git --exclude ./.ignore --exclude ./conf --exclude ./deploy --exclude ./vendor --exclude ./src/vendor .
	cp etc/systemd/$(APP).service ./.ignore/SOURCES/
	cp etc/$(APP)/$(APP).yaml  ./.ignore/SOURCES/
	rpmbuild --define "_topdir $(CURDIR)/.ignore" \
		--define "_ver $(PROGVER)" \
		--define "_releaseno $(RELEASE)" \
		-ba etc/systemd/$(APP).spec

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
	sed -i "s/$(PROGVER)/$(RUN_ARGS)/g" src/$(APP)/$(APP).go


.PHONY: install
install:
	echo go build -o $(BINARY) $(PROGVER)

.PHONY: release-artifacts
release-artifacts: glide-install
	mkdir -p ./.ignore
	GOOS=darwin GOARCH=amd64 go build -o .ignore/$(APP)-$(PROGVER).darwin-amd64 $(APP)
	GOOS=linux  GOARCH=amd64 go build -o .ignore/$(APP)-$(PROGVER).linux-amd64 $(APP)

.PHONY: docker
docker:
	docker build -f ./deploy/Dockerfile -t $(APP):$(PROGVER) .

# build docker without running modules
.PHONY: local-docker
local-docker:
	docker build -f ./deploy/Dockerfile.local -t $(APP):local .

.PHONY: style
style:
	! gofmt -d $$(find src/$(APP) -path ./vendor -prune -o -name '*.go' -print) | grep '^'

.PHONY: test
test:
	go test -v -race -coverprofile=cover.out $(APP)/...

.PHONY: test-cover
test-cover: test
	go tool cover -html=cover.out

.PHONY: codecov
codecov: test
	curl -s https://codecov.io/bash | bash

.PHONEY: it
it:
	./it.sh

.PHONY: clean
clean:
	rm -rf ./$(APP) ./.ignore ./coverage.txt ./vendor ./src/vendor
