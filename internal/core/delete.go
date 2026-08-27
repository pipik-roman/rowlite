package core

import (
	"context"
	"fmt"
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

	// Step 5: Fast-path for dry-run simulation
	if dryRun {
		_, matched, err := ScanAndSelect(ctx, filePath, filters, posOpts)
		if err != nil {
			return 0, err
		}
		return len(matched), nil
	}

	// Step 6: Determine pushdown strategy (Single-Pass vs Two-Pass)
	plan := PlanPushdown(posOpts)

	// Single-Pass execution for forward queries (StrategyEarlyExit, StrategyAll)
	if (plan.Strategy == StrategyEarlyExit || plan.Strategy == StrategyAll) && !plan.FallbackSlicing {
		matchCount := 0
		transformer := func(physicalRowIdx int, cells []string) ([]RowAction, error) {
			if cells == nil {
				return nil, nil // boundary event: nothing to append on delete
			}

			// If upper match bound was already reached in StrategyEarlyExit, skip predicate evaluations
			if plan.Strategy == StrategyEarlyExit && matchCount >= plan.MaxMatches {
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
					rowMap := make(map[string]string, len(cfg.Headers))
					for cIdx, h := range cfg.Headers {
						if cIdx < len(cells) {
							rowMap[h] = cells[cIdx]
						}
					}
					return []RowAction{
						{
							Type:           ActionDelete,
							JournalOp:      "DELETE",
							JournalPayload: rowMap,
						},
					}, nil
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
			rowMap := make(map[string]string, len(cfg.Headers))
			for cIdx, h := range cfg.Headers {
				if cIdx < len(cells) {
					rowMap[h] = cells[cIdx]
				}
			}
			return []RowAction{
				{
					Type:           ActionDelete,
					JournalOp:      "DELETE",
					JournalPayload: rowMap,
				},
			}, nil
		}

		return []RowAction{{Type: ActionKeep}}, nil
	}

	return StreamTransformTable(ctx, filePath, cfg, transformer)
}
