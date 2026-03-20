# agent-memory v0.1.0 -- Technical Reference

> Auto-generated from source code on 2026-03-20
> 34 files, 4657 lines, 16 CLI commands, 14 MCP tools, 5 hooks

---

## Architecture

### Package Tree

```
  cmd                18 files   1619 lines   18 functions
  internal/store      4 files   1168 lines   24 functions
  internal/mcp        1 files    858 lines   18 functions
  internal/chunker    2 files    392 lines    9 functions
  internal/embed      4 files    294 lines    6 functions
  internal/config     3 files    109 lines    6 functions
  internal/tagger     1 files     86 lines    2 functions
  internal/common     1 files     20 lines    1 function
  (root)              1 files      7 lines    1 function
```

### Dependency Graph

```
main
  -> cmd
       -> internal/store
       -> internal/config
       -> internal/mcp
       -> internal/embed
       -> internal/chunker
       -> internal/tagger
       -> internal/common

internal/mcp
  -> internal/store
  -> internal/config
  -> internal/embed
  -> internal/chunker
  -> internal/tagger
  -> internal/common

internal/store
  -> internal/embed (cosine similarity)

internal/tagger
  -> internal/store (FTS search for inference)
```

### External Dependencies

| Module | Purpose |
|--------|---------|
| `modernc.org/sqlite` | Pure-Go SQLite with FTS5 (no CGO) |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/segmentio/ksuid` | K-Sortable Globally Unique IDs |

---

## Package: `cmd`

Entry point for all CLI commands. Uses Cobra command tree rooted at `rootCmd`.

### root.go (60 lines)

**Variables:**

- `var Version string` -- Build version, injected via ldflags (default: `"dev"`)

**Functions:**

- `func Execute()` -- Runs the root Cobra command; exits with code 1 on error.
- `func openStore() (*store.SQLiteStore, error)` -- Opens the SQLite store using config/flags for DB path.

**Global Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--db` | | `""` | Database path (default: `~/.agent-memory/memory.db`) |
| `--workspace` | `-w` | `""` | Workspace (default: from config or `"default"`) |

---

### add.go (179 lines)

**Command:** `agent-memory add [content]`

Add a new document to memory. Content can be passed as argument, via `--file`, or piped via stdin.

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--tag` | `-t` | `nil` | Tags (repeatable, e.g. `-t type:note -t topic:dns`) |
| `--source` | `-s` | `"cli"` | Source identifier (`cli`, `hook`, `mcp`) |
| `--pin` | | `false` | Pin this document |
| `--file` | `-f` | `""` | Read content from file (use `-` for stdin) |

**Functions:**

- `func runAdd(cmd, args) error` -- Resolves content, adds document, runs auto-tagging, chunking, and embedding.
- `func embedChunksForDoc(s *store.SQLiteStore, docID string)` -- Embeds unembedded chunks for a document if embeddings are enabled.
- `func resolveContent(args []string) (string, error)` -- Resolves content from args, `--file` flag, or stdin. Enforces 1MB size limit.

---

### search.go (126 lines)

**Command:** `agent-memory search [query]`

Search memory documents. Auto-detects search mode based on configuration.

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--limit` | `-n` | `10` | Max results |
| `--json` | | `false` | Output as JSON |
| `--semantic` | | `false` | Semantic search only (requires embeddings) |
| `--fts` | | `false` | FTS search only (ignore embeddings) |

**Search mode resolution:**
1. `--fts` forces FTS5-only search.
2. `--semantic` forces semantic-only search (requires `OPENAI_API_KEY`).
3. If embeddings are enabled and API key present, uses hybrid search (FTS 30% + semantic 70%).
4. Otherwise, falls back to FTS5.

---

### context.go (154 lines)

**Command:** `agent-memory context`

