package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
)

const (
	timestampFormat    = "2006-01-02 15:04:05"
	defaultSortDir     = "asc"
	descSortDir        = "desc"
	fallbackCreatedAt  = "created_at"
	fallbackInsertedAt = "inserted_at"
	fallbackUpdatedAt  = "updated_at"
	fallbackModifiedAt = "modified_at"
)

// AuditConfig represents custom timestamp column mapping
type AuditConfig struct {
	Insert string `yaml:"insert,omitempty"`
	Edit   string `yaml:"edit,omitempty"`
}

// AutoIncrementConfig represents auto-increment column mapping and step
type AutoIncrementConfig struct {
	Column string `yaml:"column,omitempty"`
	Step   int    `yaml:"step,omitempty"`
}

func (a *AutoIncrementConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		col, step := parseAutoIncrementString(s)
		a.Column = col
		a.Step = step
		return nil
	}
	type rawAutoInc AutoIncrementConfig
	var raw rawAutoInc
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*a = AutoIncrementConfig(raw)
	return nil
}

func parseAutoIncrementString(s string) (string, int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0
	}
	parts := strings.SplitN(s, ":", 2)
	col := strings.TrimSpace(parts[0])
	step := 1
	if len(parts) > 1 {
		if parsedStep, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && parsedStep > 0 {
			step = parsedStep
		}
	}
	return col, step
}

// RowliteConfig represents configurations nested under rowlite: namespace
type RowliteConfig struct {
	Unique        []string            `yaml:"unique,flow"`
	Sort          string              `yaml:"sort,omitempty"`
	Audit         AuditConfig         `yaml:"audit,omitempty"`
	JournalLog    string              `yaml:"journal_log,omitempty"`
	AutoIncrement AutoIncrementConfig `yaml:"autoincrement,omitempty"`
}

// rawFrontmatter represents the raw YAML structure parsed from frontmatter
type rawFrontmatter struct {
	Rowlite RowliteConfig `yaml:"rowlite"`
}

type SortRule struct {
	ColIdx int
	Dir    string
}

// TableConfig represents work-optimized runtime configurations and metadata
type TableConfig struct {
	Headers          []string
	Alignments       []string
	UniqueIndices    []int
	AuditIndices     []int
	SortRules        []SortRule
	JournalLog       string
	Unique           []string
	Sort             string
	Audit            AuditConfig
	AutoIncrement    AutoIncrementConfig
	AutoIncrementIdx int
	MaxAutoIncVal    int
	RowCount         int
	IsSortColUnique  bool
	ETag             string
}

func (tc TableConfig) NumColumns() int {
	return len(tc.Headers)
}

func (tc TableConfig) IsSorted() bool {
	return len(tc.SortRules) > 0
}

// IsSortedBy checks if the primary sort rule matches the given column name (case-insensitive) and direction (e.g. "asc" or "desc").
// If sortingDir is empty, defaultSortDir ("asc") is used.
func (tc TableConfig) IsSortedBy(colName string, sortingDir string) bool {
	if len(tc.SortRules) == 0 {
		return false
	}
	dir := strings.ToLower(strings.TrimSpace(sortingDir))
	if dir == "" {
		dir = defaultSortDir
	}
	colIdx, _, err := ResolveColumn(tc.Headers, colName)
	if err != nil {
		return false
	}
	return tc.SortRules[0].ColIdx == colIdx && tc.SortRules[0].Dir == dir
}


// MatchedRow represents a single table row along with its 0-based index.
type MatchedRow struct {
	Index int      // 0-based row index in the original table rows slice
	Row   []string // Row cell values
}

// MatchedRows represents a collection of matched rows.
type MatchedRows []MatchedRow

// Rows extracts and returns a 2D slice of strings for rendering and formatting.
func (m MatchedRows) Rows() [][]string {
	res := make([][]string, len(m))
	for i, mr := range m {
		res[i] = mr.Row
	}
	return res
}

// Indices extracts and returns the slice of original 0-based row indices.
func (m MatchedRows) Indices() []int {
	res := make([]int, len(m))
	for i, mr := range m {
		res[i] = mr.Index
	}
	return res
}

// Filter represents a single WHERE filter condition
type Filter struct {
	Column   string
	Operator string
	Value    string
}

// ParseFilter parses a WHERE condition like "priority==high"
func ParseFilter(expr string) (Filter, error) {
	var col, op, val string
	if strings.Contains(expr, "==") {
		parts := strings.SplitN(expr, "==", 2)
		col, op, val = strings.TrimSpace(parts[0]), "==", strings.TrimSpace(parts[1])
	} else if strings.Contains(expr, "!=") {
		parts := strings.SplitN(expr, "!=", 2)
		col, op, val = strings.TrimSpace(parts[0]), "!=", strings.TrimSpace(parts[1])
	} else if strings.Contains(expr, " like ") {
		parts := strings.SplitN(expr, " like ", 2)
		col, op, val = strings.TrimSpace(parts[0]), "like", strings.TrimSpace(parts[1])
	} else if strings.Contains(expr, " regex ") {
		parts := strings.SplitN(expr, " regex ", 2)
		col, op, val = strings.TrimSpace(parts[0]), "regex", strings.TrimSpace(parts[1])
	} else {
		// Fallbacks without spaces around operators
		if idx := strings.Index(expr, "like"); idx > 0 {
			col, op, val = strings.TrimSpace(expr[:idx]), "like", strings.TrimSpace(expr[idx+4:])
		} else if idx := strings.Index(expr, "regex"); idx > 0 {
			col, op, val = strings.TrimSpace(expr[:idx]), "regex", strings.TrimSpace(expr[idx+5:])
		} else {
			return Filter{}, fmt.Errorf("invalid filter expression: %s (must contain ==, !=, like, or regex)", expr)
		}
	}
	return Filter{Column: col, Operator: op, Value: val}, nil
}

