package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/adrg/frontmatter"
)

// PushdownStrategy defines the streaming evaluation strategy for positional queries.
type PushdownStrategy int

const (
	// StrategyAll collects all matched rows (unbounded query or complex fallback).
	StrategyAll PushdownStrategy = iota
	// StrategyEarlyExit breaks row scanning as soon as the upper positional limit is reached.
	StrategyEarlyExit
	// StrategyRingBuffer maintains a bounded circular buffer of capacity K for tail/negative index queries.
	StrategyRingBuffer
	// StrategyDualPartition collects top H matches in a fixed slice and streams remaining into a size-T ring buffer.
	StrategyDualPartition
)

// PushdownPlan specifies the execution parameters for stream scanning.
type PushdownPlan struct {
	Strategy       PushdownStrategy
	MaxMatches     int          // Upper match bound for StrategyEarlyExit (scanner breaks when matchCount >= MaxMatches)
	TargetSet      map[int]bool // Sparse match ordinals targeted (if non-nil)
	ContinuousMin  int          // Inclusive starting match ordinal for continuous slice
	ContinuousMax  int          // Inclusive ending match ordinal for continuous slice
	IsContinuous   bool         // If true, match is targeted if ContinuousMin <= matchCount <= ContinuousMax
	RingCapacity   int          // Capacity of circular ring buffer for StrategyRingBuffer
	HeadCapacity   int          // Head capacity for StrategyDualPartition
	TailCapacity   int          // Tail capacity for StrategyDualPartition
	ApplyWindow    bool         // True if offset/limit window should be applied post-collection
	WindowOffset   int
	WindowLimit    *int
	FallbackSlicing bool        // True if PositionalOptions.ApplyPositional should be executed on collected rows
	OriginalOpts   PositionalOptions
}

// MatchesTarget checks whether a 0-based match ordinal is targeted by this pushdown plan.
func (p *PushdownPlan) MatchesTarget(matchOrdinal int) bool {
	if p.IsContinuous {
		return matchOrdinal >= p.ContinuousMin && matchOrdinal <= p.ContinuousMax
	}
	if p.TargetSet != nil {
		return p.TargetSet[matchOrdinal]
	}
	return true
}

