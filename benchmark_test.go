package fastlog

import (
	"os"
	"testing"
	"time"
)

// BenchmarkFastlogInfo benchmarks Info logging with fastlog (async mode)
func BenchmarkFastlogInfo(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
		SyncMode:   false, // Async mode
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

// BenchmarkFastlogInfoJSON benchmarks Info logging with JSON format
func BenchmarkFastlogInfoJSON(b *testing.B) {
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

// BenchmarkFastlogInfoWithFields benchmarks Info logging with fields
func BenchmarkFastlogInfoWithFields(b *testing.B) {
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
			logger.WithFields(map[string]interface{}{
				"user_id":    123,
				"request_id": "abc-123",
				"status":     "ok",
			}).Info("test message")
		}
	})
}

// BenchmarkFastlogInfof benchmarks formatted Info logging
func BenchmarkFastlogInfof(b *testing.B) {
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

// BenchmarkFastlogDisabled benchmarks disabled log level (should be very fast)
func BenchmarkFastlogDisabled(b *testing.B) {
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

// BenchmarkFastlogConcurrent benchmarks concurrent logging
func BenchmarkFastlogConcurrent(b *testing.B) {
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

// BenchmarkFastlogAllocations measures allocations per log operation
func BenchmarkFastlogAllocations(b *testing.B) {
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

// BenchmarkFastlogJSONAllocations measures allocations for JSON logging
func BenchmarkFastlogJSONAllocations(b *testing.B) {
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

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message")
	}
}

// BenchmarkFastlogWithCaller benchmarks logging with caller information
func BenchmarkFastlogWithCaller(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:       INFO,
		Stdout:      false,
		JSONFormat:  false,
		FilePath:    "/dev/null",
		EnableCaller: true,
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("test message with caller")
		}
	})
}

// BenchmarkFastlogStdout benchmarks logging to stdout
func BenchmarkFastlogStdout(b *testing.B) {
	// Redirect stdout to /dev/null for benchmarking
	oldStdout := os.Stdout
	devNull, _ := os.OpenFile("/dev/null", os.O_WRONLY, 0)
	os.Stdout = devNull
	defer func() {
		os.Stdout = oldStdout
		devNull.Close()
	}()

	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     true,
		JSONFormat: false,
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

// BenchmarkFastlogFile benchmarks logging to file
func BenchmarkFastlogFile(b *testing.B) {
	tmpFile, err := os.CreateTemp("", "bench-*.log")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	logger, err := NewLogger(LoggerConfig{
		Level:       INFO,
		Stdout:      false,
		JSONFormat:  false,
		FilePath:    tmpFile.Name(),
		RotationDir: "", // Disable rotation for benchmark
		MaxFileSize: 0,  // Disable rotation
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

// BenchmarkFastlogMixedLevels benchmarks different log levels
func BenchmarkFastlogMixedLevels(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      DEBUG,
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
		i := 0
		for pb.Next() {
			switch i % 5 {
			case 0:
				logger.Debug("debug message")
			case 1:
				logger.Info("info message")
			case 2:
				logger.Warn("warn message")
			case 3:
				logger.Error("error message")
			case 4:
				logger.Infof("formatted message: %d", i)
			}
			i++
		}
	})
}

// BenchmarkFastlogLargeMessage benchmarks logging large messages
func BenchmarkFastlogLargeMessage(b *testing.B) {
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

	largeMsg := make([]byte, 1024)
	for i := range largeMsg {
		largeMsg[i] = 'A'
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info(string(largeMsg))
		}
	})
}

// BenchmarkFastlogManyFields benchmarks logging with many fields
func BenchmarkFastlogManyFields(b *testing.B) {
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

	fields := map[string]interface{}{
		"field1":  "value1",
		"field2":  123,
		"field3":  true,
		"field4":  45.67,
		"field5":  "value5",
		"field6":  789,
		"field7":  false,
		"field8":  12.34,
		"field9":  "value9",
		"field10": 101112,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.WithFields(fields).Info("test message")
		}
	})
}

// BenchmarkFastlogThroughput measures throughput over time
func BenchmarkFastlogThroughput(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
		QueueSize:  100000,
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ResetTimer()
	start := time.Now()
	for i := 0; i < b.N; i++ {
		logger.Info("throughput test message")
	}
	elapsed := time.Since(start)
	b.ReportMetric(float64(b.N)/elapsed.Seconds(), "ops/sec")
}

// ============================================================================
// PHASE 6: Comprehensive Benchmark Suite
// ============================================================================

// BenchmarkFastlog_Text_Enabled benchmarks text logging in enabled state
func BenchmarkFastlog_Text_Enabled(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      DEBUG, // All levels enabled
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
		SyncMode:   false, // Async mode
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

// BenchmarkFastlog_Text_Disabled benchmarks text logging in disabled state
func BenchmarkFastlog_Text_Disabled(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      ERROR, // INFO/WARN/DEBUG disabled
		Stdout:     false,
		JSONFormat: false,
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
			logger.Info("test message") // Should be filtered
		}
	})
}

// BenchmarkFastlog_Sync_Enabled benchmarks synchronous logging mode
func BenchmarkFastlog_Sync_Enabled(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
		SyncMode:   true, // Sync mode
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

// BenchmarkFastlog_Async_Enabled benchmarks asynchronous logging mode
func BenchmarkFastlog_Async_Enabled(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:      INFO,
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
		SyncMode:   false, // Async mode
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

// BenchmarkFastlog_WithFields_3 benchmarks logging with 3 fields
func BenchmarkFastlog_WithFields_3(b *testing.B) {
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

	fields := map[string]interface{}{
		"user_id":    123,
		"request_id": "abc-123",
		"status":     "ok",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.WithFields(fields).Info("test message")
		}
	})
}

// BenchmarkFastlog_WithFields_10 benchmarks logging with 10 fields
func BenchmarkFastlog_WithFields_10(b *testing.B) {
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

	fields := map[string]interface{}{
		"field1":  "value1",
		"field2":  123,
		"field3":  true,
		"field4":  45.67,
		"field5":  "value5",
		"field6":  789,
		"field7":  false,
		"field8":  12.34,
		"field9":  "value9",
		"field10": 101112,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.WithFields(fields).Info("test message")
		}
	})
}

// BenchmarkFastlog_Text_vs_JSON compares text vs JSON formatting
func BenchmarkFastlog_Text_vs_JSON(b *testing.B) {
	// Text mode
	b.Run("Text", func(b *testing.B) {
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
				logger.Info("test message")
			}
		})
	})

	// JSON mode
	b.Run("JSON", func(b *testing.B) {
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
	})
}

// BenchmarkFastlog_Sync_vs_Async compares sync vs async modes
func BenchmarkFastlog_Sync_vs_Async(b *testing.B) {
	// Sync mode
	b.Run("Sync", func(b *testing.B) {
		logger, err := NewLogger(LoggerConfig{
			Level:      INFO,
			Stdout:     false,
			JSONFormat: false,
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
				logger.Info("test message")
			}
		})
	})

	// Async mode
	b.Run("Async", func(b *testing.B) {
		logger, err := NewLogger(LoggerConfig{
			Level:      INFO,
			Stdout:     false,
			JSONFormat: false,
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
				logger.Info("test message")
			}
		})
	})
}