// ParseFilters parses multiple WHERE conditions into a slice of Filter objects.
func ParseFilters(whereClauses []string) ([]Filter, error) {
	if len(whereClauses) == 0 {
		return nil, nil
	}
	filters := make([]Filter, 0, len(whereClauses))
	for _, expr := range whereClauses {
		flt, err := ParseFilter(expr)
		if err != nil {
			return nil, err
		}
		filters = append(filters, flt)
	}
	return filters, nil
}

// EvaluateFilter checks if a row cell matches the filter
func EvaluateFilter(headers []string, row []string, filter Filter) (bool, error) {
	colIdx, _, err := ResolveColumn(headers, filter.Column)
	if err != nil {
		return false, err
	}
	if colIdx >= len(row) {
		return false, nil
	}

	cellValue := row[colIdx]
	switch filter.Operator {
	case "==":
		return cellValue == filter.Value, nil
	case "!=":
		return cellValue != filter.Value, nil
	case "like":
		return strings.Contains(strings.ToLower(cellValue), strings.ToLower(filter.Value)), nil
	case "regex":
		re, err := regexp.Compile(filter.Value)
		if err != nil {
			return false, fmt.Errorf("invalid regex %q: %w", filter.Value, err)
		}
		return re.MatchString(cellValue), nil
	default:
		return false, fmt.Errorf("unknown operator %q", filter.Operator)
	}
}

// MatchesRow evaluates whether a single row satisfies all provided filters (AND logic).
func MatchesRow(headers []string, row []string, filters []Filter) (bool, error) {
	for _, flt := range filters {
		m, err := EvaluateFilter(headers, row, flt)
		if err != nil {
			return false, err
		}
		if !m {
			return false, nil
		}
	}
	return true, nil
}

// SelectRows scans rows and returns all rows that match the filters along with their 0-based table indices.
func SelectRows(headers []string, rows [][]string, filters []Filter) (MatchedRows, error) {
	var matches MatchedRows
	for i, row := range rows {
		matched, err := MatchesRow(headers, row, filters)
		if err != nil {
			return nil, err
		}
		if matched {
			matches = append(matches, MatchedRow{
				Index: i,
				Row:   row,
			})
		}
	}
	return matches, nil
}

type ContentMode int

const (
	ContentModeAll  ContentMode = iota // Read all rows of the table
	ContentModeLast                    // Read only the last row of the table
	ContentModeNone                    // Read no rows of the table
	ContentModeAuto                    // Decide automatically based on unique/sort rules
)

