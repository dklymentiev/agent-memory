package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steamfoundry/agent-memory/internal/store"
)

var promptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Manage reusable prompt templates",
	Long:  "Save, retrieve, and list reusable prompt templates.",
}

var promptSaveCmd = &cobra.Command{
	Use:   "save <name> [content]",
	Short: "Save a prompt template",
	Long:  "Save a prompt template by name. Content can be passed as argument or piped via stdin.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runPromptSave,
}

var promptGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Retrieve a prompt template by name",
	Args:  cobra.ExactArgs(1),
	RunE:  runPromptGet,
}

var promptListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all saved prompt templates",
	RunE:  runPromptList,
}

var promptSaveTags []string

func init() {
	promptSaveCmd.Flags().StringSliceVarP(&promptSaveTags, "tag", "t", nil, "additional tags")
	promptCmd.AddCommand(promptSaveCmd)
	promptCmd.AddCommand(promptGetCmd)
	promptCmd.AddCommand(promptListCmd)
	rootCmd.AddCommand(promptCmd)
}

func runPromptSave(cmd *cobra.Command, args []string) error {
	name := args[0]

	var content string
	if len(args) > 1 {
		content = strings.Join(args[1:], " ")
	} else {
		// Try stdin
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(io.LimitReader(os.Stdin, maxContentSize+1))
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			content = string(data)
		} else {
			return fmt.Errorf("provide content as argument or pipe via stdin")
		}
	}

	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("content cannot be empty")
	}

	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	tags := append([]string{"type:prompt", "prompt:" + name}, promptSaveTags...)
	doc := &store.Document{
		Content:   content,
		Tags:      tags,
		Workspace: workspace,
		Source:    "cli",
	}

	if err := s.Add(doc); err != nil {
		return err
	}

	fmt.Printf("%s  prompt:%s saved\n", doc.ID, name)
	return nil
}

func runPromptGet(cmd *cobra.Command, args []string) error {
	name := args[0]

	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	docs, err := s.List(store.ListOptions{
		Tag:   "prompt:" + name,
		Limit: 1,
	})
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return fmt.Errorf("prompt not found: %s", name)
	}

	fmt.Print(docs[0].Content)
	return nil
}

func runPromptList(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	docs, err := s.List(store.ListOptions{
		Tag:   "type:prompt",
		Limit: 100,
	})
	if err != nil {
		return err
	}

	if len(docs) == 0 {
		fmt.Println("No prompt templates found.")
		return nil
	}

	for _, d := range docs {
		// Extract prompt name from tags
		name := ""
		for _, t := range d.Tags {
			if strings.HasPrefix(t, "prompt:") {
				name = strings.TrimPrefix(t, "prompt:")
				break
			}
		}
		content := d.Content
		if len(content) > 60 {
			content = content[:60] + "..."
		}
		content = strings.ReplaceAll(content, "\n", " ")
		fmt.Printf("%s  %-20s  %s\n", d.ID, name, content)
	}

	return nil
}
