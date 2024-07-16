// fastlog - A high-performance logging package for Go.
//
// Copyright (c) 2024 AMAR SINGH RATHOUR
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package fastlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel defines the severity of the log message.
type LogLevel int

// Enumeration of different log levels.
const (
	DEBUG LogLevel = iota // Debug level for detailed internal information.
	INFO                  // Info level for general operational messages.
	WARN                  // Warn level for warnings about potential issues.
	ERROR                 // Error level for error messages indicating problems.
	FATAL                 // Fatal level for critical issues that require program exit.
)

// Constants for retry mechanism
const (
	retryInterval = 100 * time.Millisecond // Interval between retries
	maxRetries    = 5                      // Maximum number of retries
)

// Constants for buffer size, flush interval, and max log file size.
const (
	bufferSize     = 4096             // Buffer size in bytes.
	flushInterval  = 5 * time.Second  // Interval to flush buffer.
	maxLogFileSize = 10 * 1024 * 1024 // Maximum log file size before rotation (10 MB).
)

// LoggerConfig defines the configuration for the Logger.
type LoggerConfig struct {
	Level       LogLevel // Log level threshold.
	FilePath    string   // Path to the log file.
	RotationDir string   // Directory for rotated log files.
	Stdout      bool     // Whether to log to stdout instead of a file.
	JSONFormat  bool     // Whether to log in JSON format.
}

// Logger represents a logger with buffering and log rotation capabilities.
type Logger struct {
	mu           sync.Mutex    // Mutex to protect concurrent access to the logger.
	level        LogLevel      // Minimum log level for messages to be logged.
	writer       io.Writer     // Writer to which log messages are written.
	buffer       *bufio.Writer // Buffered writer for efficient logging.
	queue        chan string   // Channel for log messages to be processed.
	done         chan struct{} // Channel to signal logger shutdown.
	rotationDir  string        // Directory for log rotation.
	baseFileName string        // Base file name for log rotation.
	stdout       bool          // Flag indicating if logs are written to stdout.
	jsonFormat   bool          // Flag indicating if logs are in JSON format.
	file         *os.File      // File handle for log file.
}

// logEntry defines the structure of a log entry when JSONFormat is enabled.
type logEntry struct {
	Timestamp string      `json:"timestamp"` // Timestamp of the log entry.
	Level     string      `json:"level"`     // Log level of the entry.
	Message   interface{} `json:"message"`   // Log message.
}

// NewLogger creates a new Logger based on the provided LoggerConfig.
func NewLogger(config LoggerConfig) (*Logger, error) {
	var writer io.Writer
	var err error
	var file *os.File
	// Ensure the log directory exists
	if !config.Stdout {
		logDir := filepath.Dir(config.FilePath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}
	// Determine the writer based on config.
	if config.Stdout {
		writer = os.Stdout
	} else {
		file, err = os.OpenFile(config.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		writer = file
	}

	// Initialize the Logger.
	logger := &Logger{
		level:        config.Level,
		writer:       writer,
		buffer:       bufio.NewWriterSize(writer, bufferSize),
		queue:        make(chan string, 1000),
		done:         make(chan struct{}),
		rotationDir:  config.RotationDir,
		baseFileName: config.FilePath,
		stdout:       config.Stdout,
		jsonFormat:   config.JSONFormat,
		file:         file,
	}

	// Start background goroutines for processing log queue and periodic buffer flush.
	go logger.processQueue()
	go logger.periodicFlush()

	return logger, nil
}

// log writes a log message to the queue if the log level is above the threshold.
func (l *Logger) log(level LogLevel, args ...interface{}) {
	if level < l.level {
		return
	}

	timestamp := time.Now().Format(time.RFC3339)
	levelStr := [...]string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"}[level]

	var logMessage string
	if l.jsonFormat {
		entry := logEntry{
			Timestamp: timestamp,
			Level:     levelStr,
			Message:   args,
		}
		jsonEntry, err := json.Marshal(entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal log entry: %v\n", err)
			return
		}
		logMessage = string(jsonEntry) + "\n"
	} else {
		message := fmt.Sprint(args...)
		logMessage = fmt.Sprintf("%s [%s] %s\n", timestamp, levelStr, message)
	}

	// Push log message to queue with retry mechanism
	for retries := 0; retries < maxRetries; retries++ {
		select {
		case l.queue <- logMessage:
			// Log message enqueued successfully
			return
		default:
			// Queue is full, wait for retry interval
			time.Sleep(retryInterval)
		}
	}

	// If all retries failed, log to stderr
	fmt.Fprintf(os.Stderr, "logger queue full, dropping log message after %d retries: %s\n", maxRetries, logMessage)

}

// processQueue handles log messages from the queue and writes them to the buffer.
func (l *Logger) processQueue() {
	for {
		select {
		case logMessage := <-l.queue:
			func() {
				l.mu.Lock()
				defer l.mu.Unlock()
				fileLimit := l.logFileSizeExceeded()
				if !l.stdout && l.file != nil && fileLimit {
					if err := l.rotateLogFile(); err != nil {
						fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
					}
				}
				if l.file != nil {
					// Ensure file is still open
					if _, err := l.buffer.WriteString(logMessage); err != nil {
						fmt.Fprintf(os.Stderr, "failed to write log message: %v\n", err)
					}
					if err := l.buffer.Flush(); err != nil {
						fmt.Fprintf(os.Stderr, "failed to flush buffer: %v\n", err)
					}
				}
			}()
		case <-l.done:
			return
		}
	}
}

// periodicFlush periodically flushes the buffer to ensure logs are written to the file.
func (l *Logger) periodicFlush() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			if err := l.buffer.Flush(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to flush buffer: %v\n", err)
			}
			l.mu.Unlock()
		case <-l.done:
			return
		}
	}
}