// ParseFrontmatterAndContent parses the frontmatter and table from a file according to the ContentMode
func ParseFrontmatterAndContent(ctx context.Context, filePath string, mode ContentMode) (TableConfig, [][]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return TableConfig{}, nil, err
	}
	defer file.Close()

	var raw rawFrontmatter
	restBytes, err := frontmatter.Parse(file, &raw)
	if err != nil {
		file.Seek(0, 0)
		restBytes, err = io.ReadAll(file)
		if err != nil {
			return TableConfig{}, nil, err
		}
		raw = rawFrontmatter{}
	}

	// Calculate frontmatter line offset to ensure 1-based file line numbers
	allBytes, _ := os.ReadFile(filePath)
	totalLines := countLines(allBytes)
	restLines := countLines(restBytes)
	fmLineCount := 0
	if totalLines >= restLines {
		fmLineCount = totalLines - restLines
	}

	if IsDebug(ctx) {
		fmt.Fprintf(os.Stderr, "DEBUG [table scan]: reading %s (frontmatter lines: %d, total lines: %d)\n", filePath, fmLineCount, totalLines)
	}

	scanner := bufio.NewScanner(bytes.NewReader(restBytes))
	var headers []string
	var alignments []string
	headerLineNum := 0
	lineInRest := 0
	tableStarted := false
	var lastTableErr error

	// Find headers and alignments
	for scanner.Scan() {
		lineInRest++
		lineNum := fmLineCount + lineInRest
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if isTableRow(trimmed) {
			cells, err := SplitGFMLine(line)
			if err == nil {
				if !tableStarted {
					headers = cells
					headerLineNum = lineNum
					tableStarted = true
					if IsDebug(ctx) {
						fmt.Fprintf(os.Stderr, "DEBUG [table scan]: candidate table header found at line %d: %v\n", headerLineNum, headers)
					}
					continue
				}
				if len(alignments) == 0 {
					if lineNum == headerLineNum+1 {
						sepErr := ValidateSeparatorLine(cells)
						if sepErr != nil {
							if IsDebug(ctx) {
								fmt.Fprintf(os.Stderr, "DEBUG [table scan]: separator validation failed at line %d: %v\n", lineNum, sepErr)
							}
							lastTableErr = fmt.Errorf("invalid separator row at line %d: %w", lineNum, sepErr)
						} else if len(cells) != len(headers) {
							if IsDebug(ctx) {
								fmt.Fprintf(os.Stderr, "DEBUG [table scan]: column count mismatch at line %d (header has %d cols, separator has %d cols)\n", lineNum, len(headers), len(cells))
							}
							lastTableErr = fmt.Errorf("separator row column count (%d) at line %d mismatch with header row column count (%d) at line %d", len(cells), lineNum, len(headers), headerLineNum)
						} else {
							alignments = cells
							if IsDebug(ctx) {
								fmt.Fprintf(os.Stderr, "DEBUG [table scan]: valid table separator row found at line %d: %v\n", lineNum, alignments)
							}
							break
						}
					} else {
						break
					}
				}
			}
		}
	}

	if len(headers) == 0 || len(alignments) == 0 {
		if lastTableErr != nil {
			return TableConfig{}, nil, fmt.Errorf("gfm table error in %s: %w", filePath, lastTableErr)
		}
		if tableStarted {
			return TableConfig{}, nil, fmt.Errorf("gfm table error in %s: candidate header found at line %d, but no valid separator row followed", filePath, headerLineNum)
		}
		return TableConfig{}, nil, fmt.Errorf("gfm table headers and alignments not found in %s", filePath)
	}

	cfg := TableConfig{
		Headers:       headers,
		Alignments:    alignments,
		JournalLog:    raw.Rowlite.JournalLog,
		Unique:        raw.Rowlite.Unique,
		Sort:          raw.Rowlite.Sort,
		Audit:         raw.Rowlite.Audit,
		AutoIncrement: raw.Rowlite.AutoIncrement,
	}
	if cfg.AutoIncrement.Column != "" && cfg.AutoIncrement.Step <= 0 {
		cfg.AutoIncrement.Step = 1
	}

	// Resolve derived Sort fields
	if cfg.Sort != "" {
		rulesParts := strings.Split(cfg.Sort, ",")
		for _, part := range rulesParts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			parts := strings.SplitN(part, ":", 2)
			sortColName := strings.TrimSpace(parts[0])
			dir := defaultSortDir
			if len(parts) > 1 {
				dir = strings.ToLower(strings.TrimSpace(parts[1]))
			}
			colIdx, _, err := ResolveColumn(headers, sortColName)
			if err != nil {
				return TableConfig{}, nil, fmt.Errorf("sort column %w", err)
			}
			cfg.SortRules = append(cfg.SortRules, SortRule{ColIdx: colIdx, Dir: dir})
		}
	}

	// Resolve derived Unique column indices
	for _, u := range cfg.Unique {
		colIdx, _, err := ResolveColumn(headers, u)
		if err == nil {
			cfg.UniqueIndices = append(cfg.UniqueIndices, colIdx)
		}
	}

	// Resolve isSortColUnique: true if the sequence of unique columns forms a prefix of the sort columns in the same order
	cfg.IsSortColUnique = false
	if len(cfg.UniqueIndices) > 0 && len(cfg.UniqueIndices) <= len(cfg.SortRules) {
		prefixMatch := true
		for i, uIdx := range cfg.UniqueIndices {
			if cfg.SortRules[i].ColIdx != uIdx {
				prefixMatch = false
				break
			}
		}
		cfg.IsSortColUnique = prefixMatch
	}

	// Resolve derived Audit column indices
	if cfg.Audit.Insert != "" {
		if idx, _, err := ResolveColumn(headers, cfg.Audit.Insert); err == nil {
			cfg.AuditIndices = append(cfg.AuditIndices, idx)
		}
	} else {
		if idx, _, err := ResolveColumn(headers, fallbackCreatedAt); err == nil {
			cfg.AuditIndices = append(cfg.AuditIndices, idx)
		} else if idx, _, err := ResolveColumn(headers, fallbackInsertedAt); err == nil {
			cfg.AuditIndices = append(cfg.AuditIndices, idx)
		}
	}

	if cfg.Audit.Edit != "" {
		if idx, _, err := ResolveColumn(headers, cfg.Audit.Edit); err == nil {
			cfg.AuditIndices = append(cfg.AuditIndices, idx)
		}
	} else {
		if idx, _, err := ResolveColumn(headers, fallbackUpdatedAt); err == nil {
			cfg.AuditIndices = append(cfg.AuditIndices, idx)
		} else if idx, _, err := ResolveColumn(headers, fallbackModifiedAt); err == nil {
			cfg.AuditIndices = append(cfg.AuditIndices, idx)
		}
	}

	// Resolve derived AutoIncrement column index
	cfg.AutoIncrementIdx = -1
	if cfg.AutoIncrement.Column != "" {
		if idx, _, err := ResolveColumn(headers, cfg.AutoIncrement.Column); err == nil {
			cfg.AutoIncrementIdx = idx
		}
	}

	// Determine actual content mode to use
	actualMode := mode
	if mode == ContentModeAuto {
		if len(cfg.UniqueIndices) > 0 && !cfg.IsSortColUnique {
			actualMode = ContentModeAll
		} else if cfg.IsSorted() {
			actualMode = ContentModeLast
		} else {
			actualMode = ContentModeNone
		}
	}

	var rows [][]string
	var lastRow []string
	rowCount := 0
	maxAutoIncVal := 0

	// Scan remaining content for table rows with strict validation
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if isTableRow(trimmed) {
			cells, err := SplitGFMLine(line)
			if err != nil {
				return TableConfig{}, nil, fmt.Errorf("malformed table row: %w", err)
			}
			if len(cells) != len(headers) {
				return TableConfig{}, nil, fmt.Errorf("inconsistent column count: row has %d columns, expected %d", len(cells), len(headers))
			}
			rowCount++
			if cfg.AutoIncrementIdx >= 0 && cfg.AutoIncrementIdx < len(cells) {
				if v, err := strconv.Atoi(strings.TrimSpace(cells[cfg.AutoIncrementIdx])); err == nil && v > maxAutoIncVal {
					maxAutoIncVal = v
				}
			}
			switch actualMode {
			case ContentModeAll:
				rows = append(rows, cells)
			case ContentModeLast:
				lastRow = cells
			}
		} else {
			// Once we hit a non-table line, GFM table ends
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return TableConfig{}, nil, err
	}

	cfg.RowCount = rowCount
	cfg.MaxAutoIncVal = maxAutoIncVal
	cfg.ETag = ComputeETag(allBytes)

	if actualMode == ContentModeLast && lastRow != nil {
		rows = [][]string{lastRow}
	}

	if IsDebug(ctx) {
		if jsonBytes, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			fmt.Fprintf(os.Stderr, "DEBUG TableConfig:\n%s\n", string(jsonBytes))
		} else {
			fmt.Fprintf(os.Stderr, "DEBUG TableConfig: %+v\n", cfg)
		}
	}

	return cfg, rows, nil
}

