# Architecture

This document describes the internal architecture of agent-memory for contributors
and anyone who wants to understand how the code is organized.

## Design Principles

1. **Single binary, single file** -- no Docker, no external database, no runtime dependencies.
   The entire system compiles to one static Go binary; all data lives in one SQLite file.

2. **Works without configuration** -- `go install` and start using immediately. Semantic search
   is opt-in; everything else works out of the box with FTS5.

3. **Security by default** -- file permissions restricted, sensitive data scrubbed automatically,
   prompt injection mitigated, all queries sanitized.

4. **MCP-first** -- the MCP server is the primary integration surface. CLI commands exist for
   manual use and scripting, but the MCP tools are what AI agents interact with.

## System Overview

```
                   Claude Code / MCP Client
                          |
                     stdin/stdout
                          |
                    +-----v------+
                    |  MCP Server |  internal/mcp
                    |  (JSON-RPC) |
                    +-----+------+
                          |
          +-------+-------+-------+-------+
          |       |       |       |       |
       +--v--+ +--v--+ +--v--+ +--v--+ +--v--+
       |Store| |Chunk| |Embed| | Tag | | Cfg |
       +--+--+ +-----+ +-----+ +-----+ +-----+
          |     internal/ internal/ internal/ internal/
          |     chunker   embed     tagger    config
          |
     +----v----+
     |  SQLite  |
     | (WAL)    |
     |  FTS5    |
     +----------+
     memory.db
```

```
                    Claude Code Hooks
                          |
                      shell exec
                          |
                    +-----v------+
                    |  cmd/hook  |
                    |  (5 events)|
                    +-----+------+
                          |
                       +--v--+
                       |Store|
                       +--+--+
                          |
                     +----v----+
                     |  SQLite  |
                     +----------+
```

## Package Structure

```
agent-memory/
  main.go                 Entry point (7 lines -- just calls cmd.Execute)
  cmd/                    CLI layer -- one file per command
    root.go               Global flags (--db, --workspace), config init
    add.go                Add documents (from args, file, or stdin)
    search.go             FTS / semantic / hybrid search
    context.go            Progressive disclosure context assembly
    hook.go               Claude Code hook event handlers
    mcp.go                Starts MCP server
    ...                   (delete, export, focus, health, init, list,
                           prompt, stats, timeline, update, version)
  internal/
    store/                Storage layer
      store.go            Interface + types (Document, SearchResult, ListOptions, Stats)
      sqlite.go           SQLite implementation (CRUD, FTS5, semantic search, sessions)
      schema.sql          DDL (embedded via go:embed)
      sqlite_test.go      Unit tests
    mcp/
      server.go           MCP JSON-RPC server (14 tool handlers)
    chunker/
      chunker.go          Markdown-aware document splitting
      chunker_test.go     Tests
    embed/
      embedder.go         Embedder interface
      factory.go          Provider-agnostic embedder creation + model/runtime auto-download
      onnx.go             Local ONNX embedder (all-MiniLM-L6-v2, 384 dims)
      openai.go           OpenAI API client (text-embedding-3-small, 1536 dims)
      cosine.go           Cosine similarity + top-K
    tagger/
      autotag.go          Tag inference from similar documents
    config/
      config.go           Config load/save (JSON)
      paths.go            XDG-compliant path resolution
      validate.go         Workspace name validation
    common/
      common.go           Shared constants (MaxContentSize) and helpers (MergeTags)
```

## Data Flow

### Adding a Document

```
User/Agent
  |
  v
cmd/add.go  or  mcp/server.go (memory_add)
  |
  |  1. Validate content (max 1MB, not empty)
  |  2. Resolve workspace (flag > config > "default")
  v
store.Add()
  |  3. Generate KSUID
  |  4. Compute SHA-256 content hash
  |  5. Check for duplicate in same workspace
  |  6. INSERT into documents table
  |  7. FTS5 trigger fires (auto-syncs to documents_fts)
  v
tagger.InferTags()
  |  8. FTS search using first 200 chars of content
  |  9. Count tag frequency across top 10 results
  | 10. Merge inferred tags (freq >= 2) with user tags
  v
store.Update()  (if new tags were inferred)
  |
  v
chunker.Chunk()  (if content > 800 chars)
  | 11. Split by headers > lists > paragraphs > sentences
  | 12. Apply overlap (150 chars between chunks)
  | 13. Merge chunks smaller than 80 chars
  v
store.AddChunks()
  | 14. DELETE old chunks, INSERT new ones (in transaction)
  v
embed (if enabled)
  | 15. Load config, create embedder via factory (local ONNX or OpenAI)
  | 16. Batch-embed chunk texts (local: ~0.4s/query, OpenAI: API call)
  | 17. Store embeddings as BLOB (little-endian float32, 384 or 1536 dims)
  v
Done -> return {id, status: "created"}
```

