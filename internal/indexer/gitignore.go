package indexer

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

// GitIgnore wraps go-gitignore and supports nested .gitignore files.
type GitIgnore struct {
	matchers []*ignore.GitIgnore
}

// LoadGitIgnore reads .gitignore files from dir and all parents.
func LoadGitIgnore(dir string) (*GitIgnore, error) {
	gi := &GitIgnore{}

	// Walk up to find .gitignore files
	current := dir
	for {
		path := filepath.Join(current, ".gitignore")
		if _, err := os.Stat(path); err == nil {
			m, err := ignore.CompileIgnoreFile(path)
			if err == nil {
				gi.matchers = append(gi.matchers, m)
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// Always ignore common dirs
	defaults := []string{
		".git",
		"node_modules",
		".venv",
		"__pycache__",
		".cache",
		"dist",
		".next",
		".nuxt",
	}
	m := ignore.CompileIgnoreLines(defaults...)
	gi.matchers = append(gi.matchers, m)

	return gi, nil
}

// ShouldIgnore returns true if the path should be excluded.
func (gi *GitIgnore) ShouldIgnore(path string) bool {
	for _, m := range gi.matchers {
		if m.MatchesPath(path) {
			return true
		}
	}
	return false
}

// LoadExcludePatterns reads a list of additional exclude patterns.
func LoadExcludePatterns(patterns []string) *ignore.GitIgnore {
	m := ignore.CompileIgnoreLines(patterns...)
	return m
}

// ParseGitIgnoreFile reads a .gitignore file and returns patterns.
func ParseGitIgnoreFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}