// SerializeConfig formats frontmatter config as valid YAML frontmatter under rowlite: root
func SerializeConfig(cfg TableConfig) string {
	if len(cfg.Unique) == 0 && cfg.Sort == "" && cfg.Audit.Insert == "" && cfg.Audit.Edit == "" && cfg.JournalLog == "" && cfg.AutoIncrement.Column == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("rowlite:\n")
	if len(cfg.Unique) > 0 {
		sb.WriteString("  unique: [")
		for i, u := range cfg.Unique {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(u)
		}
		sb.WriteString("]\n")
	}
	if cfg.Sort != "" {
		sb.WriteString("  sort: ")
		sb.WriteString(cfg.Sort)
		sb.WriteByte('\n')
	}
	if cfg.Audit.Insert != "" || cfg.Audit.Edit != "" {
		sb.WriteString("  audit:\n")
		if cfg.Audit.Insert != "" {
			sb.WriteString("    insert: \"")
			sb.WriteString(cfg.Audit.Insert)
			sb.WriteString("\"\n")
		}
		if cfg.Audit.Edit != "" {
			sb.WriteString("    edit: \"")
			sb.WriteString(cfg.Audit.Edit)
			sb.WriteString("\"\n")
		}
	}
	if cfg.JournalLog != "" {
		sb.WriteString("  journal_log: ")
		sb.WriteString(cfg.JournalLog)
		sb.WriteByte('\n')
	}
	if cfg.AutoIncrement.Column != "" {
		step := cfg.AutoIncrement.Step
		if step <= 0 {
			step = 1
		}
		sb.WriteString("  autoincrement:\n")
		sb.WriteString("    column: \"")
		sb.WriteString(cfg.AutoIncrement.Column)
		sb.WriteString("\"\n    step: ")
		sb.WriteString(strconv.Itoa(step))
		sb.WriteByte('\n')
	}
	sb.WriteString("---\n")
	return sb.String()
}

