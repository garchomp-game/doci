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
	Long: `doci — ドキュメントを SQLite にインデックスし、FTS5 で高速検索できるCLI。

日本語/CJK対応のハイブリッドトークナイザ (unicode61 + trigram) で、
キーワード検索を 2ms 以下で実行します。
AI エージェントとの連携向けに --json / --context / --paths-only を提供。`,
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
