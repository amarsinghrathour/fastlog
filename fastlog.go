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
	"path"
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

// NewLogger creates a new Logger instance based on the provided LoggerConfig.
//
// Parameters:
//
//	config: LoggerConfig containing the configuration options for the Logger.
//
// Returns:
//
//	(*Logger, error): A pointer to the Logger instance and any error encountered during initialization.
//
// Example usage:
//
//	config := LoggerConfig{
//	  Level:      INFO,
//	  FilePath:   "logs/myapp.log",
//	  RotationDir: "logs/rotation",
//	  Stdout:     false,
//	  JSONFormat: true,
//	}
//	logger, err := NewLogger(config)
//	if err != nil {
//	  log.Fatalf("Failed to create logger: %v", err)
//	}
//	defer logger.Close()
//
// Notes:
// - If Stdout is true, log messages are directed to standard output (os.Stdout).
// - If Stdout is false, a log file is created or opened based on FilePath for writing log messages.
// - The log directory specified in FilePath or RotationDir is created if it doesn't exist.
// - The logger starts background goroutines for processing log messages from the queue and periodic buffer flushing.
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

// log writes a formatted log message to the logger's queue if the specified log level
// is greater than or equal to the logger's current log level threshold.
//
// Parameters:
//
//	level: The LogLevel of the log message.
//	args: Variadic arguments representing the content of the log message.
//
// Example usage:
//
//	logger.log(INFO, "This is an informational message")
//
// Notes:
//   - The log message format depends on whether JSON formatting is enabled in the logger configuration.
//   - If JSON formatting is enabled, the log message is marshaled into JSON format.
//   - Otherwise, the log message is formatted as timestamp [log_level] message.
//   - If the specified log level is lower than the logger's current log level threshold,
//     the log message is not logged.
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

	l.pushLogMessage(logMessage)
}

// pushLogMessage attempts to push a log message to the logger's queue.
// It retries pushing the log message to the queue with a retry mechanism if the queue is full.
// If retries exceed the maximum retries (maxRetries), indicating persistent queue full conditions,
// the log message is dropped.
//
// Parameters:
//
//	logMessage: The log message to be pushed to the queue.
//
// Example usage:
//
//	logger.pushLogMessage("This is a log message")
func (l *Logger) pushLogMessage(logMessage string) {
	// Push log message to queue with retry mechanism
	for retries := 0; retries < maxRetries; retries++ {
		select {
		case l.queue <- logMessage:
			// Log message enqueued successfully
			return
		case <-time.After(retryInterval):

		}
	}

	// If retries exceed maxRetries, indicating that the queue remains full for too long, the log message is dropped
	fmt.Fprintf(os.Stderr, "logger queue full, dropping log message: %s\n", logMessage)
}

// processQueue continuously dequeues log messages from the queue and processes each message.
// It handles each log message by invoking the processMessage method to write it to the
// appropriate output destination (stdout or file), and performs log file rotation if necessary.
//
// This method runs indefinitely until the done channel is closed, indicating the logger
// should shut down. It efficiently handles concurrent log message processing using a select
// statement, ensuring it is responsive to both incoming log messages and shutdown signals.
//
// Notes:
//   - It assumes processMessage method is responsible for managing the write lock (l.mu).
//
// Example usage:
//
//	go logger.processQueue()
func (l *Logger) processQueue() {
	for {
		select {
		case logMessage := <-l.queue:
			// Process each log message
			l.processMessage(logMessage)
		case <-l.done:

			// Exit the loop and return when done channel is closed
			return
		}
	}
}

// processMessage handles a log message, ensuring it is written to the appropriate
// output destination (stdout or file), and performs log file rotation if necessary.
//
// It acquires a write lock to protect concurrent access to the logger state, checks
// if the log file size has exceeded the limit, and rotates the log file if needed.
// If the logger is configured to write to stdout or if a log file is open, it writes
// the log message to the buffer, flushes the buffer to ensure the message is written
// to the file immediately, and logs any encountered errors to stderr.
//
// Parameters:
//
//	logMessage: The log message to be processed and written.
//
// Notes:
//   - This method assumes the caller has already acquired a write lock on l.mu.
//   - It does not perform any action if both stdout and file are closed.
//
// Example usage:
//
//	logger.processMessage("This is a log message")
func (l *Logger) processMessage(logMessage string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Check if log file rotation is necessary
	fileLimit := l.logFileSizeExceeded()
	if !l.stdout && l.file != nil && fileLimit {
		// Rotate log file if size limit exceeded
		if err := l.rotateLogFile(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
		}
	}
	// Write log message to the appropriate destination
	if l.stdout || l.file != nil {
		// Ensure file is still open
		if _, err := l.buffer.WriteString(logMessage); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write log message: %v\n", err)
		}
		if err := l.buffer.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to flush buffer: %v\n", err)
		}
	}

}

// newLogFile generates a new log file with a timestamp appended to the base file name,
// opens it for writing, and returns a pointer to the opened file.
// It uses the rotation directory (l.rotationDir) and the base file name (l.baseFileName)
// to construct the new log file's path.
//
// If the file creation or opening fails, it returns an error with a descriptive message
// wrapped around the underlying error.
//
// Example usage:
//
//	newFile, err := l.newLogFile()
//	if err != nil {
//	    fmt.Printf("Failed to create new log file: %v\n", err)
//	    return
//	}
//	defer newFile.Close()
//
// Note: Ensure that the rotation directory exists and is accessible before calling this method.
func (l *Logger) newLogFile() (*os.File, error) {
	// Generate new log file name with timestamp
	timestamp := time.Now().Format("20060102-150405")
	fileNameWithoutExt := path.Ext(l.baseFileName) // Get the extension
	fileNameWithoutExt = l.baseFileName[:len(l.baseFileName)-len(fileNameWithoutExt)]
	newLogFileName := fmt.Sprintf("%s/%s-%s.log", l.rotationDir, fileNameWithoutExt, timestamp)

	// Open new log file for writing
	newFile, err := os.OpenFile(newLogFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open new log file during rotation: %w", err)
	}

	return newFile, nil
}