// CompareCells evaluates sorting ordering between two strings
func CompareCells(a, b string) int {
	fa, errA := strconv.ParseFloat(a, 64)
	fb, errB := strconv.ParseFloat(b, 64)
	if errA == nil && errB == nil {
		if fa < fb {
			return -1
		}
		if fa > fb {
			return 1
		}
		return 0
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// CompareRows compares two rows based on SortRules.
// It returns:
//   -1 if rowA < rowB
//    1 if rowA > rowB
//    0 if rowA == rowB
func CompareRows(rowA, rowB []string, rules []SortRule) int {
	for _, rule := range rules {
		if rule.ColIdx >= len(rowA) || rule.ColIdx >= len(rowB) {
			continue
		}
		valA := rowA[rule.ColIdx]
		valB := rowB[rule.ColIdx]
		cmp := CompareCells(valA, valB)
		if cmp != 0 {
			if rule.Dir == descSortDir {
				return -cmp
			}
			return cmp
		}
	}
	return 0
}

// SortRows stable-sorts rows in-place based on sort rules (e.g. "id:asc,priority:desc")
func SortRows(headers []string, rows [][]string, sortRule string) error {
	if sortRule == "" {
		return nil
	}

	var rules []SortRule
	rulesParts := strings.Split(sortRule, ",")
	for _, part := range rulesParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts := strings.SplitN(part, ":", 2)
		colName := strings.TrimSpace(parts[0])
		dir := defaultSortDir
		if len(parts) > 1 {
			dir = strings.ToLower(strings.TrimSpace(parts[1]))
		}
		colIdx, _, err := ResolveColumn(headers, colName)
		if err != nil {
			return fmt.Errorf("sort column %w", err)
		}
		rules = append(rules, SortRule{ColIdx: colIdx, Dir: dir})
	}

	sort.SliceStable(rows, func(i, j int) bool {
		return CompareRows(rows[i], rows[j], rules) < 0
	})

	return nil
}

// ComputeRowKey derives the row key from unique columns or 1-based row index
func ComputeRowKey(cfg TableConfig, rowIdx int, rowMap map[string]string) string {
	if len(cfg.Unique) > 0 {
		var parts []string
		for _, uCol := range cfg.Unique {
			for k, v := range rowMap {
				if Normalize(k) == Normalize(uCol) {
					parts = append(parts, v)
					break
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ":")
		}
	}
	return strconv.Itoa(rowIdx)
}

func migrateJournalIfNeeded(journalPath string) error {
	content, err := os.ReadFile(journalPath)
	if err != nil || len(content) == 0 {
		return nil
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return nil
	}

	headerLine := strings.TrimSpace(lines[0])
	if !isTableRow(headerLine) {
		return nil
	}

	headers, err := SplitGFMLine(headerLine)
	if err != nil {
		return nil
	}

	// Check if header has 3 columns: timestamp, operation, details
	if len(headers) == 3 && Normalize(headers[0]) == "timestamp" && Normalize(headers[1]) == "operation" && Normalize(headers[2]) == "details" {
		var newLines []string
		newLines = append(newLines, "| timestamp | operation | row_key | details |")
		newLines = append(newLines, "| --- | --- | --- | --- |")

		rowIdx := 0
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "" || !isTableRow(line) {
				continue
			}
			cells, err := SplitGFMLine(line)
			if err != nil || len(cells) < 3 {
				continue
			}
			if IsSeparatorLine(cells) {
				continue
			}
			rowIdx++
			ts := cells[0]
			op := cells[1]
			details := cells[2]

			derivedKey := strconv.Itoa(rowIdx)
			var payload map[string]interface{}
			if jsonErr := json.Unmarshal([]byte(details), &payload); jsonErr == nil {
				for _, k := range []string{"id", "tag", "key", "code", "slug"} {
					if val, ok := payload[k]; ok && val != nil {
						derivedKey = fmt.Sprintf("%v", val)
						break
					}
				}
			}

			newLines = append(newLines, FormatGFMLine([]string{ts, op, derivedKey, details}))
		}
		newLines = append(newLines, "")
		_ = os.WriteFile(journalPath, []byte(strings.Join(newLines, "\n")), 0644)
	}
	return nil
}

// WriteJournalRecord logs operation details into an append-only journal file with 4 columns
func WriteJournalRecord(ctx context.Context, journalPath string, op string, rowKey string, payload map[string]string) error {
	_ = migrateJournalIfNeeded(journalPath)

	timestamp := time.Now().Format(timestampFormat)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(journalPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err == nil && stat.Size() == 0 {
		file.WriteString("| timestamp | operation | row_key | details |\n| --- | --- | --- | --- |\n")
	}

	logRow := []string{timestamp, op, rowKey, string(payloadBytes)}
	_, err = file.WriteString(FormatGFMLine(logRow))
	if err == nil {
		_, err = file.WriteString("\n")
	}
	return err
}

// IsInsertAuditColumn checks if a column is an auto-filled creation timestamp column
func IsInsertAuditColumn(colName string, cfg TableConfig) bool {
	if cfg.Audit.Insert != "" && Normalize(colName) == Normalize(cfg.Audit.Insert) {
		return true
	}
	if cfg.Audit.Insert == "" && (Normalize(colName) == Normalize(fallbackCreatedAt) || Normalize(colName) == Normalize(fallbackInsertedAt)) {
		return true
	}
	return false
}

// IsEditAuditColumn checks if a column is an auto-filled modification timestamp column
func IsEditAuditColumn(colName string, cfg TableConfig) bool {
	if cfg.Audit.Edit != "" && Normalize(colName) == Normalize(cfg.Audit.Edit) {
		return true
	}
	if cfg.Audit.Edit == "" && (Normalize(colName) == Normalize(fallbackUpdatedAt) || Normalize(colName) == Normalize(fallbackModifiedAt)) {
		return true
	}
	return false
}

// IsAuditColumn checks if a column is an auto-filled audit column (explicit or zero-config fallback)
func IsAuditColumn(colName string, cfg TableConfig) bool {
	return IsInsertAuditColumn(colName, cfg) || IsEditAuditColumn(colName, cfg)
}

// InitTable writes or updates a GFM table with YAML frontmatter configuration
func InitTable(ctx context.Context, filePath string, columns []string, cfg TableConfig) error {
	if len(columns) == 0 {
		return fmt.Errorf("columns list cannot be empty")
	}

	for _, u := range cfg.Unique {
		_, _, err := ResolveColumn(columns, u)
		if err != nil {
			return fmt.Errorf("unique column %w", err)
		}
	}

	if cfg.Sort != "" {
		parts := strings.SplitN(cfg.Sort, ":", 2)
		sortCol := strings.TrimSpace(parts[0])
		_, _, err := ResolveColumn(columns, sortCol)
		if err != nil {
			return fmt.Errorf("sort column %w", err)
		}
	}

	if cfg.AutoIncrement.Column != "" {
		_, _, err := ResolveColumn(columns, cfg.AutoIncrement.Column)
		if err != nil {
			return fmt.Errorf("autoincrement column %w", err)
		}
	}

	// Build finalColumns by appending auto-generated audit columns to user columns list if not already present
	var finalColumns []string
	finalColumns = append(finalColumns, columns...)

	if cfg.Audit.Insert != "" {
		found := false
		for _, c := range finalColumns {
			if c == cfg.Audit.Insert {
				found = true
				break
			}
		}
		if !found {
			finalColumns = append(finalColumns, cfg.Audit.Insert)
		}
	}

	if cfg.Audit.Edit != "" {
		found := false
		for _, c := range finalColumns {
			if c == cfg.Audit.Edit {
				found = true
				break
			}
		}
		if !found {
			finalColumns = append(finalColumns, cfg.Audit.Edit)
		}
	}

	var existingRows [][]string
	var oldHeaders []string
	var alignments []string
	if _, err := os.Stat(filePath); err == nil {
		cfgParsed, rows, parseErr := ParseFrontmatterAndContent(ctx, filePath, ContentModeAll)
		if parseErr == nil {
			existingRows = rows
			oldHeaders = cfgParsed.Headers
			alignments = cfgParsed.Alignments
		}
	}

	if len(alignments) == 0 {
		alignments = make([]string, len(finalColumns))
		for i := range alignments {
			alignments[i] = "---"
		}
	} else if len(alignments) != len(finalColumns) {
		newAlignments := make([]string, len(finalColumns))
		for i := range newAlignments {
			if i < len(alignments) {
				newAlignments[i] = alignments[i]
			} else {
				newAlignments[i] = "---"
			}
		}
		alignments = newAlignments
	}

	cfg.Headers = finalColumns
	cfg.Alignments = alignments
	frontmatterStr := SerializeConfig(cfg)

	tempPath := filePath + ".tmp"
	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempPath)
	}()

	writer := bufio.NewWriter(tempFile)
	if frontmatterStr != "" {
		_, err = writer.WriteString(frontmatterStr)
		if err != nil {
			return err
		}
	}

	_, err = writer.WriteString(FormatGFMLine(finalColumns))
	if err == nil {
		_, err = writer.WriteString("\n")
	}
	if err != nil {
		return err
	}

	_, err = writer.WriteString(FormatGFMLine(alignments))
	if err == nil {
		_, err = writer.WriteString("\n")
	}
	if err != nil {
		return err
	}

	var mappedRows [][]string
	for _, row := range existingRows {
		newRow := make([]string, len(finalColumns))
		for i, col := range finalColumns {
			oldIdx := -1
			for j, oh := range oldHeaders {
				if oh == col {
					oldIdx = j
					break
				}
			}
			if oldIdx != -1 && oldIdx < len(row) {
				newRow[i] = row[oldIdx]
			} else {
				newRow[i] = ""
			}
		}
		mappedRows = append(mappedRows, newRow)
	}

	if cfg.Sort != "" {
		_ = SortRows(finalColumns, mappedRows, cfg.Sort)
	}

	for _, row := range mappedRows {
		_, err = writer.WriteString(FormatGFMLine(row))
		if err == nil {
			_, err = writer.WriteString("\n")
		}
		if err != nil {
			return err
		}
	}

	err = writer.Flush()
	if err != nil {
		return err
	}
	tempFile.Close()

	return os.Rename(tempPath, filePath)
}

