package chunker

import (
	"strings"
	"testing"
)

func TestChunkShortText(t *testing.T) {
	chunks := Chunk("Short text", 800, 150, 80)
	if len(chunks) != 1 {
		t.Errorf("got %d chunks, want 1", len(chunks))
	}
}

func TestChunkEmpty(t *testing.T) {
	chunks := Chunk("", 800, 150, 80)
	if len(chunks) != 0 {
		t.Errorf("got %d chunks, want 0", len(chunks))
	}
}

func TestChunkByHeaders(t *testing.T) {
	text := "# Section 1\n\nContent of section 1 with enough text to be meaningful.\n\n" +
		"## Section 2\n\nContent of section 2 with enough text to be meaningful.\n\n" +
		"## Section 3\n\nContent of section 3 with enough text to be meaningful."

	chunks := Chunk(text, 100, 20, 20)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks from 3 sections, got %d", len(chunks))
	}
}

func TestChunkByParagraphs(t *testing.T) {
	var paragraphs []string
	for i := 0; i < 10; i++ {
		paragraphs = append(paragraphs, strings.Repeat("Word ", 50))
	}
	text := strings.Join(paragraphs, "\n\n")

	chunks := Chunk(text, 300, 50, 40)
	if len(chunks) < 3 {
		t.Errorf("expected at least 3 chunks, got %d", len(chunks))
	}
}

func TestChunkListItems(t *testing.T) {
	text := "# Tools\n\n" +
		"- Tool 1: Description of tool one that is fairly long\n" +
		"- Tool 2: Description of tool two that is fairly long\n" +
		"- Tool 3: Description of tool three that is fairly long\n" +
		"- Tool 4: Description of tool four that is fairly long\n" +
		"- Tool 5: Description of tool five that is fairly long\n"

	chunks := Chunk(text, 150, 20, 20)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks from 5 list items, got %d", len(chunks))
	}
	// Each chunk should contain the header
	for i, c := range chunks {
		if !strings.Contains(c, "# Tools") {
			t.Errorf("chunk %d missing header", i)
		}
	}
}

func TestChunkMaxSize(t *testing.T) {
	text := strings.Repeat("A sentence with several words. ", 100)
	target := 400
	chunks := Chunk(text, target, 50, 40)

	for i, c := range chunks {
		if len(c) > target*2+200 { // allow overlap margin
			t.Errorf("chunk %d too large: %d chars (target %d)", i, len(c), target)
		}
	}
}

func TestChunkDefaults(t *testing.T) {
	text := strings.Repeat("Test content. ", 200)
	chunks := Chunk(text, 0, 0, 0)
	if len(chunks) == 0 {
		t.Error("expected chunks with default params")
	}
}
