package tagger

// Well-known tag prefixes for agent-memory.
var Prefixes = map[string]string{
	"type":    "Document type (note, worklog, decision, research, artifact)",
	"topic":   "Topic area (dns, auth, deployment, etc.)",
	"project": "Project identifier",
	"source":  "How it was created (cli, hook, mcp)",
	"tool":    "Tool that generated the content",
	"status":  "Document status (draft, active, archived)",
	"date":    "Date (YYYY-MM-DD)",
}
