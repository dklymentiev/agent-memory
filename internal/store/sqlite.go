package store

import (
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/segmentio/ksuid"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// SQLiteStore implements Store using SQLite + FTS5.
type SQLiteStore struct {
	db *sql.DB
}

// Open opens or creates a .mem SQLite database at path.
func Open(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	// Restrict DB file permissions
	os.Chmod(path, 0600)

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func newID() string {
	return ksuid.New().String()
}

func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

func tagsToJSON(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

func tagsFromJSON(s string) []string {
	var tags []string
	if err := json.Unmarshal([]byte(s), &tags); err != nil {
		return nil
	}
	return tags
}

// Add inserts a new document. If ID is empty, a KSUID is generated.
// Returns ErrDuplicate if content hash already exists in the workspace.
func (s *SQLiteStore) Add(doc *Document) error {
	if doc.ID == "" {
		doc.ID = newID()
	}
	if doc.Workspace == "" {
		doc.Workspace = "default"
	}
	if doc.Source == "" {
		doc.Source = "cli"
	}
	doc.ContentHash = contentHash(doc.Content)

	// Check for duplicate content in same workspace
	var existing string
	err := s.db.QueryRow(
		"SELECT id FROM documents WHERE content_hash = ? AND workspace = ?",
		doc.ContentHash, doc.Workspace,
	).Scan(&existing)
	if err == nil {
		return fmt.Errorf("duplicate content (existing doc: %s)", existing)
	}

	now := time.Now().UTC()
	doc.CreatedAt = now
	doc.UpdatedAt = now

	_, err = s.db.Exec(
		`INSERT INTO documents (id, content, content_hash, tags, workspace, source, pinned, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.ID, doc.Content, doc.ContentHash,
		tagsToJSON(doc.Tags), doc.Workspace, doc.Source,
		boolToInt(doc.Pinned),
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	return err
}

// Get retrieves a document by ID.
func (s *SQLiteStore) Get(id string) (*Document, error) {
	row := s.db.QueryRow(
		`SELECT id, content, content_hash, tags, workspace, source, pinned, created_at, updated_at
		 FROM documents WHERE id = ?`, id,
	)
	return scanDocument(row)
}

// Update modifies an existing document.
func (s *SQLiteStore) Update(doc *Document) error {
	doc.ContentHash = contentHash(doc.Content)
	doc.UpdatedAt = time.Now().UTC()

	res, err := s.db.Exec(
		`UPDATE documents SET content = ?, content_hash = ?, tags = ?, workspace = ?,
		 source = ?, pinned = ?, updated_at = ? WHERE id = ?`,
		doc.Content, doc.ContentHash, tagsToJSON(doc.Tags), doc.Workspace,
		doc.Source, boolToInt(doc.Pinned), doc.UpdatedAt.Format(time.RFC3339), doc.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("document not found: %s", doc.ID)
	}
	return nil
}

// Delete removes a document by ID.
func (s *SQLiteStore) Delete(id string) error {
	res, err := s.db.Exec("DELETE FROM documents WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("document not found: %s", id)
	}
	return nil
}

// List returns documents matching the given options.
func (s *SQLiteStore) List(opts ListOptions) ([]Document, error) {
	var clauses []string
	var args []any

	if opts.Workspace != "" {
		clauses = append(clauses, "workspace = ?")
		args = append(args, opts.Workspace)
	}
	if opts.Tag != "" {
		// Escape LIKE wildcards to prevent injection
		escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(opts.Tag)
		clauses = append(clauses, "tags LIKE ? ESCAPE '\\'")
		args = append(args, "%"+escaped+"%")
	}
	if opts.Source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, opts.Source)
	}
	if opts.Pinned != nil {
		clauses = append(clauses, "pinned = ?")
		args = append(args, boolToInt(*opts.Pinned))
	}

	query := "SELECT id, content, content_hash, tags, workspace, source, pinned, created_at, updated_at FROM documents"
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at DESC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		doc, err := scanDocumentRows(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, *doc)
	}
	return docs, rows.Err()
}

// Search performs FTS5 full-text search with BM25 ranking.
func (s *SQLiteStore) Search(query string, workspace string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Sanitize query for FTS5 to prevent injection
	query = sanitizeFTS(query)
	if query == "" {
		return nil, nil
	}

	sqlQuery := `
		SELECT d.id, d.content, d.content_hash, d.tags, d.workspace, d.source, d.pinned,
		       d.created_at, d.updated_at, rank
		FROM documents_fts fts
		JOIN documents d ON d.rowid = fts.rowid
		WHERE documents_fts MATCH ?`

	var args []any
	args = append(args, query)

	if workspace != "" {
		sqlQuery += " AND d.workspace = ?"
		args = append(args, workspace)
	}

	sqlQuery += " ORDER BY rank LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var tagsJSON, createdStr, updatedStr string
		var pinnedInt int

		err := rows.Scan(
			&r.ID, &r.Content, &r.ContentHash, &tagsJSON,
			&r.Workspace, &r.Source, &pinnedInt,
			&createdStr, &updatedStr, &r.Rank,
		)
		if err != nil {
			return nil, err
		}
		r.Tags = tagsFromJSON(tagsJSON)
		r.Pinned = pinnedInt != 0
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		results = append(results, r)
	}
	return results, rows.Err()
}

// helpers

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

type scannable interface {
	Scan(dest ...any) error
}

func scanDocument(row scannable) (*Document, error) {
	var doc Document
	var tagsJSON, createdStr, updatedStr string
	var pinnedInt int

	err := row.Scan(
		&doc.ID, &doc.Content, &doc.ContentHash, &tagsJSON,
		&doc.Workspace, &doc.Source, &pinnedInt,
		&createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	doc.Tags = tagsFromJSON(tagsJSON)
	doc.Pinned = pinnedInt != 0
	doc.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	doc.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &doc, nil
}

func scanDocumentRows(rows *sql.Rows) (*Document, error) {
	return scanDocument(rows)
}

// sanitizeFTS cleans user input for safe use in FTS5 MATCH queries.
func sanitizeFTS(s string) string {
	replacer := strings.NewReplacer(
		"\"", "", "'", "", "(", "", ")", "",
		"[", "", "]", "", "{", "", "}", "",
		"*", "", ":", " ", "/", " ", "\\", " ",
		"\n", " ", "\r", " ", "\t", " ",
		"^", "", "~", "", "@", "",
	)
	result := replacer.Replace(s)
	parts := strings.Fields(result)
	if len(parts) > 30 {
		parts = parts[:30]
	}
	return strings.Join(parts, " ")
}
