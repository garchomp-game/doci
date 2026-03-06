package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/garchomp-game/doci/internal/output"
	"github.com/garchomp-game/doci/internal/store"
	"github.com/spf13/cobra"
)

var (
	searchTag     []string
	searchLimit   int
	searchContext bool
	searchJSON    bool
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "FTS5 キーワード検索",
	Long:  "SQLite FTS5 を使ったキーワード全文検索。--tag でfrontmatter tagsの絞り込みも可能。",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]

		db, err := store.OpenRead(dbPath)
		if err != nil {
			return fmt.Errorf("DB接続エラー: %w", err)
		}
		defer db.Close()

		start := time.Now()
		home, _ := os.UserHomeDir()

		fmt.Printf("\n\033[1m🔍 検索: \"%s\"\033[0m", query)
		if len(searchTag) > 0 {
			fmt.Printf(" (tags: %s)", strings.Join(searchTag, ", "))
		}
		fmt.Println("\n")

		// Build query with optional tag filter
		var sqlQuery string
		var sqlArgs []interface{}

		if len(searchTag) > 0 {
			// Join with files and filter by tags
			conditions := make([]string, len(searchTag))
			for i, tag := range searchTag {
				conditions[i] = "f.tags LIKE ?"
				sqlArgs = append(sqlArgs, "%\""+tag+"\"%")
			}
			sqlQuery = fmt.Sprintf(`
				SELECT f.path, f.title, f.tags,
				       snippet(snippets_fts, 0, '>>>', '<<<', '...', 40) AS snip
				FROM snippets_fts
				JOIN snippets s ON s.id = snippets_fts.rowid
				JOIN files f ON f.id = s.file_id
				WHERE snippets_fts MATCH ?
				AND (%s)
				ORDER BY rank LIMIT ?`,
				strings.Join(conditions, " OR "))
			sqlArgs = append([]interface{}{query}, sqlArgs...)
			sqlArgs = append(sqlArgs, searchLimit)
		} else {
			sqlQuery = `
				SELECT f.path, f.title, f.tags,
				       snippet(snippets_fts, 0, '>>>', '<<<', '...', 40) AS snip
				FROM snippets_fts
				JOIN snippets s ON s.id = snippets_fts.rowid
				JOIN files f ON f.id = s.file_id
				WHERE snippets_fts MATCH ?
				ORDER BY rank LIMIT ?`
			sqlArgs = []interface{}{query, searchLimit}
		}

		rows, err := db.Query(sqlQuery, sqlArgs...)
		if err != nil {
			return fmt.Errorf("FTS5エラー: %w\n  → doci index --reset で再構築", err)
		}
		defer rows.Close()

		// Dedup by file
		seen := make(map[string]bool)
		n := 0
		for rows.Next() {
			var path string
			var title, tags, snip sql.NullString
			rows.Scan(&path, &title, &tags, &snip)

			if seen[path] {
				continue
			}
			seen[path] = true

			p := strings.Replace(path, home, "~", 1)

			if searchContext {
				// --context: output full chunk content for piping
				if snip.Valid {
					fmt.Printf("--- %s ---\n%s\n\n", p, snip.String)
				}
			} else {
				t := ""
				if title.Valid && title.String != "" {
					t = fmt.Sprintf(" \033[33m[%s]\033[0m", title.String)
				}
				fmt.Printf("  \033[32m%s\033[0m%s\n", p, t)
				if snip.Valid {
					s := strings.ReplaceAll(snip.String, "\n", " ")
					if len(s) > 120 {
						s = s[:120] + "..."
					}
					fmt.Printf("    \033[2m%s\033[0m\n", s)
				}
			}
			n++
		}

		elapsed := time.Since(start)
		if !searchContext {
			fmt.Printf("\n  \033[1m%d 件\033[0m (%s)\n",
				n, output.FmtDuration(elapsed))
		}
		return nil
	},
}

func init() {
	searchCmd.Flags().StringSliceVar(&searchTag, "tag", nil, "frontmatter tag で絞り込み")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 30, "表示件数上限")
	searchCmd.Flags().BoolVar(&searchContext, "context", false, "チャンク本文を出力（パイプ向け）")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "JSON出力")
	rootCmd.AddCommand(searchCmd)
}
