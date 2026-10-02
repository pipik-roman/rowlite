package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
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
type JSONRenderer struct {
	IncludeRowIndex bool
	Matched         core.MatchedRows
}

func (j JSONRenderer) Render(out io.Writer, headers []string, rows [][]string) error {
	var list []map[string]any
	for i, row := range rows {
		obj := make(map[string]any)
		if j.IncludeRowIndex && i < len(j.Matched) {
			obj["__row_index__"] = j.Matched[i].Index
		}
		for colIdx, h := range headers {
			val := ""
			if colIdx < len(row) {
				val = row[colIdx]
			}
			obj[h] = val
		}
		list = append(list, obj)
	}
	if list == nil {
		list = []map[string]any{}
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

func getTerminalWidth() int {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		w, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err == nil && w > 20 {
			return w
		}
	}
	return 100
}

// WrapSoft wraps text into lines of at most width runes at whitespace boundaries.
func WrapSoft(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{""}
	}

	paragraphs := strings.Split(text, "\n")
	var result []string

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			result = append(result, "")
			continue
		}

		words := strings.Fields(p)
		var currentLine strings.Builder
		currentLen := 0

		for _, word := range words {
			wLen := utf8.RuneCountInString(word)
			if currentLen == 0 {
				if wLen <= width {
					currentLine.WriteString(word)
					currentLen = wLen
				} else {
					runes := []rune(word)
					for len(runes) > width {
						result = append(result, string(runes[:width]))
						runes = runes[width:]
					}
					currentLine.WriteString(string(runes))
					currentLen = len(runes)
				}
			} else if currentLen+1+wLen <= width {
				currentLine.WriteString(" ")
				currentLine.WriteString(word)
				currentLen += 1 + wLen
			} else {
				result = append(result, currentLine.String())
				currentLine.Reset()
				currentLine.WriteString(word)
				currentLen = wLen
			}
		}
		if currentLine.Len() > 0 {
			result = append(result, currentLine.String())
		}
	}

	if len(result) == 0 {
		return []string{""}
	}
	return result
}

// VerticalCardRenderer formats and writes each record as an expanded vertical card.
type VerticalCardRenderer struct {
	MaxWidth int
}

