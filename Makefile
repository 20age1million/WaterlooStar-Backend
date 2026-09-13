.DEFAULT_GOAL := help
.PHONY: help up down run build test vet generate sqlc openapi migrate-up migrate-down migrate-version seed tidy check

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## up: start PostgreSQL in the background
up:
	docker compose up -d

## down: stop PostgreSQL, keeping its data volume
down:
	docker compose down

## migrate-up: apply all pending migrations
migrate-up:
	go run ./cmd/migrate up

## migrate-down: roll back the most recent migration
migrate-down:
	go run ./cmd/migrate down-one

## migrate-version: print the current schema version
migrate-version:
	go run ./cmd/migrate version

## seed: load development fixtures (development only; rewrites all listings)
seed:
	go run ./cmd/seed

## generate: run every code generator (OpenAPI server + sqlc queries)
generate: openapi sqlc

## openapi: regenerate the gin server interface from api/openapi.yaml
openapi:
	go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml

## sqlc: regenerate typed queries from migrations/ and internal/db/queries/
sqlc:
	go tool sqlc generate

## run: start the API
run:
	go run ./cmd/api

## build: compile the API binary into bin/
build:
	go build -o bin/api ./cmd/api

## test: run the test suite
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## tidy: tidy module requirements
tidy:
	go mod tidy

## check: everything CI would run
check: generate vet test build
