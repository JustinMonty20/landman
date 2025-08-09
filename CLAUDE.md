# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Landman is a Go application for data gathering and sanitization of NC county GIS data. The architecture is built around a connector pattern where each county has its own connector implementing a standard interface.

### Architecture

- **Connector Pattern**: `internal/gis/connector.go` defines the main interface with `GetData()` and `Diff()` methods
- **Base County Connector**: `internal/gis/counties/county.go` provides shared functionality including rate limiting, URL validation, and HTTP client management
- **County-Specific Connectors**: Each county (currently Union County) extends the base connector with custom query parameters and data fetching logic
- **Models**: `internal/models/parcel.go` defines the core data structures for parcels and geometry with changelog tracking

### Key Components

- **Rate Limiting**: Built-in rate limiting using `golang.org/x/time/rate` to respect government API limits
- **HTTP Client**: Production-ready HTTP client with proper timeouts, connection pooling, and keep-alive settings
- **Query Parameter Interface**: Flexible `QueryParams` interface allows each county to define custom parameters while maintaining type safety
- **GeoJSON Support**: Uses `gjson` for efficient JSON parsing and `go-geom` for geometry handling

## Development Commands

### Building and Running
```bash
make build          # Build the binary
make run           # Build and run the application
make clean         # Remove build artifacts
```

### Testing
```bash
make test          # Run all tests with race detection
go test -v ./...   # Alternative test command
```

### Dependencies
```bash
make deps          # Install/update dependencies
go mod tidy        # Clean up module dependencies
```

### Database Migrations
```bash
make migrate-up    # Run database migrations up
make migrate-down  # Rollback one migration
```

## Testing Strategy

Tests use table-driven testing patterns. See `internal/gis/counties/county_test.go` for examples of testing error conditions, URL validation, and constructor methods.

## Error Handling Patterns

- Constructor functions return `(Type, error)` for validation
- URL validation ensures proper scheme (http/https) and non-empty host
- Rate limiting prevents API abuse with configurable requests per second

## Current Implementation Status

The project currently focuses on Union County data collection. The main executable queries total parcel counts using ArcGIS REST services. Future expansion planned for additional counties following the same connector pattern.

## Dependencies

- `github.com/tidwall/gjson` - JSON parsing
- `github.com/twpayne/go-geom` - Geometry handling  
- `golang.org/x/time/rate` - Rate limiting
- Standard library for HTTP, URL parsing, and testing