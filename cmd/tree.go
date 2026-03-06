package cmd

import (
	"fmt"

	"github.com/garchomp-game/doci/internal/output"
	"github.com/spf13/cobra"
)

var treeDepth int

var treeCmd = &cobra.Command{
	Use:   "tree [target]",
	Short: "ドキュメントツリーを表示",
	Long:  "ディレクトリのファイル構成をツリー形式で表示します。AIにフォルダ構造を把握させるのに有用です。",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := "."
		if len(args) > 0 {
			target = args[0]
		}
		fmt.Println()
		return output.PrintTree(target, treeDepth)
	},
}

func init() {
	treeCmd.Flags().IntVar(&treeDepth, "depth", 0, "最大深度 (0=無制限)")
	rootCmd.AddCommand(treeCmd)
}