Assembles relevant context using progressive disclosure: pinned docs, recent docs, then relevant docs.

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--limit` | `-n` | `5` | Max documents per section |
| `--query` | `-q` | `""` | Additional search query (default: current directory name) |
| `--budget` | | `1000` | Character budget for context output |

**Context layers:**
1. **Pinned** -- First line of each pinned document (titles only).
2. **Recent** -- First 100 chars of recent documents.
3. **Relevant** -- FTS search results (full content up to 200 chars each).

**Functions:**

- `func firstLineOf(s string) string` -- Returns the first non-empty line of a string.

---

### delete.go (33 lines)

**Command:** `agent-memory delete <id>`

Deletes a memory document by its KSUID.

---

### embeddings.go (205 lines)

**Command:** `agent-memory embeddings <subcommand>`

Manage embedding generation for semantic search.

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `status` | Show embeddings status (provider, model, chunk stats) |
| `enable` | Enable embeddings (requires `OPENAI_API_KEY`). Sets provider to `openai`, model to `text-embedding-3-small` |
| `disable` | Disable embeddings (clears provider/model from config) |
| `run` | Generate embeddings for unembedded chunks. `--all` flag re-embeds all chunks |

**`run` flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | Re-embed all chunks (not just unembedded) |

Processes in batches of 100 chunks per API call.

---

### export.go (72 lines)

**Command:** `agent-memory export`

Export documents as JSON or markdown.

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--format` | | `"json"` | Output format (`json`, `markdown`) |
| `--tag` | `-t` | `""` | Filter by tag |
| `--all` | | `false` | Export all workspaces |

---

### focus.go (37 lines)

**Command:** `agent-memory focus [workspace]`

Set the active workspace. Without arguments, prints current workspace.

---

### health.go (35 lines)

**Command:** `agent-memory health`

Check database health. Runs `SELECT 1` and prints `ok` or exits with code 1.

---

### hook.go (322 lines)

**Command:** `agent-memory hook [event]`

Process Claude Code hook events.

**Supported events:**

| Event | Behavior |
|-------|----------|
| `post-tool-use` | Captures tool outputs (skips Read, Glob, Grep, Bash, TaskOutput). Scrubs sensitive data. Stores as `[ToolName] output...` with tags `source:hook`, `tool:<name>`. |
| `session-start` | Outputs pinned + recent memories wrapped in `<agent-memory-context>` XML tags. Sanitizes against prompt injection. |
| `stop` | No-op placeholder. |
| `user-prompt-submit` | Captures user prompts (scrubbed, max 1000 chars) with tags `source:hook`, `type:user-prompt`. |
| `session-end` | Builds session summary from last 5 recent docs. Stores with `type:session-summary` tag. Closes session record if `session_id` provided. |

**Types:**

```go
type HookInput struct {
    SessionID string          `json:"session_id"`
    Event     string          `json:"event"`
    ToolName  string          `json:"tool_name"`
    ToolInput json.RawMessage `json:"tool_input"`
    Output    string          `json:"output"`
}
```

**Sensitive patterns scrubbed** (11 regex patterns):
- Password/passwd/pwd assignments
- API keys, secrets, tokens, access keys
- Bearer tokens
- AWS credentials
- PEM private keys
- JWT tokens (three-segment base64)
- URL-embedded credentials (`://user:pass@host`)
- GCP service account keys
- High-entropy hex strings (64+ chars)

**Prompt injection deny-list** (17 patterns):
- `ignore previous instructions`, `disregard previous`, `forget your instructions`
- `new instructions:`, `system prompt:`, `you are now`, `act as`, `pretend to be`
- `override your`, `ignore your rules`, `bypass your`, `from now on you`
- `your new role is`, `you must now`, `do not follow your`, `stop being`, `instead of following`
- `<system>`, `</system>` tags

**Functions:**

- `func truncate(s string, max int) string` -- Truncates string to max length with `...` suffix.
- `func scrubSensitive(s string) string` -- Applies all sensitive patterns, replacing matches with `[REDACTED]`.
- `func sanitizeContextOutput(s string) string` -- Strips XML tags, code fences, and checks instruction deny-list.

---

### init_cmd.go (59 lines)

**Command:** `agent-memory init`

Creates `.agent-memory/` directory with local database in the current project. Appends `.agent-memory/` to `.gitignore` if present.

---

### list.go (83 lines)

**Command:** `agent-memory list`

List memory documents with filters.

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--limit` | `-n` | `20` | Max results |
| `--tag` | `-t` | `""` | Filter by tag |
| `--source` | `-s` | `""` | Filter by source |
| `--pinned` | | `false` | Show only pinned |
| `--json` | | `false` | Output as JSON |

---

### mcp.go (28 lines)

**Command:** `agent-memory mcp`

Starts a Model Context Protocol server over stdin/stdout.

---

### prompt.go (159 lines)

**Command:** `agent-memory prompt <subcommand>`

Manage reusable prompt templates.

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `save <name> [content]` | Save a prompt template. Content via arg or stdin. Tags: `type:prompt`, `prompt:<name>` |
| `get <name>` | Retrieve and print a prompt template by name |
| `list` | List all saved prompt templates (up to 100) |

**`save` flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--tag` | `-t` | `nil` | Additional tags |

