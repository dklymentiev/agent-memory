// Package store defines the storage interface for agent-memory.
package store

import "time"

// Document represents a memory document.
type Document struct {
	ID          string    `json:"id"`
	Content     string    `json:"content"`
	ContentHash string    `json:"content_hash,omitempty"`
	Tags        []string  `json:"tags"`
	Workspace   string    `json:"workspace"`
	Source      string    `json:"source"`
	Pinned      bool      `json:"pinned"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SearchResult wraps a document with its search rank.
type SearchResult struct {
	Document
	Rank float64 `json:"rank"`
}

// ListOptions controls filtering for list queries.
type ListOptions struct {
	Workspace string
	Tag       string
	Source    string
	Pinned    *bool
	Limit     int
	Offset    int
}

// Store is the interface for document persistence.
type Store interface {
	Add(doc *Document) error
	Get(id string) (*Document, error)
	Update(doc *Document) error
	Delete(id string) error
	List(opts ListOptions) ([]Document, error)
	Search(query string, workspace string, limit int) ([]SearchResult, error)
	Close() error
}
