package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show memory statistics",
	RunE:  runStats,
}

var statsJSON bool

func init() {
	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(statsCmd)
}

func runStats(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	st, err := s.Stats()
	if err != nil {
		return err
	}

	if statsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(st)
	}

	fmt.Printf("Documents: %d\n", st.DocCount)
	fmt.Printf("DB size:   %s\n", formatBytes(st.DBSize))
	if len(st.WorkspaceCounts) > 0 {
		fmt.Println("\nWorkspaces:")
		for ws, count := range st.WorkspaceCounts {
			fmt.Printf("  %-20s %d\n", ws, count)
		}
	}

	return nil
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
