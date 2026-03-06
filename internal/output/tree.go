package output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PrintTree prints an indented file tree for the given directory.
func PrintTree(dir string, maxDepth int) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	fmt.Printf("📂 %s\n", filepath.Base(abs))
	return walkTree(abs, "", 0, maxDepth)
}

func walkTree(dir, prefix string, depth, maxDepth int) error {
	if maxDepth > 0 && depth >= maxDepth {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	// Filter out hidden and common noise
	var visible []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "__pycache__" || name == "dist" {
			continue
		}
		visible = append(visible, e)
	}

	// Sort: dirs first, then files
	sort.Slice(visible, func(i, j int) bool {
		di, dj := visible[i].IsDir(), visible[j].IsDir()
		if di != dj {
			return di
		}
		return visible[i].Name() < visible[j].Name()
	})

	for i, e := range visible {
		isLast := i == len(visible)-1
		connector := "├── "
		childPrefix := "│   "
		if isLast {
			connector = "└── "
			childPrefix = "    "
		}

		name := e.Name()
		if e.IsDir() {
			fmt.Printf("%s%s📁 %s\n", prefix, connector, name)
			walkTree(filepath.Join(dir, name), prefix+childPrefix, depth+1, maxDepth)
		} else {
			ext := filepath.Ext(name)
			icon := fileIcon(ext)
			fmt.Printf("%s%s%s %s\n", prefix, connector, icon, name)
		}
	}
	return nil
}

func fileIcon(ext string) string {
	switch ext {
	case ".md", ".mdx":
		return "📝"
	case ".go":
		return "🔵"
	case ".ts", ".tsx":
		return "🟦"
	case ".js", ".jsx":
		return "🟨"
	case ".py":
		return "🐍"
	case ".json":
		return "📋"
	case ".yaml", ".yml":
		return "⚙️"
	case ".sql":
		return "🗄️"
	default:
		return "📄"
	}
}

// FmtSize formats byte count to human readable.
func FmtSize(b int64) string {
	const u = 1024
	if b < u {
		return fmt.Sprintf("%dB", b)
	}
	d, e := int64(u), 0
	for n := b / u; n >= u; n /= u {
		d *= u
		e++
	}
	return fmt.Sprintf("%.1f%s", float64(b)/float64(d), []string{"KB", "MB", "GB", "TB"}[e])
}

// FmtInt formats integer with comma separator.
func FmtInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var r []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			r = append(r, ',')
		}
		r = append(r, byte(c))
	}
	return string(r)
}
