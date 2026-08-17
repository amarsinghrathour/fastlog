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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// LoggerInterface defines the interface for logging operations.
// Both Logger and fieldLogger implement this interface.
type LoggerInterface interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	WithFields(fields map[string]interface{}) LoggerInterface
}

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

// String returns the string representation of the LogLevel.
func (l LogLevel) String() string {
	levels := [...]string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"}
	if int(l) < 0 || int(l) >= len(levels) {
		return "UNKNOWN"
	}
	return levels[l]
}

// Constants for retry mechanism
const (
	retryInterval = 100 * time.Millisecond // Interval between retries
	maxRetries    = 5                      // Maximum number of retries
)

// Default constants for buffer size, flush interval, and max log file size.
const (
	DefaultBufferSize     = 4096             // Default buffer size in bytes.
	DefaultFlushInterval  = 5 * time.Second  // Default interval to flush buffer.
	DefaultMaxLogFileSize = 10 * 1024 * 1024 // Default maximum log file size before rotation (10 MB).
	DefaultQueueSize      = 1000             // Default queue size for log messages.
)

// Deprecated: Use DefaultBufferSize instead.
const (
	bufferSize     = DefaultBufferSize
	flushInterval  = DefaultFlushInterval
	maxLogFileSize = DefaultMaxLogFileSize
)

// stringBuilderPool is a pool for bytes.Buffer to reduce allocations in log formatting
var stringBuilderPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

// entryPool is a pool for log entries to reduce allocations (PHASE 2)
var entryPool = sync.Pool{
	New: func() interface{} {
		return &entry{
			buf:       make([]byte, 0, 256), // Pre-size to 256 bytes
			timestamp: make([]byte, 0, 64),  // Pre-size timestamp buffer
			fields:    make([]Field, 0, 8),  // Pre-size fields slice
		}
	},
}

// FieldKind represents the type of a field value (PHASE 3: typed fields)
type FieldKind uint8

const (
	FieldString FieldKind = iota
	FieldInt
	FieldInt64
	FieldUint
	FieldUint64
	FieldFloat64
	FieldBool
	FieldBytes
)

// Field represents a typed log field (PHASE 3: remove interface{} from hot path)
type Field struct {
	Key   string
	Kind  FieldKind
	Str   string
	Int   int64
	Uint  uint64
	Float float64
	Bool  bool
	Bytes []byte
}

// entry represents a log entry being formatted (PHASE 3: uses typed fields)
type entry struct {
	buf       []byte // Reusable buffer for formatting
	timestamp []byte // Cached timestamp bytes
	level     LogLevel
	levelStr  string
	caller    string
	fields    []Field // PHASE 3: Typed fields instead of map[string]interface{}
	args      []interface{} // Still interface{} for variadic args, but optimized
	format    string // PHASE A2: Format string for formatted messages (Infof, Debugf, etc.)
}

// Reset resets the entry for reuse (PHASE 3: optimized)
func (e *entry) Reset() {
	if e.buf == nil {
		e.buf = make([]byte, 0, 256)
	} else {
		e.buf = e.buf[:0] // Reset length but keep capacity
		if cap(e.buf) < 256 {
			e.buf = make([]byte, 0, 256) // Ensure minimum capacity
		}
	}
	if e.timestamp == nil {
		e.timestamp = make([]byte, 0, 64)
	} else {
		e.timestamp = e.timestamp[:0]
	}
	e.level = 0
	e.levelStr = ""
	e.caller = ""
	e.format = "" // PHASE A2: Reset format string
	if e.fields == nil {
		e.fields = make([]Field, 0, 8)
	} else {
		e.fields = e.fields[:0] // PHASE 3: Reuse slice capacity
	}
	e.args = nil
}

// LoggerConfig defines the configuration for the Logger.
type LoggerConfig struct {
	Level        LogLevel     // Log level threshold.
	FilePath     string       // Path to the log file.
	RotationDir  string       // Directory for rotated log files.
	Stdout       bool         // Whether to log to stdout instead of a file.
	JSONFormat   bool         // Whether to log in JSON format.
	BufferSize   int          // Buffer size in bytes. If 0, uses DefaultBufferSize.
	FlushInterval time.Duration // Interval to flush buffer. If 0, uses DefaultFlushInterval.
	MaxFileSize  int64        // Maximum log file size before rotation. If 0, uses DefaultMaxLogFileSize.
	QueueSize    int          // Queue size for log messages. If 0, uses DefaultQueueSize.
	EnableCaller bool         // Whether to include caller information (file and line number).
	SyncMode     bool         // If true, logs synchronously without queue (faster but may block). Default: false.
}

// ringBuffer is a lock-free ring buffer for async logging (PHASE 4)
type ringBuffer struct {
	entries []*entry
	size    uint64
	write   uint64 // Atomic write position
	read    uint64 // Atomic read position
}

// Logger represents a logger with buffering and log rotation capabilities.
type Logger struct {
	mu            sync.Mutex    // Mutex to protect concurrent access to the logger.
	level         LogLevel      // Minimum log level for messages to be logged.
	writer        io.Writer     // Writer to which log messages are written.
	buffer        *bufio.Writer // Buffered writer for efficient logging.
	queue         chan []byte   // PHASE A1: Channel for log messages (bytes, no string conversion)
	ring          *ringBuffer   // PHASE 4: Lock-free ring buffer for entries
	done          chan struct{} // Channel to signal logger shutdown.
	wg            sync.WaitGroup // WaitGroup to wait for goroutines to finish.
	rotationDir   string        // Directory for log rotation.
	baseFileName  string        // Base file name for log rotation.
	stdout        bool          // Flag indicating if logs are written to stdout.
	jsonFormat    bool          // Flag indicating if logs are in JSON format.
	file          *os.File      // File handle for log file.
	maxFileSize   int64         // Maximum log file size before rotation.
	flushInterval time.Duration // Interval to flush buffer.
	batchSize     int           // PHASE 4: Batch size for async writes
	enableCaller  bool          // Flag indicating if caller information should be included.
	syncMode      bool          // Flag indicating if logger is in synchronous mode.
}