// logFileSizeExceeded checks if the current log file size exceeds the maximum limit.
func (l *Logger) logFileSizeExceeded() bool {
	if l.file == nil {
		return false
	}
	stat, err := l.file.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to stat log file: %v\n", err)
		return false
	}
	return stat.Size() >= maxLogFileSize
}

// rotateLogFile handles log file rotation when the log file size exceeds the limit.
func (l *Logger) rotateLogFile() error {

	if l.stdout {
		return nil
	}
	// Flush buffer before rotation
	if err := l.buffer.Flush(); err != nil {
		return fmt.Errorf("failed to flush buffer during rotation: %w", err)
	}

	// Close current log file if open
	if l.file != nil {
		if err := l.file.Close(); err != nil {
			return fmt.Errorf("failed to close log file during rotation: %w", err)
		}
		l.file = nil
	}
	// Ensure the rotation directory exists
	if err := os.MkdirAll(l.rotationDir, 0755); err != nil {
		return fmt.Errorf("failed to create rotation directory: %w", err)
	}
	// Generate new log file name with timestamp
	timestamp := time.Now().Format("20060102-150405")
	newLogFileName := fmt.Sprintf("%s/%s-%s.log", l.rotationDir, filepath.Base(l.baseFileName), timestamp)

	// Open new log file for writing
	newFile, err := os.OpenFile(newLogFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open new log file during rotation: %w", err)
	}

	// Update logger state with new file and reset buffer
	l.file = newFile
	l.writer = newFile
	l.buffer.Reset(newFile)

	return nil
}

// Close flushes the buffer and closes the log file.
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure the done channel is closed once
	select {
	case <-l.done:
		// already closed
	default:
		close(l.done)
	}

	// Flush buffer before closing
	if err := l.buffer.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to flush buffer during close: %v\n", err)
	}

	// Close log file if not in stdout mode and file is open
	if !l.stdout && l.file != nil {
		if err := l.file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close log file: %v\n", err)
		}
		l.file = nil // Ensure file reference is cleared
	}
}

// Debug logs a message at the DEBUG level.
func (l *Logger) Debug(args ...interface{}) {
	l.log(DEBUG, args...)
}

// Info logs a message at the INFO level.
func (l *Logger) Info(args ...interface{}) {
	l.log(INFO, args...)
}

// Warn logs a message at the WARN level.
func (l *Logger) Warn(args ...interface{}) {
	l.log(WARN, args...)
}

// Error logs a message at the ERROR level.
func (l *Logger) Error(args ...interface{}) {
	l.log(ERROR, args...)
}

// Fatal logs a message at the FATAL level and exits the application.
func (l *Logger) Fatal(args ...interface{}) {
	l.log(FATAL, args...)
	l.Close() // Ensure log file is closed before exiting
	os.Exit(1)
}
