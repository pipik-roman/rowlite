package core

import (
	"bytes"
	"os"

	"github.com/adrg/frontmatter"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// RowliteTable holds parsed GFM table cells and column metadata
type RowliteTable struct {
	Headers []string
	Rows    [][]string
}

// LoadTable reads a Markdown file, extracts optional YAML frontmatter, and parses GFM tables.
func LoadTable(filePath string) (*RowliteTable, error) {
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// 1. Separate optional Frontmatter metadata from markdown body
	var metadata map[string]interface{}
	markdownBody, err := frontmatter.Parse(bytes.NewReader(fileBytes), &metadata)
	if err != nil {
		// Fallback: If frontmatter is missing or malformed, treat the entire file as body
		markdownBody = fileBytes
	}

	// 2. Initialize goldmark parser with GitHub Flavored Markdown (GFM) extension
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
	)

	// Build the AST (Abstract Syntax Tree)
	pc := parser.NewContext()
	reader := text.NewReader(markdownBody)
	_ = md.Parser().Parse(reader, parser.WithContext(pc))

	// In Phase 2/3, we will implement full AST traversal to extract row data and headers.
	return &RowliteTable{
		Headers: []string{"id", "item_name"},
		Rows:    [][]string{{"1", "Initial Scaffolded Table"}},
	}, nil
}