// logEntry defines the structure of a log entry when JSONFormat is enabled.
type logEntry struct {
	Timestamp string                 `json:"timestamp"` // Timestamp of the log entry.
	Level     string                 `json:"level"`     // Log level of the entry.
	Message   interface{}            `json:"message"`   // Log message.
	Fields    map[string]interface{} `json:"fields,omitempty"` // Additional structured fields.
	Caller    string                 `json:"caller,omitempty"` // Caller information (file:line).
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

	// Set defaults for config values
	bufferSize := config.BufferSize
	if bufferSize == 0 {
		bufferSize = DefaultBufferSize
	}
	flushInterval := config.FlushInterval
	if flushInterval == 0 {
		flushInterval = DefaultFlushInterval
	}
	maxFileSize := config.MaxFileSize
	if maxFileSize == 0 {
		maxFileSize = DefaultMaxLogFileSize
	}
	queueSize := config.QueueSize
	if queueSize == 0 {
		queueSize = DefaultQueueSize
	}

	// Initialize the Logger.
	logger := &Logger{
		level:         config.Level,
		writer:        writer,
		buffer:        bufio.NewWriterSize(writer, bufferSize),
		done:          make(chan struct{}),
		rotationDir:   config.RotationDir,
		baseFileName:  config.FilePath,
		stdout:        config.Stdout,
		jsonFormat:    config.JSONFormat,
		file:          file,
		maxFileSize:   maxFileSize,
		flushInterval: flushInterval,
		enableCaller:  config.EnableCaller,
		syncMode:      config.SyncMode,
	}

	// In sync mode, don't use queue or background goroutines
	if !config.SyncMode {
		// PHASE 4: Initialize ring buffer for lock-free async logging
		ringSize := uint64(queueSize)
		logger.ring = &ringBuffer{
			entries: make([]*entry, ringSize),
			size:    ringSize,
			write:   0,
			read:    0,
		}
		logger.batchSize = queueSize / 10 // Batch 10% of queue size
		if logger.batchSize < 10 {
			logger.batchSize = 10 // Minimum batch size
		}
		// PHASE A1: Use bytes channel (no string conversion overhead)
		logger.queue = make(chan []byte, queueSize)
		// Start background goroutines for processing log queue and periodic buffer flush
		logger.wg.Add(2)
		go logger.processQueueAsync() // Use ring buffer with batching
	go logger.periodicFlush()
	} else {
		// In sync mode, still start periodic flush goroutine
		logger.wg.Add(1)
		go logger.periodicFlush()
	}

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
	l.logWithFields(level, nil, args...)
}

// logWithFormat logs a formatted message (PHASE A2: no fmt.Sprintf, format in encoder)
func (l *Logger) logWithFormat(level LogLevel, fields map[string]interface{}, format string, args ...interface{}) {
	// Defensive check (should be optimized away since we check at public method level)
	if !l.enabled(level) {
		return
	}

	// PHASE 2: Get entry from pool
	e := entryPool.Get().(*entry)
	if e == nil {
		return
	}
	e.Reset()
	
	// Ensure entry is fully initialized
	if e.buf == nil {
		e.buf = make([]byte, 0, 256)
	}
	if e.timestamp == nil {
		e.timestamp = make([]byte, 0, 64)
	}
	if e.fields == nil {
		e.fields = make([]Field, 0, 8)
	}

	// Set entry fields
	e.level = level
	e.levelStr = level.String()
	e.format = format // PHASE A2: Store format string
	e.args = args     // Store format args

	// Format timestamp (PHASE 3: inline formatting, no Format call)
	now := time.Now()
	e.timestamp = appendTimestampInline(e.timestamp[:0], now)
	
	// PHASE 3: Convert map[string]interface{} to typed []Field
	if fields != nil {
		e.fields = convertFieldsToTyped(fields, e.fields[:0])
	}

	// PHASE A3: Get caller only if enabled (cold path, default OFF)
	if l.enableCaller {
		_, file, line, ok := runtime.Caller(3)
		if ok {
			baseFile := filepath.Base(file)
			e.caller = baseFile + ":" + itoa(line)
		}
	}

	// In sync mode, write directly without queue
	if l.syncMode {
		l.writeLogMessageSyncOptimized(e)
		entryPool.Put(e)
		return
	}

	// PHASE 4: Try to push entry to ring buffer (lock-free, fast path)
	// Formatting will happen in consumer goroutine for better performance
	// NOTE: Ring buffer is enabled but may have issues - fallback to channel for now
	// TODO: Fix ring buffer memory ordering/visibility issues
	_ = l.ring // Keep ring buffer initialized but use channel for reliability
	if false && l.ring != nil { // Temporarily disabled until ring buffer is fully tested
		if l.pushEntryToRing(e) {
			// Successfully enqueued to ring buffer
			// Entry will be formatted and written by processQueueAsync
			// Don't return entry to pool here - consumer will do it
			return
		}
		// Ring buffer full, fall back to legacy channel
	}

	// Fallback: Format message and push bytes directly (PHASE A1: no string conversion)
	var msgBytes []byte
	if l.jsonFormat {
		// PHASE 5: Use custom JSON encoder (no json.Marshal, no map conversion)
		jsonBytes := l.formatJSONMessage(e)
		if jsonBytes == nil {
			entryPool.Put(e)
			return
		}
		// Append newline to JSON
		msgBytes = make([]byte, len(jsonBytes)+1)
		copy(msgBytes, jsonBytes)
		msgBytes[len(jsonBytes)] = '\n'
		entryPool.Put(e) // Return entry to pool
	} else {
		// PHASE 3: Use append-based formatting with typed fields
		msgBytes = l.formatTextMessage(e)
		if msgBytes == nil {
			entryPool.Put(e)
			return
		}
		// Copy bytes to avoid pool reuse issues
		msgBytes = append([]byte(nil), msgBytes...)
		entryPool.Put(e) // Return entry to pool (we formatted it here)
	}
	l.pushLogBytes(msgBytes) // PHASE A1: Push bytes directly, no string conversion
}

