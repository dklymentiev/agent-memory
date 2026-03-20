package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steamfoundry/agent-memory/internal/store"
)

var addCmd = &cobra.Command{
	Use:   "add [content]",
	Short: "Add a memory document",
	Long:  "Add a new document to memory. Content can be passed as argument, via --file, or piped via stdin.",
	RunE:  runAdd,
}

var (
	addTags   []string
	addSource string
	addPinned bool
	addFile   string
)

func init() {
	addCmd.Flags().StringSliceVarP(&addTags, "tag", "t", nil, "tags (repeatable, e.g. -t type:note -t topic:dns)")
	addCmd.Flags().StringVarP(&addSource, "source", "s", "cli", "source (cli, hook, mcp)")
	addCmd.Flags().BoolVar(&addPinned, "pin", false, "pin this document")
	addCmd.Flags().StringVarP(&addFile, "file", "f", "", "read content from file (use - for stdin)")
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	content, err := resolveContent(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("content cannot be empty")
	}

	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	doc := &store.Document{
		Content:   content,
		Tags:      addTags,
		Workspace: workspace,
		Source:    addSource,
		Pinned:    addPinned,
	}

	if err := s.Add(doc); err != nil {
		return err
	}

	fmt.Println(doc.ID)
	return nil
}

const maxContentSize = 1 << 20 // 1MB

func resolveContent(args []string) (string, error) {
	var content string

	if addFile == "-" {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, maxContentSize+1))
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		content = string(data)
	} else if addFile != "" {
		data, err := os.ReadFile(addFile)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		content = string(data)
	} else if len(args) > 0 {
		content = strings.Join(args, " ")
	} else {
		// Try stdin if not a terminal
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(io.LimitReader(os.Stdin, maxContentSize+1))
			if err != nil {
				return "", fmt.Errorf("read stdin: %w", err)
			}
			content = string(data)
		} else {
			return "", fmt.Errorf("provide content as argument, --file, or pipe via stdin")
		}
	}

	if len(content) > maxContentSize {
		return "", fmt.Errorf("content too large (%d bytes, max %d)", len(content), maxContentSize)
	}
	return content, nil
}
