# Security Policy

## Reporting Vulnerabilities

If you discover a security vulnerability in agent-memory, please report it responsibly.

**Report:** Use [GitHub private vulnerability reports](https://github.com/dklymentiev/agent-memory/security/advisories/new) (preferred).
**Response time:** I aim to acknowledge within 48 hours.

Please include:
- Description of the vulnerability
- Steps to reproduce
- Impact assessment
- Suggested fix (if any)

Do NOT open a public GitHub issue for security vulnerabilities.

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.1.x   | Yes       |

## Security Architecture

agent-memory stores documents in a local SQLite database. It runs as a CLI tool or MCP server -- no network server, no authentication layer.

### Data Protection

- **File permissions** -- database (0600) and config directory (0700) are created with restricted permissions
- **Content dedup** -- SHA-256 hashing prevents duplicate storage
- **Content size cap** -- 1MB per document maximum
- **Workspace validation** -- names validated against `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`

### Hook Security (sensitive data scrubbing)

The PostToolUse and UserPromptSubmit hooks automatically scrub these patterns before storing content:

- Password, API key, secret, and token assignments
- Bearer authorization tokens
- AWS access key ID and secret access key
- PEM private keys (`-----BEGIN PRIVATE KEY-----`)
- JWT tokens (three dot-separated base64 segments)
- URL-embedded credentials (`://user:pass@host`)
- GCP service account JSON keys
- High-entropy hex strings (64+ chars) following key/secret/token words

All matches are replaced with `[REDACTED]`.

### Prompt Injection Protection

When memory content is injected into agent sessions (via the `session-start` hook), it is sanitized:

1. **XML boundary markers** -- context wrapped in `<agent-memory-context type="data" readonly="true">` tags
2. **Tag stripping** -- all XML/HTML tags within memory content are removed
3. **Code fence removal** -- triple backtick fences are stripped
4. **Instruction deny-list** -- 17 known injection patterns (e.g., "ignore previous instructions", "system prompt:", "you are now") trigger `[REDACTED: potential prompt injection]`

### Query Sanitization

- **FTS5 queries** -- special characters stripped, queries limited to 30 words
- **LIKE wildcards** -- `%` and `_` escaped in tag filter queries
- **Date inputs** -- validated against `YYYY-MM-DD` format

### Dependencies

agent-memory compiles to a single static binary with no runtime dependencies:

| Dependency | Purpose | Security Notes |
|-----------|---------|----------------|
| `modernc.org/sqlite` | Pure-Go SQLite | No CGO, no shared libraries |
| `github.com/spf13/cobra` | CLI framework | Well-audited, widely used |
| `github.com/segmentio/ksuid` | Unique IDs | No crypto dependency |

No network listeners (except MCP stdio). No external database. No Docker.