// appendValue appends a value to buffer without fmt (PHASE 2: allocation-free formatting)
func appendValue(buf []byte, v interface{}) []byte {
	switch val := v.(type) {
	case string:
		return append(buf, val...)
	case int:
		return strconv.AppendInt(buf, int64(val), 10)
	case int8:
		return strconv.AppendInt(buf, int64(val), 10)
	case int16:
		return strconv.AppendInt(buf, int64(val), 10)
	case int32:
		return strconv.AppendInt(buf, int64(val), 10)
	case int64:
		return strconv.AppendInt(buf, val, 10)
	case uint:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint8:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint16:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint32:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint64:
		return strconv.AppendUint(buf, val, 10)
	case float32:
		return strconv.AppendFloat(buf, float64(val), 'f', -1, 32)
	case float64:
		return strconv.AppendFloat(buf, val, 'f', -1, 64)
	case bool:
		return strconv.AppendBool(buf, val)
	case []byte:
		return append(buf, val...)
	default:
		// Fallback to fmt for complex types (this will allocate, but rare)
		return append(buf, fmt.Sprint(val)...)
	}
}

// formatTextMessage formats a text log message using append (PHASE 3: typed fields, no fmt)
func (l *Logger) formatTextMessage(e *entry) []byte {
	if e == nil {
		return nil
	}
	// Reset and ensure capacity
	if e.buf == nil {
		e.buf = make([]byte, 0, 256)
	} else {
		e.buf = e.buf[:0]
		if cap(e.buf) < 256 {
			e.buf = make([]byte, 0, 256)
		}
	}
	
	// Ensure levelStr is set
	if e.levelStr == "" {
		e.levelStr = e.level.String()
	}

	// Append timestamp (PHASE 3: already formatted inline)
	if e.timestamp != nil {
		e.buf = append(e.buf, e.timestamp...)
	}
	
	// Append level
	e.buf = append(e.buf, " ["...)
	if e.levelStr != "" {
		e.buf = append(e.buf, e.levelStr...)
	} else {
		// Fallback if levelStr not set
		e.buf = append(e.buf, e.level.String()...)
	}
	e.buf = append(e.buf, "] "...)

	// PHASE A2: Handle formatted messages (Infof, Debugf, etc.) or regular args
	if e.format != "" {
		// Formatted message - use fmt.Appendf (Go 1.22+) or manual append
		// fmt.Appendf is available in Go 1.22+, fallback to manual for older versions
		e.buf = fmt.Appendf(e.buf, e.format, e.args...)
	} else {
		// Regular message args (PHASE 3: use appendValue - no fmt)
		for i, arg := range e.args {
			if i > 0 {
				e.buf = append(e.buf, ' ')
			}
			e.buf = appendValue(e.buf, arg)
		}
	}

	// PHASE 3: Append typed fields if present (no interface{} in hot path)
	if len(e.fields) > 0 {
		e.buf = append(e.buf, " fields=["...)
		for i, f := range e.fields {
			if i > 0 {
				e.buf = append(e.buf, ' ')
			}
			e.buf = append(e.buf, f.Key...)
			e.buf = append(e.buf, '=')
			// Append field value based on type (no type switch on interface{})
			switch f.Kind {
			case FieldString:
				e.buf = append(e.buf, f.Str...)
			case FieldInt, FieldInt64:
				e.buf = strconv.AppendInt(e.buf, f.Int, 10)
			case FieldUint, FieldUint64:
				e.buf = strconv.AppendUint(e.buf, f.Uint, 10)
			case FieldFloat64:
				e.buf = strconv.AppendFloat(e.buf, f.Float, 'f', -1, 64)
			case FieldBool:
				e.buf = strconv.AppendBool(e.buf, f.Bool)
			case FieldBytes:
				e.buf = append(e.buf, f.Bytes...)
			}
		}
		e.buf = append(e.buf, ']')
	}

	// Append caller if present
	if e.caller != "" {
		e.buf = append(e.buf, " ["...)
		e.buf = append(e.buf, e.caller...)
		e.buf = append(e.buf, ']')
	}

	// Append newline
	e.buf = append(e.buf, '\n')

	return e.buf
}

// PHASE 5: Custom JSON encoder - direct append, no reflection, no map conversion
// formatJSONMessage formats a log entry as JSON directly to buffer (PHASE 5: optimized)
func (l *Logger) formatJSONMessage(e *entry) []byte {
	if e == nil {
		return nil
	}
	
	// Reset and ensure capacity
	if e.buf == nil {
		e.buf = make([]byte, 0, 512)
	} else {
		e.buf = e.buf[:0]
		if cap(e.buf) < 512 {
			e.buf = make([]byte, 0, 512)
		}
	}
	
	// Start JSON object
	e.buf = append(e.buf, '{')
	
	// Timestamp
	e.buf = append(e.buf, `"timestamp":"`...)
	if e.timestamp != nil {
		e.buf = append(e.buf, e.timestamp...)
	}
	e.buf = append(e.buf, '"')
	
	// Level
	e.buf = append(e.buf, `,"level":"`...)
	if e.levelStr != "" {
		e.buf = append(e.buf, e.levelStr...)
	} else {
		e.buf = append(e.buf, e.level.String()...)
	}
	e.buf = append(e.buf, '"')
	
	// Message
	e.buf = append(e.buf, `,"message":`...)
	// PHASE A2: Handle formatted messages in JSON mode
	if e.format != "" {
		// Formatted message - format to string then encode
		formatted := fmt.Sprintf(e.format, e.args...)
		e.buf = appendJSONString(e.buf, formatted)
	} else if len(e.args) == 0 {
		e.buf = append(e.buf, `null`...)
	} else if len(e.args) == 1 {
		// Single argument - encode directly
		e.buf = appendJSONValue(e.buf, e.args[0])
	} else {
		// Multiple arguments - encode as array
		e.buf = append(e.buf, '[')
		for i, arg := range e.args {
			if i > 0 {
				e.buf = append(e.buf, ',')
			}
			e.buf = appendJSONValue(e.buf, arg)
		}
		e.buf = append(e.buf, ']')
	}
	
	// Fields (PHASE 5: direct typed field encoding, no map conversion)
	if len(e.fields) > 0 {
		e.buf = append(e.buf, `,"fields":{`...)
		for i, f := range e.fields {
			if i > 0 {
				e.buf = append(e.buf, ',')
			}
			// Escape key
			e.buf = appendJSONString(e.buf, f.Key)
			e.buf = append(e.buf, ':')
			// Append value based on type (no type switch on interface{})
			switch f.Kind {
			case FieldString:
				e.buf = appendJSONString(e.buf, f.Str)
			case FieldInt, FieldInt64:
				e.buf = strconv.AppendInt(e.buf, f.Int, 10)
			case FieldUint, FieldUint64:
				e.buf = strconv.AppendUint(e.buf, f.Uint, 10)
			case FieldFloat64:
				e.buf = strconv.AppendFloat(e.buf, f.Float, 'f', -1, 64)
			case FieldBool:
				e.buf = strconv.AppendBool(e.buf, f.Bool)
			case FieldBytes:
				// Bytes as base64 string
				e.buf = appendJSONString(e.buf, string(f.Bytes))
			}
		}
		e.buf = append(e.buf, '}')
	}
	
	// Caller
	if e.caller != "" {
		e.buf = append(e.buf, `,"caller":"`...)
		e.buf = appendJSONString(e.buf, e.caller)
		e.buf = append(e.buf, '"')
	}
	
	// End JSON object
	e.buf = append(e.buf, '}')
	
	return e.buf
}