// PlanPushdown inspects PositionalOptions and constructs an optimized PushdownPlan.
func PlanPushdown(opts PositionalOptions) PushdownPlan {
	if opts.IsZero() {
		return PushdownPlan{
			Strategy:     StrategyAll,
			OriginalOpts: opts,
		}
	}

	// 1. Index specification priority
	if opts.Index != "" {
		spec, err := ParseRawIndexSpec(opts.Index)
		if err != nil {
			return PushdownPlan{Strategy: StrategyAll, FallbackSlicing: true, OriginalOpts: opts}
		}

		switch spec.Type {
		case IndexSpecSingle:
			if spec.Single >= 0 {
				plan := PushdownPlan{
					Strategy:      StrategyEarlyExit,
					ContinuousMin: spec.Single,
					ContinuousMax: spec.Single,
					IsContinuous:  true,
					MaxMatches:    spec.Single + 1,
					OriginalOpts:  opts,
				}
				if opts.Offset > 0 || opts.Limit != nil {
					plan.ApplyWindow = true
					plan.WindowOffset = opts.Offset
					plan.WindowLimit = opts.Limit
				}
				return plan
			}
			// Single negative index (e.g. -1 for last row)
			k := -spec.Single
			return PushdownPlan{
				Strategy:     StrategyRingBuffer,
				RingCapacity: k,
				OriginalOpts: opts,
			}

		case IndexSpecList:
			allNonNegative := true
			maxVal := 0
			targetSet := make(map[int]bool, len(spec.List))
			for _, v := range spec.List {
				if v < 0 {
					allNonNegative = false
					break
				}
				targetSet[v] = true
				if v > maxVal {
					maxVal = v
				}
			}
			if allNonNegative {
				plan := PushdownPlan{
					Strategy:     StrategyEarlyExit,
					TargetSet:    targetSet,
					MaxMatches:   maxVal + 1,
					OriginalOpts: opts,
				}
				if opts.Offset > 0 || opts.Limit != nil {
					plan.ApplyWindow = true
					plan.WindowOffset = opts.Offset
					plan.WindowLimit = opts.Limit
				}
				return plan
			}
			return PushdownPlan{Strategy: StrategyAll, FallbackSlicing: true, OriginalOpts: opts}

		case IndexSpecRange:
			if spec.RangeStart >= 0 && spec.RangeEnd >= 0 {
				minVal := spec.RangeStart
				maxVal := spec.RangeEnd
				if minVal > maxVal {
					minVal, maxVal = maxVal, minVal
				}
				if spec.RangeStep == 1 {
					plan := PushdownPlan{
						Strategy:      StrategyEarlyExit,
						ContinuousMin: minVal,
						ContinuousMax: maxVal,
						IsContinuous:  true,
						MaxMatches:    maxVal + 1,
						OriginalOpts:  opts,
					}
					if opts.Offset > 0 || opts.Limit != nil {
						plan.ApplyWindow = true
						plan.WindowOffset = opts.Offset
						plan.WindowLimit = opts.Limit
					}
					return plan
				}
				// Stepped range
				targetSet := make(map[int]bool)
				for i := spec.RangeStart; i <= spec.RangeEnd; i += spec.RangeStep {
					targetSet[i] = true
				}
				plan := PushdownPlan{
					Strategy:     StrategyEarlyExit,
					TargetSet:    targetSet,
					MaxMatches:   spec.RangeEnd + 1,
					OriginalOpts: opts,
				}
				if opts.Offset > 0 || opts.Limit != nil {
					plan.ApplyWindow = true
					plan.WindowOffset = opts.Offset
					plan.WindowLimit = opts.Limit
				}
				return plan
			}
			if spec.RangeStart < 0 && spec.RangeEnd < 0 && spec.RangeStep == 1 {
				minNeg := spec.RangeStart
				maxNeg := spec.RangeEnd
				if minNeg > maxNeg {
					minNeg, maxNeg = maxNeg, minNeg
				}
				k := -minNeg
				return PushdownPlan{
					Strategy:        StrategyRingBuffer,
					RingCapacity:    k,
					FallbackSlicing: true,
					OriginalOpts:    opts,
				}
			}
			return PushdownPlan{Strategy: StrategyAll, FallbackSlicing: true, OriginalOpts: opts}
		}
	}

	// 2. Dual Head and Tail
	if opts.Head != nil && opts.Tail != nil {
		plan := PushdownPlan{
			Strategy:     StrategyDualPartition,
			HeadCapacity: *opts.Head,
			TailCapacity: *opts.Tail,
			OriginalOpts: opts,
		}
		if opts.Offset > 0 || opts.Limit != nil {
			plan.ApplyWindow = true
			plan.WindowOffset = opts.Offset
			plan.WindowLimit = opts.Limit
		}
		return plan
	}

	// 3. Head Only
	if opts.Head != nil {
		h := *opts.Head
		if h <= 0 {
			return PushdownPlan{Strategy: StrategyEarlyExit, MaxMatches: 0, OriginalOpts: opts}
		}
		targetStart := 0
		targetEnd := h - 1
		if opts.Offset > 0 {
			targetStart = opts.Offset
		}
		if opts.Limit != nil {
			limEnd := targetStart + *opts.Limit - 1
			if limEnd < targetEnd {
				targetEnd = limEnd
			}
		}
		if targetStart > targetEnd {
			return PushdownPlan{Strategy: StrategyEarlyExit, MaxMatches: 0, OriginalOpts: opts}
		}
		return PushdownPlan{
			Strategy:      StrategyEarlyExit,
			ContinuousMin: targetStart,
			ContinuousMax: targetEnd,
			IsContinuous:  true,
			MaxMatches:    targetEnd + 1,
			OriginalOpts:  opts,
		}
	}

	// 4. Tail Only
	if opts.Tail != nil {
		t := *opts.Tail
		if t <= 0 {
			return PushdownPlan{Strategy: StrategyEarlyExit, MaxMatches: 0, OriginalOpts: opts}
		}
		plan := PushdownPlan{
			Strategy:     StrategyRingBuffer,
			RingCapacity: t,
			OriginalOpts: opts,
		}
		if opts.Offset > 0 || opts.Limit != nil {
			plan.ApplyWindow = true
			plan.WindowOffset = opts.Offset
			plan.WindowLimit = opts.Limit
		}
		return plan
	}

	// 5. Offset and Limit Window
	if opts.Limit != nil {
		l := *opts.Limit
		o := opts.Offset
		if l <= 0 {
			return PushdownPlan{Strategy: StrategyEarlyExit, MaxMatches: 0, OriginalOpts: opts}
		}
		return PushdownPlan{
			Strategy:      StrategyEarlyExit,
			ContinuousMin: o,
			ContinuousMax: o + l - 1,
			IsContinuous:  true,
			MaxMatches:    o + l,
			OriginalOpts:  opts,
		}
	}

	if opts.Offset > 0 {
		return PushdownPlan{
			Strategy:      StrategyEarlyExit,
			ContinuousMin: opts.Offset,
			ContinuousMax: math.MaxInt - 1,
			IsContinuous:  true,
			MaxMatches:    math.MaxInt,
			OriginalOpts:  opts,
		}
	}

	return PushdownPlan{Strategy: StrategyAll, OriginalOpts: opts}
}