---

### stats.go (62 lines)

**Command:** `agent-memory stats`

Show memory statistics: document count, DB size, workspace breakdown.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON |

---

### timeline.go (80 lines)

**Command:** `agent-memory timeline`

Show chronological timeline of documents for a date range.

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--from` | | `""` | Start date, YYYY-MM-DD (default: 7 days ago) |
| `--to` | | `""` | End date, YYYY-MM-DD (default: today) |
| `--limit` | `-n` | `20` | Max results |
| `--json` | | `false` | Output as JSON |

---

### update.go (86 lines)

**Command:** `agent-memory update <id> [content]`

Update document content and/or tags. Content can be passed as argument or piped via stdin.

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--tag` | `-t` | `nil` | Replace tags (repeatable) |

Re-chunks content if updated content exceeds 800 chars.

---

### version.go (19 lines)

**Command:** `agent-memory version`

Prints version string (set via ldflags at build time).

---

## Package: `internal/store`

SQLite-backed document storage with FTS5 full-text search, chunk management, and session tracking.

### store.go (68 lines)

**Interfaces:**

```go
type Store interface {
    Add(doc *Document) error
    Get(id string) (*Document, error)
    Update(doc *Document) error
    Delete(id string) error
    List(opts ListOptions) ([]Document, error)
    Search(query string, workspace string, limit int) ([]SearchResult, error)
    SearchSemantic(queryEmbedding []float32, workspace string, limit int) ([]SearchResult, error)
    HybridSearch(query string, queryEmbedding []float32, workspace string, limit int) ([]SearchResult, error)
    UpdateChunkEmbedding(chunkID int64, embedding []float32) error
    GetUnembeddedChunks(limit int) ([]ChunkRecord, error)
    Stats() (*Stats, error)
    AddChunks(docID string, chunks []string) error
    Timeline(startDate, endDate, workspace string, limit int) ([]Document, error)
    SessionStart(id, project, workspace string) error
    SessionEnd(id, summary string) error
    Close() error
}
```

**Types:**

```go
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

type SearchResult struct {
    Document
    Rank float64 `json:"rank"`
}

type ListOptions struct {
    Workspace string
    Tag       string
    Source    string
    Pinned    *bool
    Limit     int
    Offset    int
}

type Stats struct {
    DocCount        int            `json:"doc_count"`
    WorkspaceCounts map[string]int `json:"workspace_counts"`
    DBSize          int64          `json:"db_size_bytes"`
}

type ChunkRecord struct {
    ID        int64
    DocID     string
    ChunkIdx  int
    ChunkText string
}
```

### sqlite.go (808 lines)

**Type:** `SQLiteStore struct { db *sql.DB }`

**Constants:**

- `currentSchemaVersion = 1`

**Functions:**

