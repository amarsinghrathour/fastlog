package fastlog

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Helper function to create a temporary log file for testing
func createTempLogFile(t *testing.T) (*os.File, string) {
	t.Helper()
	tmpfile, err := os.CreateTemp("", "logfile-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	return tmpfile, tmpfile.Name()
}

// Helper function to remove a file after testing
func removeFile(t *testing.T, filepath string) {
	t.Helper()
	if err := os.Remove(filepath); err != nil {
		t.Fatalf("Failed to remove file: %v", err)
	}
}

func TestNewLogger(t *testing.T) {
	tmpfile, logfilePath := createTempLogFile(t)
	defer removeFile(t, logfilePath)
	defer func() { _ = tmpfile.Close() }()

	loggerConfig := LoggerConfig{
		Level:       DEBUG,
		FilePath:    logfilePath,
		RotationDir: filepath.Dir(logfilePath),
		Stdout:      false,
		JSONFormat:  false,
	}

	logger, err := NewLogger(loggerConfig)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()
}

func TestLogMessage(t *testing.T) {
	tmpfile, logfilePath := createTempLogFile(t)
	defer removeFile(t, logfilePath)
	defer func() { _ = tmpfile.Close() }()

	loggerConfig := LoggerConfig{
		Level:       DEBUG,
		FilePath:    logfilePath,
		RotationDir: filepath.Dir(logfilePath),
		Stdout:      false,
		JSONFormat:  false,
	}

	logger, err := NewLogger(loggerConfig)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("This is an info message")

	time.Sleep(1 * time.Second) // Give some time for the logger to process the queue

	content, err := os.ReadFile(logfilePath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	expected := "[INFO] This is an info message"
	if !strings.Contains(string(content), expected) {
		t.Errorf("Expected log message to contain %q, but got %q", expected, string(content))
	}
}

func TestJSONLogFormat(t *testing.T) {
	tmpfile, logfilePath := createTempLogFile(t)
	defer removeFile(t, logfilePath)
	defer func() { _ = tmpfile.Close() }()

	loggerConfig := LoggerConfig{
		Level:       DEBUG,
		FilePath:    logfilePath,
		RotationDir: filepath.Dir(logfilePath),
		Stdout:      false,
		JSONFormat:  true,
	}

	logger, err := NewLogger(loggerConfig)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("This is a JSON info message")

	time.Sleep(1 * time.Second) // Give some time for the logger to process the queue

	content, err := os.ReadFile(logfilePath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	expected := `"level":"INFO"`
	if !strings.Contains(string(content), expected) {
		t.Errorf("Expected log message to contain %q, but got %q", expected, string(content))
	}
}

func TestLogToStdout(t *testing.T) {
	// Create a pipe to capture stdout
	r, w, _ := os.Pipe()
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	// Redirect stdout to the pipe
	old := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	loggerConfig := LoggerConfig{
		Level:      DEBUG,
		Stdout:     true,  // Log to stdout
		JSONFormat: false, // Not using JSON format for this test
	}

	// Create a logger instance
	logger, err := NewLogger(loggerConfig)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Log a message
	logger.Info("This is an info message to stdout")

	// Wait for the log message to be written
	time.Sleep(100 * time.Millisecond)

	// Capture output from the pipe
	_ = w.Close()
	logOutput, _ := io.ReadAll(r)

	// Check the captured output
	expectedLog := "[INFO] This is an info message to stdout"
	if !strings.Contains(string(logOutput), expectedLog) {
		t.Errorf("Expected log message '%s', got '%s'", expectedLog, logOutput)
	}
}