// RingBuffer provides a fixed-capacity FIFO circular queue for MatchedRow items.
type RingBuffer struct {
	data     []MatchedRow
	capacity int
	size     int
	start    int
}

// NewRingBuffer allocates a circular buffer with a fixed capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 1
	}
	return &RingBuffer{
		data:     make([]MatchedRow, capacity),
		capacity: capacity,
	}
}

// Push adds an item to the circular buffer, overwriting the oldest element if full.
func (r *RingBuffer) Push(item MatchedRow) {
	if r.size < r.capacity {
		r.data[r.size] = item
		r.size++
	} else {
		r.data[r.start] = item
		r.start = (r.start + 1) % r.capacity
	}
}

// Slice returns the ordered elements currently stored in the circular buffer.
func (r *RingBuffer) Slice() MatchedRows {
	result := make(MatchedRows, r.size)
	for i := 0; i < r.size; i++ {
		result[i] = r.data[(r.start+i)%r.capacity]
	}
	return result
}

// ScanAndSelect parses table structure from disk and streams rows with predicate and positional pushdown.
func ScanAndSelect(ctx context.Context, filePath string, filters []Filter, posOpts PositionalOptions) (TableConfig, MatchedRows, error) {
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

	// Calculate frontmatter line offset
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

	// Step 1: Find headers and alignments
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

	// Resolve derived configuration indices
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
	for _, u := range cfg.Unique {
		if colIdx, _, err := ResolveColumn(headers, u); err == nil {
			cfg.UniqueIndices = append(cfg.UniqueIndices, colIdx)
		}
	}
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

	// Step 2: Stream table rows with predicate and positional pushdown
	plan := PlanPushdown(posOpts)
	if plan.Strategy == StrategyEarlyExit && plan.MaxMatches <= 0 {
		return cfg, MatchedRows{}, nil
	}

	var results MatchedRows
	var headResults MatchedRows
	var ring *RingBuffer
	if plan.Strategy == StrategyRingBuffer {
		ring = NewRingBuffer(plan.RingCapacity)
	} else if plan.Strategy == StrategyDualPartition {
		ring = NewRingBuffer(plan.TailCapacity)
		headResults = make(MatchedRows, 0, plan.HeadCapacity)
	}

	physicalRowIdx := 0
	matchCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if !isTableRow(trimmed) {
			break
		}

		cells, err := SplitGFMLine(line)
		if err != nil {
			physicalRowIdx++
			continue
		}

		matched, err := MatchesRow(headers, cells, filters)
		if err != nil {
			return TableConfig{}, nil, err
		}

		if matched {
			rowItem := MatchedRow{
				Index: physicalRowIdx,
				Row:   cells,
			}

			switch plan.Strategy {
			case StrategyEarlyExit:
				if plan.MatchesTarget(matchCount) {
					results = append(results, rowItem)
				}
				matchCount++
				if matchCount >= plan.MaxMatches {
					// Early termination: break immediately!
					goto scanComplete
				}

			case StrategyRingBuffer:
				ring.Push(rowItem)
				matchCount++

			case StrategyDualPartition:
				if matchCount < plan.HeadCapacity {
					headResults = append(headResults, rowItem)
				} else {
					ring.Push(rowItem)
				}
				matchCount++

			case StrategyAll:
				results = append(results, rowItem)
				matchCount++
			}
		}

		physicalRowIdx++
	}

