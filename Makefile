BINARY := bookindex
DIST_DIR := dist
LOCAL_GOOS := $(shell go env GOOS)
LOCAL_GOARCH := $(shell go env GOARCH)
GOOS ?= linux
GOARCH ?= amd64

.PHONY: build-local test build-linux build-linux-amd64 build-linux-arm64 clean

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

clean:
	rm -rf $(DIST_DIR)