- `func Open(path string) (*SQLiteStore, error)` -- Opens/creates SQLite DB with WAL mode, foreign keys, 5s busy timeout. Applies schema, verifies version, restricts file permissions to 0600.
- `func (s *SQLiteStore) Close() error` -- Closes the database connection.
- `func (s *SQLiteStore) DB() *sql.DB` -- Returns underlying `*sql.DB` for direct queries.
- `func (s *SQLiteStore) Add(doc *Document) error` -- Inserts document with KSUID, SHA-256 dedup per workspace.
- `func (s *SQLiteStore) Get(id string) (*Document, error)` -- Retrieves a document by ID.
- `func (s *SQLiteStore) Update(doc *Document) error` -- Updates document content, tags, and metadata.
- `func (s *SQLiteStore) Delete(id string) error` -- Deletes document by ID (cascades to chunks).
- `func (s *SQLiteStore) List(opts ListOptions) ([]Document, error)` -- Lists documents with workspace/tag/source/pinned filters. LIKE wildcards are escaped. Ordered by `created_at DESC`.
- `func (s *SQLiteStore) Search(query, workspace string, limit int) ([]SearchResult, error)` -- FTS5 full-text search with BM25 ranking. Query sanitized via `sanitizeFTS()`.
- `func (s *SQLiteStore) SearchSemantic(queryEmb []float32, workspace string, limit int) ([]SearchResult, error)` -- Cosine similarity search over chunk embeddings. Best chunk score per document. Selection sort for top-k.
- `func (s *SQLiteStore) HybridSearch(query string, queryEmb []float32, workspace string, limit int) ([]SearchResult, error)` -- Combines FTS5 BM25 (weight 0.3) + semantic cosine similarity (weight 0.7). Normalizes BM25 ranks to 0..1.
- `func (s *SQLiteStore) Stats() (*Stats, error)` -- Returns doc count, per-workspace counts, DB size (via PRAGMA page_count * page_size).
- `func (s *SQLiteStore) AddChunks(docID string, chunks []string) error` -- Inserts text chunks for a document (replaces existing). Runs in a transaction.
- `func (s *SQLiteStore) Timeline(startDate, endDate, workspace string, limit int) ([]Document, error)` -- Returns documents in date range. Validates YYYY-MM-DD format.
- `func (s *SQLiteStore) SessionStart(id, project, workspace string) error` -- Creates a new session record with status `active`.
- `func (s *SQLiteStore) SessionEnd(id, summary string) error` -- Closes a session, sets status to `ended`.
- `func (s *SQLiteStore) UpdateChunkEmbedding(chunkID int64, embedding []float32) error` -- Stores embedding as raw little-endian float32 bytes.
- `func (s *SQLiteStore) GetUnembeddedChunks(limit int) ([]ChunkRecord, error)` -- Returns chunks where `embedding IS NULL`.
- `func (s *SQLiteStore) GetAllChunks(limit int) ([]ChunkRecord, error)` -- Returns all chunks (for re-embedding with `--all`).
- `func (s *SQLiteStore) GetUnembeddedChunksByDoc(docID string, limit int) ([]ChunkRecord, error)` -- Returns unembedded chunks for a specific document.
- `func (s *SQLiteStore) ChunkStats() (total int, withEmbeddings int, err error)` -- Returns chunk counts.

**Internal helpers:**

- `func newID() string` -- Generates a KSUID.
- `func contentHash(content string) string` -- SHA-256 hex digest.
- `func tagsToJSON(tags []string) string` -- Marshals tags to JSON array string.
- `func tagsFromJSON(s string) []string` -- Unmarshals JSON array string to tags.
- `func sanitizeFTS(s string) string` -- Strips special characters, limits to 30 words.
- `func float32ToBytes(v []float32) []byte` -- Serializes float32 slice to little-endian bytes.
- `func bytesToFloat32(b []byte) []float32` -- Deserializes little-endian bytes to float32 slice.
- `func boolToInt(b bool) int` -- Converts bool to 0/1.
- `func scanDocument(row scannable) (*Document, error)` -- Scans a row into a Document.

### schema.sql (75 lines)

**Schema version:** 1

---

## SQLite Schema

### Table: `schema_version`

```sql
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER NOT NULL,
    applied_at TEXT DEFAULT (datetime('now'))
);
```

### Table: `documents`

```sql
CREATE TABLE IF NOT EXISTS documents (
    id TEXT PRIMARY KEY,              -- KSUID
    content TEXT NOT NULL,
    content_hash TEXT,                -- SHA-256 hex
    tags TEXT DEFAULT '[]',           -- JSON array of strings
    workspace TEXT DEFAULT 'default',
    source TEXT DEFAULT 'cli',        -- cli, hook, mcp
    pinned INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now')),  -- RFC3339
    updated_at TEXT DEFAULT (datetime('now'))   -- RFC3339
);
```

### Table: `chunks`

```sql
CREATE TABLE IF NOT EXISTS chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    doc_id TEXT REFERENCES documents(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    chunk_text TEXT NOT NULL,
    embedding BLOB,                   -- raw little-endian float32 bytes
    UNIQUE(doc_id, chunk_index)
);
```

### Table: `sessions`

```sql
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    project TEXT,
    workspace TEXT DEFAULT 'default',
    status TEXT DEFAULT 'active',     -- active, ended
    started_at TEXT DEFAULT (datetime('now')),
    ended_at TEXT,
    summary TEXT
);
```

### Table: `observations`

```sql
CREATE TABLE IF NOT EXISTS observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT REFERENCES sessions(id),
    tool_name TEXT,
    content TEXT NOT NULL,
    type TEXT DEFAULT 'observation',
    created_at TEXT DEFAULT (datetime('now'))
);
```

### Indexes

