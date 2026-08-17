package fastlog_test

import (
	"os"
	"path/filepath"

	"github.com/amarsinghrathour/fastlog"
)

func ExampleNewLogger() {
	tmpDir, err := os.MkdirTemp("", "fastlog-example-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := fastlog.NewLogger(fastlog.LoggerConfig{
		Level:       fastlog.INFO,
		FilePath:    filepath.Join(tmpDir, "app.log"),
		RotationDir: tmpDir,
		JSONFormat:  false,
	})
	if err != nil {
		panic(err)
	}
	defer logger.Close()

	logger.Info("service started", "port", 8080)
}
