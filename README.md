# doci — AI-Native Document Indexer

A fast, local CLI that indexes documents into SQLite with FTS5 full-text search, designed as a retrieval layer for AI coding assistants.

## What it does

- **Indexes** documents from any directory into SQLite (~0.2s for 72 files)
- **Searches** via hybrid FTS5 + trigram + LIKE (~1–3ms)
- **Japanese/CJK native** — auto-detects CJK and uses optimal tokenizer
- **AI-ready** — `--json`, `--context`, `--paths-only` for piping to LLMs

> **Current scope**: Fast local FTS indexer. Embedding/semantic search is planned but not yet implemented.

## Install

```bash
make install           # ~/.local/bin (no sudo)
make install-system    # /usr/local/bin (sudo)
make install-man       # man pages
make help              # show all targets
```

Requires: Go 1.22+, CGO enabled (for SQLite)

## Usage

```bash
# Index documents
doci index ./docs --use-gitignore --reset

# Search
doci search "Guard"                     # keyword
doci search "認証 Guard" --score        # CJK + English AND
doci search "提案" --score              # 2-char CJK (LIKE fallback)
doci search "ウォークスルー" --score     # 5-char CJK (trigram)

# Query syntax
doci search "word1 word2"               # AND
doci search "word1 OR word2"            # OR
doci search "word1 NOT word2"           # exclude
doci search '"exact phrase"'            # phrase
doci search "pref*"                     # prefix

# AI integration
doci search "Guard" --json --limit 5    # structured JSON
doci search "Guard" --context           # full chunk content
doci search "Guard" --paths-only        # file paths only

# Explore
doci tree ./docs --depth 3
doci stats
```

## CJK / Japanese Search

doci auto-detects Japanese/Chinese/Korean characters and selects the optimal search strategy:

| Query | Strategy | Example |
|-------|----------|---------|
| ASCII only | `unicode61` FTS5 | `"Guard"` |
| CJK ≥ 3 chars | `trigram` + `unicode61` UNION | `"ウォークスルー"` |
| CJK < 3 chars | Above + `LIKE` fallback | `"提案"` |
| Mixed CJK + ASCII | Hybrid (all strategies) | `"認証 Guard"` |

## Performance (72-file doc set)

| Metric | Value |
|--------|-------|
| Index build | **0.2s** (944 chunks) |
| FTS5 search | **~1ms** |
| CJK LIKE search | **~3ms** |
| DB size | 4.4MB |

## Features

| Feature | Status |
|---------|--------|
| FTS5 keyword search | ✅ |
| Trigram tokenizer (CJK) | ✅ |
| LIKE fallback (short CJK) | ✅ |
| CJK AND search | ✅ |
| `.gitignore` support | ✅ |
| Frontmatter tags + `--tag` | ✅ |
| `--context` (full chunk) | ✅ |
| `--json` output | ✅ |
| `--score` (relevance) | ✅ |
| `--paths-only` | ✅ |
| `--max-file-size` | ✅ |
| Chunk overlap (30 lines + 10) | ✅ |
| `man` pages | ✅ |
| `--incremental` | 🔜 Planned |
| `--watch` | 🔜 Planned |
| Embedding / semantic search | 🔜 Planned |
| `--mcp` (MCP server) | 🔜 Planned |

## Dependencies

- [Cobra](https://cobra.dev/) — CLI framework
- [fastwalk](https://github.com/charlievieth/fastwalk) — parallel FS traversal
- [go-sqlite3](https://github.com/mattn/go-sqlite3) — SQLite with FTS5
- [go-gitignore](https://github.com/sabhiram/go-gitignore) — .gitignore parser

## Docs

- [Requirements & Design](docs/requirements.md)
