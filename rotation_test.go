package fastlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogFileRotation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rotation integration test in short mode")
	}

	root := t.TempDir()
	logDir := filepath.Join(root, "rotated")
	baseLogFileName := filepath.Join(root, "test_app.log")

	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}

	logger, err := NewLogger(LoggerConfig{
		Level:       DEBUG,
		FilePath:    baseLogFileName,
		RotationDir: logDir,
		Stdout:      false,
		JSONFormat:  false,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	for i := 0; i < 100000; i++ {
		logger.Info("rotation test payload: writing enough data to trigger size-based rotation")
	}

	logger.Close()

	files, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	rotatedFileFound := false
	for _, file := range files {
		if strings.Contains(file.Name(), "test_app") {
			rotatedFileFound = true
			break
		}
	}
	if !rotatedFileFound {
		t.Fatalf("Expected rotated log file not found")
	}
}