### Searching

```
Query arrives (CLI or MCP)
  |
  v
Is embedding enabled? (provider = "local" or "openai")
  |                    |
  no                  yes
  |                    |
  v                    v
FTS5 search        Hybrid search
  |                    |
  |            +-------+--------+
  |            |                |
  |        FTS5 search    Semantic search
  |        (BM25 rank)    (cosine similarity)
  |            |                |
  |            |   Normalize    | Already 0..1
  |            |   to 0..1     |
  |            |                |
  |            +----+     +----+
  |                 |     |
  |              0.3*FTS + 0.7*semantic
  |                    |
  v                    v
Return ranked results (Document + score)
```

### Hook Event Flow

```
Claude Code triggers hook event
  |
  v
Shell: agent-memory hook <event-name>
  |
  |  Reads JSON from stdin
  |  (session_id, tool_name, output, etc.)
  |
  v
post-tool-use:
  |  Skip noisy tools (Read, Glob, Grep, Bash, TaskOutput)
  |  Scrub sensitive patterns (12 regex rules)
  |  Truncate to 500 chars
  |  Store with tags [source:hook, tool:<name>]
  |
session-start:
  |  Query pinned docs (up to 5)
  |  Query recent docs (up to 5)
  |  Sanitize against prompt injection
  |  Output wrapped in <agent-memory-context> XML
  |  (printed to stdout -> Claude Code reads it)
  |
user-prompt-submit:
  |  Scrub sensitive patterns
  |  Truncate to 1000 chars
  |  Store with tags [source:hook, type:user-prompt]
  |
session-end:
  |  Build summary from last 5 docs
  |  Store with tag [type:session-summary]
  |  Close session record in sessions table
```

## Storage

### SQLite Configuration

Opened with these pragmas:
- `journal_mode=WAL` -- allows concurrent readers with one writer
- `foreign_keys=1` -- enforces chunk -> document CASCADE delete
- `busy_timeout=5000` -- waits up to 5s on lock contention

### Schema (v1)

```
schema_version         Track schema version for future migrations
  version INTEGER
  applied_at TEXT

documents              Main document storage
  id TEXT PK           KSUID (K-Sortable Unique ID)
  content TEXT
  content_hash TEXT    SHA-256 hex digest
  tags TEXT            JSON array: '["type:note","topic:dns"]'
  workspace TEXT       Isolation boundary
  source TEXT          Origin: "cli", "hook", "mcp"
  pinned INTEGER       0 or 1
  created_at TEXT      RFC3339
  updated_at TEXT      RFC3339

documents_fts          FTS5 virtual table (content-sync mode)
  Backed by: documents table
  Indexed columns: content, tags
  Sync: via AFTER INSERT/UPDATE/DELETE triggers

chunks                 Document chunks for embedding
  id INTEGER PK
  doc_id TEXT FK       -> documents.id (CASCADE DELETE)
  chunk_index INTEGER
  chunk_text TEXT
  embedding BLOB       Raw little-endian float32 bytes (1536 dims = 6KB)

sessions               Agent session tracking
  id TEXT PK
  project TEXT
  workspace TEXT
  status TEXT          "active" or "ended"
  started_at TEXT
  ended_at TEXT
  summary TEXT

observations           Per-session observations (reserved for future use)
  id INTEGER PK
  session_id TEXT FK   -> sessions.id
  tool_name TEXT
  content TEXT
  type TEXT
  created_at TEXT
```

### Indexes

| Index | Purpose |
|-------|---------|
| `idx_documents_workspace` | Fast workspace filtering |
| `idx_documents_hash` | Duplicate detection |
| `idx_documents_created` | Timeline queries, recent docs |
| `idx_chunks_doc` | Fetch chunks by document |

## MCP Protocol

The MCP server communicates over stdio using JSON-RPC 2.0 (newline-delimited).

### Lifecycle

