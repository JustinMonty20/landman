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

.PHONY: all build test clean run deps db-create migrate-up migrate-down migrate-create migrate-version check-db-container check-postgres-url check-database-url help

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

migrate-up: check-db-container check-database-url ## Run pending SQL migrations using docker exec psql
	$(DOCKER) exec -i $(DB_CONTAINER) psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -c "CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());"
	@for file in $(MIGRATIONS_PATH)/*.up.sql; do \
		[ -e "$$file" ] || continue; \
		version=$$(basename "$$file" | cut -d_ -f1); \
		applied=$$($(DOCKER) exec $(DB_CONTAINER) psql "$(DATABASE_URL)" -At -c "SELECT 1 FROM schema_migrations WHERE version = '$$version';"); \
		if [ "$$applied" = "1" ]; then \
			echo "Skipping migration $$version; already applied"; \
		else \
			echo "Applying migration $$file"; \
			$(DOCKER) exec -i $(DB_CONTAINER) psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f /dev/stdin < "$$file"; \
			$(DOCKER) exec -i $(DB_CONTAINER) psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -c "INSERT INTO schema_migrations (version) VALUES ('$$version');"; \
		fi; \
	done

migrate-down: check-db-container check-database-url ## Rollback the latest SQL migration using docker exec psql
	@version=$$($(DOCKER) exec $(DB_CONTAINER) psql "$(DATABASE_URL)" -At -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;"); \
	if [ -z "$$version" ]; then \
		echo "No applied migrations to roll back"; \
		exit 0; \
	fi; \
	file=$$(ls $(MIGRATIONS_PATH)/$$version*_*.down.sql 2>/dev/null | head -n 1); \
	if [ -z "$$file" ]; then \
		echo "No down migration found for version $$version"; \
		exit 1; \
	fi; \
	echo "Rolling back migration $$file"; \
	$(DOCKER) exec -i $(DB_CONTAINER) psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f /dev/stdin < "$$file"; \
	$(DOCKER) exec -i $(DB_CONTAINER) psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -c "DELETE FROM schema_migrations WHERE version = '$$version';"

migrate-version: check-db-container check-database-url ## Show applied SQL migration versions using docker exec psql
	$(DOCKER) exec -i $(DB_CONTAINER) psql "$(DATABASE_URL)" -c "SELECT version, applied_at FROM schema_migrations ORDER BY version;"

migrate-create: ## Create a new SQL migration: make migrate-create name=create_users
	@if [ -z "$(name)" ]; then \
		echo "Migration name is required. Example:"; \
		echo "  make migrate-create name=create_users"; \
		exit 1; \
	fi
	@version=$$(date +%Y%m%d%H%M%S); \
	touch "$(MIGRATIONS_PATH)/$${version}_$(name).up.sql" "$(MIGRATIONS_PATH)/$${version}_$(name).down.sql"; \
	echo "Created $(MIGRATIONS_PATH)/$${version}_$(name).up.sql"; \
	echo "Created $(MIGRATIONS_PATH)/$${version}_$(name).down.sql"

# Auto-documented help (from https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html)
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