| Index | Table | Column(s) |
|-------|-------|-----------|
| `idx_documents_workspace` | documents | workspace |
| `idx_documents_hash` | documents | content_hash |
| `idx_documents_created` | documents | created_at |
| `idx_chunks_doc` | chunks | doc_id |

### FTS5 Virtual Table

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
    content,
    tags,
    content='documents',
    content_rowid='rowid'
);
```

Content-sync FTS5 table backed by the `documents` table. Auto-synced via triggers:
- `documents_ai` -- AFTER INSERT
- `documents_ad` -- AFTER DELETE
- `documents_au` -- AFTER UPDATE (delete + re-insert)

---

## Package: `internal/mcp`

### server.go (858 lines)

Model Context Protocol server over stdio (JSON-RPC 2.0, newline-delimited).

**Type:** `Server struct { store, workspace, version }`

**Functions:**

- `func NewServer(s *store.SQLiteStore, workspace string, version string) *Server` -- Creates a new MCP server.
- `func (s *Server) Run() error` -- Starts the server with graceful shutdown on SIGTERM/SIGINT. Reads JSON-RPC from stdin, writes responses to stdout.

**Protocol version:** `2024-11-05`

**Capabilities:** `{ "tools": {} }`

---

## MCP Protocol -- Tool Definitions

### memory_add

Add a new memory document.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | Document content (max 1MB) |
| `tags` | array[string] | no | Tags |
| `workspace` | string | no | Workspace (default: current) |

**Returns:** `{"id": "<ksuid>", "status": "created"}`

Auto-tags from similar documents, chunks content >800 chars, embeds chunks if enabled.

### memory_search

Search memory documents using full-text search.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `workspace` | string | no | Workspace filter |
| `limit` | integer | no | Max results (default 10) |

**Returns:** Array of `SearchResult` objects.

Auto-upgrades to hybrid search (FTS 30% + semantic 70%) when embeddings are enabled.

### memory_context

Get smart context with progressive disclosure.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | no | Optional relevance query |
| `limit` | integer | no | Max docs per section |
| `budget` | integer | no | Character budget (default 2000) |

**Returns:** `{"context": "<markdown>"}`

Three layers: Pinned (short), Recent (100 chars), Relevant (200 chars).

### memory_list

List memory documents with filters.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workspace` | string | no | Workspace filter |
| `tag` | string | no | Tag filter |
| `limit` | integer | no | Max results (default 20) |

**Returns:** Array of `Document` objects.

### memory_focus

