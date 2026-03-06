package indexer

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter holds parsed YAML frontmatter from a markdown file.
type Frontmatter struct {
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Sidebar     struct {
		Label string `yaml:"label"`
	} `yaml:"sidebar"`
}

// ParseFrontmatter extracts YAML frontmatter from content.
// Returns (frontmatter, body without frontmatter).
func ParseFrontmatter(content string) (*Frontmatter, string) {
	if !strings.HasPrefix(content, "---") {
		return nil, content
	}

	// Find closing ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, content
	}

	yamlStr := rest[:idx]
	body := rest[idx+4:] // skip \n---

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlStr), &fm); err != nil {
		return nil, content
	}

	return &fm, body
}
