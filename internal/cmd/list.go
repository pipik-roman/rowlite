package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/pipik-roman/rowlite/internal/core"
)

// TableRenderer defines the interface for formatting and printing table outputs.
type TableRenderer interface {
	Render(out io.Writer, headers []string, rows [][]string) error
}

// GFMRenderer formats and writes the table in Github Flavored Markdown.
type GFMRenderer struct {
	Alignments []string
}

func (g GFMRenderer) Render(out io.Writer, headers []string, rows [][]string) error {
	if _, err := fmt.Fprintln(out, core.FormatGFMLine(headers)); err != nil {
		return err
	}
	seps := make([]string, len(headers))
	for i := range seps {
		if i < len(g.Alignments) && strings.TrimSpace(g.Alignments[i]) != "" {
			seps[i] = g.Alignments[i]
		} else {
			seps[i] = "---"
		}
	}
	if _, err := fmt.Fprintln(out, core.FormatGFMLine(seps)); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(out, core.FormatGFMLine(row)); err != nil {
			return err
		}
	}
	return nil
}

// PrettyRenderer formats and writes the table as an easy-to-read, Unicode-boxed aligned table.
type PrettyRenderer struct {
	Alignments []string
}

func formatAlignedCell(val string, colWidth int, alignStr string) string {
	valLen := utf8.RuneCountInString(val)
	pad := colWidth - valLen
	if pad <= 0 {
		return val
	}

	alignStr = strings.TrimSpace(alignStr)
	hasLeft := strings.HasPrefix(alignStr, ":")
	hasRight := strings.HasSuffix(alignStr, ":")

	if hasLeft && hasRight {
		// Center alignment
		leftPad := pad / 2
		rightPad := pad - leftPad
		return strings.Repeat(" ", leftPad) + val + strings.Repeat(" ", rightPad)
	} else if hasRight {
		// Right alignment
		return strings.Repeat(" ", pad) + val
	}
	// Left alignment (default)
	return val + strings.Repeat(" ", pad)
}

func (p PrettyRenderer) Render(out io.Writer, headers []string, rows [][]string) error {
	if len(headers) == 0 {
		return nil
	}

	// Calculate maximum display width of each column
	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = utf8.RuneCountInString(h)
	}

	for _, row := range rows {
		for i, val := range row {
			if i < len(colWidths) {
				w := utf8.RuneCountInString(val)
				if w > colWidths[i] {
					colWidths[i] = w
				}
			}
		}
	}

	var sb strings.Builder

	// Top border: ┌─────┬──────────┐
	sb.WriteString("┌")
	for i, w := range colWidths {
		if i > 0 {
			sb.WriteString("┬")
		}
		sb.WriteString(strings.Repeat("─", w+2))
	}
	sb.WriteString("┐\n")

	// Header row: │ col1 │ col2     │
	sb.WriteString("│")
	for i, h := range headers {
		align := ""
		if i < len(p.Alignments) {
			align = p.Alignments[i]
		}
		sb.WriteString(" ")
		sb.WriteString(formatAlignedCell(h, colWidths[i], align))
		sb.WriteString(" │")
	}
	sb.WriteString("\n")

	// Separator line: ├─────┼──────────┤
	sb.WriteString("├")
	for i, w := range colWidths {
		if i > 0 {
			sb.WriteString("┼")
		}
		sb.WriteString(strings.Repeat("─", w+2))
	}
	sb.WriteString("┤\n")

	// Data rows: │ val1 │ val2     │
	for _, row := range rows {
		sb.WriteString("│")
		for i, w := range colWidths {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			align := ""
			if i < len(p.Alignments) {
				align = p.Alignments[i]
			}
			sb.WriteString(" ")
			sb.WriteString(formatAlignedCell(val, w, align))
			sb.WriteString(" │")
		}
		sb.WriteString("\n")
	}

	// Bottom border: └─────┴──────────┘
	sb.WriteString("└")
	for i, w := range colWidths {
		if i > 0 {
			sb.WriteString("┴")
		}
		sb.WriteString(strings.Repeat("─", w+2))
	}
	sb.WriteString("┘\n")

	_, err := io.WriteString(out, sb.String())
	return err
}

// JSONRenderer formats and writes the table as a JSON array of objects.
type JSONRenderer struct{}

func (j JSONRenderer) Render(out io.Writer, headers []string, rows [][]string) error {
	var list []map[string]string
	for _, row := range rows {
		obj := make(map[string]string)
		for i, h := range headers {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			obj[h] = val
		}
		list = append(list, obj)
	}
	if list == nil {
		list = []map[string]string{}
	}
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(list)
}

// CSVRenderer formats and writes the table in CSV or TSV format.
type CSVRenderer struct {
	Comma rune
}

func (c CSVRenderer) Render(out io.Writer, headers []string, rows [][]string) error {
	w := csv.NewWriter(out)
	if c.Comma != 0 {
		w.Comma = c.Comma
	}
	if err := w.Write(headers); err != nil {
		return err
	}
	for _, row := range rows {
		writeRow := make([]string, len(headers))
		for i := range headers {
			if i < len(row) {
				writeRow[i] = row[i]
			}
		}
		if err := w.Write(writeRow); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// isTerminal returns true if the writer is an interactive terminal.
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

func NewListCmd() *cobra.Command {
	const whereFlagName = "where"
	var whereFlags []string
	var formatFlag string

	cmd := &cobra.Command{
		Use:   "list <file>",
		Short: "List matching rows from table",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			formatFlag = strings.ToLower(strings.TrimSpace(formatFlag))
			switch formatFlag {
			case "auto", "json", "csv", "tsv", "md", "markdown", "pretty":
				// Valid format
			default:
				return fmt.Errorf("invalid format: %q (must be one of: auto, json, csv, tsv, md, markdown, pretty)", formatFlag)
			}

			filters, err := core.ParseFilters(whereFlags)
			if err != nil {
				return err
			}

			cfg, rows, err := core.ParseFrontmatterAndContent(cmd.Context(), filePath, core.ContentModeAll)
			if err != nil {
				return err
			}
			headers := cfg.Headers
			if len(headers) == 0 {
				return fmt.Errorf("no gfm table headers found in %s", filePath)
			}

			matched, err := core.SelectRows(headers, rows, filters)
			if err != nil {
				return err
			}
			matchedRows := matched.Rows()

			var renderer TableRenderer
			switch formatFlag {
			case "json":
				renderer = JSONRenderer{}
			case "csv":
				renderer = CSVRenderer{Comma: ','}
			case "tsv":
				renderer = CSVRenderer{Comma: '\t'}
			case "md", "markdown":
				renderer = GFMRenderer{Alignments: cfg.Alignments}
			case "pretty":
				renderer = PrettyRenderer{Alignments: cfg.Alignments}
			case "auto":
				if isTerminal(cmd.OutOrStdout()) {
					renderer = PrettyRenderer{Alignments: cfg.Alignments}
				} else {
					renderer = GFMRenderer{Alignments: cfg.Alignments}
				}
			}

			return renderer.Render(cmd.OutOrStdout(), headers, matchedRows)
		},
	}

	cmd.Flags().StringSliceVar(&whereFlags, whereFlagName, nil, "WHERE conditions (supported operators: ==, !=, like, regex)")
	cmd.Flags().StringVarP(&formatFlag, "format", "o", "auto", "Output format (auto, json, csv, tsv, md, pretty)")

	return cmd
}