Switch active workspace.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workspace` | string | yes | Workspace name |

**Returns:** `{"workspace": "<name>", "status": "switched"}`

### memory_delete

Delete a memory document by ID.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Document ID |

**Returns:** `{"id": "<ksuid>", "status": "deleted"}`

### memory_update

Update a document's content and/or tags.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Document ID |
| `content` | string | no | New content |
| `tags` | array[string] | no | Replace tags |

**Returns:** `{"id": "<ksuid>", "status": "updated"}`

Re-chunks and re-embeds if content updated and >800 chars.

### memory_stats

Get memory statistics.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| (none) | | | |

**Returns:** `Stats` object: `doc_count`, `workspace_counts`, `db_size_bytes`.

### memory_timeline

Get documents in chronological order for a date range.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `start_date` | string | yes | Start date (YYYY-MM-DD) |
| `end_date` | string | no | End date (YYYY-MM-DD, default: today) |
| `workspace` | string | no | Workspace filter |
| `limit` | integer | no | Max results (default 20) |

**Returns:** Array of `Document` objects.

### memory_save_prompt

Save a reusable prompt template by name.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Prompt template name |
| `content` | string | yes | Prompt template content |
| `tags` | array[string] | no | Additional tags |

**Returns:** `{"id": "<ksuid>", "name": "<name>", "status": "saved"}`

### memory_get_prompt

Retrieve a saved prompt template by name.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Prompt template name |

**Returns:** `{"name": "<name>", "content": "<content>", "id": "<ksuid>"}`

### memory_suggest_tags

Suggest relevant tags for given content based on similar documents.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | Content to suggest tags for |
| `limit` | integer | no | Max tag suggestions (default 5) |

**Returns:** `{"tags": ["tag1", "tag2", ...]}`

### memory_session_start

Start a new session for tracking agent work.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | no | Project name |
| `workspace` | string | no | Workspace (default: current) |

**Returns:** `{"id": "<ksuid>", "status": "started"}`

### memory_session_end

End a session and record summary.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Session ID |
| `summary` | string | yes | Session summary |

**Returns:** `{"id": "<ksuid>", "status": "ended"}`

---

## Package: `internal/embed`

### embedder.go (10 lines)

**Interface:**

```go
type Embedder interface {
    Embed(text string) ([]float32, error)
    EmbedBatch(texts []string) ([][]float32, error)
    Dimensions() int
    Close() error
}
```

### openai.go (167 lines)

**Constants:**

- `openaiEmbeddingsURL = "https://api.openai.com/v1/embeddings"`
- `defaultOpenAIModel = "text-embedding-3-small"`
- `openaiDimensions = 1536`

**Type:** `OpenAIEmbedder struct { apiKey, model, client }`

**Functions:**

- `func NewOpenAIEmbedder(apiKey string, model string) (*OpenAIEmbedder, error)` -- Creates embedder. Validates API key format (must start with `sk-`, min 20 chars). Default model: `text-embedding-3-small`.
- `func (e *OpenAIEmbedder) Embed(text string) ([]float32, error)` -- Embeds a single text (delegates to `EmbedBatch`).
- `func (e *OpenAIEmbedder) EmbedBatch(texts []string) ([][]float32, error)` -- Batch embedding with one automatic retry on failure (1s delay).
- `func (e *OpenAIEmbedder) Dimensions() int` -- Returns 1536.
- `func (e *OpenAIEmbedder) Close() error` -- No-op (no local resources).

HTTP client timeout: 30 seconds. Response body limit: 10MB.

### cosine.go (57 lines)

**Functions:**

- `func CosineSimilarity(a, b []float32) float64` -- Computes cosine similarity. Returns 0 for mismatched lengths or empty vectors.
- `func TopK(query []float32, vectors [][]float32, k int) []int` -- Returns indices of top-k most similar vectors via selection sort.

---

## Package: `internal/chunker`

### chunker.go (308 lines)

Markdown-aware document chunking for embedding.

**Constants:**

- `DefaultTargetSize = 800` -- Target characters per chunk.
- `DefaultOverlap = 150` -- Overlap characters between chunks.
- `DefaultMinSize = 80` -- Minimum chunk size before merging.

**Functions:**

- `func Chunk(text string, targetSize, overlap, minSize int) []string` -- Main entry point. Splits text into overlapping chunks preserving semantic boundaries. Returns nil for empty text. Returns single-element slice if text <= targetSize.

**Chunking hierarchy** (tries each in order):
1. Split by markdown headers (`# `, `## `, `### `)
2. Split by list items (`- `, `* `) -- requires 4+ items; preserves header in each chunk
3. Split by paragraphs (double newline)
4. Split by sentences (`.!?` followed by whitespace)
5. Hard split at word boundaries

**Internal functions:**

- `splitByHeaders(text string) []string` -- Splits at `^#{1,3}\s` boundaries.
- `splitByListItems(text string, targetSize int) []string` -- Splits lists, repeating header per chunk.
- `splitByParagraphs(text string, targetSize int) []string` -- Splits at double newlines.
- `splitBySentences(text string, targetSize int) []string` -- Splits at sentence endings.
- `hardSplit(text string, targetSize int) []string` -- Splits at last space before targetSize.
- `mergeSmallChunks(chunks []string, minSize int) []string` -- Merges chunks smaller than minSize with previous.
- `applyOverlap(chunks []string, overlap, targetSize int) []string` -- Prepends tail of previous chunk to each chunk.

---

## Package: `internal/tagger`

### autotag.go (86 lines)

**Functions:**

- `func InferTags(s *store.SQLiteStore, content string, workspace string, maxTags int) []string` -- Suggests tags based on FTS5 search of similar documents. Uses first 200 chars as query. Returns tags appearing in 2+ of the top 10 results, sorted by frequency. Skips `source:` and `date:` prefixed tags.

---

## Package: `internal/config`

### config.go (48 lines)

**Type:**

```go
type Config struct {
    ActiveWorkspace   string `json:"active_workspace"`
    EmbeddingProvider string `json:"embedding_provider,omitempty"`  // "", "openai"
    EmbeddingModel    string `json:"embedding_model,omitempty"`     // "text-embedding-3-small"
    DBPath            string `json:"db_path,omitempty"`
}
```

**Functions:**

- `func Load() *Config` -- Reads `config.json` from config dir or returns defaults (`active_workspace: "default"`).
- `func (c *Config) Save() error` -- Writes config to disk with 0600 permissions.

