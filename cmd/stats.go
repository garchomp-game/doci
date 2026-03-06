package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/garchomp-game/doci/internal/output"
	"github.com/garchomp-game/doci/internal/store"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "インデックス統計情報",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.OpenRead(dbPath)
		if err != nil {
			return fmt.Errorf("DB接続エラー: %w", err)
		}
		defer db.Close()

		var fc, cc int64
		var ts int64
		db.QueryRow("SELECT COUNT(*) FROM files").Scan(&fc)
		db.QueryRow("SELECT COALESCE(SUM(size_bytes),0) FROM files").Scan(&ts)
		db.QueryRow("SELECT COUNT(*) FROM snippets").Scan(&cc)

		fmt.Println("\n" + strings.Repeat("═", 50))
		fmt.Println("📊 doci インデックス統計")
		fmt.Println(strings.Repeat("═", 50))
		fmt.Printf("  ファイル: \033[32m%s\033[0m  サイズ: \033[36m%s\033[0m  チャンク: \033[33m%s\033[0m\n",
			output.FmtInt(fc), output.FmtSize(ts), output.FmtInt(cc))

		// Extensions
		rows, _ := db.Query("SELECT extension, COUNT(*), SUM(size_bytes) FROM files WHERE extension IS NOT NULL GROUP BY extension ORDER BY COUNT(*) DESC LIMIT 15")
		if rows != nil {
			fmt.Println("\n  🔹 拡張子別 TOP 15:")
			for rows.Next() {
				var ext string
				var cnt, sz int64
				rows.Scan(&ext, &cnt, &sz)
				fmt.Printf("    %-10s %8s files (%8s)\n", ext, output.FmtInt(cnt), output.FmtSize(sz))
			}
			rows.Close()
		}

		// Tags
		tagRows, _ := db.Query("SELECT tags FROM files WHERE tags IS NOT NULL AND tags != ''")
		if tagRows != nil {
			tagMap := make(map[string]int)
			for tagRows.Next() {
				var tags string
				tagRows.Scan(&tags)
				// Parse JSON array
				tags = strings.Trim(tags, "[]\"")
				for _, t := range strings.Split(tags, "\",\"") {
					t = strings.TrimSpace(t)
					if t != "" {
						tagMap[t]++
					}
				}
			}
			tagRows.Close()
			if len(tagMap) > 0 {
				fmt.Println("\n  🏷️  Tags:")
				for tag, count := range tagMap {
					fmt.Printf("    %-20s %d files\n", tag, count)
				}
			}
		}

		// Embeddings
		var ec int64
		db.QueryRow("SELECT COUNT(*) FROM embeddings").Scan(&ec)
		if ec > 0 {
			fmt.Printf("\n  🧠 Embeddings: %s チャンク\n", output.FmtInt(ec))
		}

		// DB size
		if fi, err := os.Stat(dbPath); err == nil {
			fmt.Printf("\n  DB: \033[36m%s\033[0m (%s)\n", output.FmtSize(fi.Size()), dbPath)
		}

		lastIndexed, _ := db.GetMeta("last_indexed_at")
		if lastIndexed != "" {
			fmt.Printf("  最終インデックス: %s\n", lastIndexed)
		}

		fmt.Println(strings.Repeat("═", 50))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
