package cmd

import (
	"fmt"
	"time"

	"github.com/garchomp-game/doci/internal/indexer"
	"github.com/garchomp-game/doci/internal/output"
	"github.com/spf13/cobra"
)

var (
	useGitIgnore bool
	reset        bool
	incremental  bool
	excludes     []string
)

var indexCmd = &cobra.Command{
	Use:   "index [target]",
	Short: "ドキュメントをインデックス構築",
	Long:  "指定ディレクトリのファイルを走査し、SQLite FTS5 インデックスを構築します。",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := "."
		if len(args) > 0 {
			target = args[0]
		}

		fmt.Println("╔══════════════════════════════════════════════════╗")
		fmt.Println("║  🔍 doci — AI-Native Document Indexer           ║")
		fmt.Println("╚══════════════════════════════════════════════════╝")
		fmt.Printf("   DB: %s\n   対象: %s\n\n", dbPath, target)

		if useGitIgnore {
			fmt.Println("   📋 .gitignore: 有効")
		}
		if reset {
			fmt.Println("   🗑️  リセット: DB削除して再構築")
		}
		if incremental {
			fmt.Println("   ⚡ 差分インデックス")
		}
		fmt.Println()

		cfg := indexer.Config{
			Target:       target,
			UseGitIgnore: useGitIgnore,
			Exclude:      excludes,
			Reset:        reset,
			Incremental:  incremental,
			Verbose:      verbose,
		}

		start := time.Now()
		result, err := indexer.Run(cfg, dbPath)
		if err != nil {
			return fmt.Errorf("インデックス構築失敗: %w", err)
		}

		fmt.Println("════════════════════════════════════════════════════")
		fmt.Println("📊 インデックス構築完了")
		fmt.Println("════════════════════════════════════════════════════")
		fmt.Printf("   ファイル数:   %s\n", output.FmtInt(result.FileCount))
		fmt.Printf("   チャンク数:   %s\n", output.FmtInt(result.ChunkCount))
		if result.ErrorCount > 0 {
			fmt.Printf("   エラー:       %d\n", result.ErrorCount)
		}
		fmt.Printf("   DBサイズ:     %s\n", output.FmtSize(result.DBSize))
		fmt.Printf("   所要時間:     %.2f秒\n", time.Since(start).Seconds())
		fmt.Println("════════════════════════════════════════════════════")
		fmt.Println("\n🔍 検索: doci search \"keyword\"")
		return nil
	},
}

func init() {
	indexCmd.Flags().BoolVar(&useGitIgnore, "use-gitignore", false, ".gitignore を尊重")
	indexCmd.Flags().BoolVar(&reset, "reset", false, "DB削除して再構築")
	indexCmd.Flags().BoolVar(&incremental, "incremental", false, "差分のみ再インデックス")
	indexCmd.Flags().StringSliceVar(&excludes, "exclude", nil, "追加除外パターン")
	rootCmd.AddCommand(indexCmd)
}
