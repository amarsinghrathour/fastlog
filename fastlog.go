package fastlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	
	// Determine the writer based on config.
	if config.Stdout {
		writer = os.Stdout
	} else {
		writer, err = os.OpenFile(config.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
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
	
	l.queue <- logMessage
}

// processQueue handles log messages from the queue and writes them to the buffer.
func (l *Logger) processQueue() {
	for {
		select {
		case logMessage := <-l.queue:
			l.mu.Lock()
			if !l.stdout && l.logFileSizeExceeded() {
				if err := l.rotateLogFile(); err != nil {
					fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
				}
			}
			if _, err := l.buffer.WriteString(logMessage); err != nil {
				fmt.Fprintf(os.Stderr, "failed to write log message: %v\n", err)
			}
			l.mu.Unlock()
		case <-l.done:
			return
		}
	}
}

// periodicFlush periodically flushes the buffer to ensure logs are written to the file.
func (l *Logger) periodicFlush() {
	ticker := time.NewTicker(flushInterval)
	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			if err := l.buffer.Flush(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to flush buffer: %v\n", err)
			}
			l.mu.Unlock()
		case <-l.done:
			ticker.Stop()
			return
		}
	}
}

// logFileSizeExceeded checks if the current log file size exceeds the maximum limit.
func (l *Logger) logFileSizeExceeded() bool {
	file, ok := l.writer.(*os.File)
	if !ok {
		return false
	}
	stat, err := file.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to stat log file: %v\n", err)
		return false
	}
	return stat.Size() >= maxLogFileSize
}

// rotateLogFile handles log file rotation when the log file size exceeds the limit.
func (l *Logger) rotateLogFile() error {
	file, ok := l.writer.(*os.File)
	if !ok {
		return fmt.Errorf("writer is not a file")
	}
	if err := l.buffer.Flush(); err != nil {
		return fmt.Errorf("failed to flush buffer during rotation: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close log file during rotation: %w", err)
	}
	
	timestamp := time.Now().Format("20060102-150405")
	newLogFileName := fmt.Sprintf("%s/%s-%s.log", l.rotationDir, l.baseFileName, timestamp)
	if err := os.Rename(l.baseFileName, newLogFileName); err != nil {
		return fmt.Errorf("failed to rename log file during rotation: %w", err)
	}
	
	newFile, err := os.OpenFile(l.baseFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open new log file during rotation: %w", err)
	}
	
	l.writer = newFile
	l.buffer = bufio.NewWriterSize(newFile, bufferSize)
	return nil
}

// Close flushes the buffer and closes the log file.
func (l *Logger) Close() {
	close(l.done)
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.buffer.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to flush buffer during close: %v\n", err)
	}
	file, ok := l.writer.(*os.File)
	if ok && !l.stdout {
		if err := file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close log file: %v\n", err)
		}
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
	l.Close()
	os.Exit(1)
}
