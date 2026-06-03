-include .env
export

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=landman
BINARY_UNIX=$(BINARY_NAME)_unix
MIGRATIONS_PATH=./db/migrations
DB_BOOTSTRAP_PATH=./db/bootstrap/create_database.sql
DOCKER ?= docker
DB_CONTAINER ?= landman-postgis
GOOSE_IMAGE ?= golang:1.25-alpine
GOOSE_VERSION ?= latest
GOOSE_CMD = $(DOCKER) run --rm --network container:$(DB_CONTAINER) -v "$(CURDIR)/$(MIGRATIONS_PATH):/migrations" -e GOOSE_DRIVER=postgres -e GOOSE_DBSTRING="$(DATABASE_URL)" -e GOCACHE=/tmp/.cache/go-build -e GOMODCACHE=/tmp/pkg/mod -e GOBIN=/tmp/bin -u "$$(id -u):$$(id -g)" $(GOOSE_IMAGE) sh -c 'go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION) >/dev/null 2>&1 && /tmp/bin/goose -dir /migrations "$$@"' goose

.PHONY: all build test clean run deps db-create db-drop migrate-up migrate-down migrate-create migrate-version migrate-status check-db-container check-postgres-url check-database-url help

all: test build

build: ## Build the binary
	$(GOBUILD) -o $(BINARY_NAME) -v ./cmd/main.go

test: ## Run tests
	$(GOTEST) -v -race ./...

clean: ## Remove build artifacts
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

run: ## Run the application
	$(GOBUILD) -o $(BINARY_NAME) -v ./main.go
	./$(BINARY_NAME)

deps: ## Get dependencies
	$(GOGET) -u github.com/tidwall/gjson

check-db-container:
	@if ! $(DOCKER) inspect $(DB_CONTAINER) >/dev/null 2>&1; then \
		echo "DB container '$(DB_CONTAINER)' was not found."; \
		echo "Set DB_CONTAINER in .env to your running PostGIS container name."; \
		echo "Example: DB_CONTAINER=my-postgis"; \
		exit 1; \
	fi

check-postgres-url:
	@if [ -z "$(POSTGRES_URL)" ]; then \
		echo "POSTGRES_URL is not set. Example:"; \
		echo "  POSTGRES_URL='postgres://user:password@localhost:5432/postgres?sslmode=disable'"; \
		exit 1; \
	fi

check-database-url:
	@if [ -z "$(DATABASE_URL)" ]; then \
		echo "DATABASE_URL is not set. Example:"; \
		echo "  DATABASE_URL='postgres://user:password@localhost:5432/landman?sslmode=disable'"; \
		exit 1; \
	fi

db-create: check-db-container check-postgres-url ## Create the landman database if it does not exist using docker exec psql
	$(DOCKER) exec -i $(DB_CONTAINER) psql "$(POSTGRES_URL)" -f /dev/stdin < $(DB_BOOTSTRAP_PATH)

db-drop: check-db-container check-postgres-url ## Drop the local landman database after terminating active connections
	$(DOCKER) exec -i $(DB_CONTAINER) psql "$(POSTGRES_URL)" -v ON_ERROR_STOP=1 -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'landman' AND pid <> pg_backend_pid();" -c "DROP DATABASE IF EXISTS landman;"

migrate-up: check-db-container check-database-url ## Run pending SQL migrations with Dockerized goose
	$(GOOSE_CMD) up

migrate-down: check-db-container check-database-url ## Roll back the latest SQL migration with Dockerized goose
	$(GOOSE_CMD) down

migrate-version: check-db-container check-database-url ## Show the current goose migration version
	$(GOOSE_CMD) version

migrate-status: check-db-container check-database-url ## Show goose migration status
	$(GOOSE_CMD) status

migrate-create: check-db-container ## Create a new goose SQL migration: make migrate-create name=create_users
	@if [ -z "$(name)" ]; then \
		echo "Migration name is required. Example:"; \
		echo "  make migrate-create name=create_users"; \
		exit 1; \
	fi
	$(GOOSE_CMD) create $(name) sql

# Auto-documented help (from https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html)
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
