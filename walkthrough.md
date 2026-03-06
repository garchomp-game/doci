# doci v1.3 — Walkthrough

## Changes

| # | Item | File | Result |
|---|------|------|--------|
| 1 | CJK LIKE fallback (< 3 CJK chars) | [search.go](file:///home/garchomp-game/file-indexer-go/cmd/search.go) | `cjkRuneCount < 3` → `LIKE '%query%'` UNION |
| 2 | `--json` query field was empty | [search.go](file:///home/garchomp-game/file-indexer-go/cmd/search.go) | [outputJSON(query, ...)](file:///home/garchomp-game/file-indexer-go/cmd/search.go#252-268) |
| 3 | rootCmd.Long still said "Embedding" | [root.go](file:///home/garchomp-game/file-indexer-go/cmd/root.go) | Updated to FTS5 + trigram |
| 4 | `--context` dedup (multi-chunk) | [search.go](file:///home/garchomp-game/file-indexer-go/cmd/search.go) | Skip dedup when `--context` set |
| 5 | CJK note in search --help | [search.go](file:///home/garchomp-game/file-indexer-go/cmd/search.go) | Added CJK/Japanese section |

## Test Results

| Query | Hits | Notes |
|-------|------|-------|
| `提案` (2-char CJK) | 1 | LIKE fallback active, dataset has 1 file |
| `認証` (2-char CJK) | 5 ✅ | LIKE + unicode61 hybrid |
| `ウォークスルー` (5-char CJK) | 3 ✅ | trigram |
| `Guard` (ASCII) | 3 ✅ | unicode61, no regression |
| `--json query` | `"query": "Guard"` ✅ | Fixed |
| `--context dedup` | 2 chunks from same file ✅ | Multi-chunk working |

## Commit

```
16abc32 feat: v1.3 — CJK LIKE fallback + JSON query fix + context dedup
```

Pushed to `origin/main`, `~/.local/bin/doci` updated.
