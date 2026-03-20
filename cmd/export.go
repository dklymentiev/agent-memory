package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steamfoundry/agent-memory/internal/store"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export documents as JSON or markdown",
	RunE:  runExport,
}

var (
	exportFormat string
	exportTag    string
	exportAll    bool
)

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "json", "output format (json, markdown)")
	exportCmd.Flags().StringVarP(&exportTag, "tag", "t", "", "filter by tag")
	exportCmd.Flags().BoolVar(&exportAll, "all", false, "export all workspaces")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	opts := store.ListOptions{
		Tag:   exportTag,
		Limit: 0, // all
	}
	if !exportAll {
		opts.Workspace = workspace
	}

	docs, err := s.List(opts)
	if err != nil {
		return err
	}

	switch exportFormat {
	case "json":
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(docs)
	case "markdown", "md":
		for i, d := range docs {
			if i > 0 {
				fmt.Print("\n---\n\n")
			}
			fmt.Printf("## %s\n\n", d.ID)
			if len(d.Tags) > 0 {
				fmt.Printf("**Tags:** %s\n\n", strings.Join(d.Tags, ", "))
			}
			fmt.Printf("**Workspace:** %s | **Created:** %s\n\n", d.Workspace, d.CreatedAt.Format("2006-01-02 15:04"))
			fmt.Println(d.Content)
		}
		return nil
	default:
		return fmt.Errorf("unknown format: %s (use json or markdown)", exportFormat)
	}
}