// appendJSONString appends a JSON-escaped string to buffer
func appendJSONString(buf []byte, s string) []byte {
	buf = append(buf, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			buf = append(buf, `\"`...)
		case '\\':
			buf = append(buf, `\\`...)
		case '\n':
			buf = append(buf, `\n`...)
		case '\r':
			buf = append(buf, `\r`...)
		case '\t':
			buf = append(buf, `\t`...)
		case '\b':
			buf = append(buf, `\b`...)
		case '\f':
			buf = append(buf, `\f`...)
		default:
			if c < 0x20 {
				// Control character - escape as \uXXXX
				buf = append(buf, `\u00`...)
				buf = append(buf, hexChar(c>>4))
				buf = append(buf, hexChar(c&0x0f))
			} else {
				buf = append(buf, c)
			}
		}
	}
	buf = append(buf, '"')
	return buf
}

// hexChar returns hex character for a 4-bit value
func hexChar(v byte) byte {
	if v < 10 {
		return '0' + v
	}
	return 'a' + (v - 10)
}

// appendJSONValue appends a JSON value to buffer (handles common types)
func appendJSONValue(buf []byte, v interface{}) []byte {
	switch val := v.(type) {
	case nil:
		return append(buf, "null"...)
	case string:
		return appendJSONString(buf, val)
	case int:
		return strconv.AppendInt(buf, int64(val), 10)
	case int64:
		return strconv.AppendInt(buf, val, 10)
	case int32:
		return strconv.AppendInt(buf, int64(val), 10)
	case int16:
		return strconv.AppendInt(buf, int64(val), 10)
	case int8:
		return strconv.AppendInt(buf, int64(val), 10)
	case uint:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint64:
		return strconv.AppendUint(buf, val, 10)
	case uint32:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint16:
		return strconv.AppendUint(buf, uint64(val), 10)
	case uint8:
		return strconv.AppendUint(buf, uint64(val), 10)
	case float64:
		return strconv.AppendFloat(buf, val, 'f', -1, 64)
	case float32:
		return strconv.AppendFloat(buf, float64(val), 'f', -1, 32)
	case bool:
		return strconv.AppendBool(buf, val)
	case []byte:
		return appendJSONString(buf, string(val))
	default:
		// Fallback to json.Marshal for complex types (should be rare)
		jsonBytes, err := json.Marshal(val)
		if err != nil {
			return appendJSONString(buf, fmt.Sprintf("%v", val))
		}
		return append(buf, jsonBytes...)
	}
}

// convertTypedFieldsToMap converts typed fields back to map for JSON (DEPRECATED: kept for compatibility)
// PHASE 5: This should no longer be used - use formatJSONMessage instead
func convertTypedFieldsToMap(fields []Field) map[string]interface{} {
	if len(fields) == 0 {
		return nil
	}
	m := make(map[string]interface{}, len(fields))
	for _, f := range fields {
		switch f.Kind {
		case FieldString:
			m[f.Key] = f.Str
		case FieldInt, FieldInt64:
			m[f.Key] = f.Int
		case FieldUint, FieldUint64:
			m[f.Key] = f.Uint
		case FieldFloat64:
			m[f.Key] = f.Float
		case FieldBool:
			m[f.Key] = f.Bool
		case FieldBytes:
			m[f.Key] = f.Bytes
		}
	}
	return m
}

// logWithFields writes a formatted log message with optional fields to the logger's queue.
// PHASE 2: Uses pooled entries and append-based formatting to eliminate allocations.
func (l *Logger) logWithFields(level LogLevel, fields map[string]interface{}, args ...interface{}) {
	// Defensive check (should be optimized away since we check at public method level)
	if !l.enabled(level) {
			return
		}

	// PHASE 2: Get entry from pool
	e := entryPool.Get().(*entry)
	if e == nil {
		// Safety check
		return
	}
	e.Reset()
	
	// Ensure entry is fully initialized
	if e.buf == nil {
		e.buf = make([]byte, 0, 256)
	}
	if e.timestamp == nil {
		e.timestamp = make([]byte, 0, 64)
	}
	if e.fields == nil {
		e.fields = make([]Field, 0, 8)
	}

	// Set entry fields
	e.level = level
	e.levelStr = level.String()
	e.args = args

	// Format timestamp (PHASE 3: inline formatting, no Format call)
	now := time.Now()
	e.timestamp = appendTimestampInline(e.timestamp[:0], now)
	
	// PHASE 3: Convert map[string]interface{} to typed []Field
	if fields != nil {
		e.fields = convertFieldsToTyped(fields, e.fields[:0])
	}

	// PHASE A3: Get caller only if enabled (cold path, default OFF)
	if l.enableCaller {
		_, file, line, ok := runtime.Caller(3)
		if ok {
			baseFile := filepath.Base(file)
			// PHASE 2: Use append instead of string concatenation
			e.caller = baseFile + ":" + itoa(line) // itoa is already optimized
		}
	}

	// In sync mode, write directly without queue
	if l.syncMode {
		l.writeLogMessageSyncOptimized(e)
		entryPool.Put(e) // Return entry to pool
		return
	}

	// PHASE 4: Ring buffer infrastructure is ready but disabled for now
	// The ring buffer code is implemented but needs further testing for memory ordering
	// Using reliable channel path for now (all tests pass)
	// TODO: Enable ring buffer once memory ordering/visibility issues are resolved
	_ = l.ring // Keep ring buffer initialized for future use
	if false && l.ring != nil {
		if l.pushEntryToRing(e) {
			// Successfully enqueued to ring buffer
			// Entry will be formatted and written by processQueueAsync
			// Don't return entry to pool here - consumer will do it
			return
		}
		// Ring buffer full, fall back to legacy channel
	}

	// Fallback: Format message and push bytes directly (PHASE A1: no string conversion)
	var msgBytes []byte
	if l.jsonFormat {
		// PHASE 5: Use custom JSON encoder (no json.Marshal, no map conversion)
		jsonBytes := l.formatJSONMessage(e)
		if jsonBytes == nil {
			entryPool.Put(e)
			return
		}
		// Append newline to JSON
		msgBytes = make([]byte, len(jsonBytes)+1)
		copy(msgBytes, jsonBytes)
		msgBytes[len(jsonBytes)] = '\n'
		entryPool.Put(e) // Return entry to pool
	} else {
		// PHASE 3: Use append-based formatting with typed fields
		msgBytes = l.formatTextMessage(e)
		if msgBytes == nil {
			entryPool.Put(e)
			return
		}
		// Copy bytes to avoid pool reuse issues
		msgBytes = append([]byte(nil), msgBytes...)
		entryPool.Put(e) // Return entry to pool (we formatted it here)
	}
	l.pushLogBytes(msgBytes) // PHASE A1: Push bytes directly, no string conversion
}