// periodicFlush periodically flushes the buffer to ensure logged messages are written to the output.
// It uses a ticker with the specified flushInterval duration to trigger buffer flushes.
// The method locks the logger's mutex (l.mu) during the flush operation to prevent concurrent access.
//
// The flush operation is aborted if the logger's done channel is closed, signaling shutdown.
// If an error occurs during buffer flushing, it is printed to stderr.
//
// Example usage:
//
//	go logger.periodicFlush()
//
// Note: Ensure that the logger's done channel is properly closed to terminate the periodic flushing routine.
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

// logFileSizeExceeded checks if the current log file size exceeds the maximum limit (maxLogFileSize).
// It returns true if the log file size is greater than or equal to maxLogFileSize, otherwise false.
// If l.file is nil (no log file open), it returns false.
//
// Returns false and prints an error to stderr if there is an error obtaining the file stat information.
//
// Example usage:
//
//	if logger.logFileSizeExceeded() {
//	    fmt.Println("Log file size exceeded maximum limit.")
//	}
func (l *Logger) logFileSizeExceeded() bool {
	if l.file == nil {
		return false // No log file open, size not exceeded
	}
	stat, err := l.file.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to stat log file: %v\n", err)
		return false
	}
	return stat.Size() >= maxLogFileSize
}

// rotateLogFile handles log file rotation when the log file size exceeds the maximum limit.
// It flushes the buffer, closes the current log file (if open), ensures the rotation directory exists,
// creates a new log file with a timestamped name in the rotation directory, and updates the logger state.
//
// If the logger is configured to log to stdout (`l.stdout` is true), rotation is skipped.
//
// Returns an error if flushing the buffer fails, closing the log file fails, creating the rotation directory fails,
// or opening the new log file fails.
//
// Example usage:
//
//	if err := logger.rotateLogFile(); err != nil {
//	    fmt.Printf("Failed to rotate log file: %v\n", err)
//	}
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

	// Open new log file for writing
	newFile, err := l.newLogFile()
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
// It ensures that all pending log messages are flushed to the underlying log file or stdout,
// and then closes the log file if it was opened during logger initialization.
// The method also ensures that the logger's resources are cleaned up properly by closing
// the done channel once to signal the termination of background goroutines.
//
// If an error occurs during flushing the buffer or closing the log file, it prints an error
// message to stderr with details about the specific error encountered.
//
// Example usage:
//
//	logger.Close()
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
// It logs the provided arguments at the DEBUG level using the log method. Debug-level messages
// are used for detailed internal information, typically useful for debugging purposes.
//
// Args:
//
//	args: Variadic list of arguments to log as the message.
//
// Example usage:
//
//	logger.Debug("Received request", requestID)
func (l *Logger) Debug(args ...interface{}) {
	l.log(DEBUG, args...)
}

// Info logs a message at the INFO level.
// It logs the provided arguments at the INFO level using the log method. If the log level
// threshold is set to INFO or lower, the message will be written to the log output (stdout or file),
// providing general operational messages or information.
//
// Args:
//
//	args: Variadic list of arguments to log as the message.
//
// Example usage:
//
//	logger.Info("Application started successfully.")
func (l *Logger) Info(args ...interface{}) {
	l.log(INFO, args...)
}

// Warn logs a message at the WARN level.
// It logs the provided arguments at the WARN level using the log method. If the log level
// threshold is set to WARN or lower, the message will be written to the log output (stdout or file),
// indicating potential issues or warnings.
//
// Args:
//
//	args: Variadic list of arguments to log as the message.
//
// Example usage:
//
//	logger.Warn("Connection timeout. Retrying...")
func (l *Logger) Warn(args ...interface{}) {
	l.log(WARN, args...)
}

// Error logs a message at the ERROR level.
// It logs the provided arguments at the ERROR level using the log method. If the log level
// threshold is set to ERROR or lower, the message will be written to the log output (stdout or file),
// ensuring it is captured for error reporting.
//
// Args:
//
//	args: Variadic list of arguments to log as the message.
//
// Example usage:
//
//	logger.Error("Failed to process request:", err)
func (l *Logger) Error(args ...interface{}) {
	l.log(ERROR, args...)
}

// Fatal logs a message at the FATAL level, writes it to the log output, and exits the application.
// It logs the provided arguments at the FATAL level using the log method, ensuring the message is
// written even if it exceeds the log level threshold. After logging, it closes the log file using
// Close method to flush any remaining log messages in the buffer and close the file handle. Finally,
// it terminates the application by calling os.Exit(1), indicating a critical error.
//
// This method is intended for logging critical errors that necessitate immediate application termination.
//
// Args:
//
//	args: Variadic list of arguments to log as the message.
//
// Example usage:
//
//	logger.Fatal("Critical error occurred: database connection lost")
func (l *Logger) Fatal(args ...interface{}) {
	l.log(FATAL, args...)
	l.Close() // Ensure log file is closed before exiting
	os.Exit(1)
}
