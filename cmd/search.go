package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search memory documents",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runSearch,
}

var (
	searchLimit  int
	searchJSON   bool
)

func init() {
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 10, "max results")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	results, err := s.Search(query, workspace, searchLimit)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	if searchJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	for i, r := range results {
		if i > 0 {
			fmt.Println("---")
		}
		fmt.Printf("id: %s\n", r.ID)
		if len(r.Tags) > 0 {
			fmt.Printf("tags: %s\n", strings.Join(r.Tags, ", "))
		}
		fmt.Printf("workspace: %s\n", r.Workspace)
		fmt.Printf("created: %s\n", r.CreatedAt.Format("2006-01-02 15:04"))
		content := r.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Printf("\n%s\n", content)
	}

	return nil
}
