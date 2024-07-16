package fastlog

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Helper function to create a temporary log file for testing
func createTempLogFile(t *testing.T) (*os.File, string) {
	t.Helper()
	tmpfile, err := ioutil.TempFile("", "logfile-*.log")
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
	defer tmpfile.Close()

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
	defer tmpfile.Close()

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

	content, err := ioutil.ReadFile(logfilePath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	expected := "[INFO] This is an info message"
	if !strings.Contains(string(content), expected) {
		t.Errorf("Expected log message to contain %q, but got %q", expected, string(content))
	}
}

func TestLogFileRotation(t *testing.T) {
	logDir := "test_logs"
	baseLogFileName := "test_app.log"
	defer func() {
		os.RemoveAll(logDir)
		os.Remove(baseLogFileName)
	}()

	err := os.MkdirAll(logDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}

	loggerConfig := LoggerConfig{
		Level:       DEBUG,
		FilePath:    baseLogFileName,
		RotationDir: logDir,
		Stdout:      false,
		JSONFormat:  false,
	}

	logger, err := NewLogger(loggerConfig)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("This is a test message to trigger log rotation")

	// Log messages to trigger rotation
	for i := 0; i < 100000; i++ {
		logger.Info("This is a log message that will cause rotation. Writing a large amount of data to reach the log file size limit quickly. This is a log message that will cause rotation. Writing a large amount of data to reach the log file size limit quickly.")
	}

	// Ensure logs are flushed and rotation has occurred
	time.Sleep(10 * time.Second)

	// Check for rotated log files
	files, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	rotatedFileFound := false
	for _, file := range files {
		fmt.Println("file: ", file.Name())
		if strings.Contains(file.Name(), "test_app") {
			rotatedFileFound = true
			break
		}
	}

	if !rotatedFileFound {
		t.Fatalf("Expected rotated log file not found")
	}

	// Ensure logger is still functional after rotation
	logger.Info("Logging after rotation")
}

func TestJSONLogFormat(t *testing.T) {
	tmpfile, logfilePath := createTempLogFile(t)
	defer removeFile(t, logfilePath)
	defer tmpfile.Close()

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

	content, err := ioutil.ReadFile(logfilePath)
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
	defer r.Close()
	defer w.Close()

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
	w.Close()
	logOutput, _ := ioutil.ReadAll(r)

	// Check the captured output
	expectedLog := "[INFO] This is an info message to stdout"
	if !strings.Contains(string(logOutput), expectedLog) {
		t.Errorf("Expected log message '%s', got '%s'", expectedLog, logOutput)
	}
}

func BenchmarkLoggerStandardOutPut(b *testing.B) {
	// Setup logger configuration
	loggerConfig := LoggerConfig{
		Level:      DEBUG,
		Stdout:     true,
		JSONFormat: false,
	}

	// Create a logger instance
	logger, err := NewLogger(loggerConfig)
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("Benchmarking log message", "key", i)
	}

}
func BenchmarkLogging(b *testing.B) {
	// Setup logger configuration
	loggerConfig := LoggerConfig{
		Level:       DEBUG,
		FilePath:    "benchmark.log",  // Adjust as needed
		RotationDir: "benchmark_logs", // Adjust as needed
		Stdout:      false,
		JSONFormat:  false,
	}

	// Create a logger instance
	logger, err := NewLogger(loggerConfig)
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Reset the benchmark timer
	b.ResetTimer()

	// Benchmark logging performance
	for i := 0; i < b.N; i++ {
		logger.Info("Benchmarking log message")
	}

	// Cleanup log file after benchmark
	os.Remove(loggerConfig.FilePath)
	os.RemoveAll(loggerConfig.RotationDir)
}
