package fastlog

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
