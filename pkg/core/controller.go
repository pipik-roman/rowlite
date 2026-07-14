package core

import (
	"bufio"
	"bytes"
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

// TableConfig represents frontmatter configurations
type TableConfig struct {
	Columns     []string `yaml:"columns,flow"`
	Unique      []string `yaml:"unique,flow"`
	Sort        string   `yaml:"sort,omitempty"`
	AuditFields bool     `yaml:"audit_fields,omitempty"`
	JournalLog  string   `yaml:"journal_log,omitempty"`
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

// SerializeConfig formats frontmatter config as valid YAML frontmatter
func SerializeConfig(cfg TableConfig) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	if len(cfg.Columns) > 0 {
		sb.WriteString("columns: [")
		for i, col := range cfg.Columns {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(col)
		}
		sb.WriteString("]\n")
	}
	if len(cfg.Unique) > 0 {
		sb.WriteString("unique: [")
		for i, u := range cfg.Unique {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(u)
		}
		sb.WriteString("]\n")
	}
	if cfg.Sort != "" {
		sb.WriteString("sort: " + cfg.Sort + "\n")
	}
	if cfg.AuditFields {
		sb.WriteString("audit_fields: true\n")
	}
	if cfg.JournalLog != "" {
		sb.WriteString("journal_log: " + cfg.JournalLog + "\n")
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

	// Check if file is empty to write header
	stat, err := file.Stat()
	if err == nil && stat.Size() == 0 {
		file.WriteString("| timestamp | operation | details |\n| --- | --- | --- |\n")
	}

	logRow := []string{timestamp, op, string(payloadBytes)}
	_, err = file.WriteString(FormatGFMLine(logRow) + "\n")
	return err
}
