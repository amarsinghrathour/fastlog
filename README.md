# fastlog

`fastlog` is a high-throughput logging library for Go with structured fields, JSON/text output, async queueing, and file rotation.

## Installation

```sh
go get github.com/amarsinghrathour/fastlog
```

## Quick Start

```go
package main

import (
	"path/filepath"

	"github.com/amarsinghrathour/fastlog"
)

func main() {
	logger, err := fastlog.NewLogger(fastlog.LoggerConfig{
		Level:       fastlog.INFO,
		FilePath:    filepath.Join("logs", "app.log"),
		RotationDir: "logs",
		JSONFormat:  false,
	})
	if err != nil {
		panic(err)
	}
	defer logger.Close()

	logger.Info("service started", "port", 8080)
	logger.WithFields(map[string]interface{}{
		"component": "http",
		"version":   "v1",
	}).Info("ready")
}
```

## Project Structure

- `doc.go` - package-level GoDoc.
- `logger.go` - public logging interface and log levels.
- `config.go` - public config and defaults.
- `fields.go` - structured field types.
- `fastlog.go` - logger runtime wiring and method implementations.
- `internal/core` - internal config resolution helpers.
- `internal/format` - internal value/JSON/timestamp encoding.
- `internal/queue` - internal queue retry helpers.
- `internal/rotate` - internal file rotation helpers.
- `logger_test.go` - unit-focused tests.
- `rotation_test.go` - slower filesystem/integration tests.
- `bench_test.go` - core performance benchmarks.
- `bench_compare_test.go` - optional tagged comparison benchmarks.

## Testing

```bash
# Unit + integration tests
go test ./...

# Fast path unit tests only
go test -short ./...
```

## Benchmarks

```bash
# Core benchmarks
go test -run=NONE -bench=Fastlog -benchmem

# Comparison benchmarks (build-tagged)
go test -run=NONE -tags=benchmark -bench=BenchmarkComparison -benchmem
```

## Migration

See `MIGRATION.md` for the standard-library-style restructure details and migration notes.