//go:build benchmark
// +build benchmark

package fastlog

import (
	"io"
	"log"
	"log/slog"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"go.uber.org/zap"
)

// This file contains comparison benchmarks with other popular Go logging libraries.
// To run these benchmarks, you need to install the comparison libraries:
//   go get go.uber.org/zap
//   go get github.com/rs/zerolog
//
// Then run: go test -tags=benchmark -bench=BenchmarkComparison -benchmem -benchtime=3s

var (
	devNull io.Writer
)

func init() {
	var err error
	devNull, err = os.OpenFile("/dev/null", os.O_WRONLY, 0)
	if err != nil {
		// Fallback to os.DevNull on Windows
		devNull = os.NewFile(0, os.DevNull)
	}
}

// ============================================================================
// Basic Text Logging Comparisons
// ============================================================================

// BenchmarkComparisonFastlog benchmarks fastlog (text format, async mode)
func BenchmarkComparisonFastlog(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
		SyncMode:   false, // Async mode (default)
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message")
		}
	})
}

// BenchmarkComparisonFastlogSync benchmarks fastlog (text format, sync mode - fastest)
func BenchmarkComparisonFastlogSync(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
		SyncMode:   true, // Sync mode for maximum speed
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message")
		}
	})
}

// BenchmarkComparisonFastlogJSON benchmarks fastlog (JSON format)
func BenchmarkComparisonFastlogJSON(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: true,
		FilePath:   "/dev/null",
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message")
		}
	})
}

// BenchmarkComparisonStandardLog benchmarks Go's standard library log package
func BenchmarkComparisonStandardLog(b *testing.B) {
	logger := log.New(devNull, "", log.LstdFlags)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Println("test message")
		}
	})
}

// BenchmarkComparisonSlogText benchmarks Go's structured log (slog) with text handler
func BenchmarkComparisonSlogText(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(devNull, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message")
		}
	})
}

// BenchmarkComparisonSlogJSON benchmarks Go's structured log (slog) with JSON handler
func BenchmarkComparisonSlogJSON(b *testing.B) {
	logger := slog.New(slog.NewJSONHandler(devNull, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message")
		}
	})
}

// BenchmarkComparisonZap benchmarks Uber's zap logger (production mode)
func BenchmarkComparisonZap(b *testing.B) {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"/dev/null"}
	config.ErrorOutputPaths = []string{"/dev/null"}
	logger, err := config.Build()
	if err != nil {
		b.Fatalf("Failed to create zap logger: %v", err)
	}
	defer logger.Sync()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message")
		}
	})
}

// BenchmarkComparisonZapSugar benchmarks Uber's zap logger (sugared, more convenient API)
func BenchmarkComparisonZapSugar(b *testing.B) {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"/dev/null"}
	config.ErrorOutputPaths = []string{"/dev/null"}
	baseLogger, err := config.Build()
	if err != nil {
		b.Fatalf("Failed to create zap logger: %v", err)
	}
	defer baseLogger.Sync()
	logger := baseLogger.Sugar()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message")
		}
	})
}

// BenchmarkComparisonZerolog benchmarks zerolog (zero-allocation JSON logger)
func BenchmarkComparisonZerolog(b *testing.B) {
	logger := zerolog.New(devNull).With().Timestamp().Logger().Level(zerolog.InfoLevel)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info().Msg("test message")
		}
	})
}

// ============================================================================
// Structured Logging with Fields Comparisons
// ============================================================================

// BenchmarkComparisonFastlogWithFields benchmarks fastlog with structured fields (async)
func BenchmarkComparisonFastlogWithFields(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: true,
		FilePath:   "/dev/null",
		SyncMode:   false,
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.WithFields(map[string]interface{}{
				"user_id":    123,
				"request_id": "abc-123",
				"status":     "ok",
			}).Info("test message")
		}
	})
}

// BenchmarkComparisonFastlogWithFieldsSync benchmarks fastlog with structured fields (sync mode)
func BenchmarkComparisonFastlogWithFieldsSync(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: true,
		FilePath:   "/dev/null",
		SyncMode:   true,
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.WithFields(map[string]interface{}{
				"user_id":    123,
				"request_id": "abc-123",
				"status":     "ok",
			}).Info("test message")
		}
	})
}

// BenchmarkComparisonSlogWithFields benchmarks slog with structured fields
func BenchmarkComparisonSlogWithFields(b *testing.B) {
	logger := slog.New(slog.NewJSONHandler(devNull, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message",
				"user_id", 123,
				"request_id", "abc-123",
				"status", "ok",
			)
		}
	})
}

// BenchmarkComparisonZapWithFields benchmarks zap with structured fields
func BenchmarkComparisonZapWithFields(b *testing.B) {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"/dev/null"}
	config.ErrorOutputPaths = []string{"/dev/null"}
	logger, err := config.Build()
	if err != nil {
		b.Fatalf("Failed to create zap logger: %v", err)
	}
	defer logger.Sync()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message",
				zap.Int("user_id", 123),
				zap.String("request_id", "abc-123"),
				zap.String("status", "ok"),
			)
		}
	})
}

