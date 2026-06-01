BINARY := bookindex
DIST_DIR := dist
DEPLOY_PATH ?= /usr/local/bin/$(BINARY)
LINUX_AMD64_BINARY := $(DIST_DIR)/$(BINARY)-linux-amd64
LOCAL_GOOS := $(shell go env GOOS)
LOCAL_GOARCH := $(shell go env GOARCH)
GOOS ?= linux
GOARCH ?= amd64

.PHONY: build build-local test build-linux build-linux-amd64 build-linux-arm64 push deploy check-vps clean

build: build-linux-amd64

build-local:
	mkdir -p $(DIST_DIR)
	go build -trimpath -o $(DIST_DIR)/$(BINARY)-$(LOCAL_GOOS)-$(LOCAL_GOARCH) .

test:
	go test ./...

build-linux:
	mkdir -p $(DIST_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -o $(DIST_DIR)/$(BINARY)-$(GOOS)-$(GOARCH) .

build-linux-amd64:
	$(MAKE) build-linux GOOS=linux GOARCH=amd64

build-linux-arm64:
	$(MAKE) build-linux GOOS=linux GOARCH=arm64

push: check-vps
	scp $(LINUX_AMD64_BINARY) $(VPS):$(DEPLOY_PATH)

deploy: check-vps build push

check-vps:
	@test -n "$(VPS)" || (echo "Set VPS on the command line, for example: make deploy VPS=my-ssh-host"; exit 1)

clean:
	rm -rf $(DIST_DIR)