// ParseInputData parses a string as JSON or CSV into a list of row maps based on headers
func ParseInputData(cfg TableConfig, input string) ([]map[string]string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, fmt.Errorf("empty input data")
	}

	// Filter out auto-generated audit columns from the user positional inputs list
	var userHeaders []string
	for _, h := range cfg.Headers {
		if !IsAuditColumn(h, cfg) {
			userHeaders = append(userHeaders, h)
		}
	}

	// Check if JSON object
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		var row map[string]string
		err := json.Unmarshal([]byte(trimmed), &row)
		if err != nil {
			return nil, fmt.Errorf("invalid row JSON: %w", err)
		}
		return []map[string]string{row}, nil
	}

	// Check if JSON array
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		var rows []map[string]string
		err := json.Unmarshal([]byte(trimmed), &rows)
		if err != nil {
			return nil, fmt.Errorf("invalid rows JSON array: %w", err)
		}
		return rows, nil
	}

	// Treat as CSV
	reader := csv.NewReader(strings.NewReader(trimmed))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invalid CSV content: %w", err)
	}

	var rows []map[string]string
	for recordIdx, record := range records {
		if len(record) != len(userHeaders) {
			return nil, fmt.Errorf("csv row %d has inconsistent column count: got %d, expected %d", recordIdx+1, len(record), len(userHeaders))
		}
		rowMap := make(map[string]string)
		for i, h := range userHeaders {
			rowMap[h] = record[i]
		}
		rows = append(rows, rowMap)
	}

	return rows, nil
}

// CheckUniqueness validates that the row does not violate uniqueness rules
func CheckUniqueness(
	row []string,
	cfg TableConfig,
	existingRows [][]string,
	validatedRows [][]string,
) error {
	for _, colIdx := range cfg.UniqueIndices {
		if colIdx >= len(row) {
			continue
		}
		newVal := row[colIdx]

		// Check against existing rows
		for _, r := range existingRows {
			if colIdx < len(r) {
				existingVal := r[colIdx]
				if existingVal == newVal {
					return fmt.Errorf("duplicate key error: value %q for column %q already exists", newVal, cfg.Headers[colIdx])
				}
			}
		}

		// Check against previously validated rows in this batch
		for _, r := range validatedRows {
			if colIdx < len(r) {
				batchVal := r[colIdx]
				if batchVal == newVal {
					return fmt.Errorf("duplicate key error: value %q for column %q already exists in append batch", newVal, cfg.Headers[colIdx])
				}
			}
		}
	}
	return nil
}

