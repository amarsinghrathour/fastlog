# Migration Guide

This document covers the standard-library-style repository restructure of `fastlog`.

## What changed

- Added explicit package docs in `doc.go`.
- Split exported API surface into:
  - `logger.go`
  - `config.go`
  - `fields.go`
- Introduced internal implementation packages:
  - `internal/core`
  - `internal/format`
  - `internal/queue`
  - `internal/rotate`
- Split tests by concern:
  - unit tests in `fastlog_test.go`
  - integration tests in `integration_test.go`
- Added runnable GoDoc example in `example_test.go`.

## Compatibility notes

- Module import path is unchanged:
  - `github.com/amarsinghrathour/fastlog`
- Existing primary logger APIs remain available:
  - `NewLogger`
  - `LoggerConfig`
  - `Logger` logging methods (`Debug`, `Info`, `Warn`, `Error`, `Fatal`, and `*f` variants)
  - `WithFields`
- Integration rotation test now respects `go test -short`.

## Recommended workflow updates

- Run fast checks:

```bash
go test -short ./...
```

- Run full suite:

```bash
go test ./...
```

- Run benchmarks:

```bash
go test -run=NONE -bench=Fastlog -benchmem
```

## Notes for contributors

- Avoid importing from `internal/*` outside this module.
- Keep exported API declarations in `logger.go`, `config.go`, and `fields.go`.
- Keep formatting/queue/rotation mechanics behind internal package boundaries.
