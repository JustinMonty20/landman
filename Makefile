# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=landman
BINARY_UNIX=$(BINARY_NAME)_unix

.PHONY: all build test clean run deps migrate help

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
	$(GOBUILD) -o $(BINARY_NAME) -v ./cmd/main.go
	./$(BINARY_NAME)

deps: ## Get dependencies
	$(GOGET) -u github.com/tidwall/gjson
	$(GOGET) -u github.com/golang-migrate/migrate/v4
	$(GOGET) -u github.com/golang-migrate/migrate/v4/database/postgres
	$(GOGET) -u github.com/golang-migrate/migrate/v4/source/file

migrate-up: ## Run database migrations up
	migrate -path=./db/migrations -database "postgres://user:password@localhost:5432/parcel_db?sslmode=disable" up

migrate-down: ## Rollback database migrations
	migrate -path=./db/migrations -database "postgres://user:password@localhost:5432/parcel_db?sslmode=disable" down 1

# Auto-documented help (from https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html)
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