// AppendRows appends multiple rows validating uniqueness, sorting, auditing, and change logging
func AppendRows(
	ctx context.Context,
	filePath string,
	cfg TableConfig,
	existingRows [][]string,
	newRows []map[string]string,
) error {
	return AppendRowsWithOptions(ctx, filePath, cfg, existingRows, newRows, "")
}

// AppendRowsWithOptions appends multiple rows validating precondition ETag, uniqueness, sorting, auditing, and change logging
func AppendRowsWithOptions(
	ctx context.Context,
	filePath string,
	cfg TableConfig,
	existingRows [][]string,
	newRows []map[string]string,
	ifMatch string,
) error {
	if err := VerifyETag(ifMatch, cfg.ETag); err != nil {
		return err
	}

	if len(newRows) == 0 {
		return nil
	}

	timestamp := time.Now().Format(timestampFormat)

	currentMax := cfg.MaxAutoIncVal
	var autoIncColName string
	if cfg.AutoIncrementIdx >= 0 && cfg.AutoIncrementIdx < len(cfg.Headers) {
		autoIncColName = cfg.Headers[cfg.AutoIncrementIdx]
	}

	// Pre-process and validate all new rows
	var validatedRows [][]string
	var resolvedRows []map[string]string
	for _, rowMap := range newRows {
		if len(rowMap) == 0 {
			return fmt.Errorf("append error: row data is empty")
		}

		// Map keys in rowMap (user inputs) to exact GFM headers using case-insensitive/normalized lookup
		resolvedMap := make(map[string]string)
		for k, v := range rowMap {
			_, resolvedName, err := ResolveColumn(cfg.Headers, k)
			if err != nil {
				return fmt.Errorf("append error: %w", err)
			}
			resolvedMap[resolvedName] = v
		}

		// Fill auto-increment column if empty or update currentMax if explicit value is passed
		if cfg.AutoIncrementIdx >= 0 {
			if resolvedMap[autoIncColName] == "" {
				step := cfg.AutoIncrement.Step
				if step <= 0 {
					step = 1
				}
				nextVal := currentMax + step
				resolvedMap[autoIncColName] = strconv.Itoa(nextVal)
				currentMax = nextVal
			} else {
				if v, err := strconv.Atoi(strings.TrimSpace(resolvedMap[autoIncColName])); err == nil && v > currentMax {
					currentMax = v
				}
			}
		}

		// Auto-stamp audit columns using the pre-resolved AuditIndices
		for _, colIdx := range cfg.AuditIndices {
			h := cfg.Headers[colIdx]
			if resolvedMap[h] == "" {
				resolvedMap[h] = timestamp
			}
		}

		rowCells := make([]string, len(cfg.Headers))
		for i, h := range cfg.Headers {
			rowCells[i] = resolvedMap[h]
		}

		// Perform uniqueness checks
		err := CheckUniqueness(rowCells, cfg, existingRows, validatedRows)
		if err != nil {
			return err
		}

		// Perform sorting checks if table has sort rules
		if cfg.IsSorted() {
			var lastRow []string
			if len(validatedRows) > 0 {
				lastRow = validatedRows[len(validatedRows)-1]
			} else if len(existingRows) > 0 {
				lastRow = existingRows[len(existingRows)-1]
			}
			if lastRow != nil {
				cmp := CompareRows(lastRow, rowCells, cfg.SortRules)
				if cfg.IsSortColUnique {
					if cmp >= 0 {
						return fmt.Errorf("unsorted append error: row is not strictly sorted after the previous row")
					}
				} else {
					if cmp > 0 {
						return fmt.Errorf("unsorted append error: row is not sorted after the previous row")
					}
				}
			}
		}

		validatedRows = append(validatedRows, rowCells)
		resolvedRows = append(resolvedRows, resolvedMap)
	}

	// Write phase via StreamTransformTable (single-pass streaming rewrite that strictly validates table structure,
	// injects appended rows directly before post-table prose, and commits change journal records).
	transformer := func(physicalRowIdx int, cells []string) ([]RowAction, error) {
		if cells != nil {
			return []RowAction{{Type: ActionKeep}}, nil
		}

		// Boundary event at end of table rows: inject all validatedRows
		var actions []RowAction
		for i, rowCells := range validatedRows {
			actions = append(actions, RowAction{
				Type:           ActionInsert,
				Cells:          rowCells,
				JournalOp:      "APPEND",
				JournalPayload: resolvedRows[i],
			})
		}
		return actions, nil
	}

	_, err := StreamTransformTable(ctx, filePath, cfg, transformer)
	return err
}

// Append parses the table and appends a raw input string (JSON/CSV)
func Append(ctx context.Context, filePath string, input string) error {
	return AppendWithOptions(ctx, filePath, input, "")
}

// AppendWithOptions parses the table and appends a raw input string with OCC precondition validation
func AppendWithOptions(ctx context.Context, filePath string, input string, ifMatch string) error {
	cfg, existingRows, err := ParseFrontmatterAndContent(ctx, filePath, ContentModeAuto)
	if err != nil {
		return err
	}
	parsedRows, err := ParseInputData(cfg, input)
	if err != nil {
		return err
	}
	return AppendRowsWithOptions(ctx, filePath, cfg, existingRows, parsedRows, ifMatch)
}

// AppendRow parses the table and appends a single parsed row map
func AppendRow(ctx context.Context, filePath string, rowMap map[string]string) error {
	return AppendRowWithOptions(ctx, filePath, rowMap, "")
}