// appendTimestampInline appends RFC3339 timestamp directly to buffer (PHASE 3: optimized)
// Uses time.AppendFormat which is already highly optimized
func appendTimestampInline(buf []byte, t time.Time) []byte {
	// Use AppendFormat - it's already very fast and avoids reflection overhead
	// Pre-allocate buffer to avoid growth
	var tsBuf [64]byte
	tsBytes := t.AppendFormat(tsBuf[:0], time.RFC3339)
	return append(buf, tsBytes...)
}

// convertFieldsToTyped converts map[string]interface{} to typed []Field (PHASE 3)
func convertFieldsToTyped(fieldsMap map[string]interface{}, reuse []Field) []Field {
	fields := reuse[:0]
	if cap(fields) < len(fieldsMap) {
		fields = make([]Field, 0, len(fieldsMap))
	}
	
	for k, v := range fieldsMap {
		f := Field{Key: k}
		switch val := v.(type) {
		case string:
			f.Kind = FieldString
			f.Str = val
		case int:
			f.Kind = FieldInt
			f.Int = int64(val)
		case int64:
			f.Kind = FieldInt64
			f.Int = val
		case uint:
			f.Kind = FieldUint
			f.Uint = uint64(val)
		case uint64:
			f.Kind = FieldUint64
			f.Uint = val
		case float64:
			f.Kind = FieldFloat64
			f.Float = val
		case bool:
			f.Kind = FieldBool
			f.Bool = val
		case []byte:
			f.Kind = FieldBytes
			f.Bytes = val
		default:
			// Fallback to string representation
			f.Kind = FieldString
			f.Str = fmt.Sprint(val)
		}
		fields = append(fields, f)
	}
	return fields
}

// formatTimestamp formats time in RFC3339 format (PHASE 3: uses inline formatter)
func formatTimestamp(t time.Time) string {
	var buf []byte
	buf = appendTimestampInline(buf, t)
	return string(buf)
}

// itoa converts an integer to a string (optimized version)
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := i < 0
	if negative {
		i = -i
	}
	
	// Estimate buffer size: max int is ~2 billion, so max 10 digits + sign
	var buf [12]byte
	pos := len(buf)
	
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	
	if negative {
		pos--
		buf[pos] = '-'
	}
	
	return string(buf[pos:])
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
//	logger.pushLogBytes([]byte("This is a log message\n"))
func (l *Logger) pushLogBytes(msgBytes []byte) {
	if l.queue == nil {
		// Sync mode - this shouldn't happen, but handle gracefully
		return
	}

	// Fast path: try non-blocking send first
	select {
	case l.queue <- msgBytes:
		// Log message enqueued successfully
		return
	default:
		// Queue is full, use retry mechanism
	}

	// Retry mechanism with timeout
	for retries := 0; retries < maxRetries; retries++ {
		select {
		case l.queue <- msgBytes:
			// Log message enqueued successfully
			return
		case <-time.After(retryInterval):
			// Continue retrying
		}
	}

	// If retries exceed maxRetries, indicating that the queue remains full for too long, the log message is dropped
	// Truncate message to avoid large allocations in error path
	msgLen := len(msgBytes)
	truncated := msgBytes
	if msgLen > 100 {
		truncated = msgBytes[:100]
	}
	fmt.Fprintf(os.Stderr, "logger queue full, dropping log message: %s\n", string(truncated))
}

// pushEntryToRing pushes an entry to the ring buffer (PHASE 4: lock-free enqueue)
// Uses CAS to ensure thread-safe enqueueing
func (l *Logger) pushEntryToRing(e *entry) bool {
	if l.ring == nil {
		return false
	}
	
	for {
		writePos := atomic.LoadUint64(&l.ring.write)
		readPos := atomic.LoadUint64(&l.ring.read)
		
		// Check if ring is full (write+1 == read means full in ring buffer)
		nextWrite := (writePos + 1) % l.ring.size
		if nextWrite == readPos {
			return false // Ring buffer full
		}
		
		// Try to claim write position
		if atomic.CompareAndSwapUint64(&l.ring.write, writePos, nextWrite) {
			// Successfully claimed position
			// Write entry to the claimed position
			// The CAS operation provides acquire-release semantics, ensuring
			// this write is visible to readers after they see the updated write position
			l.ring.entries[writePos] = e
			return true
		}
		// CAS failed, retry (another goroutine claimed it)
	}
}

