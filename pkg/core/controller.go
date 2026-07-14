package core

import (
	"bufio"
	"bytes"
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

// AuditConfig represents custom timestamp column mapping
type AuditConfig struct {
	Insert string `yaml:"insert,omitempty"`
	Edit   string `yaml:"edit,omitempty"`
}

// RowliteConfig represents configurations nested under rowlite: namespace
type RowliteConfig struct {
	Unique      []string    `yaml:"unique,flow"`
	Sort        string      `yaml:"sort,omitempty"`
	Audit       AuditConfig `yaml:"audit,omitempty"`
	JournalLog  string      `yaml:"journal_log,omitempty"`
}

// TableConfig represents frontmatter configurations
type TableConfig struct {
	Rowlite RowliteConfig `yaml:"rowlite"`
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

// EvaluateFilter checks if a row cell matches the filter
func EvaluateFilter(headers []string, row []string, filter Filter) (bool, error) {
	colIdx := -1
	for idx, h := range headers {
		if h == filter.Column {
			colIdx = idx
			break
		}
	}
	if colIdx == -1 {
		return false, fmt.Errorf("column %q not found in table headers", filter.Column)
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
			return false, fmt.Errorf("invalid regex %q: %v", filter.Value, err)
		}
		return re.MatchString(cellValue), nil
	default:
		return false, fmt.Errorf("unknown operator %q", filter.Operator)
	}
}

// ParseFrontmatterAndContent parses the frontmatter and table from a file
func ParseFrontmatterAndContent(filePath string) (TableConfig, [][]string, []string, []string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return TableConfig{}, nil, nil, nil, err
	}
	defer file.Close()

	var cfg TableConfig
	restBytes, err := frontmatter.Parse(file, &cfg)
	if err != nil {
		file.Seek(0, 0)
		restBytes, err = io.ReadAll(file)
		if err != nil {
			return TableConfig{}, nil, nil, nil, err
		}
		cfg = TableConfig{}
	}

	scanner := bufio.NewScanner(bytes.NewReader(restBytes))
	var rows [][]string
	var headers []string
	var alignments []string
	tableStarted := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if len(trimmed) > 0 && trimmed[0] == '|' && strings.Count(trimmed, "|") >= 2 {
			cells, err := SplitGFMLine(line)
			if err == nil {
				if !tableStarted {
					headers = cells
					tableStarted = true
					continue
				}
				if len(alignments) == 0 && IsSeparatorLine(cells) {
					alignments = cells
					continue
				}
				if len(cells) != len(headers) {
					return TableConfig{}, nil, nil, nil, fmt.Errorf("inconsistent column count: row has %d columns, expected %d", len(cells), len(headers))
				}
				rows = append(rows, cells)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return TableConfig{}, nil, nil, nil, err
	}

	return cfg, rows, headers, alignments, nil
}

// SerializeConfig formats frontmatter config as valid YAML frontmatter under rowlite: root
func SerializeConfig(cfg TableConfig) string {
	if len(cfg.Rowlite.Unique) == 0 && cfg.Rowlite.Sort == "" && cfg.Rowlite.Audit.Insert == "" && cfg.Rowlite.Audit.Edit == "" && cfg.Rowlite.JournalLog == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("rowlite:\n")
	if len(cfg.Rowlite.Unique) > 0 {
		sb.WriteString("  unique: [")
		for i, u := range cfg.Rowlite.Unique {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(u)
		}
		sb.WriteString("]\n")
	}
	if cfg.Rowlite.Sort != "" {
		sb.WriteString("  sort: " + cfg.Rowlite.Sort + "\n")
	}
	if cfg.Rowlite.Audit.Insert != "" || cfg.Rowlite.Audit.Edit != "" {
		sb.WriteString("  audit:\n")
		if cfg.Rowlite.Audit.Insert != "" {
			sb.WriteString("    insert: \"" + cfg.Rowlite.Audit.Insert + "\"\n")
		}
		if cfg.Rowlite.Audit.Edit != "" {
			sb.WriteString("    edit: \"" + cfg.Rowlite.Audit.Edit + "\"\n")
		}
	}
	if cfg.Rowlite.JournalLog != "" {
		sb.WriteString("  journal_log: " + cfg.Rowlite.JournalLog + "\n")
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

// SortRows stable-sorts rows in-place based on a sort rule (e.g., "id:asc")
func SortRows(headers []string, rows [][]string, sortRule string) error {
	if sortRule == "" {
		return nil
	}
	parts := strings.SplitN(sortRule, ":", 2)
	colName := strings.TrimSpace(parts[0])
	direction := "asc"
	if len(parts) > 1 {
		direction = strings.ToLower(strings.TrimSpace(parts[1]))
	}

	colIdx := -1
	for idx, h := range headers {
		if h == colName {
			colIdx = idx
			break
		}
	}
	if colIdx == -1 {
		return fmt.Errorf("sort column %q not found in table headers", colName)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		valI := rows[i][colIdx]
		valJ := rows[j][colIdx]
		cmp := CompareCells(valI, valJ)
		if direction == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	return nil
}

// WriteJournalRecord logs operation details into an append-only journal file
func WriteJournalRecord(journalPath string, op string, payload map[string]string) error {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
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
		file.WriteString("| timestamp | operation | details |\n| --- | --- | --- |\n")
	}

	logRow := []string{timestamp, op, string(payloadBytes)}
	_, err = file.WriteString(FormatGFMLine(logRow) + "\n")
	return err
}

// IsInsertAuditColumn checks if a column is an auto-filled creation timestamp column
func IsInsertAuditColumn(colName string, cfg TableConfig) bool {
	if cfg.Rowlite.Audit.Insert != "" && colName == cfg.Rowlite.Audit.Insert {
		return true
	}
	lowerCol := strings.ToLower(colName)
	if cfg.Rowlite.Audit.Insert == "" && (lowerCol == "created_at" || lowerCol == "inserted_at") {
		return true
	}
	return false
}

// IsEditAuditColumn checks if a column is an auto-filled modification timestamp column
func IsEditAuditColumn(colName string, cfg TableConfig) bool {
	if cfg.Rowlite.Audit.Edit != "" && colName == cfg.Rowlite.Audit.Edit {
		return true
	}
	lowerCol := strings.ToLower(colName)
	if cfg.Rowlite.Audit.Edit == "" && (lowerCol == "updated_at" || lowerCol == "modified_at") {
		return true
	}
	return false
}

// IsAuditColumn checks if a column is an auto-filled audit column (explicit or zero-config fallback)
func IsAuditColumn(colName string, cfg TableConfig) bool {
	return IsInsertAuditColumn(colName, cfg) || IsEditAuditColumn(colName, cfg)
}

// InitTable writes or updates a GFM table with YAML frontmatter configuration
func InitTable(filePath string, columns []string, cfg TableConfig) error {
	if len(columns) == 0 {
		return fmt.Errorf("columns list cannot be empty")
	}

	for _, u := range cfg.Rowlite.Unique {
		found := false
		for _, c := range columns {
			if c == u {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unique column %q not found in columns list", u)
		}
	}

	if cfg.Rowlite.Sort != "" {
		parts := strings.SplitN(cfg.Rowlite.Sort, ":", 2)
		sortCol := strings.TrimSpace(parts[0])
		found := false
		for _, c := range columns {
			if c == sortCol {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("sort column %q not found in columns list", sortCol)
		}
	}

	// Build finalColumns by appending auto-generated audit columns to user columns list if not already present
	var finalColumns []string
	finalColumns = append(finalColumns, columns...)

	if cfg.Rowlite.Audit.Insert != "" {
		found := false
		for _, c := range finalColumns {
			if c == cfg.Rowlite.Audit.Insert {
				found = true
				break
			}
		}
		if !found {
			finalColumns = append(finalColumns, cfg.Rowlite.Audit.Insert)
		}
	}

	if cfg.Rowlite.Audit.Edit != "" {
		found := false
		for _, c := range finalColumns {
			if c == cfg.Rowlite.Audit.Edit {
				found = true
				break
			}
		}
		if !found {
			finalColumns = append(finalColumns, cfg.Rowlite.Audit.Edit)
		}
	}

	var existingRows [][]string
	var oldHeaders []string
	var alignments []string
	if _, err := os.Stat(filePath); err == nil {
		_, rows, hdrs, align, parseErr := ParseFrontmatterAndContent(filePath)
		if parseErr == nil {
			existingRows = rows
			oldHeaders = hdrs
			alignments = align
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

	_, err = writer.WriteString(FormatGFMLine(finalColumns) + "\n")
	if err != nil {
		return err
	}

	_, err = writer.WriteString(FormatGFMLine(alignments) + "\n")
	if err != nil {
		return err
	}

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
		_, err = writer.WriteString(FormatGFMLine(newRow) + "\n")
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
func ParseInputData(headers []string, cfg TableConfig, input string) ([]map[string]string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, fmt.Errorf("empty input data")
	}

	// Filter out auto-generated audit columns from the user positional inputs list
	var userHeaders []string
	for _, h := range headers {
		if !IsAuditColumn(h, cfg) {
			userHeaders = append(userHeaders, h)
		}
	}

	// Check if JSON object
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		var row map[string]string
		err := json.Unmarshal([]byte(trimmed), &row)
		if err != nil {
			return nil, fmt.Errorf("invalid row JSON: %v", err)
		}
		return []map[string]string{row}, nil
	}

	// Check if JSON array
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		var rows []map[string]string
		err := json.Unmarshal([]byte(trimmed), &rows)
		if err != nil {
			return nil, fmt.Errorf("invalid rows JSON array: %v", err)
		}
		return rows, nil
	}

	// Treat as CSV
	reader := csv.NewReader(strings.NewReader(trimmed))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invalid CSV content: %v", err)
	}

	var rows []map[string]string
	for recordIdx, record := range records {
		if len(record) != len(userHeaders) {
			return nil, fmt.Errorf("CSV row %d has inconsistent column count: got %d, expected %d", recordIdx+1, len(record), len(userHeaders))
		}
		rowMap := make(map[string]string)
		for i, h := range userHeaders {
			rowMap[h] = record[i]
		}
		rows = append(rows, rowMap)
	}

	return rows, nil
}

// AppendRows appends multiple rows validating uniqueness, sorting, auditing, and change logging
func AppendRows(filePath string, newRows []map[string]string) error {
	if len(newRows) == 0 {
		return nil
	}

	cfg, rows, headers, alignments, err := ParseFrontmatterAndContent(filePath)
	if err != nil {
		return err
	}
	if len(headers) == 0 {
		return fmt.Errorf("table headers not initialized or table not found in %s", filePath)
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// Pre-process and validate all new rows
	var validatedRows [][]string
	for _, rowMap := range newRows {
		// Unique checks
		if len(cfg.Rowlite.Unique) > 0 {
			for _, u := range cfg.Rowlite.Unique {
				newVal := rowMap[u]
				colIdx := -1
				for idx, h := range headers {
					if h == u {
						colIdx = idx
						break
					}
				}
				if colIdx != -1 {
					// Check against existing rows
					for _, r := range rows {
						if colIdx < len(r) && r[colIdx] == newVal {
							return fmt.Errorf("duplicate key error: value %q for column %q already exists", newVal, u)
						}
					}
					// Check against other new rows in this batch
					for _, r := range validatedRows {
						if colIdx < len(r) && r[colIdx] == newVal {
							return fmt.Errorf("duplicate key error: value %q for column %q already exists in append batch", newVal, u)
						}
					}
				}
			}
		}

		// Auto-stamp audit columns
		for _, h := range headers {
			if IsAuditColumn(h, cfg) {
				if rowMap[h] == "" {
					rowMap[h] = timestamp
				}
			}
		}

		rowCells := make([]string, len(headers))
		for i, h := range headers {
			rowCells[i] = rowMap[h]
		}
		validatedRows = append(validatedRows, rowCells)
	}

	// Write phase
	if cfg.Rowlite.Sort == "" {
		// We can append multiple rows directly to the file
		for _, rowCells := range validatedRows {
			err = AppendRowDirectly(filePath, rowCells)
			if err != nil {
				return err
			}
		}
	} else {
		// We need to write atomically after sorting
		allRows := append(rows, validatedRows...)
		err = SortRows(headers, allRows, cfg.Rowlite.Sort)
		if err != nil {
			return err
		}
		err = AtomicWriteTable(filePath, cfg, headers, alignments, allRows)
		if err != nil {
			return err
		}
	}

	// Logging journal
	if cfg.Rowlite.JournalLog != "" {
		for _, rowMap := range newRows {
			_ = WriteJournalRecord(cfg.Rowlite.JournalLog, "APPEND", rowMap)
		}
	}

	return nil
}

// AppendRow appends a single row validating uniqueness, sorting, auditing, and change logging
func AppendRow(filePath string, newRowVals map[string]string) error {
	return AppendRows(filePath, []map[string]string{newRowVals})
}

// SortTableAction sorts the table either one-time or triggers auto-sorting configuration updates
func SortTableAction(filePath string, byRule string, trigger bool) error {
	cfg, rows, headers, alignments, err := ParseFrontmatterAndContent(filePath)
	if err != nil {
		return err
	}
	if len(headers) == 0 {
		return fmt.Errorf("table headers not found in %s", filePath)
	}

	sortRule := byRule
	if sortRule == "" {
		sortRule = cfg.Rowlite.Sort
	}
	if sortRule == "" {
		return fmt.Errorf("no sort rule specified or configured in frontmatter")
	}

	if trigger {
		cfg.Rowlite.Sort = sortRule
		err = SortRows(headers, rows, sortRule)
		if err != nil {
			return err
		}
		return InitTable(filePath, headers, cfg)
	}

	err = SortRows(headers, rows, sortRule)
	if err != nil {
		return err
	}

	return AtomicWriteTable(filePath, cfg, headers, alignments, rows)
}
