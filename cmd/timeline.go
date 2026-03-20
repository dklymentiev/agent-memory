package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var timelineCmd = &cobra.Command{
	Use:   "timeline",
	Short: "Show chronological timeline of documents",
	Long:  "Returns documents in chronological order for a date range.",
	RunE:  runTimeline,
}

var (
	timelineFrom  string
	timelineTo    string
	timelineLimit int
	timelineJSON  bool
)

func init() {
	timelineCmd.Flags().StringVar(&timelineFrom, "from", "", "start date (YYYY-MM-DD, default: 7 days ago)")
	timelineCmd.Flags().StringVar(&timelineTo, "to", "", "end date (YYYY-MM-DD, default: today)")
	timelineCmd.Flags().IntVarP(&timelineLimit, "limit", "n", 20, "max results")
	timelineCmd.Flags().BoolVar(&timelineJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(timelineCmd)
}

func runTimeline(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	from := timelineFrom
	if from == "" {
		from = time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	}
	to := timelineTo
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}

	docs, err := s.Timeline(from, to, workspace, timelineLimit)
	if err != nil {
		return err
	}

	if len(docs) == 0 {
		fmt.Println("No documents found in range.")
		return nil
	}

	if timelineJSON {
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
		fmt.Printf("%s  %s  %s%s\n", d.ID, d.CreatedAt.Format("2006-01-02 15:04"), content, tags)
	}

	return nil
}
