package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ParseSetExpressions parses key-value assignment strings (e.g. "status=done", "count=5", "notes=checked").
// Supports multiple arguments and comma-separated pairs.
func ParseSetExpressions(setList []string) (map[string]string, error) {
	if len(setList) == 0 {
		return nil, fmt.Errorf("no assignment expressions provided")
	}

	assignments := make(map[string]string)
	for _, item := range setList {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		// Handle comma-separated pairs: "status=done, priority=high"
		var parts []string
		if strings.Contains(item, ",") && !strings.Contains(item, `\,`) {
			parts = strings.Split(item, ",")
		} else {
			parts = []string{item}
		}

		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				return nil, fmt.Errorf("invalid assignment expression %q (expected format: column=value)", part)
			}
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			if k == "" {
				return nil, fmt.Errorf("column name cannot be empty in assignment %q", part)
			}
			assignments[k] = v
		}
	}

	if len(assignments) == 0 {
		return nil, fmt.Errorf("no valid column assignments found in expressions")
	}

	return assignments, nil
}

// ParseUpdateData parses a raw JSON or key-value string into an assignment map.
func ParseUpdateData(input string) (map[string]string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, fmt.Errorf("empty update data string")
	}

	// Try JSON object: {"status":"active", "priority":"high"}
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		var assignments map[string]string
		err := json.Unmarshal([]byte(trimmed), &assignments)
		if err != nil {
			// Try map[string]any for numeric/boolean values
			var anyMap map[string]any
			if anyErr := json.Unmarshal([]byte(trimmed), &anyMap); anyErr == nil {
				assignments = make(map[string]string, len(anyMap))
				for k, v := range anyMap {
					assignments[k] = fmt.Sprint(v)
				}
				return assignments, nil
			}
			return nil, fmt.Errorf("invalid update JSON object: %w", err)
		}
		return assignments, nil
	}

	// Fallback to comma/equals parsing
	return ParseSetExpressions([]string{trimmed})
}

// ApplyRowUpdate applies assignment mutations against a row's pre-state snapshot and auto-stamps edit audit columns.
func ApplyRowUpdate(
	headers []string,
	originalRow []string,
	resolvedUpdates map[string]string,
	cfg TableConfig,
	timestamp string,
) ([]string, map[string]string, error) {
	newRow := make([]string, len(headers))
	for i := range headers {
		if i < len(originalRow) {
			newRow[i] = originalRow[i]
		}
	}

	mutatedMap := make(map[string]string, len(resolvedUpdates))

	// Apply explicit updates against original snapshot
	for colName, newVal := range resolvedUpdates {
		colIdx, canonicalName, err := ResolveColumn(headers, colName)
		if err != nil {
			return nil, nil, err
		}
		newRow[colIdx] = newVal
		mutatedMap[canonicalName] = newVal
	}

	// Auto-stamp edit audit columns if not explicitly overridden
	for _, colIdx := range cfg.AuditIndices {
		if colIdx < len(headers) {
			colName := headers[colIdx]
			if IsEditAuditColumn(colName, cfg) {
				if _, explicitlySet := mutatedMap[colName]; !explicitlySet {
					newRow[colIdx] = timestamp
					mutatedMap[colName] = timestamp
				}
			}
		}
	}

	return newRow, mutatedMap, nil
}

// UpdateWithOptions parses the table and updates matching rows applying WHERE filters, positional slicing, constraints, and OCC.
// It requires allowAll to be true if neither WHERE filters nor positional options are specified.
// If dryRun is true, it simulates the selection and returns the number of matching rows without modifying the file.
func UpdateWithOptions(
	ctx context.Context,
	filePath string,
	where []string,
	posOpts PositionalOptions,
	updates map[string]string,
	ifMatch string,
	allowAll bool,
	dryRun bool,
) (int, error) {
	if len(updates) == 0 {
		return 0, fmt.Errorf("no column assignments specified for update")
	}

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

	// Step 3: Resolve update column names to canonical header names
	resolvedUpdates := make(map[string]string, len(updates))
	for k, v := range updates {
		_, canonicalName, err := ResolveColumn(headers, k)
		if err != nil {
			return 0, err
		}
		resolvedUpdates[canonicalName] = v
	}

	// Step 4: Parse WHERE filters
	filters, err := ParseFilters(where)
	if err != nil {
		return 0, err
	}

	// Step 5: Enforce conditionless safety guardrail
	if len(filters) == 0 && posOpts.IsZero() && !allowAll {
		return 0, fmt.Errorf("updating all rows in table requires explicit confirmation flag (--all)")
	}

	// Step 6: Fast-path for dry-run simulation
	if dryRun {
		_, matched, err := ScanAndSelect(ctx, filePath, filters, posOpts)
		if err != nil {
			return 0, err
		}
		return len(matched), nil
	}

	// Step 7: Check if any sort column is modified
	isSortKeyModified := false
	if cfg.IsSorted() {
		for _, rule := range cfg.SortRules {
			if rule.ColIdx < len(headers) {
				sortCol := headers[rule.ColIdx]
				if _, ok := resolvedUpdates[sortCol]; ok {
					isSortKeyModified = true
					break
				}
			}
		}
	}

	// Step 8: Check if any unique column is modified
	isUniqueKeyModified := false
	for _, uIdx := range cfg.UniqueIndices {
		if uIdx < len(headers) {
			uCol := headers[uIdx]
			if _, ok := resolvedUpdates[uCol]; ok {
				isUniqueKeyModified = true
				break
			}
		}
	}

	// Check if autoincrement column is updated and validate
	if cfg.AutoIncrementIdx >= 0 && cfg.AutoIncrementIdx < len(headers) {
		autoIncCol := headers[cfg.AutoIncrementIdx]
		if _, ok := resolvedUpdates[autoIncCol]; ok {
			// AutoIncrement column updated explicitly - allowed if valid integer
		}
	}

	// EXECUTION PATH A: In-Memory Stable Re-sort Path (only when a sort key is mutated)
	if isSortKeyModified {
		return inMemoryUpdateAndSort(ctx, filePath, cfg, filters, posOpts, resolvedUpdates, isUniqueKeyModified)
	}

	// EXECUTION PATH B: Fast Streaming Workfile Path (O(1) memory for standard updates)
	return streamUpdateWorkfile(ctx, filePath, cfg, filters, posOpts, resolvedUpdates, isUniqueKeyModified)
}