// AppendRowWithOptions parses the table and appends a single parsed row map with OCC precondition validation
func AppendRowWithOptions(ctx context.Context, filePath string, rowMap map[string]string, ifMatch string) error {
	cfg, existingRows, err := ParseFrontmatterAndContent(ctx, filePath, ContentModeAuto)
	if err != nil {
		return err
	}
	return AppendRowsWithOptions(ctx, filePath, cfg, existingRows, []map[string]string{rowMap}, ifMatch)
}

// SortTableAction sorts the table either one-time or triggers auto-sorting configuration updates
func SortTableAction(ctx context.Context, filePath string, byRule string, trigger bool) error {
	cfg, rows, err := ParseFrontmatterAndContent(ctx, filePath, ContentModeAll)
	if err != nil {
		return err
	}

	sortRule := byRule
	if sortRule == "" {
		sortRule = cfg.Sort
	}
	if sortRule == "" {
		return fmt.Errorf("no sort rule specified or configured in frontmatter")
	}

	if trigger {
		cfg.Sort = sortRule
		return InitTable(ctx, filePath, cfg.Headers, cfg)
	}

	err = SortRows(cfg.Headers, rows, sortRule)
	if err != nil {
		return err
	}

	return AtomicWriteTable(ctx, filePath, cfg, rows)
}

// CountWithOptions returns the number of matching rows applying WHERE filters and PositionalOptions.
func CountWithOptions(ctx context.Context, filePath string, where []string, posOpts PositionalOptions) (int, error) {
	filters, err := ParseFilters(where)
	if err != nil {
		return 0, err
	}

	// Optimize: if no filters and no positional constraints, fast-path row count without row parsing
	if len(filters) == 0 && posOpts.IsZero() {
		cfg, _, err := ParseFrontmatterAndContent(ctx, filePath, ContentModeNone)
		if err != nil {
			return 0, err
		}
		return cfg.RowCount, nil
	}

	// Stream with early termination pushdown
	_, matches, err := ScanAndSelect(ctx, filePath, filters, posOpts)
	if err != nil {
		return 0, err
	}

	return len(matches), nil
}

// Count returns the number of matching rows in the table
func Count(ctx context.Context, filePath string, where []string) (int, error) {
	return CountWithOptions(ctx, filePath, where, PositionalOptions{})
}


// Normalize transforms a string into a canonical lowercase form stripping separators.
// Example: "Item Name" -> "itemname", "item_name" -> "itemname"
func Normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

// ResolveColumn finds the zero-based index of a column by name.
// It tries: 1. Exact Match, 2. Case-Insensitive Match, 3. Normalized Match.
func ResolveColumn(headers []string, target string) (int, string, error) {
	trimmed := strings.TrimSpace(target)
	// 1. Exact Match
	for i, h := range headers {
		if h == target || h == trimmed {
			return i, h, nil
		}
	}

	// 2. Case-Insensitive Match
	targetLower := strings.ToLower(trimmed)
	for i, h := range headers {
		if strings.ToLower(h) == targetLower {
			return i, h, nil
		}
	}

	// 3. Normalized Match (ignores spaces, underscores, hyphens)
	targetNorm := Normalize(target)
	for i, h := range headers {
		if Normalize(h) == targetNorm {
			return i, h, nil
		}
	}

	// 4. Failed: Return error with fuzzy suggestions and available columns
	suggestion := getFuzzySuggestion(headers, target)
	if suggestion != "" {
		return -1, "", fmt.Errorf(
			"column %q not found. Did you mean %q?\nAvailable columns: [%s]",
			target,
			suggestion,
			strings.Join(headers, ", "),
		)
	}

	return -1, "", fmt.Errorf(
		"column %q not found. Available columns: [%s]",
		target,
		strings.Join(headers, ", "),
	)
}

func getFuzzySuggestion(headers []string, target string) string {
	targetLower := strings.ToLower(target)
	bestDist := 3 // max edit distance threshold
	var bestMatch string
	for _, h := range headers {
		hLower := strings.ToLower(h)
		dist := levenshtein(targetLower, hLower)
		if dist < bestDist {
			bestDist = dist
			bestMatch = h
		}
	}
	return bestMatch
}

func levenshtein(s, t string) int {
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
	}
	for i := range d {
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for j := 1; j <= len(t); j++ {
		for i := 1; i <= len(s); i++ {
			if s[i-1] == t[j-1] {
				d[i][j] = d[i-1][j-1]
			} else {
				min := d[i-1][j] + 1
				if d[i][j-1]+1 < min {
					min = d[i][j-1] + 1
				}
				if d[i-1][j-1]+1 < min {
					min = d[i-1][j-1] + 1
				}
				d[i][j] = min
			}
		}
	}
	return d[len(s)][len(t)]
}

type contextKey string

const debugContextKey contextKey = "debug"

// WithDebug returns a context indicating whether debug printing is enabled.
func WithDebug(ctx context.Context, debug bool) context.Context {
	return context.WithValue(ctx, debugContextKey, debug)
}

// IsDebug returns true if debug printing is enabled in the context.
func IsDebug(ctx context.Context) bool {
	val, ok := ctx.Value(debugContextKey).(bool)
	return ok && val
}

func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte("\n"))
	if !bytes.HasSuffix(data, []byte("\n")) {
		n++
	}
	return n
}
