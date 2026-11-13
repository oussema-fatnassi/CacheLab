-include .env
export

.PHONY: build test lint server client loadtest cover

build:
	go build -o bin/cachelab ./cmd/cachelab

test:
	go test -race -count=1 ./...

lint:
	go vet ./...

server:
	go run ./cmd/cachelab

client:
	go run ./cmd/client

loadtest:
	go run ./cmd/loadtest $(ARGS)

cover:
	@echo Production coverage: ./cmd/cachelab ./internal/cache ./api
	go test -count=1 -covermode=atomic -coverprofile=coverage.out ./cmd/cachelab ./internal/cache ./api
	go tool cover -func=coverage.out
	go run ./tools/coverreport coverage.out