func inMemoryUpdateAndSort(
	ctx context.Context,
	filePath string,
	cfg TableConfig,
	filters []Filter,
	posOpts PositionalOptions,
	resolvedUpdates map[string]string,
	isUniqueKeyModified bool,
) (int, error) {
	cfgAll, allRows, err := ParseFrontmatterAndContent(ctx, filePath, ContentModeAll)
	if err != nil {
		return 0, err
	}

	matched, err := SelectRowsStream(cfgAll.Headers, allRows, filters, posOpts)
	if err != nil {
		return 0, err
	}
	if len(matched) == 0 {
		return 0, nil
	}

	targetSet := make(map[int]bool, len(matched))
	for _, mr := range matched {
		targetSet[mr.Index] = true
	}

	timestamp := time.Now().Format(timestampFormat)
	type journalEntry struct {
		rowKey  string
		payload map[string]string
	}
	var journalEntries []journalEntry

	seenBatchKeys := make(map[string]bool)

	// Validate and apply updates
	for idx, row := range allRows {
		if targetSet[idx] {
			newRow, mutated, err := ApplyRowUpdate(cfgAll.Headers, row, resolvedUpdates, cfgAll, timestamp)
			if err != nil {
				return 0, err
			}

			// Validate unique keys
			if isUniqueKeyModified {
				for _, uIdx := range cfgAll.UniqueIndices {
					val := newRow[uIdx]
					if strings.TrimSpace(val) == "" {
						return 0, fmt.Errorf("unique column %q cannot be empty", cfgAll.Headers[uIdx])
					}
					if seenBatchKeys[val] {
						return 0, fmt.Errorf("duplicate key error: value %q for column %q already exists in update batch", val, cfgAll.Headers[uIdx])
					}
					// Check collision against untouched rows
					for otherIdx, otherRow := range allRows {
						if !targetSet[otherIdx] && uIdx < len(otherRow) {
							if otherRow[uIdx] == val {
								return 0, fmt.Errorf("duplicate key error: value %q for column %q already exists in table", val, cfgAll.Headers[uIdx])
							}
						}
					}
					seenBatchKeys[val] = true
				}
			}

			allRows[idx] = newRow

			if cfgAll.JournalLog != "" {
				rowMap := make(map[string]string, len(cfgAll.Headers))
				for cIdx, h := range cfgAll.Headers {
					if cIdx < len(newRow) {
						rowMap[h] = newRow[cIdx]
					}
				}
				rowKey := ComputeRowKey(cfgAll, idx+1, rowMap)
				journalEntries = append(journalEntries, journalEntry{rowKey: rowKey, payload: mutated})
			}
		}
	}

	// Stable re-sort
	if cfgAll.Sort != "" {
		if err := SortRows(cfgAll.Headers, allRows, cfgAll.Sort); err != nil {
			return 0, err
		}
	}

	// Atomic disk serialization
	if err := AtomicWriteTable(ctx, filePath, cfgAll, allRows); err != nil {
		return 0, err
	}

	// Commit journal records
	if cfgAll.JournalLog != "" {
		for _, je := range journalEntries {
			_ = WriteJournalRecord(ctx, cfgAll.JournalLog, "UPDATE", je.rowKey, je.payload)
		}
	}

	return len(matched), nil
}