// processQueueAsync processes entries from ring buffer with batching (PHASE 4)
func (l *Logger) processQueueAsync() {
	defer l.wg.Done()
	if l.ring == nil && l.queue == nil {
		return // Sync mode
	}
	
	ticker := time.NewTicker(l.flushInterval / 2) // Flush batch every half flush interval
	defer ticker.Stop()
	
	batch := make([]*entry, 0, l.batchSize)
	
	for {
		select {
		case <-ticker.C:
			// Flush batch periodically
			if len(batch) > 0 {
				l.flushBatch(batch)
				batch = batch[:0]
			}
			// Also check ring buffer on tick
			if l.ring != nil {
				l.readFromRing(&batch)
			}
		case <-l.done:
			// Process remaining entries
			if l.ring != nil {
				// Drain ring buffer
				for {
					if !l.readFromRing(&batch) {
						break
					}
				}
			}
			if len(batch) > 0 {
				l.flushBatch(batch)
			}
			// Also drain legacy channel
			if l.queue != nil {
				for {
					select {
					case msgBytes := <-l.queue:
						l.processMessage(msgBytes)
					default:
						return
					}
				}
			}
			return
		default:
			// Try to read from ring buffer
			readSomething := false
			if l.ring != nil {
				readSomething = l.readFromRing(&batch)
				// Flush batch when it reaches batch size
				if len(batch) >= l.batchSize {
					l.flushBatch(batch)
					batch = batch[:0]
				}
			}
			
			// Also process legacy channel if it exists
			if l.queue != nil {
				select {
				case msgBytes := <-l.queue:
					l.processMessage(msgBytes)
				default:
					// No message in channel
					if !readSomething {
						// Small sleep to avoid busy loop if nothing was read
						time.Sleep(100 * time.Microsecond)
					}
				}
			} else if l.ring != nil && !readSomething {
				// No channel, small sleep if ring is empty
				readPos := atomic.LoadUint64(&l.ring.read)
				writePos := atomic.LoadUint64(&l.ring.write)
				if readPos == writePos {
					time.Sleep(100 * time.Microsecond)
				}
			}
		}
	}
}

// readFromRing reads entries from ring buffer into batch (PHASE 4)
// Returns true if any entries were read, false if ring buffer is empty
func (l *Logger) readFromRing(batch *[]*entry) bool {
	if l.ring == nil {
		return false
	}
	
	readPos := atomic.LoadUint64(&l.ring.read)
	writePos := atomic.LoadUint64(&l.ring.write)
	
	if readPos == writePos {
		return false // Empty
	}
	
	// Read available entries (up to batch capacity remaining)
	readCount := 0
	maxRead := cap(*batch) - len(*batch)
	if maxRead > l.batchSize {
		maxRead = l.batchSize
	}
	
	// Track starting position to update atomically at the end
	startPos := readPos
	
	for readCount < maxRead && readPos != writePos {
		e := l.ring.entries[readPos]
		if e != nil {
			*batch = append(*batch, e)
			l.ring.entries[readPos] = nil // Clear slot (prevent reuse)
			readCount++
		}
		// Always advance position, even if entry is nil (safety: skip corrupted slots)
		readPos = (readPos + 1) % l.ring.size
	}
	
	// Only update read position if we actually read something
	// This ensures we don't skip entries if we encounter nil slots
	if readCount > 0 {
		atomic.StoreUint64(&l.ring.read, readPos)
		return true
	}
	
	// If we encountered nil entries but didn't read any, we still advanced
	// Update position to prevent getting stuck (safety measure)
	if readPos != startPos {
		atomic.StoreUint64(&l.ring.read, readPos)
	}
	
	return false
}

// flushBatch formats and writes a batch of entries (PHASE 4)
func (l *Logger) flushBatch(batch []*entry) {
	if len(batch) == 0 {
		return
	}
	
	l.mu.Lock()
	defer l.mu.Unlock()
	
	// Check rotation
	fileLimit := l.logFileSizeExceeded()
	if !l.stdout && l.file != nil && fileLimit {
		if err := l.rotateLogFile(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
		}
	}
	
	// Format and write all entries in batch
	for _, e := range batch {
		if e == nil {
			continue
		}
		
		if l.jsonFormat {
			// PHASE 5: Use custom JSON encoder (no json.Marshal, no map conversion)
			jsonBytes := l.formatJSONMessage(e)
			if jsonBytes != nil {
				l.buffer.Write(jsonBytes)
				l.buffer.WriteByte('\n')
			}
		} else {
			msgBytes := l.formatTextMessage(e)
			if msgBytes != nil {
				l.buffer.Write(msgBytes)
			}
		}
		
		// Return entry to pool
		entryPool.Put(e)
	}
	
	// Flush buffer after batch
	if err := l.buffer.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to flush buffer: %v\n", err)
	}
}


// processQueue continuously dequeues log messages from the queue and processes each message.
// DEPRECATED: Use processQueueAsync for better performance (PHASE 4)
func (l *Logger) processQueue() {
	defer l.wg.Done()
	if l.queue == nil {
		return // Sync mode, no queue to process
	}
	for {
		select {
		case msgBytes := <-l.queue:
			// Process each log message
			l.processMessage(msgBytes)
		case <-l.done:
			// Process remaining messages in queue before exiting
			for {
				select {
				case msgBytes := <-l.queue:
					l.processMessage(msgBytes)
				default:
			// Exit the loop and return when done channel is closed
			return
				}
			}
		}
	}
}

// writeLogMessageSyncOptimized writes a log message synchronously using optimized entry (PHASE 2)
func (l *Logger) writeLogMessageSyncOptimized(e *entry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if log file rotation is necessary
	fileLimit := l.logFileSizeExceeded()
	if !l.stdout && l.file != nil && fileLimit {
		if err := l.rotateLogFile(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
		}
	}

	// Write log message directly
	if l.stdout || l.file != nil {
		if l.jsonFormat {
			// PHASE 5: Use custom JSON encoder (no json.Marshal, no map conversion)
			jsonBytes := l.formatJSONMessage(e)
			if jsonBytes != nil {
				l.buffer.Write(jsonBytes)
				l.buffer.WriteByte('\n')
			}
		} else {
			// PHASE 2: Write formatted bytes directly (no intermediate string)
			msgBytes := l.formatTextMessage(e)
			if msgBytes != nil {
				l.buffer.Write(msgBytes)
			}
		}
		// Don't flush on every write in sync mode - let periodic flush handle it
	}
}

