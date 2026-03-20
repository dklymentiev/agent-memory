package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steamfoundry/agent-memory/internal/config"
	"github.com/steamfoundry/agent-memory/internal/store"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize agent-memory in current project",
	Long:  "Creates .agent-memory/ directory with a local database in the current project.",
	RunE:  runInit,
}

var setupHooks bool

func init() {
	initCmd.Flags().BoolVar(&setupHooks, "setup-hooks", false, "install Claude Code hooks")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	dir := filepath.Join(cwd, "."+config.AppName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	localDB := filepath.Join(dir, config.DBFileName)
	s, err := store.Open(localDB)
	if err != nil {
		return err
	}
	s.Close()

	// Add to .gitignore if it exists
	gitignorePath := filepath.Join(cwd, ".gitignore")
	entry := ".agent-memory/"
	if data, err := os.ReadFile(gitignorePath); err == nil {
		if !strings.Contains(string(data), entry) {
			f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0644)
			if err == nil {
				fmt.Fprintf(f, "\n%s\n", entry)
				f.Close()
			}
		}
	}

	fmt.Printf("Initialized agent-memory in %s\n", dir)

	if setupHooks {
		return installHooks(cwd)
	}

	return nil
}

func installHooks(projectDir string) error {
	fmt.Println("Hook installation will be available in a future version.")
	return nil
}