func (v VerticalCardRenderer) Render(out io.Writer, headers []string, rows [][]string) error {
	w := v.MaxWidth
	if w <= 0 {
		w = getTerminalWidth()
	}
	if w < 40 {
		w = 40
	}

	maxHeaderLen := 0
	for _, h := range headers {
		hl := utf8.RuneCountInString(h)
		if hl > maxHeaderLen {
			maxHeaderLen = hl
		}
	}

	valWidth := w - maxHeaderLen - 4
	if valWidth < 20 {
		valWidth = 20
	}

	for rIdx, row := range rows {
		recordLabel := fmt.Sprintf("-[ RECORD %d ]", rIdx+1)
		dashes := w - utf8.RuneCountInString(recordLabel)
		if dashes < 2 {
			dashes = 2
		}
		if _, err := fmt.Fprintf(out, "%s%s\n", recordLabel, strings.Repeat("-", dashes)); err != nil {
			return err
		}

		for cIdx, h := range headers {
			val := ""
			if cIdx < len(row) {
				val = row[cIdx]
			}
			wrapped := WrapSoft(val, valWidth)
			paddedHeader := formatAlignedCell(h, maxHeaderLen, "right")
			for subIdx, subLine := range wrapped {
				if subIdx == 0 {
					if _, err := fmt.Fprintf(out, "%s : %s\n", paddedHeader, subLine); err != nil {
						return err
					}
				} else {
					if _, err := fmt.Fprintf(out, "%s   %s\n", strings.Repeat(" ", maxHeaderLen), subLine); err != nil {
						return err
					}
				}
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	return nil
}

type ColumnPlacement struct {
	ColIdx int
	Width  int
}

type LinePlacement struct {
	StartIndex int
	Columns    []ColumnPlacement
}

func computeLinePlacements(headers []string, rows [][]string, W int) []LinePlacement {
	totalCols := len(headers)
	colMax := make([]int, totalCols)
	for i, h := range headers {
		colMax[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, val := range row {
			if i < totalCols {
				lines := strings.Split(strings.TrimSpace(val), "\n")
				for _, l := range lines {
					w := utf8.RuneCountInString(strings.TrimRight(l, "\r"))
					if w > colMax[i] {
						colMax[i] = w
					}
				}
			}
		}
	}

	totalNaturalWidth := 1
	for c := 0; c < totalCols; c++ {
		colW := colMax[c]
		minW := utf8.RuneCountInString(headers[c])
		if colW < minW {
			colW = minW
		}
		if colW < 1 {
			colW = 1
		}
		colMax[c] = colW
		totalNaturalWidth += colW + 3
	}

	// If the entire table fits naturally within W on a single line, do not stretch or fold.
	if totalNaturalWidth <= W {
		lp := LinePlacement{
			StartIndex: 0,
			Columns:    make([]ColumnPlacement, totalCols),
		}
		for c := 0; c < totalCols; c++ {
			lp.Columns[c] = ColumnPlacement{ColIdx: c, Width: colMax[c]}
		}
		return []LinePlacement{lp}
	}

	var lines []LinePlacement
	colIdx := 0

	for lineIdx := 0; colIdx < totalCols; lineIdx++ {
		startIdx := lineIdx
		maxStart := W / 2
		if maxStart < 1 {
			maxStart = 1
		}
		if startIdx > maxStart {
			startIdx = maxStart
		}

		availWidth := W - startIdx - 1
		if availWidth < 10 {
			availWidth = 10
		}
		halfLine := availWidth / 2

		lp := LinePlacement{
			StartIndex: startIdx,
		}

		currentX := startIdx + 1

		for colIdx < totalCols {
			c := colIdx
			isLong := colMax[c] > halfLine

			if isLong {
				if len(lp.Columns) == 0 {
					colW := W - currentX - 3
					if colW < halfLine {
						colW = halfLine
					}
					lp.Columns = append(lp.Columns, ColumnPlacement{ColIdx: c, Width: colW})
					currentX += colW + 3
					colIdx++
					break
				} else {
					rem := W - currentX - 3
					if rem >= halfLine {
						lp.Columns = append(lp.Columns, ColumnPlacement{ColIdx: c, Width: rem})
						currentX += rem + 3
						colIdx++
						break
					} else {
						break
					}
				}
			} else {
				colW := colMax[c]
				minW := utf8.RuneCountInString(headers[c])
				if colW < minW {
					colW = minW
				}
				if currentX+colW+3 <= W || len(lp.Columns) == 0 {
					if currentX+colW+3 > W {
						colW = W - currentX - 3
						if colW < 5 {
							colW = 5
						}
					}
					lp.Columns = append(lp.Columns, ColumnPlacement{ColIdx: c, Width: colW})
					currentX += colW + 3
					colIdx++
				} else {
					break
				}
			}
		}

		if len(lp.Columns) > 0 {
			used := startIdx + 1
			for _, cp := range lp.Columns {
				used += cp.Width + 3
			}
			rem := W - used
			if rem > 0 {
				lp.Columns[len(lp.Columns)-1].Width += rem
			}
			lines = append(lines, lp)
		}
	}

	return lines
}

// StaggeredTableRenderer formats table using single-tier staggered folding rules.
type StaggeredTableRenderer struct {
	Alignments []string
	MaxWidth   int
}

func (s StaggeredTableRenderer) Render(out io.Writer, headers []string, rows [][]string) error {
	if len(headers) == 0 {
		return nil
	}

	w := s.MaxWidth
	if w <= 0 {
		w = getTerminalWidth()
	}
	if w < 40 {
		w = 40
	}

	lines := computeLinePlacements(headers, rows, w)
	if len(lines) == 0 {
		return nil
	}

	var sb strings.Builder

	// Top border of Line 0
	sb.WriteString("┌")
	for i, cp := range lines[0].Columns {
		if i > 0 {
			sb.WriteString("┬")
		}
		sb.WriteString(strings.Repeat("─", cp.Width+2))
	}
	sb.WriteString("┐\n")

	// Header row for Line 0
	sb.WriteString("│")
	for _, cp := range lines[0].Columns {
		align := ""
		if cp.ColIdx < len(s.Alignments) {
			align = s.Alignments[cp.ColIdx]
		}
		sb.WriteString(" ")
		sb.WriteString(formatAlignedCell(headers[cp.ColIdx], cp.Width, align))
		sb.WriteString(" │")
	}
	sb.WriteString("\n")

	// Subsequent header lines (Line 1..k)
	for lIdx := 1; lIdx < len(lines); lIdx++ {
		lp := lines[lIdx]
		sb.WriteString(strings.Repeat("│", lp.StartIndex))
		sb.WriteString("┌")
		for i, cp := range lp.Columns {
			if i > 0 {
				sb.WriteString("┬")
			}
			sb.WriteString(strings.Repeat("─", cp.Width+2))
		}
		sb.WriteString("┤\n")

		sb.WriteString(strings.Repeat("│", lp.StartIndex))
		sb.WriteString("│")
		for _, cp := range lp.Columns {
			align := ""
			if cp.ColIdx < len(s.Alignments) {
				align = s.Alignments[cp.ColIdx]
			}
			sb.WriteString(" ")
			sb.WriteString(formatAlignedCell(headers[cp.ColIdx], cp.Width, align))
			sb.WriteString(" │")
		}
		sb.WriteString("\n")
	}

	// Separator between Header block and Data rows
	if len(lines) == 1 {
		sb.WriteString("├")
		for i, cp := range lines[0].Columns {
			if i > 0 {
				sb.WriteString("┼")
			}
			sb.WriteString(strings.Repeat("─", cp.Width+2))
		}
		sb.WriteString("┤\n")
	} else {
		lastLine := lines[len(lines)-1]
		sb.WriteString("╞")
		for r := 0; r < lastLine.StartIndex; r++ {
			sb.WriteString("╧")
		}
		usedSoFar := 1 + lastLine.StartIndex
		for i, cp := range lastLine.Columns {
			if i > 0 {
				sb.WriteString("╧")
				usedSoFar++
			}
			segLen := cp.Width + 2
			sb.WriteString(strings.Repeat("═", segLen))
			usedSoFar += segLen
		}
		if w > usedSoFar+1 {
			sb.WriteString(strings.Repeat("═", w-1-usedSoFar))
		}
		sb.WriteString("╡\n")
	}

	// Data rows
	for rIdx, row := range rows {
		for lIdx, lp := range lines {
			if lIdx > 0 {
				sb.WriteString(strings.Repeat("│", lp.StartIndex))
				sb.WriteString("┌")
				for i, cp := range lp.Columns {
					if i > 0 {
						sb.WriteString("┬")
					}
					sb.WriteString(strings.Repeat("─", cp.Width+2))
				}
				sb.WriteString("┤\n")
			}

			wrapped := make([][]string, len(lp.Columns))
			maxH := 1
			for cIdx, cp := range lp.Columns {
				val := ""
				if cp.ColIdx < len(row) {
					val = row[cp.ColIdx]
				}
				cellLines := WrapSoft(val, cp.Width)
				wrapped[cIdx] = cellLines
				if len(cellLines) > maxH {
					maxH = len(cellLines)
				}
			}

			for sub := 0; sub < maxH; sub++ {
				if lp.StartIndex > 0 {
					sb.WriteString(strings.Repeat("│", lp.StartIndex))
				}
				sb.WriteString("│")
				for cIdx, cp := range lp.Columns {
					lineText := ""
					if sub < len(wrapped[cIdx]) {
						lineText = wrapped[cIdx][sub]
					}
					align := ""
					if cp.ColIdx < len(s.Alignments) {
						align = s.Alignments[cp.ColIdx]
					}
					sb.WriteString(" ")
					sb.WriteString(formatAlignedCell(lineText, cp.Width, align))
					sb.WriteString(" │")
				}
				sb.WriteString("\n")
			}
		}

		if rIdx < len(rows)-1 {
			if len(lines) == 1 {
				sb.WriteString("├")
				for i, cp := range lines[0].Columns {
					if i > 0 {
						sb.WriteString("┼")
					}
					sb.WriteString(strings.Repeat("─", cp.Width+2))
				}
				sb.WriteString("┤\n")
			} else {
				lastLine := lines[len(lines)-1]
				sb.WriteString("├")
				for r := 0; r < lastLine.StartIndex; r++ {
					sb.WriteString("┴")
				}
				used := 1 + lastLine.StartIndex
				sb.WriteString(strings.Repeat("─", w-1-used))
				sb.WriteString("┤\n")
			}
		} else {
			if len(lines) == 1 {
				sb.WriteString("└")
				for i, cp := range lines[0].Columns {
					if i > 0 {
						sb.WriteString("┴")
					}
					sb.WriteString(strings.Repeat("─", cp.Width+2))
				}
				sb.WriteString("┘\n")
			} else {
				lastLine := lines[len(lines)-1]
				sb.WriteString("└")
				for r := 0; r < lastLine.StartIndex; r++ {
					sb.WriteString("┴")
				}
				used := 1 + lastLine.StartIndex
				sb.WriteString(strings.Repeat("─", w-1-used))
				sb.WriteString("┘\n")
			}
		}
	}

	_, err := io.WriteString(out, sb.String())
	return err
}

func NewListCmd() *cobra.Command {
	const whereFlagName = "where"
	var whereFlags []string
	var formatFlag string
	var nowrapFlag bool
	var expandedFlag bool
	var maxWidthFlag int
	var headFlag int
	var tailFlag int
	var indexFlag string
	var offsetFlag int
	var limitFlag int
	var includeRowNumberFlag bool
	var includeMetadataFlag bool

	cmd := &cobra.Command{
		Use:   "list <file>",
		Short: "List matching rows from table",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			if expandedFlag {
				formatFlag = "expanded"
			}
			formatFlag = strings.ToLower(strings.TrimSpace(formatFlag))
			switch formatFlag {
			case "auto", "json", "csv", "tsv", "md", "markdown", "pretty", "expanded":
				// Valid format
			default:
				return fmt.Errorf("invalid format: %q (must be one of: auto, json, csv, tsv, md, markdown, pretty, expanded)", formatFlag)
			}

			var posOpts core.PositionalOptions
			if cmd.Flags().Changed("head") {
				posOpts.Head = &headFlag
			}
			if cmd.Flags().Changed("tail") {
				posOpts.Tail = &tailFlag
			}
			if cmd.Flags().Changed("index") {
				posOpts.Index = indexFlag
			}
			if cmd.Flags().Changed("offset") {
				posOpts.Offset = offsetFlag
			}
			if cmd.Flags().Changed("limit") {
				posOpts.Limit = &limitFlag
			}
			posOpts.IncludeRowNumber = includeRowNumberFlag

			filters, err := core.ParseFilters(whereFlags)
			if err != nil {
				return err
			}

			cfg, matched, err := core.ScanAndSelect(cmd.Context(), filePath, filters, posOpts)
			if err != nil {
				return err
			}
			headers := cfg.Headers
			if len(headers) == 0 {
				return fmt.Errorf("no gfm table headers found in %s", filePath)
			}

			if formatFlag == "json" {
				if includeMetadataFlag {
					dataSlice := make([]map[string]any, len(matched))
					for i, mr := range matched {
						obj := make(map[string]any)
						if includeRowNumberFlag {
							obj["__row_index__"] = mr.Index
						}
						for colIdx, h := range headers {
							val := ""
							if colIdx < len(mr.Row) {
								val = mr.Row[colIdx]
							}
							obj[h] = val
						}
						dataSlice[i] = obj
					}

					metaResponse := map[string]any{
						"status": "success",
						"meta": map[string]any{
							"etag":              cfg.ETag,
							"total_rows":        cfg.RowCount,
							"returned_rows":     len(matched),
							"offset":            posOpts.Offset,
							"indices_retrieved": matched.Indices(),
						},
						"schema": map[string]any{
							"columns": headers,
						},
						"data": dataSlice,
					}
					encoder := json.NewEncoder(cmd.OutOrStdout())
					encoder.SetEscapeHTML(false)
					encoder.SetIndent("", "  ")
					return encoder.Encode(metaResponse)
				}

				renderer := JSONRenderer{
					IncludeRowIndex: includeRowNumberFlag,
					Matched:         matched,
				}
				return renderer.Render(cmd.OutOrStdout(), headers, matched.Rows())
			}

			if includeMetadataFlag {
				switch formatFlag {
				case "csv":
					fmt.Fprintf(cmd.OutOrStdout(), "# etag: %s, total_rows: %d\n", cfg.ETag, cfg.RowCount)
				case "tsv":
					fmt.Fprintf(cmd.OutOrStdout(), "# etag: %s\ttotal_rows: %d\n", cfg.ETag, cfg.RowCount)
				default:
					fmt.Fprintf(cmd.OutOrStdout(), "ETag: %s | Rows: %d\n\n", cfg.ETag, cfg.RowCount)
				}
			}

			renderHeaders := headers
			renderAlignments := cfg.Alignments
			renderRows := matched.Rows()

			if includeRowNumberFlag {
				renderHeaders = append([]string{"#"}, headers...)
				if len(renderAlignments) > 0 {
					renderAlignments = append([]string{":---"}, renderAlignments...)
				}
				renderRows = make([][]string, len(matched))
				for i, mr := range matched {
					renderRows[i] = append([]string{strconv.Itoa(mr.Index)}, mr.Row...)
				}
			}

			var renderer TableRenderer
			switch formatFlag {
			case "csv":
				renderer = CSVRenderer{Comma: ','}
			case "tsv":
				renderer = CSVRenderer{Comma: '\t'}
			case "md", "markdown":
				renderer = GFMRenderer{Alignments: renderAlignments}
			case "expanded":
				renderer = VerticalCardRenderer{MaxWidth: maxWidthFlag}
			case "pretty":
				if nowrapFlag {
					renderer = PrettyRenderer{Alignments: renderAlignments}
				} else {
					renderer = StaggeredTableRenderer{Alignments: renderAlignments, MaxWidth: maxWidthFlag}
				}
			case "auto":
				if isTerminal(cmd.OutOrStdout()) {
					if nowrapFlag {
						renderer = PrettyRenderer{Alignments: renderAlignments}
					} else {
						renderer = StaggeredTableRenderer{Alignments: renderAlignments, MaxWidth: maxWidthFlag}
					}
				} else {
					renderer = GFMRenderer{Alignments: renderAlignments}
				}
			}

			return renderer.Render(cmd.OutOrStdout(), renderHeaders, renderRows)
		},
	}

	cmd.Flags().StringSliceVar(&whereFlags, whereFlagName, nil, "WHERE conditions (supported operators: ==, !=, like, regex)")
	cmd.Flags().StringVarP(&formatFlag, "format", "o", "auto", "Output format (auto, json, csv, tsv, md, pretty, expanded)")
	cmd.Flags().BoolVar(&nowrapFlag, "nowrap", false, "Disable folding and word-wrapping; force unconstrained single-line grid")
	cmd.Flags().BoolVarP(&expandedFlag, "expanded", "x", false, "Display records in expanded vertical key-value card view")
	cmd.Flags().IntVar(&maxWidthFlag, "max-width", 0, "Explicit maximum line width constraint (defaults to detected terminal width or 100)")
	cmd.Flags().IntVarP(&headFlag, "head", "H", 0, "Extract first N logical data rows from table start")
	cmd.Flags().IntVarP(&tailFlag, "tail", "T", 0, "Extract last N logical data rows from table end")
	cmd.Flags().StringVarP(&indexFlag, "index", "i", "", "Target 0-based logical row index, list (0,2,4), range (2..10, 2:10), or stepped slice (0..20..2)")
	cmd.Flags().IntVar(&offsetFlag, "offset", 0, "Zero-based row offset for pagination")
	cmd.Flags().IntVarP(&limitFlag, "limit", "l", 0, "Maximum number of rows to return")
	cmd.Flags().BoolVar(&includeRowNumberFlag, "include-row-number", false, "Inject 0-based row index (__row_index__ / #) into output")
	cmd.Flags().BoolVarP(&includeMetadataFlag, "include-metadata", "m", false, "Include ETag and summary metadata in output")

	return cmd
}