func streamUpdateWorkfile(
	ctx context.Context,
	filePath string,
	cfg TableConfig,
	filters []Filter,
	posOpts PositionalOptions,
	resolvedUpdates map[string]string,
	isUniqueKeyModified bool,
) (int, error) {
	plan := PlanPushdown(posOpts)
	timestamp := time.Now().Format(timestampFormat)
	seenBatchKeys := make(map[string]bool)
	seenUntouchedKeys := make(map[string]bool)

	// Single-Pass execution for forward queries (StrategyEarlyExit, StrategyAll)
	if (plan.Strategy == StrategyEarlyExit || plan.Strategy == StrategyAll) && !plan.FallbackSlicing {
		matchCount := 0
		transformer := func(physicalRowIdx int, cells []string) ([]RowAction, error) {
			if cells == nil {
				return nil, nil // boundary event
			}

			// If upper match bound was already reached in StrategyEarlyExit, skip predicate evaluations
			if plan.Strategy == StrategyEarlyExit && matchCount >= plan.MaxMatches {
				if isUniqueKeyModified {
					for _, uIdx := range cfg.UniqueIndices {
						if uIdx < len(cells) {
							val := cells[uIdx]
							if seenBatchKeys[val] {
								return nil, fmt.Errorf("duplicate key error: value %q for column %q already exists in table", val, cfg.Headers[uIdx])
							}
							seenUntouchedKeys[val] = true
						}
					}
				}
				return []RowAction{{Type: ActionKeep}}, nil
			}

			matched, err := MatchesRow(cfg.Headers, cells, filters)
			if err != nil {
				return nil, err
			}

			if matched {
				isTargeted := plan.MatchesTarget(matchCount)
				matchCount++
				if isTargeted {
					newCells, mutated, err := ApplyRowUpdate(cfg.Headers, cells, resolvedUpdates, cfg, timestamp)
					if err != nil {
						return nil, err
					}

					if isUniqueKeyModified {
						for _, uIdx := range cfg.UniqueIndices {
							if uIdx < len(newCells) {
								val := newCells[uIdx]
								if strings.TrimSpace(val) == "" {
									return nil, fmt.Errorf("unique column %q cannot be empty", cfg.Headers[uIdx])
								}
								if seenBatchKeys[val] {
									return nil, fmt.Errorf("duplicate key error: value %q for column %q already exists in update batch", val, cfg.Headers[uIdx])
								}
								if seenUntouchedKeys[val] {
									return nil, fmt.Errorf("duplicate key error: value %q for column %q already exists in table", val, cfg.Headers[uIdx])
								}
								seenBatchKeys[val] = true
							}
						}
					}

					return []RowAction{
						{
							Type:           ActionUpdate,
							Cells:          newCells,
							JournalOp:      "UPDATE",
							JournalPayload: mutated,
						},
					}, nil
				}
			}

			// Untouched row: check against previously mutated batch keys
			if isUniqueKeyModified {
				for _, uIdx := range cfg.UniqueIndices {
					if uIdx < len(cells) {
						val := cells[uIdx]
						if seenBatchKeys[val] {
							return nil, fmt.Errorf("duplicate key error: value %q for column %q already exists in table", val, cfg.Headers[uIdx])
						}
						seenUntouchedKeys[val] = true
					}
				}
			}

			return []RowAction{{Type: ActionKeep}}, nil
		}

		return StreamTransformTable(ctx, filePath, cfg, transformer)
	}

	// Two-Pass execution for backward queries (StrategyRingBuffer, StrategyDualPartition, FallbackSlicing)
	_, matched, err := ScanAndSelect(ctx, filePath, filters, posOpts)
	if err != nil {
		return 0, err
	}
	if len(matched) == 0 {
		return 0, nil
	}

	targetSet := make(map[int]bool, len(matched))
	for _, mr := range matched {
		targetSet[mr.Index] = true
	}

	transformer := func(physicalRowIdx int, cells []string) ([]RowAction, error) {
		if cells == nil {
			return nil, nil
		}

		if targetSet[physicalRowIdx] {
			newCells, mutated, err := ApplyRowUpdate(cfg.Headers, cells, resolvedUpdates, cfg, timestamp)
			if err != nil {
				return nil, err
			}

			if isUniqueKeyModified {
				for _, uIdx := range cfg.UniqueIndices {
					if uIdx < len(newCells) {
						val := newCells[uIdx]
						if strings.TrimSpace(val) == "" {
							return nil, fmt.Errorf("unique column %q cannot be empty", cfg.Headers[uIdx])
						}
						if seenBatchKeys[val] {
							return nil, fmt.Errorf("duplicate key error: value %q for column %q already exists in update batch", val, cfg.Headers[uIdx])
						}
						if seenUntouchedKeys[val] {
							return nil, fmt.Errorf("duplicate key error: value %q for column %q already exists in table", val, cfg.Headers[uIdx])
						}
						seenBatchKeys[val] = true
					}
				}
			}

			return []RowAction{
				{
					Type:           ActionUpdate,
					Cells:          newCells,
					JournalOp:      "UPDATE",
					JournalPayload: mutated,
				},
			}, nil
		}

		if isUniqueKeyModified {
			for _, uIdx := range cfg.UniqueIndices {
				if uIdx < len(cells) {
					val := cells[uIdx]
					if seenBatchKeys[val] {
						return nil, fmt.Errorf("duplicate key error: value %q for column %q already exists in table", val, cfg.Headers[uIdx])
					}
					seenUntouchedKeys[val] = true
				}
			}
		}

		return []RowAction{{Type: ActionKeep}}, nil
	}

	return StreamTransformTable(ctx, filePath, cfg, transformer)
}