scanComplete:
	// Step 3: Finalize result set based on strategy
	switch plan.Strategy {
	case StrategyRingBuffer:
		results = ring.Slice()
	case StrategyDualPartition:
		tailSlice := ring.Slice()
		if matchCount <= plan.HeadCapacity+plan.TailCapacity {
			// All matching rows were captured in headResults and ring without gap
			results = append(headResults, tailSlice...)
		} else {
			results = append(headResults, tailSlice...)
		}
	}

	// Apply post-processing window or fallback if needed
	if plan.FallbackSlicing {
		sliced, err := results.ApplyPositional(plan.OriginalOpts)
		return cfg, sliced, err
	}

	if plan.ApplyWindow {
		start := plan.WindowOffset
		if start > len(results) {
			start = len(results)
		}
		end := len(results)
		if plan.WindowLimit != nil {
			end = start + *plan.WindowLimit
			if end > len(results) {
				end = len(results)
			}
		}
		results = results[start:end]
	}

	if IsDebug(ctx) {
		if jsonBytes, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			fmt.Fprintf(os.Stderr, "DEBUG TableConfig:\n%s\n", string(jsonBytes))
		} else {
			fmt.Fprintf(os.Stderr, "DEBUG TableConfig: %+v\n", cfg)
		}
	}

	return cfg, results, nil
}

// SelectRowsStream evaluates filters and positional pushdown directly over in-memory rows.
func SelectRowsStream(headers []string, rows [][]string, filters []Filter, posOpts PositionalOptions) (MatchedRows, error) {
	plan := PlanPushdown(posOpts)
	if plan.Strategy == StrategyEarlyExit && plan.MaxMatches <= 0 {
		return MatchedRows{}, nil
	}

	var results MatchedRows
	var headResults MatchedRows
	var ring *RingBuffer
	if plan.Strategy == StrategyRingBuffer {
		ring = NewRingBuffer(plan.RingCapacity)
	} else if plan.Strategy == StrategyDualPartition {
		ring = NewRingBuffer(plan.TailCapacity)
		headResults = make(MatchedRows, 0, plan.HeadCapacity)
	}

	matchCount := 0

	for physicalRowIdx, row := range rows {
		matched, err := MatchesRow(headers, row, filters)
		if err != nil {
			return nil, err
		}

		if matched {
			rowItem := MatchedRow{
				Index: physicalRowIdx,
				Row:   row,
			}

			switch plan.Strategy {
			case StrategyEarlyExit:
				if plan.MatchesTarget(matchCount) {
					results = append(results, rowItem)
				}
				matchCount++
				if matchCount >= plan.MaxMatches {
					goto memoryScanComplete
				}

			case StrategyRingBuffer:
				ring.Push(rowItem)
				matchCount++

			case StrategyDualPartition:
				if matchCount < plan.HeadCapacity {
					headResults = append(headResults, rowItem)
				} else {
					ring.Push(rowItem)
				}
				matchCount++

			case StrategyAll:
				results = append(results, rowItem)
				matchCount++
			}
		}
	}

memoryScanComplete:
	switch plan.Strategy {
	case StrategyRingBuffer:
		results = ring.Slice()
	case StrategyDualPartition:
		tailSlice := ring.Slice()
		results = append(headResults, tailSlice...)
	}

	if plan.FallbackSlicing {
		return results.ApplyPositional(plan.OriginalOpts)
	}

	if plan.ApplyWindow {
		start := plan.WindowOffset
		if start > len(results) {
			start = len(results)
		}
		end := len(results)
		if plan.WindowLimit != nil {
			end = start + *plan.WindowLimit
			if end > len(results) {
				end = len(results)
			}
		}
		results = results[start:end]
	}

	return results, nil
}
