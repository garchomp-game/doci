# doci — AI-Native Document Indexer

A fast, local CLI that indexes documents into SQLite with FTS5 full-text search, designed as a retrieval layer for AI coding assistants.

## What it does

- **Indexes** documents from any directory into SQLite
- **Searches** via FTS5 keyword matching (~1ms)
- **Outputs** structured results (human, JSON, paths-only, context) for AI piping

> **Current scope**: Fast local FTS indexer. Embedding/semantic search is planned but not yet implemented.

## Install

```bash
# Build + install to ~/.local/bin (no sudo needed)
make install

# Or system-wide
make install-system
```

Requires: Go 1.22+, CGO enabled (for SQLite)

## Usage

```bash
# Index documents
doci index ./src/content --use-gitignore --reset

# Search (FTS5 keyword)
doci search "Guard"
doci search "認証 Guard" --score

# Query syntax
doci search "word1 word2"          # AND
doci search "word1 OR word2"       # OR
doci search '"exact phrase"'       # phrase
doci search "pref*"                # prefix

# JSON output (for AI/script consumption)
doci search "Guard" --json --limit 5

# Full chunk content (pipe to AI)
doci search "REQ-B01" --context | your-ai-command

# Paths only (for scripting)
doci search "Guard" --paths-only

# Tree view
doci tree ./src/content --depth 3

# Stats
doci stats
```

## Performance (72-file doc set)

| Metric | Value |
|--------|-------|
| Index build | **0.02s** |
| FTS5 search | **~1ms** |
| DB size | 1.1MB |

## Features

| Feature | Status |
|---------|--------|
| FTS5 keyword search | ✅ |
| `.gitignore` support | ✅ |
| Frontmatter tags parsing | ✅ |
| `--tag` filter | ✅ |
| `--context` (full chunk output) | ✅ |
| `--json` output | ✅ |
| `--score` (relevance) | ✅ |
| `--paths-only` | ✅ |
| `--max-file-size` | ✅ |
| `man` pages | ✅ |
| Unicode61 tokenizer (CJK) | ✅ |
| `--incremental` | 🔜 Planned |
| `--watch` | 🔜 Planned |
| Embedding / semantic search | 🔜 Planned |
| `--hybrid` (RRF ranking) | 🔜 Planned |
| `--mcp` (MCP server) | 🔜 Planned |

## Dependencies

- [Cobra](https://cobra.dev/) — CLI framework
- [fastwalk](https://github.com/charlievieth/fastwalk) — parallel FS traversal
- [go-sqlite3](https://github.com/mattn/go-sqlite3) — SQLite with FTS5
- [go-gitignore](https://github.com/sabhiram/go-gitignore) — .gitignore parser

## Docs

- [Requirements & Design](docs/requirements.md)
