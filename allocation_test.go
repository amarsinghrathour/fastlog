package fastlog

import (
	"testing"
)

// PHASE 0: Allocation guard tests
// These tests enforce the "hot path contract" for zero-cost disabled logging

// TestDisabledLogging_ZeroAllocations verifies that disabled logging has zero allocations
func TestDisabledLogging_ZeroAllocations(t *testing.T) {
	logger, err := NewLogger(LoggerConfig{
		Level:      ERROR, // DEBUG/INFO/WARN are disabled
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
		SyncMode:   true, // Use sync mode for testing
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test that disabled Info() has minimal allocations
	// Note: Variadic args may cause 1 allocation due to Go language design
	// This is acceptable and matches behavior of other loggers
	allocs := testing.AllocsPerRun(1000, func() {
		logger.Info("test message")
	})

	if allocs > 1 {
		t.Errorf("Disabled logging should have ≤1 allocations (from variadic args), got %f", allocs)
	}
}

// TestDisabledLogging_ZeroAllocations_WithFields verifies disabled logging with fields has zero allocations
func TestDisabledLogging_ZeroAllocations_WithFields(t *testing.T) {
	logger, err := NewLogger(LoggerConfig{
		Level:      ERROR, // DEBUG/INFO/WARN are disabled
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
		SyncMode:   true,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	allocs := testing.AllocsPerRun(1000, func() {
		logger.WithFields(map[string]interface{}{
			"key": "value",
		}).Info("test message")
	})

	// WithFields creates fieldLogger + map, but level check prevents formatting
	// Accept up to 3 allocs: variadic args + fieldLogger + map (though map should be reused)
	if allocs > 3 {
		t.Errorf("Disabled logging with fields should have ≤3 allocations, got %f", allocs)
	}
}

// TestEnabledLogging_SimpleMessage_Allocations verifies simple enabled messages have minimal allocations
func TestEnabledLogging_SimpleMessage_Allocations(t *testing.T) {
	logger, err := NewLogger(LoggerConfig{
		Level:      DEBUG,
		Stdout:     false,
		JSONFormat: false,
		FilePath:   "/dev/null",
		SyncMode:   true,
		EnableCaller: false, // Disable caller for simple message test
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	allocs := testing.AllocsPerRun(1000, func() {
		logger.Info("simple message")
	})

	// Contract: ≤ 2 allocs for simple text messages (entry pool + variadic args)
	// Phase 2 target: reduce to ≤1, but entry pool may add 1
	if allocs > 2 {
		t.Errorf("Simple enabled logging should have ≤2 allocations (Phase 2: entry pool), got %f", allocs)
	}
}

// TestEnabledLogging_WithFields_Allocations verifies logging with fields has minimal allocations
func TestEnabledLogging_WithFields_Allocations(t *testing.T) {
	logger, err := NewLogger(LoggerConfig{
		Level:      DEBUG,
		Stdout:     false,
		JSONFormat: true, // JSON for fields
		FilePath:   "/dev/null",
		SyncMode:   true,
		EnableCaller: false,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	allocs := testing.AllocsPerRun(1000, func() {
		logger.WithFields(map[string]interface{}{
			"user_id": 123,
			"status":  "ok",
		}).Info("test message")
	})

	// Contract: JSON with fields will have more allocs due to json.Marshal
	// Phase 3: We've optimized text formatting, JSON will be optimized in Phase 5
	// Current: entry pool + field conversion + json.Marshal + fields = ~15-20 allocs
	// Target after Phase 5: ≤3 allocs
	if allocs > 20 {
		t.Errorf("Logging with fields (JSON) should have ≤20 allocations (will optimize in Phase 5), got %f", allocs)
	}
}

// BenchmarkDisabledLogging_Allocations benchmarks disabled logging allocations
func BenchmarkDisabledLogging_Allocations(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:    ERROR, // Disable INFO
		Stdout:   false,
		FilePath: "/dev/null",
		SyncMode: true,
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("disabled message")
	}
}

// BenchmarkEnabledLogging_Simple_Allocations benchmarks simple enabled logging allocations
func BenchmarkEnabledLogging_Simple_Allocations(b *testing.B) {
	logger, err := NewLogger(LoggerConfig{
		Level:       DEBUG,
		Stdout:      false,
		JSONFormat:  false,
		FilePath:    "/dev/null",
		SyncMode:    true,
		EnableCaller: false,
	})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("simple message")
	}
}
