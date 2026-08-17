# fastlog

`fastlog` is a high-performance logging package for Go, designed to be extremely fast, efficient, and suitable for high-performance applications. It supports logging to both files and standard output, with features such as buffering, non-blocking I/O, periodic flushing, log rotation, and configurable log levels.

## Features

- **Log Levels**: Supports DEBUG, INFO, WARN, ERROR, and FATAL levels.
- **Buffering**: Uses a buffer to minimize I/O operations.
- **Non-blocking I/O**: Log messages are sent to a channel and processed by a separate goroutine.
- **Periodic Flushing**: Flushes logs every 5 seconds (configurable).
- **Log Rotation**: Automatically rotates log files when they exceed a predefined size.
- **Configurable Output**: Logs can be written to a file or standard output based on configuration.
- **Structured Logging**: Supports structured logging and JSON formatted logs.
- **High Performance**: Optimized for speed with minimal allocations and efficient string operations.
- **Sync Mode**: Optional synchronous mode for maximum performance (~1.1μs latency, 47% faster than async).
- **Formatted Logging**: Supports printf-style formatted logging (`Infof`, `Debugf`, etc.).
- **Caller Information**: Optional file and line number tracking.
- **Comprehensive Benchmarks**: Includes comparison benchmarks against zap, zerolog, slog, and standard log.

## Installation

To install `fastlog`, run:

```sh
go get github.com/amarsinghrathour/fastlog
```
## Usage
Here's an example of how to use fastlog:

```
    package main

    import (
        "log"
        "os"
        "path/filepath"
        "time"
    
        "github.com/yourusername/fastlog"
    )

    func main() {
        logDir := "logs"
        baseLogFileName := filepath.Join(logDir, "app.log")
    
        err := os.MkdirAll(logDir, 0755)
        if err != nil {
            log.Fatalf("Failed to create log directory: %v", err)
        }
    
        // Configure to log to a file
        loggerConfig := fastlog.LoggerConfig{
            Level:       fastlog.DEBUG,
            FilePath:    baseLogFileName,
            RotationDir: logDir,
            Stdout:      false, // Set to true to log to stdout instead of a file
            JSONFormat:  true,  // Set to true to enable JSON formatted logs
        }
    
        logger, err := fastlog.NewLogger(loggerConfig)
        if err != nil {
            log.Fatalf("Failed to create logger: %v", err)
        }
        defer logger.Close()
    
        // Log various messages
        logger.Debug("This is a debug message", "key1", "value1")
        logger.Info("This is an info message", "key2", "value2")
        logger.Warn("This is a warning message", "key3", "value3")
        logger.Error("This is an error message", "key4", "value4")
    
        // Simulate some work to generate logs over time
        for i := 0; i < 10; i++ {
            logger.Info("Working on task", "iteration", i)
            time.Sleep(1 * time.Second)
        }
    
        // Log a fatal message (this will terminate the program)
        logger.Fatal("This is a fatal message", "key5", "value5")
    }



```

## Benchmarks

`fastlog` includes comprehensive benchmarks comparing performance with other popular Go logging libraries:

- **Go Standard Library `log`**
- **`log/slog`** (Go 1.21+ structured logging)
- **Uber's `zap`** (high-performance structured logger)
- **`zerolog`** (zero-allocation JSON logger)

### Running Benchmarks

**Basic benchmarks:**
```bash
go test -bench Fastlog -benchmem -run=NONE
```

**Comprehensive benchmark suite:**
```bash
# Run all fastlog benchmarks
go test -bench Fastlog -benchmem -run=NONE

# Run with different CPU counts (multi-core scaling)
go test -bench Fastlog -benchmem -cpu=1,4,8 -run=NONE

# Run comparison benchmarks (requires external dependencies)
go test -tags=benchmark -bench BenchmarkComparison -benchmem -run=NONE
```

**See [BENCHMARKS.md](BENCHMARKS.md) for detailed benchmark guide and interpretation.**

**Comparison benchmarks (requires dependencies):**
```bash
# Install comparison libraries
go get go.uber.org/zap github.com/rs/zerolog

# Run comparison benchmarks
go test -tags=benchmark -bench=BenchmarkComparison -benchmem -benchtime=3s
```

See [BENCHMARKS.md](BENCHMARKS.md) for detailed benchmark results and [BENCHMARK_COMPARISON.md](BENCHMARK_COMPARISON.md) for comparison guide.

## Performance

### Async Mode (Default)
- **Throughput**: ~600,000 operations/second
- **Memory**: ~568 bytes/op, 7 allocations/op
- **Latency**: ~1.6 microseconds median
- **Best for**: High concurrency, non-blocking behavior

### Sync Mode (Maximum Speed)
- **Throughput**: ~880,000 operations/second (**47% faster**)
- **Memory**: ~256 bytes/op, 3 allocations/op (**55% less**)
- **Latency**: ~1.1 microseconds median (**31% lower**)
- **Best for**: Maximum performance, low contention

### Disabled Logs
- **Overhead**: ~3 nanoseconds (minimal)

**Note**: Use `SyncMode: true` in `LoggerConfig` for maximum performance. See [PERFORMANCE_OPTIMIZATION.md](PERFORMANCE_OPTIMIZATION.md) for detailed optimization guide.

See [PERFORMANCE.md](PERFORMANCE.md) for detailed performance characteristics and optimization tips.

[![MIT License](https://img.shields.io/badge/License-MIT-green.svg)](https://choosealicense.com/licenses/mit/)
[![Go Reference](https://pkg.go.dev/badge/github.com/amarsinghrathour/fastlog.svg)](https://pkg.go.dev/github.com/amarsinghrathour/fastlog)