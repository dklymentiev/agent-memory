// Package config handles configuration and XDG-compliant paths.
package config

import (
	"os"
	"path/filepath"
)

const (
	AppName    = "agent-memory"
	DBFileName = "memory.db"
)

// DataDir returns the data directory (~/.agent-memory or XDG_DATA_HOME/agent-memory).
func DataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "."+AppName)
}

// ConfigDir returns the config directory.
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "."+AppName)
}

// DefaultDBPath returns the default database file path.
func DefaultDBPath() string {
	return filepath.Join(DataDir(), DBFileName)
}

// ProjectDBPath returns the .agent-memory/memory.db path relative to a project root.
func ProjectDBPath(projectRoot string) string {
	return filepath.Join(projectRoot, "."+AppName, DBFileName)
}

// EnsureDir creates a directory if it doesn't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0700)
}
