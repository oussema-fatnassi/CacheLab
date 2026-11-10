-include .env
export

.PHONY: build test lint run client cover

build:
	go build -o bin/cachelab ./cmd/cachelab

test:
	go test -race ./...

lint:
	go vet ./...

run:
	go run ./cmd/cachelab

client:
	go run ./cmd/client

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