// BenchmarkComparisonZerologWithFields benchmarks zerolog with structured fields
func BenchmarkComparisonZerologWithFields(b *testing.B) {
	logger := zerolog.New(devNull).With().Timestamp().Logger().Level(zerolog.InfoLevel)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info().
				Int("user_id", 123).
				Str("request_id", "abc-123").
				Str("status", "ok").
				Msg("test message")
		}
	})
}

// ============================================================================
// Formatted String Logging Comparisons
// ============================================================================

// BenchmarkComparisonFastlogInfof benchmarks fastlog formatted logging
func BenchmarkComparisonFastlogInfof(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Infof("test message: %d", 123)
		}
	})
}

// BenchmarkComparisonStandardLogf benchmarks standard log formatted logging
func BenchmarkComparisonStandardLogf(b *testing.B) {
	logger := log.New(devNull, "", log.LstdFlags)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Printf("test message: %d", 123)
		}
	})
}

// BenchmarkComparisonZapSugarf benchmarks zap sugared formatted logging
func BenchmarkComparisonZapSugarf(b *testing.B) {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"/dev/null"}
	config.ErrorOutputPaths = []string{"/dev/null"}
	baseLogger, err := config.Build()
	if err != nil {
		b.Fatalf("Failed to create zap logger: %v", err)
	}
	defer baseLogger.Sync()
	logger := baseLogger.Sugar()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Infof("test message: %d", 123)
		}
	})
}

// BenchmarkComparisonZerologf benchmarks zerolog formatted logging
func BenchmarkComparisonZerologf(b *testing.B) {
	logger := zerolog.New(devNull).With().Timestamp().Logger().Level(zerolog.InfoLevel)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info().Msgf("test message: %d", 123)
		}
	})
}

// ============================================================================
// Disabled Log Level Comparisons (Performance when logging is filtered)
// ============================================================================

// BenchmarkComparisonFastlogDisabled benchmarks fastlog with disabled log level
func BenchmarkComparisonFastlogDisabled(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      ERROR, // DEBUG/INFO/WARN are disabled
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message") // Should be filtered out
		}
	})
}

// BenchmarkComparisonSlogDisabled benchmarks slog with disabled log level
func BenchmarkComparisonSlogDisabled(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(devNull, &slog.HandlerOptions{
		Level: slog.LevelError, // Info is disabled
	}))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message") // Should be filtered out
		}
	})
}

// BenchmarkComparisonZapDisabled benchmarks zap with disabled log level
func BenchmarkComparisonZapDisabled(b *testing.B) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel) // Info is disabled
	config.OutputPaths = []string{"/dev/null"}
	config.ErrorOutputPaths = []string{"/dev/null"}
	logger, err := config.Build()
	if err != nil {
		b.Fatalf("Failed to create zap logger: %v", err)
	}
	defer logger.Sync()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message") // Should be filtered out
		}
	})
}

// BenchmarkComparisonZerologDisabled benchmarks zerolog with disabled log level
func BenchmarkComparisonZerologDisabled(b *testing.B) {
	logger := zerolog.New(devNull).With().Timestamp().Logger().Level(zerolog.ErrorLevel) // Info is disabled

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info().Msg("test message") // Should be filtered out
		}
	})
}

// ============================================================================
// Allocation Comparisons
// ============================================================================

// BenchmarkComparisonFastlogAllocations benchmarks fastlog allocations
func BenchmarkComparisonFastlogAllocations(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message")
	}
}

// BenchmarkComparisonZapAllocations benchmarks zap allocations
func BenchmarkComparisonZapAllocations(b *testing.B) {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"/dev/null"}
	config.ErrorOutputPaths = []string{"/dev/null"}
	logger, err := config.Build()
	if err != nil {
		b.Fatalf("Failed to create zap logger: %v", err)
	}
	defer logger.Sync()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message")
	}
}

// BenchmarkComparisonZerologAllocations benchmarks zerolog allocations
func BenchmarkComparisonZerologAllocations(b *testing.B) {
	logger := zerolog.New(devNull).With().Timestamp().Logger().Level(zerolog.InfoLevel)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info().Msg("test message")
	}
}

// BenchmarkComparisonSlogAllocations benchmarks slog allocations
func BenchmarkComparisonSlogAllocations(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(devNull, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message")
	}
}

// ============================================================================
// Concurrent Logging Comparisons
// ============================================================================

// BenchmarkComparisonFastlogConcurrent benchmarks fastlog under high concurrency
func BenchmarkComparisonFastlogConcurrent(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
		QueueSize:  10000,
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("concurrent test message")
		}
	})
}

// BenchmarkComparisonZapConcurrent benchmarks zap under high concurrency
func BenchmarkComparisonZapConcurrent(b *testing.B) {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"/dev/null"}
	config.ErrorOutputPaths = []string{"/dev/null"}
	logger, err := config.Build()
	if err != nil {
		b.Fatalf("Failed to create zap logger: %v", err)
	}
	defer logger.Sync()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("concurrent test message")
		}
	})
}

// BenchmarkComparisonZerologConcurrent benchmarks zerolog under high concurrency
func BenchmarkComparisonZerologConcurrent(b *testing.B) {
	logger := zerolog.New(devNull).With().Timestamp().Logger().Level(zerolog.InfoLevel)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info().Msg("concurrent test message")
		}
	})
}
