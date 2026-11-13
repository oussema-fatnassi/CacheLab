-include .env
export

.PHONY: build test lint server client loadtest cover

build:
	go build -o bin/cachelab ./cmd/cachelab

test:
	go test -race ./...

lint:
	go vet ./...

server:
	go run ./cmd/cachelab

client:
	go run ./cmd/client

loadtest:
	go run ./cmd/loadtest $(ARGS)

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
