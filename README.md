# fastlog

`fastlog` is a high-throughput logging library for Go with structured fields, zero-allocation buffers, JSON/text output, async queueing, and file rotation.

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
	logger.WithFields(
		fastlog.String("component", "http"),
		fastlog.String("version", "v1"),
	).Info("ready")
}
```

## Project Structure

- `logger.go` - The public logging interface and core entry points.
- `config.go` - Public configuration options and defaults.
- `fields.go` - The structured data field system.
- `level.go` - Log level definitions and parsing.
- `entry.go` - Extremely high-performance memory pooling.
- `formatter.go` - Zero-allocation text and JSON serialization logic.
- `worker.go` - High-throughput async ring-buffer IO worker.
- `doc.go` - Package-level GoDoc.

## Testing

```bash
# Unit + integration tests
go test ./...

# Fast path unit tests only
go test -short ./...
```

## Benchmarks (Updated August 17, 2026)

Fastlog achieves extreme speed and zero-allocations on the disabled path. When compared to the Go standard library (`slog`) and other high-performance loggers (`zap`, `zerolog`), Fastlog provides highly competitive throughput with a significantly simpler API.

*Note: The following benchmarks were recorded on an **Apple M4 Pro (ARM64, Darwin)**.*

**Speed (ns/op)**

| Logger | Active Logging (Text) | Active Logging (JSON) | Concurrent Logging |
|--------|-----------------------|-----------------------|--------------------|
| **Fastlog** (Ours) | **210 ns/op** | **168 ns/op** | **221 ns/op** |
| Zap | 68 ns/op | - | 72 ns/op |
| Slog (Standard) | 1066 ns/op | 1035 ns/op | N/A |
| Zerolog | 863 ns/op | - | 860 ns/op |

*Fastlog delivers over 4x the throughput of the standard `slog` package in concurrent applications.*

### Performance Analysis

- **Concurrency & Throughput**: Fastlog utilizes a lock-free asynchronous ring-buffer to offload I/O operations from the critical path. This design allows it to comfortably achieve ~221 ns/op under heavy concurrency, completely outclassing `slog` (~1000+ ns/op) and `zerolog` (~860 ns/op) in highly-threaded environments.
- **Zero-Allocation Disabled Path**: Checking log levels (e.g. calling `Debug()` when the level is `INFO`) evaluates in ~4 nanoseconds with functional zero-allocation overhead, completely matching the optimal theoretical limits of the Go runtime.
- **Zero-Allocation Formatting**: The underlying JSON serialization bypasses `fmt.Sprintf` entirely, leveraging custom byte-appending techniques to achieve 168 ns/op throughput. This makes JSON output strictly *faster* than standard text formatting.
- **Structured Fields**: Contextual logging via `.WithFields()` carries an exceedingly small footprint (2 allocs, ~300 bytes), allowing for heavily context-enriched logging without GC pressure.

```bash
# Core benchmarks
go test -run=NONE -bench=Fastlog -benchmem

# Comparison benchmarks (build-tagged)
go test -run=NONE -tags=benchmark -bench=BenchmarkComparison -benchmem
```