package fastlog

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockWriter struct {
	buf bytes.Buffer
}

func (mw *mockWriter) Write(p []byte) (n int, err error) {
	return mw.buf.Write(p)
}

func TestLogger(t *testing.T) {
	mw := &mockWriter{}
	
	logger := &Logger{
		level:      DEBUG,
		writer:     mw,
		buffer:     bufio.NewWriterSize(mw, bufferSize),
		queue:      make(chan string, 1000),
		done:       make(chan struct{}),
		stdout:     true,
		jsonFormat: false,
	}
	go logger.processQueue()
	
	logger.Debug("Debug message")
	logger.Info("Info message")
	logger.Warn("Warn message")
	logger.Error("Error message")
	logger.Fatal("Fatal message")
	
	// Give some time for the logger to process the messages
	time.Sleep(1 * time.Second)
	
	logger.Close()
	
	logOutput := mw.buf.String()
	
	if !bytes.Contains([]byte(logOutput), []byte("Debug message")) {
		t.Errorf("Expected debug message not found in log output")
	}
	if !bytes.Contains([]byte(logOutput), []byte("Info message")) {
		t.Errorf("Expected info message not found in log output")
	}
	if !bytes.Contains([]byte(logOutput), []byte("Warn message")) {
		t.Errorf("Expected warn message not found in log output")
	}
	if !bytes.Contains([]byte(logOutput), []byte("Error message")) {
		t.Errorf("Expected error message not found in log output")
	}
	if !bytes.Contains([]byte(logOutput), []byte("Fatal message")) {
		t.Errorf("Expected fatal message not found in log output")
	}
}

func TestLogRotation(t *testing.T) {
	logDir := "testlogs"
	baseLogFileName := filepath.Join(logDir, "test.log")
	
	err := os.MkdirAll(logDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}
	defer os.RemoveAll(logDir)
	
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
	
	for i := 0; i < 100000; i++ {
		logger.Info("This is a test message for log rotation")
	}
	
	logger.Close()
	
	files, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}
	
	if len(files) < 2 {
		t.Errorf("Expected multiple rotated log files, found %d", len(files))
	}
}

func BenchmarkLogger(b *testing.B) {
	mw := &mockWriter{}
	
	logger := &Logger{
		level:      DEBUG,
		writer:     mw,
		buffer:     bufio.NewWriterSize(mw, bufferSize),
		queue:      make(chan string, 1000),
		done:       make(chan struct{}),
		stdout:     true,
		jsonFormat: false,
	}
	go logger.processQueue()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("Benchmarking log message", "key", i)
	}
	logger.Close()
}
