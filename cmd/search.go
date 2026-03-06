package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/garchomp-game/doci/internal/output"
	"github.com/garchomp-game/doci/internal/store"
	"github.com/spf13/cobra"
)

var (
	searchTag       []string
	searchLimit     int
	searchContext   bool
	searchJSON      bool
	searchPathsOnly bool
	searchScore     bool
)

// SearchResult represents a single search hit for JSON output.
type SearchResult struct {
	Path    string  `json:"path"`
	Title   string  `json:"title,omitempty"`
	Tags    string  `json:"tags,omitempty"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet,omitempty"`
	Content string  `json:"content,omitempty"`
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "FTS5 keyword search",
	Long: `Search indexed documents using SQLite FTS5.

Query syntax:
  doci search "keyword"              single keyword
  doci search "word1 word2"          AND (both must appear)
  doci search "word1 OR word2"       OR (either matches)
  doci search "word1 NOT word2"      exclude word2
  doci search '"exact phrase"'       exact phrase match
  doci search "pref*"                prefix match`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]

		db, err := store.OpenRead(dbPath)
		if err != nil {
			return fmt.Errorf("DB open error: %w", err)
		}
		defer db.Close()

		start := time.Now()
		home, _ := os.UserHomeDir()

		// Build SQL
		var sqlQuery string
		var sqlArgs []interface{}

		selectCols := `f.path, f.title, f.tags, rank,
			snippet(snippets_fts, 0, '>>>', '<<<', '...', 40) AS snip`

		if searchContext {
			// --context: return full chunk content, not snippet
			selectCols = `f.path, f.title, f.tags, rank, s.content AS snip`
		}

		baseQuery := fmt.Sprintf(`
			SELECT %s
			FROM snippets_fts
			JOIN snippets s ON s.id = snippets_fts.rowid
			JOIN files f ON f.id = s.file_id`, selectCols)

		if len(searchTag) > 0 {
			// Use file_tags table for normalized tag filtering
			tagPlaceholders := make([]string, len(searchTag))
			for i, tag := range searchTag {
				tagPlaceholders[i] = "?"
				sqlArgs = append(sqlArgs, tag)
			}
			baseQuery += fmt.Sprintf(`
				JOIN file_tags ft ON ft.file_id = f.id
				AND ft.tag IN (%s)`, strings.Join(tagPlaceholders, ","))
		}

		sqlQuery = baseQuery + `
			WHERE snippets_fts MATCH ?
			ORDER BY rank LIMIT ?`
		sqlArgs = append(sqlArgs, query, searchLimit*3) // fetch more for dedup

		rows, err := db.Query(sqlQuery, sqlArgs...)
		if err != nil {
			return fmt.Errorf("FTS5 error: %w\n  → try: doci index --reset", err)
		}
		defer rows.Close()

		// Collect results, dedup by file
		seen := make(map[string]bool)
		var results []SearchResult
		for rows.Next() {
			var path string
			var title, tags, snip sql.NullString
			var rank float64
			rows.Scan(&path, &title, &tags, &rank, &snip)

			if seen[path] {
				continue
			}
			seen[path] = true

			r := SearchResult{
				Path:  path,
				Score: -rank, // FTS5 rank is negative, lower = better
			}
			if title.Valid {
				r.Title = title.String
			}
			if tags.Valid {
				r.Tags = tags.String
			}
			if searchContext {
				if snip.Valid {
					r.Content = snip.String
				}
			} else {
				if snip.Valid {
					r.Snippet = snip.String
				}
			}
			results = append(results, r)
			if len(results) >= searchLimit {
				break
			}
		}

		elapsed := time.Since(start)

		// Output
		if searchJSON {
			return outputJSON(results, elapsed)
		}
		if searchPathsOnly {
			return outputPaths(results, home)
		}
		return outputHuman(results, query, elapsed, home)
	},
}

func outputJSON(results []SearchResult, elapsed time.Duration) error {
	wrapper := struct {
		Query   string         `json:"query"`
		Count   int            `json:"count"`
		TimeMs  float64        `json:"time_ms"`
		Results []SearchResult `json:"results"`
	}{
		Count:   len(results),
		TimeMs:  float64(elapsed.Microseconds()) / 1000.0,
		Results: results,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(wrapper)
}

func outputPaths(results []SearchResult, home string) error {
	for _, r := range results {
		fmt.Println(r.Path)
	}
	return nil
}

func outputHuman(results []SearchResult, query string, elapsed time.Duration, home string) error {
	fmt.Printf("\n\033[1m🔍 search: \"%s\"\033[0m\n\n", query)

	for _, r := range results {
		p := strings.Replace(r.Path, home, "~", 1)

		titleStr := ""
		if r.Title != "" {
			titleStr = fmt.Sprintf(" \033[33m[%s]\033[0m", r.Title)
		}

		if searchScore {
			fmt.Printf("  \033[36m%.2f\033[0m \033[32m%s\033[0m%s\n", r.Score, p, titleStr)
		} else {
			fmt.Printf("  \033[32m%s\033[0m%s\n", p, titleStr)
		}

		if r.Content != "" {
			// --context: print full content
			fmt.Printf("--- %s ---\n%s\n\n", p, r.Content)
		} else if r.Snippet != "" {
			s := strings.ReplaceAll(r.Snippet, "\n", " ")
			if len(s) > 120 {
				s = s[:120] + "..."
			}
			fmt.Printf("    \033[2m%s\033[0m\n", s)
		}
	}

	fmt.Printf("\n  \033[1m%d hits\033[0m (%s)\n",
		len(results), output.FmtDuration(elapsed))
	return nil
}

func init() {
	searchCmd.Flags().StringSliceVar(&searchTag, "tag", nil, "filter by frontmatter tag")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 10, "max results")
	searchCmd.Flags().BoolVar(&searchContext, "context", false, "output full chunk content (for piping to AI)")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "JSON output")
	searchCmd.Flags().BoolVar(&searchPathsOnly, "paths-only", false, "output only file paths")
	searchCmd.Flags().BoolVar(&searchScore, "score", false, "show relevance score")
	rootCmd.AddCommand(searchCmd)
}
