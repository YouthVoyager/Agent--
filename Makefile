APP_NAME ?= gateway
BUILD_DIR ?= bin
GO ?= go
GOCACHE ?= $(CURDIR)/.cache/go-build
PKG ?= ./...

export GOCACHE

.PHONY: build test lint race docker run clean

build:
	$(GO) build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/gateway

test:
	$(GO) test $(PKG)

lint:
	$(GO) vet $(PKG)

race:
	$(GO) test -race $(PKG)

docker:
	docker build -t gateway:dev .

run:
	$(GO) run ./cmd/gateway

clean:
	rm -rf $(BUILD_DIR)
