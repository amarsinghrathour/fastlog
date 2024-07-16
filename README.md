# fastlog

`fastlog` is a high-performance logging package for Go, designed to be extremely fast, efficient, and suitable for high-performance applications. It supports logging to both files and standard output, with features such as buffering, non-blocking I/O, periodic flushing, log rotation, and configurable log levels.

## Features

- **Log Levels**: Supports DEBUG, INFO, WARN, ERROR, and FATAL levels.
- **Buffering**: Uses a buffer to minimize I/O operations.
- **Non-blocking I/O**: Log messages are sent to a channel and processed by a separate goroutine.
- **Periodic Flushing**: Flushes logs every 5 seconds.
- **Log Rotation**: Automatically rotates log files when they exceed a predefined size.
- **Configurable Output**: Logs can be written to a file or standard output based on configuration.
- **Structured Logging**: Supports structured logging and JSON formatted logs.

## Installation

To install `fastlog`, run:

```sh
go get github.com/amarsinghrathour/fastlog
```
## Usage
Here's an example of how to use fastlog:

```
    package main

    import (
        "log"
        "os"
        "path/filepath"
        "time"
    
        "github.com/yourusername/fastlog"
    )

    func main() {
        logDir := "logs"
        baseLogFileName := filepath.Join(logDir, "app.log")
    
        err := os.MkdirAll(logDir, 0755)
        if err != nil {
            log.Fatalf("Failed to create log directory: %v", err)
        }
    
        // Configure to log to a file
        loggerConfig := fastlog.LoggerConfig{
            Level:       fastlog.DEBUG,
            FilePath:    baseLogFileName,
            RotationDir: logDir,
            Stdout:      false, // Set to true to log to stdout instead of a file
            JSONFormat:  true,  // Set to true to enable JSON formatted logs
        }
    
        logger, err := fastlog.NewLogger(loggerConfig)
        if err != nil {
            log.Fatalf("Failed to create logger: %v", err)
        }
        defer logger.Close()
    
        // Log various messages
        logger.Debug("This is a debug message", "key1", "value1")
        logger.Info("This is an info message", "key2", "value2")
        logger.Warn("This is a warning message", "key3", "value3")
        logger.Error("This is an error message", "key4", "value4")
    
        // Simulate some work to generate logs over time
        for i := 0; i < 10; i++ {
            logger.Info("Working on task", "iteration", i)
            time.Sleep(1 * time.Second)
        }
    
        // Log a fatal message (this will terminate the program)
        logger.Fatal("This is a fatal message", "key5", "value5")
    }



```

[![MIT License](https://img.shields.io/badge/License-MIT-green.svg)](https://choosealicense.com/licenses/mit/)
[![Go Reference](https://pkg.go.dev/badge/github.com/amarsinghrathour/fastlog.svg)](https://pkg.go.dev/github.com/amarsinghrathour/fastlog)