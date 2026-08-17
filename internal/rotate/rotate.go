package rotate

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// EnsureDir creates the directory tree if missing.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// RotatedPath builds the rotated log path.
func RotatedPath(rotationDir, baseFileName string, ts time.Time) string {
	ext := path.Ext(baseFileName)
	fileNameWithoutExt := baseFileName
	if ext != "" {
		fileNameWithoutExt = baseFileName[:len(baseFileName)-len(ext)]
	}

	base := filepath.Base(fileNameWithoutExt)

	var buf strings.Builder
	// Pre-allocate buffer length: dir + '/' + base + '-' + len("20060102-150405") + ext
	buf.Grow(len(rotationDir) + 1 + len(base) + 1 + 15 + len(ext))

	buf.WriteString(rotationDir)
	buf.WriteByte('/')
	buf.WriteString(base)
	buf.WriteByte('-')

	var tsBuf [16]byte
	buf.Write(ts.AppendFormat(tsBuf[:0], "20060102-150405"))

	buf.WriteString(ext)

	return buf.String()
}
