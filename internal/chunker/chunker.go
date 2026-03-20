// Package chunker implements markdown-aware document chunking for embedding.
// Ported from Knowster's Python chunker.
package chunker

import (
	"regexp"
	"strings"
)

const (
	DefaultTargetSize = 800
	DefaultOverlap    = 150
	DefaultMinSize    = 80
)

// Chunk splits a document into overlapping chunks preserving semantic boundaries.
func Chunk(text string, targetSize, overlap, minSize int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if targetSize <= 0 {
		targetSize = DefaultTargetSize
	}
	if overlap <= 0 {
		overlap = DefaultOverlap
	}
	if minSize <= 0 {
		minSize = DefaultMinSize
	}

	if len(text) <= targetSize {
		return []string{text}
	}

	sections := splitByHeaders(text)

	var chunks []string
	for _, section := range sections {
		if len(section) <= targetSize {
			chunks = append(chunks, section)
		} else if listChunks := splitByListItems(section, targetSize); len(listChunks) > 0 {
			chunks = append(chunks, listChunks...)
		} else {
			chunks = append(chunks, splitByParagraphs(section, targetSize)...)
		}
	}

	chunks = mergeSmallChunks(chunks, minSize)
	chunks = applyOverlap(chunks, overlap, targetSize)
	chunks = mergeSmallChunks(chunks, minSize)

	var result []string
	for _, c := range chunks {
		if strings.TrimSpace(c) != "" {
			result = append(result, c)
		}
	}
	return result
}

var headerRe = regexp.MustCompile(`(?m)^#{1,3}\s`)

func splitByHeaders(text string) []string {
	indices := headerRe.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		return []string{text}
	}

	var parts []string
	prev := 0
	for _, idx := range indices {
		if idx[0] > prev {
			part := strings.TrimSpace(text[prev:idx[0]])
			if part != "" {
				parts = append(parts, part)
			}
		}
		prev = idx[0]
	}
	if prev < len(text) {
		part := strings.TrimSpace(text[prev:])
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

var listItemRe = regexp.MustCompile(`^[-*]\s`)

func splitByListItems(text string, targetSize int) []string {
	lines := strings.Split(text, "\n")

	var headerLines []string
	listStart := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if listItemRe.MatchString(trimmed) {
			listStart = i
			break
		}
		headerLines = append(headerLines, line)
	}
	if listStart < 0 {
		return nil
	}

	// Count list items
	count := 0
	for _, line := range lines[listStart:] {
		if listItemRe.MatchString(strings.TrimSpace(line)) {
			count++
		}
	}
	if count < 4 {
		return nil
	}

	header := strings.TrimSpace(strings.Join(headerLines, "\n"))

	// Collect items
	var items []string
	var current string
	for _, line := range lines[listStart:] {
		trimmed := strings.TrimSpace(line)
		if listItemRe.MatchString(trimmed) {
			if current != "" {
				items = append(items, current)
			}
			current = line
		} else if trimmed == "" {
			if current != "" {
				current += "\n"
			}
		} else {
			current += "\n" + line
		}
	}
	if current != "" {
		items = append(items, current)
	}

	var chunks []string
	currentChunk := header
	for _, item := range items {
		var candidate string
		if currentChunk == header {
			candidate = header + "\n\n" + item
		} else {
			candidate = currentChunk + "\n" + item
		}
		if len(candidate) <= targetSize {
			currentChunk = candidate
		} else {
			if currentChunk != header {
				chunks = append(chunks, currentChunk)
			}
			currentChunk = header + "\n\n" + item
		}
	}
	if currentChunk != header {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

func splitByParagraphs(text string, targetSize int) []string {
	paragraphs := regexp.MustCompile(`\n\s*\n`).Split(text, -1)

	var chunks []string
	var current string

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if current == "" {
			current = para
		} else if len(current)+len(para)+2 <= targetSize {
			current = current + "\n\n" + para
		} else {
			chunks = append(chunks, current)
			current = para
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}

	var result []string
	for _, chunk := range chunks {
		if len(chunk) <= targetSize {
			result = append(result, chunk)
		} else {
			result = append(result, splitBySentences(chunk, targetSize)...)
		}
	}
	return result
}

var sentenceEndRe = regexp.MustCompile(`[.!?]\s+`)

func splitBySentences(text string, targetSize int) []string {
	// Split at sentence boundaries (after .!? followed by space)
	indices := sentenceEndRe.FindAllStringIndex(text, -1)
	var sentences []string
	prev := 0
	for _, idx := range indices {
		// Split after the punctuation, before the whitespace
		splitAt := idx[0] + 1
		sentences = append(sentences, text[prev:splitAt])
		prev = idx[1]
	}
	if prev < len(text) {
		sentences = append(sentences, text[prev:])
	}
	if len(sentences) == 0 {
		sentences = []string{text}
	}

	var chunks []string
	var current string

	for _, sent := range sentences {
		if current == "" {
			current = sent
		} else if len(current)+len(sent)+1 <= targetSize {
			current = current + " " + sent
		} else {
			chunks = append(chunks, current)
			current = sent
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}

	var result []string
	for _, chunk := range chunks {
		if len(chunk) <= targetSize {
			result = append(result, chunk)
		} else {
			result = append(result, hardSplit(chunk, targetSize)...)
		}
	}
	return result
}

func hardSplit(text string, targetSize int) []string {
	var chunks []string
	for len(text) > targetSize {
		splitAt := strings.LastIndex(text[:targetSize], " ")
		if splitAt <= 0 {
			splitAt = targetSize
		}
		chunks = append(chunks, strings.TrimSpace(text[:splitAt]))
		text = strings.TrimSpace(text[splitAt:])
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

func mergeSmallChunks(chunks []string, minSize int) []string {
	if len(chunks) == 0 {
		return chunks
	}
	merged := []string{chunks[0]}
	for _, chunk := range chunks[1:] {
		if len(chunk) < minSize && len(merged) > 0 {
			merged[len(merged)-1] = merged[len(merged)-1] + "\n\n" + chunk
		} else {
			merged = append(merged, chunk)
		}
	}
	return merged
}

func applyOverlap(chunks []string, overlap, targetSize int) []string {
	if len(chunks) <= 1 || overlap <= 0 {
		return chunks
	}

	result := []string{chunks[0]}
	for i := 1; i < len(chunks); i++ {
		prev := chunks[i-1]
		if len(prev) > overlap {
			prefixStart := prev[len(prev)-overlap:]
			spaceIdx := strings.Index(prefixStart, " ")
			if spaceIdx > 0 {
				prefixStart = prefixStart[spaceIdx+1:]
			}
			overlapText := prefixStart + "\n\n" + chunks[i]
			if len(overlapText) <= targetSize*2 {
				result = append(result, overlapText)
			} else {
				result = append(result, chunks[i])
			}
		} else {
			result = append(result, chunks[i])
		}
	}
	return result
}
