package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steamfoundry/agent-memory/internal/store"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List memory documents",
	RunE:  runList,
}

var (
	listLimit  int
	listTag    string
	listSource string
	listPinned bool
	listJSON   bool
)

func init() {
	listCmd.Flags().IntVarP(&listLimit, "limit", "n", 20, "max results")
	listCmd.Flags().StringVarP(&listTag, "tag", "t", "", "filter by tag")
	listCmd.Flags().StringVarP(&listSource, "source", "s", "", "filter by source")
	listCmd.Flags().BoolVar(&listPinned, "pinned", false, "show only pinned")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	opts := store.ListOptions{
		Workspace: workspace,
		Tag:       listTag,
		Source:    listSource,
		Limit:     listLimit,
	}
	if listPinned {
		pinned := true
		opts.Pinned = &pinned
	}

	docs, err := s.List(opts)
	if err != nil {
		return err
	}

	if len(docs) == 0 {
		fmt.Println("No documents found.")
		return nil
	}

	if listJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(docs)
	}

	for _, d := range docs {
		content := d.Content
		if len(content) > 80 {
			content = content[:80] + "..."
		}
		content = strings.ReplaceAll(content, "\n", " ")
		tags := ""
		if len(d.Tags) > 0 {
			tags = " [" + strings.Join(d.Tags, ", ") + "]"
		}
		fmt.Printf("%s  %s  %s%s\n", d.ID, d.CreatedAt.Format("2006-01-02"), content, tags)
	}

	return nil
}
