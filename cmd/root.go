package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	dbPath  string
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "doci",
	Short: "AI-Native Document Indexer",
	Long: `doci — ドキュメントを SQLite にインデックスし、AIエージェントが高速検索できるCLI。

FTS5（キーワード検索）+ Embedding（セマンティック検索）のハイブリッドで、
「何を読むべきか」の判断を 2ms 以下に短縮します。`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", ".doci.db", "SQLite DB パス")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "詳細ログ")
}