```
Client                          Server
  |                                |
  |  {"method":"initialize"}  -->  |
  |  <-- {"protocolVersion":...}   |
  |                                |
  |  {"method":"tools/list"}  -->  |
  |  <-- {"tools":[...14 tools]}   |
  |                                |
  |  {"method":"tools/call",       |
  |   "params":{"name":"memory_add",
  |   "arguments":{...}}}     -->  |
  |  <-- {"content":[{"text":...}]}|
  |                                |
  |  (SIGTERM/SIGINT)              |
  |                          close DB
```

### Error Handling

- JSON parse errors: silently skipped (continue reading)
- Unknown methods: return JSON-RPC error (-32601)
- Tool errors: returned as `{"content":[...], "isError": true}`
- Tool results: serialized to JSON, returned as text content

## Chunking Strategy

The chunker splits documents into overlapping pieces for embedding.
It tries boundaries in this priority order:

```
1. Markdown headers (# ## ###)     -- strongest semantic boundary
2. List items (- *)                -- keeps related items together
3. Paragraphs (double newline)     -- natural text breaks
4. Sentences (. ! ? + space)       -- within-paragraph splits
5. Word boundaries (space)         -- last resort hard split
```

Parameters:
- Target size: 800 chars
- Overlap: 150 chars (prepended from previous chunk)
- Minimum size: 80 chars (smaller chunks merged with neighbor)

## Semantic Search

### Embedding Storage

Embeddings are stored as raw bytes in the `chunks.embedding` BLOB column.
Each float32 value is stored as 4 bytes in little-endian order.
For text-embedding-3-small (1536 dimensions), each embedding is 6,144 bytes.

### Search Algorithm

Semantic search loads all embeddings into memory and computes cosine similarity
against the query embedding. Results are deduplicated by document (best chunk
score wins), then sorted by score.

**Scaling note:** This brute-force approach works well for personal use
(up to ~10K documents). For larger collections, consider a vector index
extension or external vector store. This is an area for future improvement.

### Hybrid Ranking

When both FTS and embeddings are available:

1. Run FTS5 search (fetch limit*3 results for better blending)
2. Run semantic search (fetch limit*3 results)
3. Normalize FTS BM25 scores to 0..1 (min-max normalization, inverted because
   BM25 rank is negative in SQLite -- more negative = better match)
4. Combine: `0.3 * fts_score + 0.7 * semantic_score`
5. Return top-K by combined score

## Security Layers

```
Input                    Protection
-----                    ----------
FTS search query    -->  sanitizeFTS(): strip special chars, limit 30 words
Tag filter (LIKE)   -->  Escape % and _ wildcards
Timeline dates      -->  Validate YYYY-MM-DD format
Workspace names     -->  Regex: ^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$
Document content    -->  Max 1MB size limit
Content dedup       -->  SHA-256 hash per workspace

Hook inputs              Protection
-----------              ----------
Tool outputs        -->  scrubSensitive(): 12 regex patterns -> [REDACTED]
User prompts        -->  scrubSensitive() + truncate to 1000 chars
Session context     -->  sanitizeContextOutput():
                           - Strip XML/HTML tags
                           - Remove code fences
                           - 17-pattern instruction deny-list

Files                    Protection
-----                    ----------
Database file       -->  chmod 0600
Config directory    -->  chmod 0700
Config file         -->  chmod 0600
```

## Known Limitations

- **Semantic search is brute-force** -- loads all embeddings into RAM.
  Practical limit: ~10K documents. For larger collections, needs a vector index.

- **No schema migration engine** -- schema_version is tracked but there is no
  automatic migration system yet. Will be needed when schema changes.

- **Hooks open/close DB per invocation** -- each hook event opens a new SQLite
  connection. WAL mode prevents contention, but there is connection overhead.

- **Embedding provider lock-in** -- switching providers (local vs OpenAI) requires
  re-embedding all chunks since vector dimensions differ (384 vs 1536).

- **Single-writer** -- SQLite WAL allows concurrent reads but only one writer.
  Not an issue for single-agent use; may need connection pooling for multi-agent.

## Future Directions

Areas being considered for future versions:

- **Data retention TTL** -- automatic cleanup of old documents
- **Web UI** -- browser-based memory viewer and search
- **Import command** -- structured import from JSON/Markdown exports
- **Observation tracking** -- link tool outputs to sessions via the observations table
