package core

import "path/filepath"

// ResolveRotationDir returns the effective rotation directory.
func ResolveRotationDir(stdout bool, filePath, configured string) string {
	if stdout {
		return configured
	}
	if configured != "" {
		return configured
	}
	return filepath.Dir(filePath)
}