### paths.go (45 lines)

**Constants:**

- `AppName = "agent-memory"`
- `DBFileName = "memory.db"`

**Functions:**

- `func DataDir() string` -- Returns `$XDG_DATA_HOME/agent-memory` or `~/.agent-memory`.
- `func ConfigDir() string` -- Returns `$XDG_CONFIG_HOME/agent-memory` or `~/.agent-memory`.
- `func DefaultDBPath() string` -- Returns `DataDir()/memory.db`.
- `func ProjectDBPath(projectRoot string) string` -- Returns `<projectRoot>/.agent-memory/memory.db`.
- `func EnsureDir(path string) error` -- Creates parent directory with 0700 permissions.

### validate.go (16 lines)

**Functions:**

- `func ValidateWorkspace(name string) error` -- Validates workspace name: 1-64 chars, alphanumeric + hyphens + underscores, must start with alphanumeric. Regex: `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`.

---

## Package: `internal/common`

### common.go (20 lines)

**Constants:**

- `MaxContentSize = 1 << 20` -- 1MB maximum document content size.

**Functions:**

- `func MergeTags(userTags, inferred []string) []string` -- Merges user-provided and auto-inferred tags. User tags take priority (no duplicates).

---

## CLI Reference (Complete)

```
agent-memory                          Root command
  add [content]                       Add a memory document
  search [query]                      Search memory documents
  context                             Get smart context for current session
  delete <id>                         Delete a memory document
  update <id> [content]               Update a memory document
  list                                List memory documents
  focus [workspace]                   Switch active workspace
  stats                               Show memory statistics
  timeline                            Show chronological timeline
  export                              Export documents (JSON/markdown)
  health                              Check database health
  version                             Print version
  init                                Initialize per-project database
  mcp                                 Start MCP server (stdio)
  hook [event]                        Handle Claude Code hook events
  prompt                              Manage prompt templates
    save <name> [content]             Save a prompt template
    get <name>                        Retrieve a prompt template
    list                              List all prompt templates
  embeddings                          Manage embeddings
    status                            Show embeddings status
    enable                            Enable embeddings
    disable                           Disable embeddings
    run                               Generate embeddings for chunks
```

---

## Hook Events Reference

| Event | Stdin Format | Behavior |
|-------|-------------|----------|
| `post-tool-use` | `{"session_id", "event", "tool_name", "tool_input", "output"}` | Stores scrubbed tool output. Skips: Read, Glob, Grep, Bash, TaskOutput. |
| `session-start` | (none) | Outputs pinned + recent memories in `<agent-memory-context>` XML wrapper. |
| `stop` | (none) | No-op. |
| `user-prompt-submit` | `{"session_id", "prompt"}` | Stores scrubbed user prompt (max 1000 chars). |
| `session-end` | `{"session_id"}` | Creates session summary from last 5 docs. Closes session record. |

---

## Configuration Reference

### Config file

Location: `~/.agent-memory/config.json` (or `$XDG_CONFIG_HOME/agent-memory/config.json`)

```json
{
  "active_workspace": "default",
  "embedding_provider": "openai",
  "embedding_model": "text-embedding-3-small",
  "db_path": ""
}
```

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `OPENAI_API_KEY` | Required for semantic search / embeddings |
| `XDG_DATA_HOME` | Override data directory (default: `~/.agent-memory`) |
| `XDG_CONFIG_HOME` | Override config directory (default: `~/.agent-memory`) |

### XDG Paths

| Path | Default | Purpose |
|------|---------|---------|
| Data dir | `~/.agent-memory/` | Database file |
| Config dir | `~/.agent-memory/` | `config.json` |
| Per-project | `.agent-memory/` | Local project database |

### Database

- Default: `~/.agent-memory/memory.db`
- Per-project: `.agent-memory/memory.db` (via `agent-memory init`)
- Custom: `agent-memory --db /path/to/custom.db`
- SQLite with WAL mode, foreign keys, 5s busy timeout
- File permissions: 0600

### Build

```makefile
VERSION ?= 0.1.0
LDFLAGS = -s -w -X github.com/dklymentiev/agent-memory/cmd.Version=$(VERSION)

build:    go build -ldflags "$(LDFLAGS)" -o agent-memory .
test:     go test ./internal/... -count=1
install:  build && cp agent-memory /usr/local/bin/
```
