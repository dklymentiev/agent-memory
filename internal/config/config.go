package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the application configuration.
type Config struct {
	ActiveWorkspace   string `json:"active_workspace"`
	EmbeddingProvider string `json:"embedding_provider,omitempty"` // "", "onnx", "openai"
	EmbeddingModel    string `json:"embedding_model,omitempty"`    // "text-embedding-3-small"
	DBPath            string `json:"db_path,omitempty"`
}

const configFileName = "config.json"

// Load reads config from disk or returns defaults.
func Load() *Config {
	cfg := &Config{
		ActiveWorkspace: "default",
	}

	data, err := os.ReadFile(configFilePath())
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "agent-memory: invalid config.json: %v\n", err)
	}
	return cfg
}

// Save writes config to disk.
func (c *Config) Save() error {
	path := configFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func configFilePath() string {
	return filepath.Join(ConfigDir(), configFileName)
}