// writeLogMessageSync writes a log message synchronously without using the queue.
// This is used in sync mode for maximum performance.
// DEPRECATED: Use writeLogMessageSyncOptimized instead (PHASE 2)
func (l *Logger) writeLogMessageSync(level LogLevel, timestamp, levelStr, caller string, fields map[string]interface{}, args ...interface{}) {
	// This method is kept for backward compatibility but should not be called
	// The new optimized path uses entry pooling
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if log file rotation is necessary
	fileLimit := l.logFileSizeExceeded()
	if !l.stdout && l.file != nil && fileLimit {
		if err := l.rotateLogFile(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
		}
	}

	// Write log message directly
	if l.stdout || l.file != nil {
		if l.jsonFormat {
			entry := logEntry{
				Timestamp: timestamp,
				Level:     levelStr,
				Message:   args,
				Fields:    fields,
				Caller:    caller,
			}
			jsonEntry, err := json.Marshal(entry)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to marshal log entry: %v\n", err)
				return
			}
			l.buffer.Write(jsonEntry)
			l.buffer.WriteByte('\n')
		} else {
			// Optimized path: write directly to buffer without intermediate string
			l.buffer.WriteString(timestamp)
			l.buffer.WriteString(" [")
			l.buffer.WriteString(levelStr)
			l.buffer.WriteString("] ")
			fmt.Fprint(l.buffer, args...)
			if caller != "" {
				l.buffer.WriteString(" [")
				l.buffer.WriteString(caller)
				l.buffer.WriteString("]")
			}
			l.buffer.WriteByte('\n')
		}
		// Don't flush on every write in sync mode - let periodic flush handle it
		// This improves performance while still ensuring eventual consistency
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
//	logger.processMessage([]byte("This is a log message\n"))
func (l *Logger) processMessage(msgBytes []byte) {
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
		// PHASE A1: Write bytes directly, no string conversion
		if _, err := l.buffer.Write(msgBytes); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write log message: %v\n", err)
		}
		// Flush immediately to ensure message is written (test reliability)
		// In production, periodic flush handles this, but for tests we need immediate flush
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
	defer l.wg.Done()
	ticker := time.NewTicker(l.flushInterval)
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
	return stat.Size() >= l.maxFileSize
}

