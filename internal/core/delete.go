package core

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// DeleteWithOptions parses the table and deletes matching rows applying WHERE filters, positional slicing, and OCC.
// It requires allowAll to be true if neither WHERE filters nor positional options are specified.
// If dryRun is true, it simulates the selection and returns the number of matching rows without modifying the file.
func DeleteWithOptions(
	ctx context.Context,
	filePath string,
	where []string,
	posOpts PositionalOptions,
	ifMatch string,
	allowAll bool,
	dryRun bool,
) (int, error) {
	// Step 1: Precondition check (Optimistic Concurrency Control)
	if ifMatch != "" {
		currentETag, err := ComputeFileETag(filePath)
		if err != nil {
			return 0, err
		}
		if err := VerifyETag(ifMatch, currentETag); err != nil {
			return 0, err
		}
	}

	// Step 2: Parse table configuration
	cfg, _, err := ParseFrontmatterAndContent(ctx, filePath, ContentModeNone)
	if err != nil {
		return 0, err
	}
	headers := cfg.Headers
	if len(headers) == 0 {
		return 0, fmt.Errorf("no gfm table found in %s", filePath)
	}

	// Step 3: Parse WHERE filters
	filters, err := ParseFilters(where)
	if err != nil {
		return 0, err
	}

	// Step 4: Enforce conditionless safety guardrail
	if len(filters) == 0 && posOpts.IsZero() && !allowAll {
		return 0, fmt.Errorf("deleting all rows in table requires explicit confirmation flag (--all)")
	}

	// Step 5: Identify targeted row indices using streaming pushdown
	_, matched, err := ScanAndSelect(ctx, filePath, filters, posOpts)
	if err != nil {
		return 0, err
	}
	if len(matched) == 0 {
		return 0, nil
	}

	// Step 6: Fast-path for dry-run simulation
	if dryRun {
		return len(matched), nil
	}

	// Step 7: Stream deletion via workfile rewrite (O(1) memory overhead)
	return streamDeleteWorkfile(ctx, filePath, cfg, matched)
}

func streamDeleteWorkfile(
	ctx context.Context,
	filePath string,
	cfg TableConfig,
	matched MatchedRows,
) (int, error) {
	targetSet := make(map[int]bool, len(matched))
	for _, mr := range matched {
		targetSet[mr.Index] = true
	}

	// Locate table header line in the source file
	_, _, headerLineNum, err := ScanTableHeaders(ctx, filePath)
	if err != nil {
		return 0, err
	}

	sourceFile, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer sourceFile.Close()

	tempPath := filePath + ".tmp"
	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return 0, err
	}
	defer func() {
		tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	writer := bufio.NewWriter(tempFile)
	scanner := bufio.NewScanner(sourceFile)

	type journalEntry struct {
		rowKey  string
		payload map[string]string
	}
	var journalEntries []journalEntry

	lineNum := 0
	physicalRowIdx := 0
	tableEnded := false
	deletedCount := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// 1. Pre-table lines (frontmatter, markdown headers, prose)
		if lineNum < headerLineNum {
			if _, err := writer.WriteString(line + "\n"); err != nil {
				return 0, err
			}
			continue
		}

		// 2. Table header and separator lines
		if lineNum == headerLineNum {
			if _, err := writer.WriteString(FormatGFMLine(cfg.Headers) + "\n"); err != nil {
				return 0, err
			}
			continue
		}
		if lineNum == headerLineNum+1 {
			if _, err := writer.WriteString(FormatGFMLine(cfg.Alignments) + "\n"); err != nil {
				return 0, err
			}
			continue
		}

		// 3. Post-table lines (after table ends)
		if tableEnded {
			if _, err := writer.WriteString(line + "\n"); err != nil {
				return 0, err
			}
			continue
		}

		trimmed := strings.TrimSpace(line)
		if !isTableRow(trimmed) {
			tableEnded = true
			if _, err := writer.WriteString(line + "\n"); err != nil {
				return 0, err
			}
			continue
		}

		// 4. Table data body row
		cells, err := SplitGFMLine(line)
		if err != nil {
			// Write malformed line as-is
			if _, err := writer.WriteString(line + "\n"); err != nil {
				return 0, err
			}
			physicalRowIdx++
			continue
		}

		if targetSet[physicalRowIdx] {
			// Targeted row: exclude from output workfile
			if cfg.JournalLog != "" {
				rowMap := make(map[string]string, len(cfg.Headers))
				for cIdx, h := range cfg.Headers {
					if cIdx < len(cells) {
						rowMap[h] = cells[cIdx]
					}
				}
				rowKey := ComputeRowKey(cfg, physicalRowIdx+1, rowMap)
				journalEntries = append(journalEntries, journalEntry{rowKey: rowKey, payload: rowMap})
			}
			deletedCount++
		} else {
			// Untouched row: write to workfile
			if _, err := writer.WriteString(FormatGFMLine(cells) + "\n"); err != nil {
				return 0, err
			}
		}

		physicalRowIdx++
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	if err := writer.Flush(); err != nil {
		return 0, err
	}

	// Close open file handles before atomic rename (critical on Windows)
	tempFile.Close()
	sourceFile.Close()

	// Atomic swap
	if err := os.Rename(tempPath, filePath); err != nil {
		return 0, err
	}

	// Commit journal records
	if cfg.JournalLog != "" {
		for _, je := range journalEntries {
			_ = WriteJournalRecord(ctx, cfg.JournalLog, "DELETE", je.rowKey, je.payload)
		}
	}

	return deletedCount, nil
}
