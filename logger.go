package fastlog

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	internalcore "github.com/amarsinghrathour/fastlog/internal/core"
	internalformat "github.com/amarsinghrathour/fastlog/internal/format"
	internalqueue "github.com/amarsinghrathour/fastlog/internal/queue"
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
	WithFields(fields ...Field) LoggerInterface
}

// Logger represents a logger with buffering and log rotation capabilities.
type Logger struct {
	mu            sync.Mutex    // Mutex to protect concurrent access to the logger.
	level         LogLevel      // Minimum log level for messages to be logged.
	writer        io.Writer     // Writer to which log messages are written.
	buffer        *bufio.Writer // Buffered writer for efficient logging.
	queue         chan []byte
	ring          *internalqueue.RingBuffer[*entry]
	done          chan struct{}  // Channel to signal logger shutdown.
	wg            sync.WaitGroup // WaitGroup to wait for goroutines to finish.
	rotationDir   string         // Directory for log rotation.
	baseFileName  string         // Base file name for log rotation.
	stdout        bool           // Flag indicating if logs are written to stdout.
	jsonFormat    bool           // Flag indicating if logs are in JSON format.
	file          *os.File       // File handle for log file.
	maxFileSize   int64          // Maximum log file size before rotation.
	flushInterval time.Duration  // Interval to flush buffer.
	batchSize     int
	enableCaller  bool // Flag indicating if caller information should be included.
	syncMode      bool // Flag indicating if logger is in synchronous mode.
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

	if !config.Stdout {
		logDir := filepath.Dir(config.FilePath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	if config.Stdout {
		writer = os.Stdout
	} else {
		file, err = os.OpenFile(config.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		writer = file
	}

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
	rotationDir := internalcore.ResolveRotationDir(config.Stdout, config.FilePath, config.RotationDir)

	logger := &Logger{
		level:         config.Level,
		writer:        writer,
		buffer:        bufio.NewWriterSize(writer, bufferSize),
		done:          make(chan struct{}),
		rotationDir:   rotationDir,
		baseFileName:  config.FilePath,
		stdout:        config.Stdout,
		jsonFormat:    config.JSONFormat,
		file:          file,
		maxFileSize:   maxFileSize,
		flushInterval: flushInterval,
		enableCaller:  config.EnableCaller,
		syncMode:      config.SyncMode,
	}

	if !config.SyncMode {

		ringSize := uint64(queueSize)
		logger.ring = internalqueue.NewRingBuffer[*entry](ringSize)
		logger.batchSize = queueSize / 10
		if logger.batchSize < 10 {
			logger.batchSize = 10
		}

		logger.queue = make(chan []byte, queueSize)

		logger.wg.Add(2)
		go logger.processQueueAsync()
		go logger.periodicFlush()
	} else {

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

// logWithFormat logs a formatted message
func (l *Logger) logWithFormat(level LogLevel, fields []Field, format string, args ...interface{}) {

	if !l.enabled(level) {
		return
	}

	e := entryPool.Get().(*entry)
	if e == nil {
		return
	}
	e.Reset()

	if e.buf == nil {
		e.buf = make([]byte, 0, 256)
	}
	if e.timestamp == nil {
		e.timestamp = make([]byte, 0, 64)
	}
	if e.fields == nil {
		e.fields = make([]Field, 0, 8)
	}

	e.level = level
	e.levelStr = level.String()
	e.format = format
	e.args = args

	now := time.Now()
	e.timestamp = internalformat.AppendRFC3339Timestamp(e.timestamp[:0], now)

	if len(fields) > 0 {
		e.fields = append(e.fields[:0], fields...)
	}

	if l.enableCaller {
		_, file, line, ok := runtime.Caller(3)
		if ok {
			baseFile := filepath.Base(file)
			e.caller = baseFile + ":" + strconv.Itoa(line)
		}
	}

	useRing := l.ring != nil
	internalcore.DispatchEntry(internalcore.DispatchOptions{
		SyncMode:   l.syncMode,
		JSONFormat: l.jsonFormat,
		UseRing:    useRing,
		TryPushRing: func() bool {
			return l.pushEntryToRing(e)
		},
		WriteSync: func() {
			l.writeLogMessageSyncOptimized(e)
		},
		FormatJSON: func() []byte {
			return l.formatJSONMessage(e)
		},
		FormatText: func() []byte {
			return l.formatTextMessage(e)
		},
		PushBytes: func(b []byte) {
			l.pushLogBytes(b)
		},
		Release: func() {
			entryPool.Put(e)
		},
	})
}

// logWithFields writes a formatted log message with optional fields to the logger's queue.
func (l *Logger) logWithFields(level LogLevel, fields []Field, args ...interface{}) {

	if !l.enabled(level) {
		return
	}

	e := entryPool.Get().(*entry)
	if e == nil {

		return
	}
	e.Reset()

	if e.buf == nil {
		e.buf = make([]byte, 0, 256)
	}
	if e.timestamp == nil {
		e.timestamp = make([]byte, 0, 64)
	}
	if e.fields == nil {
		e.fields = make([]Field, 0, 8)
	}

	e.level = level
	e.levelStr = level.String()
	e.args = args

	now := time.Now()
	e.timestamp = internalformat.AppendRFC3339Timestamp(e.timestamp[:0], now)

	if len(fields) > 0 {
		e.fields = append(e.fields[:0], fields...)
	}

	if l.enableCaller {
		_, file, line, ok := runtime.Caller(3)
		if ok {
			baseFile := filepath.Base(file)

			e.caller = baseFile + ":" + strconv.Itoa(line)
		}
	}

	useRing := l.ring != nil
	internalcore.DispatchEntry(internalcore.DispatchOptions{
		SyncMode:   l.syncMode,
		JSONFormat: l.jsonFormat,
		UseRing:    useRing,
		TryPushRing: func() bool {
			return l.pushEntryToRing(e)
		},
		WriteSync: func() {
			l.writeLogMessageSyncOptimized(e)
		},
		FormatJSON: func() []byte {
			return l.formatJSONMessage(e)
		},
		FormatText: func() []byte {
			return l.formatTextMessage(e)
		},
		PushBytes: func(b []byte) {
			l.pushLogBytes(b)
		},
		Release: func() {
			entryPool.Put(e)
		},
	})
}

// fieldLogger wraps a Logger to include fields in log entries.
type fieldLogger struct {
	logger *Logger
	fields []Field
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
	l.Close()
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

	if !l.enabled(DEBUG) {
		return
	}

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

	if !l.enabled(INFO) {
		return
	}

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

	if !l.enabled(WARN) {
		return
	}

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

	if !l.enabled(ERROR) {
		return
	}

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

	l.logWithFormat(FATAL, nil, format, args...)
	l.Close()
	os.Exit(1)
}

// WithFields creates a new logger instance with the specified fields that will be included
// in all subsequent log messages created by this logger.
//
// Parameters:
//
//	fields: Variadic Field arguments.
//
// Returns:
//
//	LoggerInterface: A new logger instance with the added fields.
//
// Example usage:
//
//	logger.WithFields(
//	  fastlog.Int("user_id", 123),
//	  fastlog.String("env", "production"),
//	).Info("User logged in")
func (l *Logger) WithFields(fields ...Field) LoggerInterface {
	return &fieldLogger{
		logger: l,
		fields: fields,
	}
}

// Debug logs a message at the DEBUG level with the attached fields.
func (fl *fieldLogger) Debug(args ...interface{}) {

	if !fl.logger.enabled(DEBUG) {
		return
	}
	fl.logger.logWithFields(DEBUG, fl.fields, args...)
}

// Info logs a message at the INFO level with the attached fields.
func (fl *fieldLogger) Info(args ...interface{}) {

	if !fl.logger.enabled(INFO) {
		return
	}
	fl.logger.logWithFields(INFO, fl.fields, args...)
}

// Warn logs a message at the WARN level with the attached fields.
func (fl *fieldLogger) Warn(args ...interface{}) {

	if !fl.logger.enabled(WARN) {
		return
	}
	fl.logger.logWithFields(WARN, fl.fields, args...)
}

// Error logs a message at the ERROR level with the attached fields.
func (fl *fieldLogger) Error(args ...interface{}) {

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

	if !fl.logger.enabled(DEBUG) {
		return
	}

	fl.logger.logWithFormat(DEBUG, fl.fields, format, args...)
}

// Infof logs a formatted message at the INFO level with the attached fields.
func (fl *fieldLogger) Infof(format string, args ...interface{}) {

	if !fl.logger.enabled(INFO) {
		return
	}

	fl.logger.logWithFormat(INFO, fl.fields, format, args...)
}

// Warnf logs a formatted message at the WARN level with the attached fields.
func (fl *fieldLogger) Warnf(format string, args ...interface{}) {

	if !fl.logger.enabled(WARN) {
		return
	}

	fl.logger.logWithFormat(WARN, fl.fields, format, args...)
}

// Errorf logs a formatted message at the ERROR level with the attached fields.
func (fl *fieldLogger) Errorf(format string, args ...interface{}) {

	if !fl.logger.enabled(ERROR) {
		return
	}

	fl.logger.logWithFormat(ERROR, fl.fields, format, args...)
}

// Fatalf logs a formatted message at the FATAL level with the attached fields and exits.
func (fl *fieldLogger) Fatalf(format string, args ...interface{}) {

	fl.logger.logWithFormat(FATAL, fl.fields, format, args...)
	fl.logger.Close()
	os.Exit(1)
}

// WithFields creates a new fieldLogger with additional fields merged with existing fields.
func (fl *fieldLogger) WithFields(fields ...Field) LoggerInterface {
	newFields := make([]Field, 0, len(fl.fields)+len(fields))
	newFields = append(newFields, fl.fields...)
	newFields = append(newFields, fields...)
	return &fieldLogger{
		logger: fl.logger,
		fields: newFields,
	}
}
