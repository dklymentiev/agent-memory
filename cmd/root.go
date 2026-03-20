package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steamfoundry/agent-memory/internal/config"
	"github.com/steamfoundry/agent-memory/internal/store"
)

var (
	dbPath    string
	workspace string
	cfg       *config.Config

	Version = "dev"
)

var rootCmd = &cobra.Command{
	Use:   "agent-memory",
	Short: "Persistent memory for AI agents",
	Long:  "Lightweight, single-binary persistent memory with FTS5 search, workspaces, auto-capture hooks, and MCP server.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "database path (default: ~/.agent-memory/memory.db)")
	rootCmd.PersistentFlags().StringVarP(&workspace, "workspace", "w", "", "workspace (default: from config)")
}

func initConfig() {
	cfg = config.Load()
	if workspace == "" {
		workspace = cfg.ActiveWorkspace
	}
	if workspace == "" {
		workspace = "default"
	}
}

func openStore() (*store.SQLiteStore, error) {
	path := dbPath
	if path == "" {
		path = cfg.DBPath
	}
	if path == "" {
		path = config.DefaultDBPath()
	}
	if err := config.EnsureDir(path); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return store.Open(path)
}