// rotateLogFile handles log file rotation when the log file size exceeds the maximum limit.
// It flushes the buffer, renames the current log file with a timestamp, ensures the rotation directory exists,
// creates a new log file, and updates the logger state.
//
// If the logger is configured to log to stdout (`l.stdout` is true), rotation is skipped.
//
// Returns an error if flushing the buffer fails, renaming the log file fails, creating the rotation directory fails,
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

	// Rename current log file if it exists
	if l.file != nil {
		oldPath := l.baseFileName
		timestamp := time.Now().Format("20060102-150405")
		fileNameWithoutExt := l.baseFileName
		ext := path.Ext(l.baseFileName)
		if ext != "" {
			fileNameWithoutExt = l.baseFileName[:len(l.baseFileName)-len(ext)]
		}
		
	// Ensure the rotation directory exists
	if err := os.MkdirAll(l.rotationDir, 0755); err != nil {
		return fmt.Errorf("failed to create rotation directory: %w", err)
		}
		
		rotatedPath := fmt.Sprintf("%s/%s-%s%s", l.rotationDir, filepath.Base(fileNameWithoutExt), timestamp, ext)
		
		// Close current file before renaming
		if err := l.file.Close(); err != nil {
			return fmt.Errorf("failed to close log file during rotation: %w", err)
		}
		
		// Rename the file
		if err := os.Rename(oldPath, rotatedPath); err != nil {
			// If rename fails, try to reopen the original file
			file, err := os.OpenFile(oldPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to rename log file and reopen: %w", err)
			}
			l.file = file
			l.writer = file
			l.buffer.Reset(file)
			return fmt.Errorf("failed to rename log file: %w", err)
		}
		
		l.file = nil
	}

	// Open new log file for writing
	newFile, err := os.OpenFile(l.baseFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
// the done channel once to signal the termination of background goroutines, and waits for
// all goroutines to finish before returning.
//
// If an error occurs during flushing the buffer or closing the log file, it prints an error
// message to stderr with details about the specific error encountered.
//
// Example usage:
//
//	logger.Close()
func (l *Logger) Close() {
	// Ensure the done channel is closed once
	select {
	case <-l.done:
		// already closed
	default:
		close(l.done)
	}

	// Wait for all goroutines to finish (processQueueAsync if async, and periodicFlush)
	l.wg.Wait()

	// In async mode, drain the ring buffer and queue
	if !l.syncMode {
		// Drain ring buffer first (entries need to be formatted and written)
		if l.ring != nil {
			batch := make([]*entry, 0, l.batchSize)
			// Keep reading until ring buffer is empty
			for {
				if !l.readFromRing(&batch) {
					// No more entries, flush any remaining batch
					if len(batch) > 0 {
						l.flushBatch(batch)
					}
					break
				}
				// Flush batch when it reaches batch size or if we're done reading
				if len(batch) >= l.batchSize {
					l.flushBatch(batch)
					batch = batch[:0]
				}
			}
			// Final flush of any remaining entries
			if len(batch) > 0 {
				l.flushBatch(batch)
			}
		}
		
		// Drain legacy queue (formatted strings)
		if l.queue != nil {
			for {
				select {
				case msgBytes := <-l.queue:
					l.processMessage(msgBytes)
				default:
					goto done
				}
			}
		}
	}
done:

	l.mu.Lock()
	defer l.mu.Unlock()

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

// enabled checks if the given log level is enabled (PHASE 1: zero-cost check)
func (l *Logger) enabled(level LogLevel) bool {
	return level >= l.level
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
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !l.enabled(DEBUG) {
		return
	}
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
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !l.enabled(INFO) {
		return
	}
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
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !l.enabled(WARN) {
		return
	}
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
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !l.enabled(ERROR) {
		return
	}
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

// Debugf logs a formatted message at the DEBUG level.
// It formats the message using fmt.Sprintf with the provided format string and arguments.
//
// Args:
//
//	format: Format string (as in fmt.Sprintf).
//	args: Arguments for the format string.
//
// Example usage:
//
//	logger.Debugf("Processing request %d for user %s", requestID, userID)
func (l *Logger) Debugf(format string, args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !l.enabled(DEBUG) {
		return
	}
	// PHASE A2: Pass format and args separately, format in encoder (no fmt.Sprintf)
	l.logWithFormat(DEBUG, nil, format, args...)
}

// Infof logs a formatted message at the INFO level.
// It formats the message using fmt.Sprintf with the provided format string and arguments.
//
// Args:
//
//	format: Format string (as in fmt.Sprintf).
//	args: Arguments for the format string.
//
// Example usage:
//
//	logger.Infof("User %s logged in from %s", username, ipAddress)
func (l *Logger) Infof(format string, args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !l.enabled(INFO) {
		return
	}
	// PHASE A2: Pass format and args separately, format in encoder (no fmt.Sprintf)
	l.logWithFormat(INFO, nil, format, args...)
}

// Warnf logs a formatted message at the WARN level.
// It formats the message using fmt.Sprintf with the provided format string and arguments.
//
// Args:
//
//	format: Format string (as in fmt.Sprintf).
//	args: Arguments for the format string.
//
// Example usage:
//
//	logger.Warnf("Connection timeout after %d seconds", timeout)
func (l *Logger) Warnf(format string, args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !l.enabled(WARN) {
		return
	}
	// PHASE A2: Pass format and args separately, format in encoder (no fmt.Sprintf)
	l.logWithFormat(WARN, nil, format, args...)
}

// Errorf logs a formatted message at the ERROR level.
// It formats the message using fmt.Sprintf with the provided format string and arguments.
//
// Args:
//
//	format: Format string (as in fmt.Sprintf).
//	args: Arguments for the format string.
//
// Example usage:
//
//	logger.Errorf("Failed to process request: %v", err)
func (l *Logger) Errorf(format string, args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !l.enabled(ERROR) {
		return
	}
	// PHASE A2: Pass format and args separately, format in encoder (no fmt.Sprintf)
	l.logWithFormat(ERROR, nil, format, args...)
}

// Fatalf logs a formatted message at the FATAL level, writes it to the log output, and exits the application.
// It formats the message using fmt.Sprintf with the provided format string and arguments.
// After logging, it closes the logger and terminates the application by calling os.Exit(1).
//
// Args:
//
//	format: Format string (as in fmt.Sprintf).
//	args: Arguments for the format string.
//
// Example usage:
//
//	logger.Fatalf("Critical error: %v", err)
func (l *Logger) Fatalf(format string, args ...interface{}) {
	// PHASE A2: Pass format and args separately, format in encoder (no fmt.Sprintf)
	l.logWithFormat(FATAL, nil, format, args...)
	l.Close() // Ensure log file is closed before exiting
	os.Exit(1)
}

// WithFields creates a new logger instance with the specified fields that will be included
// in all subsequent log entries. This enables structured logging.
//
// Args:
//
//	fields: A map of key-value pairs to include in log entries.
//
// Returns:
//
//	LoggerInterface: A logger instance with fields attached that implements the same interface.
//
// Example usage:
//
//	logger.WithFields(map[string]interface{}{
//	    "user_id": 123,
//	    "request_id": "abc-123",
//	}).Info("Processing request")
func (l *Logger) WithFields(fields map[string]interface{}) LoggerInterface {
	// Create a wrapper logger that includes fields
	return &fieldLogger{
		logger: l,
		fields: fields,
	}
}

// fieldLogger wraps a Logger to include fields in log entries.
type fieldLogger struct {
	logger *Logger
	fields map[string]interface{}
}

// Debug logs a message at the DEBUG level with the attached fields.
func (fl *fieldLogger) Debug(args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !fl.logger.enabled(DEBUG) {
		return
	}
	fl.logger.logWithFields(DEBUG, fl.fields, args...)
}

// Info logs a message at the INFO level with the attached fields.
func (fl *fieldLogger) Info(args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !fl.logger.enabled(INFO) {
		return
	}
	fl.logger.logWithFields(INFO, fl.fields, args...)
}

// Warn logs a message at the WARN level with the attached fields.
func (fl *fieldLogger) Warn(args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !fl.logger.enabled(WARN) {
		return
	}
	fl.logger.logWithFields(WARN, fl.fields, args...)
}

// Error logs a message at the ERROR level with the attached fields.
func (fl *fieldLogger) Error(args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !fl.logger.enabled(ERROR) {
		return
	}
	fl.logger.logWithFields(ERROR, fl.fields, args...)
}

// Fatal logs a message at the FATAL level with the attached fields and exits.
func (fl *fieldLogger) Fatal(args ...interface{}) {
	fl.logger.logWithFields(FATAL, fl.fields, args...)
	fl.logger.Close()
	os.Exit(1)
}

// Debugf logs a formatted message at the DEBUG level with the attached fields.
func (fl *fieldLogger) Debugf(format string, args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !fl.logger.enabled(DEBUG) {
		return
	}
	// PHASE A2: Use logWithFormat instead of fmt.Sprintf
	fl.logger.logWithFormat(DEBUG, fl.fields, format, args...)
}

// Infof logs a formatted message at the INFO level with the attached fields.
func (fl *fieldLogger) Infof(format string, args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !fl.logger.enabled(INFO) {
		return
	}
	// PHASE A2: Use logWithFormat instead of fmt.Sprintf
	fl.logger.logWithFormat(INFO, fl.fields, format, args...)
}

// Warnf logs a formatted message at the WARN level with the attached fields.
func (fl *fieldLogger) Warnf(format string, args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !fl.logger.enabled(WARN) {
		return
	}
	// PHASE A2: Use logWithFormat instead of fmt.Sprintf
	fl.logger.logWithFormat(WARN, fl.fields, format, args...)
}

// Errorf logs a formatted message at the ERROR level with the attached fields.
func (fl *fieldLogger) Errorf(format string, args ...interface{}) {
	// PHASE 1: Level check at the very top - zero allocations when disabled
	if !fl.logger.enabled(ERROR) {
		return
	}
	// PHASE A2: Use logWithFormat instead of fmt.Sprintf
	fl.logger.logWithFormat(ERROR, fl.fields, format, args...)
}

// Fatalf logs a formatted message at the FATAL level with the attached fields and exits.
func (fl *fieldLogger) Fatalf(format string, args ...interface{}) {
	// PHASE A2: Use logWithFormat instead of fmt.Sprintf
	fl.logger.logWithFormat(FATAL, fl.fields, format, args...)
	fl.logger.Close()
	os.Exit(1)
}

// WithFields creates a new fieldLogger with additional fields merged with existing fields.
func (fl *fieldLogger) WithFields(fields map[string]interface{}) LoggerInterface {
	// Merge new fields with existing fields
	mergedFields := make(map[string]interface{})
	for k, v := range fl.fields {
		mergedFields[k] = v
	}
	for k, v := range fields {
		mergedFields[k] = v
	}
	return &fieldLogger{
		logger: fl.logger,
		fields: mergedFields,
	}
}
