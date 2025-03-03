LOCAL_BIN ?= ./.env

.PHONY: install.golangci
install.golangci:
	mkdir -p $(LOCAL_BIN) && curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(LOCAL_BIN) v1.64.5

.PHONY: build
build:
	go build -o ./bin/gamesmaster

.PHONY: lint
lint:
	./.env/golangci-lint run

.PHONY: generate
generate:
	go generate ./...