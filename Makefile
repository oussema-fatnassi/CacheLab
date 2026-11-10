-include .env
export

.PHONY: build test lint run cover

build:
	go build -o bin/cachelab ./cmd/cachelab

test:
	go test -race ./...

lint:
	go vet ./...

run:
	go run ./cmd/cachelab

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
