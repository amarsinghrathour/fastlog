package fastlog

import (
	"fmt"
	"os"
	"time"

	internalcore "github.com/amarsinghrathour/fastlog/internal/core"
	internalqueue "github.com/amarsinghrathour/fastlog/internal/queue"
	internalrotate "github.com/amarsinghrathour/fastlog/internal/rotate"
)

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

		return
	}
	if internalqueue.RetrySend(l.queue, msgBytes, maxRetries, retryInterval) {
		return
	}

	msgLen := len(msgBytes)
	truncated := msgBytes
	if msgLen > 100 {
		truncated = msgBytes[:100]
	}
	fmt.Fprintf(os.Stderr, "logger queue full, dropping log message: %s\n", string(truncated))
}

// Uses CAS to ensure thread-safe enqueueing
func (l *Logger) pushEntryToRing(e *entry) bool {
	return l.ring != nil && l.ring.Push(e)
}

func (l *Logger) processQueueAsync() {
	defer l.wg.Done()
	internalcore.RunAsyncLoop(internalcore.AsyncLoopConfig[*entry]{
		Done:           l.done,
		Queue:          l.queue,
		Ring:           l.ring,
		FlushInterval:  l.flushInterval,
		BatchSize:      l.batchSize,
		FlushBatch:     l.flushBatch,
		ProcessMessage: l.processMessage,
	})
}

func (l *Logger) flushBatch(batch []*entry) {
	if len(batch) == 0 {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	fileLimit := l.logFileSizeExceeded()
	if !l.stdout && l.file != nil && fileLimit {
		if err := l.rotateLogFile(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
		}
	}

	for _, e := range batch {
		if e == nil {
			continue
		}

		if l.jsonFormat {

			jsonBytes := l.formatJSONMessage(e)
			if jsonBytes != nil {
				_, _ = l.buffer.Write(jsonBytes)
				_ = l.buffer.WriteByte('\n')
			}
		} else {
			msgBytes := l.formatTextMessage(e)
			if msgBytes != nil {
				_, _ = l.buffer.Write(msgBytes)
			}
		}

		entryPool.Put(e)
	}

	if err := l.buffer.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to flush buffer: %v\n", err)
	}
}

func (l *Logger) writeLogMessageSyncOptimized(e *entry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	fileLimit := l.logFileSizeExceeded()
	if !l.stdout && l.file != nil && fileLimit {
		if err := l.rotateLogFile(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
		}
	}

	if l.stdout || l.file != nil {
		if l.jsonFormat {

			jsonBytes := l.formatJSONMessage(e)
			if jsonBytes != nil {
				_, _ = l.buffer.Write(jsonBytes)
				_ = l.buffer.WriteByte('\n')
			}
		} else {

			msgBytes := l.formatTextMessage(e)
			if msgBytes != nil {
				_, _ = l.buffer.Write(msgBytes)
			}
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
//	logger.processMessage([]byte("This is a log message\n"))
func (l *Logger) processMessage(msgBytes []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()

	fileLimit := l.logFileSizeExceeded()
	if !l.stdout && l.file != nil && fileLimit {

		if err := l.rotateLogFile(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
		}
	}

	if l.stdout || l.file != nil {

		if _, err := l.buffer.Write(msgBytes); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write log message: %v\n", err)
		}

		if err := l.buffer.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to flush buffer: %v\n", err)
		}
	}

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
		return false
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

	if err := l.buffer.Flush(); err != nil {
		return fmt.Errorf("failed to flush buffer during rotation: %w", err)
	}

	if l.file != nil {
		oldPath := l.baseFileName

		if err := internalrotate.EnsureDir(l.rotationDir); err != nil {
			return fmt.Errorf("failed to create rotation directory: %w", err)
		}
		rotatedPath := internalrotate.RotatedPath(l.rotationDir, l.baseFileName, time.Now())

		if err := l.file.Close(); err != nil {
			return fmt.Errorf("failed to close log file during rotation: %w", err)
		}

		if err := os.Rename(oldPath, rotatedPath); err != nil {

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

	newFile, err := os.OpenFile(l.baseFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open new log file during rotation: %w", err)
	}

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

	select {
	case <-l.done:

	default:
		close(l.done)
	}

	l.wg.Wait()

	if !l.syncMode {

		if l.ring != nil {
			batch := make([]*entry, 0, l.batchSize)
			for {
				updated, readAny := l.ring.DrainBatch(batch[:0], l.batchSize)
				batch = updated
				if !readAny {
					break
				}
				if len(batch) > 0 {
					l.flushBatch(batch)
				}
			}
		}

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

	if err := l.buffer.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to flush buffer during close: %v\n", err)
	}

	if !l.stdout && l.file != nil {
		if err := l.file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close log file: %v\n", err)
		}
		l.file = nil
	}
}

func (l *Logger) enabled(level LogLevel) bool {
	return level >= l.level
}
