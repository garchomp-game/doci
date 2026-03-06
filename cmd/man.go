package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var manCmd = &cobra.Command{
	Use:    "man",
	Short:  "manページを生成・インストール",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		manDir := filepath.Join(home, ".local", "share", "man", "man1")
		os.MkdirAll(manDir, 0755)

		header := &doc.GenManHeader{
			Title:   "DOCI",
			Section: "1",
			Date:    &time.Time{},
			Source:  "doci",
			Manual:  "AI-Native Document Indexer",
		}

		if err := doc.GenManTree(rootCmd, header, manDir); err != nil {
			return fmt.Errorf("manページ生成エラー: %w", err)
		}

		fmt.Printf("✅ manページを %s に生成しました\n", manDir)
		fmt.Println("   確認: man doci")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(manCmd)
}
