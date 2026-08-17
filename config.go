package fastlog

import "time"

// Constants for retry mechanism.
const (
	retryInterval = 100 * time.Millisecond // Interval between retries.
	maxRetries    = 5                      // Maximum number of retries.
)

// Default constants for buffer size, flush interval, and max log file size.
const (
	DefaultBufferSize     = 4096             // Default buffer size in bytes.
	DefaultFlushInterval  = 5 * time.Second  // Default interval to flush buffer.
	DefaultMaxLogFileSize = 10 * 1024 * 1024 // Default maximum log file size before rotation (10 MB).
	DefaultQueueSize      = 1000             // Default queue size for log messages.
)



// LoggerConfig defines the configuration for the Logger.
type LoggerConfig struct {
	Level         LogLevel      // Log level threshold.
	FilePath      string        // Path to the log file.
	RotationDir   string        // Directory for rotated log files.
	Stdout        bool          // Whether to log to stdout instead of a file.
	JSONFormat    bool          // Whether to log in JSON format.
	BufferSize    int           // Buffer size in bytes. If 0, uses DefaultBufferSize.
	FlushInterval time.Duration // Interval to flush buffer. If 0, uses DefaultFlushInterval.
	MaxFileSize   int64         // Maximum log file size before rotation. If 0, uses DefaultMaxLogFileSize.
	QueueSize     int           // Queue size for log messages. If 0, uses DefaultQueueSize.
	EnableCaller  bool          // Whether to include caller information (file and line number).
	SyncMode      bool          // If true, logs synchronously without queue (faster but may block). Default: false.
}
