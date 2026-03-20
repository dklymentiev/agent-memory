package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/dklymentiev/agent-memory/internal/config"
)

var focusCmd = &cobra.Command{
	Use:   "focus [workspace]",
	Short: "Switch active workspace",
	Long:  "Set the active workspace. All subsequent operations will use this workspace by default.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runFocus,
}

func init() {
	rootCmd.AddCommand(focusCmd)
}

func runFocus(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		fmt.Printf("Active workspace: %s\n", cfg.ActiveWorkspace)
		return nil
	}

	if err := config.ValidateWorkspace(args[0]); err != nil {
		return err
	}
	cfg.ActiveWorkspace = args[0]
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Printf("Switched to workspace: %s\n", args[0])
	return nil
}
