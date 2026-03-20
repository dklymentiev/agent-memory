package store

import (
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	embedpkg "github.com/steamfoundry/agent-memory/internal/embed"

	"github.com/segmentio/ksuid"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// SQLiteStore implements Store using SQLite + FTS5.
type SQLiteStore struct {
	db *sql.DB
}

const currentSchemaVersion = 1

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

	// Verify schema version
	var version int
	err = db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("read schema version: %w", err)
	}
	if version > currentSchemaVersion {
		db.Close()
		return nil, fmt.Errorf("database schema version %d is newer than supported version %d; upgrade agent-memory", version, currentSchemaVersion)
	}

	// Restrict DB file permissions
	os.Chmod(path, 0600)

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for direct queries (e.g., health checks).
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
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
		// Errors intentionally ignored: see scanDocument for rationale.
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		results = append(results, r)
	}
	return results, rows.Err()
}

// Stats returns aggregate statistics about the store.
func (s *SQLiteStore) Stats() (*Stats, error) {
	st := &Stats{
		WorkspaceCounts: make(map[string]int),
	}

	// Total doc count
	if err := s.db.QueryRow("SELECT COUNT(*) FROM documents").Scan(&st.DocCount); err != nil {
		return nil, fmt.Errorf("count docs: %w", err)
	}

	// Per-workspace counts
	rows, err := s.db.Query("SELECT workspace, COUNT(*) FROM documents GROUP BY workspace ORDER BY COUNT(*) DESC")
	if err != nil {
		return nil, fmt.Errorf("workspace counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ws string
		var count int
		if err := rows.Scan(&ws, &count); err != nil {
			return nil, err
		}
		st.WorkspaceCounts[ws] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// DB file size -- use pragma to get page count * page size
	var pageCount, pageSize int64
	s.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	s.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	st.DBSize = pageCount * pageSize

	return st, nil
}

// AddChunks inserts text chunks for a document, replacing any existing chunks.
func (s *SQLiteStore) AddChunks(docID string, chunks []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Remove old chunks
	if _, err := tx.Exec("DELETE FROM chunks WHERE doc_id = ?", docID); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}

	stmt, err := tx.Prepare("INSERT INTO chunks (doc_id, chunk_index, chunk_text) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for i, chunk := range chunks {
		if _, err := stmt.Exec(docID, i, chunk); err != nil {
			return fmt.Errorf("insert chunk %d: %w", i, err)
		}
	}

	return tx.Commit()
}

// Timeline returns documents in chronological order for a date range.
func (s *SQLiteStore) Timeline(startDate, endDate, workspace string, limit int) ([]Document, error) {
	if limit <= 0 {
		limit = 20
	}

	// Validate date format to prevent injection
	if _, err := time.Parse("2006-01-02", startDate); err != nil {
		return nil, fmt.Errorf("invalid start_date format (expected YYYY-MM-DD): %w", err)
	}
	if _, err := time.Parse("2006-01-02", endDate); err != nil {
		return nil, fmt.Errorf("invalid end_date format (expected YYYY-MM-DD): %w", err)
	}

	startTime := startDate + "T00:00:00Z"
	endTime := endDate + "T23:59:59Z"

	var clauses []string
	var args []any

	clauses = append(clauses, "created_at >= ?")
	args = append(args, startTime)
	clauses = append(clauses, "created_at <= ?")
	args = append(args, endTime)

	if workspace != "" {
		clauses = append(clauses, "workspace = ?")
		args = append(args, workspace)
	}

	query := "SELECT id, content, content_hash, tags, workspace, source, pinned, created_at, updated_at FROM documents"
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT %d", limit)

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

// SessionStart creates a new session record.
func (s *SQLiteStore) SessionStart(id, project, workspace string) error {
	if workspace == "" {
		workspace = "default"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		"INSERT INTO sessions (id, project, workspace, status, started_at) VALUES (?, ?, ?, 'active', ?)",
		id, project, workspace, now,
	)
	return err
}

// SessionEnd closes a session and records its summary.
func (s *SQLiteStore) SessionEnd(id, summary string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		"UPDATE sessions SET status = 'ended', ended_at = ?, summary = ? WHERE id = ?",
		now, summary, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session not found: %s", id)
	}
	return nil
}

// UpdateChunkEmbedding stores an embedding for a chunk as raw little-endian float32 bytes.
func (s *SQLiteStore) UpdateChunkEmbedding(chunkID int64, embedding []float32) error {
	blob := float32ToBytes(embedding)
	_, err := s.db.Exec("UPDATE chunks SET embedding = ? WHERE id = ?", blob, chunkID)
	return err
}

// GetUnembeddedChunks returns chunks that don't have embeddings yet.
func (s *SQLiteStore) GetUnembeddedChunks(limit int) ([]ChunkRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		"SELECT id, doc_id, chunk_index, chunk_text FROM chunks WHERE embedding IS NULL LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []ChunkRecord
	for rows.Next() {
		var c ChunkRecord
		if err := rows.Scan(&c.ID, &c.DocID, &c.ChunkIdx, &c.ChunkText); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// GetAllChunks returns all chunks (for re-embedding with --all).
func (s *SQLiteStore) GetAllChunks(limit int) ([]ChunkRecord, error) {
	if limit <= 0 {
		limit = 100000
	}
	rows, err := s.db.Query(
		"SELECT id, doc_id, chunk_index, chunk_text FROM chunks LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []ChunkRecord
	for rows.Next() {
		var c ChunkRecord
		if err := rows.Scan(&c.ID, &c.DocID, &c.ChunkIdx, &c.ChunkText); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// GetUnembeddedChunksByDoc returns chunks for a specific document that don't have embeddings yet.
func (s *SQLiteStore) GetUnembeddedChunksByDoc(docID string, limit int) ([]ChunkRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		"SELECT id, doc_id, chunk_index, chunk_text FROM chunks WHERE doc_id = ? AND embedding IS NULL LIMIT ?",
		docID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []ChunkRecord
	for rows.Next() {
		var c ChunkRecord
		if err := rows.Scan(&c.ID, &c.DocID, &c.ChunkIdx, &c.ChunkText); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// ChunkStats returns total chunk count and count of chunks with embeddings.
func (s *SQLiteStore) ChunkStats() (total int, withEmbeddings int, err error) {
	err = s.db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&total)
	if err != nil {
		return
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM chunks WHERE embedding IS NOT NULL").Scan(&withEmbeddings)
	return
}

// SearchSemantic finds documents by cosine similarity to query embedding.
// Returns doc IDs with scores, deduplicated (best chunk score per doc).
func (s *SQLiteStore) SearchSemantic(queryEmbedding []float32, workspace string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Load all chunks with embeddings, optionally filtered by workspace
	var rows *sql.Rows
	var err error
	if workspace != "" {
		rows, err = s.db.Query(
			`SELECT c.doc_id, c.embedding FROM chunks c
			 JOIN documents d ON d.id = c.doc_id
			 WHERE c.embedding IS NOT NULL AND d.workspace = ?`,
			workspace,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT c.doc_id, c.embedding FROM chunks c
			 WHERE c.embedding IS NOT NULL`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Compute cosine similarity, keep best per doc
	type docScore struct {
		docID string
		score float64
	}
	bestScores := make(map[string]float64)
	for rows.Next() {
		var docID string
		var blob []byte
		if err := rows.Scan(&docID, &blob); err != nil {
			return nil, err
		}
		emb := bytesToFloat32(blob)
		if len(emb) == 0 {
			continue
		}
		score := embedpkg.CosineSimilarity(queryEmbedding, emb)
		if score > bestScores[docID] {
			bestScores[docID] = score
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by score descending
	sorted := make([]docScore, 0, len(bestScores))
	for id, sc := range bestScores {
		sorted = append(sorted, docScore{id, sc})
	}
	// Selection sort for top limit
	for i := 0; i < limit && i < len(sorted); i++ {
		maxIdx := i
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].score > sorted[maxIdx].score {
				maxIdx = j
			}
		}
		sorted[i], sorted[maxIdx] = sorted[maxIdx], sorted[i]
	}
	n := limit
	if n > len(sorted) {
		n = len(sorted)
	}
	sorted = sorted[:n]

	// Fetch full documents
	results := make([]SearchResult, 0, n)
	for _, ds := range sorted {
		doc, err := s.Get(ds.docID)
		if err != nil {
			continue
		}
		results = append(results, SearchResult{Document: *doc, Rank: ds.score})
	}
	return results, nil
}

// HybridSearch combines FTS5 BM25 + semantic cosine similarity.
// ftsWeight=0.3, semanticWeight=0.7 by default.
func (s *SQLiteStore) HybridSearch(query string, queryEmbedding []float32, workspace string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	const ftsWeight = 0.3
	const semanticWeight = 0.7

	// Collect scores per doc from both sources
	type combinedScore struct {
		fts      float64
		semantic float64
	}
	scores := make(map[string]*combinedScore)

	// 1. FTS5 search
	ftsResults, err := s.Search(query, workspace, limit*3) // fetch more for better blending
	if err == nil && len(ftsResults) > 0 {
		// BM25 rank values from SQLite are negative (more negative = better match).
		// Find min/max for normalization.
		minRank := ftsResults[0].Rank
		maxRank := ftsResults[0].Rank
		for _, r := range ftsResults {
			if r.Rank < minRank {
				minRank = r.Rank
			}
			if r.Rank > maxRank {
				maxRank = r.Rank
			}
		}
		for _, r := range ftsResults {
			var normalized float64
			if minRank == maxRank {
				normalized = 1.0
			} else {
				// More negative = better, so invert: best rank -> 1.0
				normalized = (maxRank - r.Rank) / (maxRank - minRank)
			}
			sc := scores[r.ID]
			if sc == nil {
				sc = &combinedScore{}
				scores[r.ID] = sc
			}
			sc.fts = normalized
		}
	}

	// 2. Semantic search
	if queryEmbedding != nil {
		semResults, err := s.SearchSemantic(queryEmbedding, workspace, limit*3)
		if err == nil {
			for _, r := range semResults {
				sc := scores[r.ID]
				if sc == nil {
					sc = &combinedScore{}
					scores[r.ID] = sc
				}
				sc.semantic = r.Rank // already 0..1 cosine similarity
			}
		}
	}

	// 3. Combine and sort
	type ranked struct {
		docID string
		score float64
	}
	var combined []ranked
	for id, sc := range scores {
		total := ftsWeight*sc.fts + semanticWeight*sc.semantic
		combined = append(combined, ranked{id, total})
	}

	// Selection sort for top limit
	for i := 0; i < limit && i < len(combined); i++ {
		maxIdx := i
		for j := i + 1; j < len(combined); j++ {
			if combined[j].score > combined[maxIdx].score {
				maxIdx = j
			}
		}
		combined[i], combined[maxIdx] = combined[maxIdx], combined[i]
	}
	n := limit
	if n > len(combined) {
		n = len(combined)
	}

	// Build results -- try to reuse docs from FTS results to avoid extra queries
	ftsMap := make(map[string]SearchResult, len(ftsResults))
	for _, r := range ftsResults {
		ftsMap[r.ID] = r
	}

	results := make([]SearchResult, 0, n)
	for i := 0; i < n; i++ {
		r := combined[i]
		if fr, ok := ftsMap[r.docID]; ok {
			fr.Rank = r.score
			results = append(results, fr)
		} else {
			doc, err := s.Get(r.docID)
			if err != nil {
				continue
			}
			results = append(results, SearchResult{Document: *doc, Rank: r.score})
		}
	}
	return results, nil
}

// float32ToBytes serializes a float32 slice to raw little-endian bytes.
func float32ToBytes(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// bytesToFloat32 deserializes raw little-endian bytes to a float32 slice.
func bytesToFloat32(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
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
	// Errors from time.Parse are intentionally ignored: timestamps are always
	// written by this package in RFC3339 format, so parse failures cannot occur
	// in practice. A zero time.Time is an acceptable fallback if they did.
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
