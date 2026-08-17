package core

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PositionalOptions defines query parameters for positional row selections.
type PositionalOptions struct {
	Head             *int   // First N rows (>= 1)
	Tail             *int   // Last N rows (>= 1)
	Index            string // Single int ("0", "-1"), array ("0,2,4"), range ("2..10", "2:10"), step ("0..20..2", "0:20:2")
	Offset           int    // 0-based row offset for pagination (>= 0)
	Limit            *int   // Maximum number of rows to return (>= 1)
	IncludeRowNumber bool   // Whether to include visual/metadata __row_index__
}

// IsZero returns true if no positional filtering or slicing options are set.
func (p PositionalOptions) IsZero() bool {
	return p.Head == nil && p.Tail == nil && strings.TrimSpace(p.Index) == "" && p.Offset == 0 && p.Limit == nil && !p.IncludeRowNumber
}

// normalizeRelativeIndex converts negative index (relative to totalRows) to 0-based index.
func normalizeRelativeIndex(idx int, totalRows int) (int, error) {
	if idx < 0 {
		idx = totalRows + idx
	}
	if idx < 0 || idx >= totalRows {
		return 0, fmt.Errorf("row index %d out of range (table has %d rows, valid range 0..%d or -%d..-1)", idx, totalRows, totalRows-1, totalRows)
	}
	return idx, nil
}

var (
	rangeRegex = regexp.MustCompile(`^(-?\d+)(?:\.\.|:)(-?\d+)(?:(?:\.\.|:)(-?\d+))?$`)
)

// ParseIndexSpec parses a flexible index specification against the given totalRows.
// Supports:
//   - Single integer: "0", "5", "-1"
//   - Comma-separated or JSON list: "0, 2, 4", "[0, 2, 4]"
//   - Continuous range: "2..10", "2:10", "-5..-1"
//   - Stepped range: "0..20..2", "0:20:2", "-10..-1..2"
func ParseIndexSpec(spec string, totalRows int) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	if totalRows <= 0 {
		return nil, fmt.Errorf("cannot target row index in an empty table")
	}

	// 1. Try JSON array of integers: [0, 2, 4]
	if strings.HasPrefix(spec, "[") && strings.HasSuffix(spec, "]") {
		var arr []int
		if err := json.Unmarshal([]byte(spec), &arr); err == nil {
			result := make([]int, 0, len(arr))
			for _, val := range arr {
				norm, err := normalizeRelativeIndex(val, totalRows)
				if err != nil {
					return nil, err
				}
				result = append(result, norm)
			}
			return result, nil
		}
	}

	// 2. Try continuous or stepped range: "2..10", "2:10", "0..20..2", "0:20:2"
	if matches := rangeRegex.FindStringSubmatch(spec); matches != nil {
		startRaw, _ := strconv.Atoi(matches[1])
		endRaw, _ := strconv.Atoi(matches[2])
		step := 1
		if matches[3] != "" {
			var err error
			step, err = strconv.Atoi(matches[3])
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step value %q in slice expression (must be a positive integer)", matches[3])
			}
		}

		startNorm, err := normalizeRelativeIndex(startRaw, totalRows)
		if err != nil {
			return nil, fmt.Errorf("invalid range start: %w", err)
		}
		endNorm, err := normalizeRelativeIndex(endRaw, totalRows)
		if err != nil {
			return nil, fmt.Errorf("invalid range end: %w", err)
		}

		var result []int
		if startNorm <= endNorm {
			for i := startNorm; i <= endNorm; i += step {
				result = append(result, i)
			}
		} else {
			// Descending range if start > end
			for i := startNorm; i >= endNorm; i -= step {
				result = append(result, i)
			}
		}
		return result, nil
	}

	// 3. Try comma-separated list of integers: "0, 2, 4" or single integer "3"
	parts := strings.Split(spec, ",")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		val, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid index specification %q: %w", spec, err)
		}
		norm, err := normalizeRelativeIndex(val, totalRows)
		if err != nil {
			return nil, err
		}
		result = append(result, norm)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("invalid empty index specification %q", spec)
	}

	return result, nil
}

// ResolvePositionalIndices evaluates PositionalOptions over totalRows to return an ordered slice of target row indices.
func ResolvePositionalIndices(totalRows int, opts PositionalOptions) ([]int, error) {
	if opts.Offset < 0 {
		return nil, fmt.Errorf("offset cannot be negative (got %d)", opts.Offset)
	}
	if opts.Limit != nil && *opts.Limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than 0 (got %d)", *opts.Limit)
	}
	if opts.Head != nil && *opts.Head <= 0 {
		return nil, fmt.Errorf("head must be greater than 0 (got %d)", *opts.Head)
	}
	if opts.Tail != nil && *opts.Tail <= 0 {
		return nil, fmt.Errorf("tail must be greater than 0 (got %d)", *opts.Tail)
	}

	if totalRows <= 0 {
		return []int{}, nil
	}

	var indices []int

	// Precedence 1: Explicit Index specifier
	if strings.TrimSpace(opts.Index) != "" {
		var err error
		indices, err = ParseIndexSpec(opts.Index, totalRows)
		if err != nil {
			return nil, err
		}
	} else if opts.Head != nil || opts.Tail != nil {
		// Precedence 2: Head and/or Tail
		headVal := 0
		if opts.Head != nil {
			headVal = *opts.Head
		}
		tailVal := 0
		if opts.Tail != nil {
			tailVal = *opts.Tail
		}

		if opts.Head != nil && opts.Tail == nil {
			count := headVal
			if count > totalRows {
				count = totalRows
			}
			indices = make([]int, count)
			for i := 0; i < count; i++ {
				indices[i] = i
			}
		} else if opts.Head == nil && opts.Tail != nil {
			count := tailVal
			if count > totalRows {
				count = totalRows
			}
			start := totalRows - count
			indices = make([]int, count)
			for i := 0; i < count; i++ {
				indices[i] = start + i
			}
		} else {
			// Both Head and Tail specified
			if headVal+tailVal >= totalRows {
				indices = make([]int, totalRows)
				for i := 0; i < totalRows; i++ {
					indices[i] = i
				}
			} else {
				topCount := headVal
				botCount := tailVal
				indices = make([]int, 0, topCount+botCount)
				for i := 0; i < topCount; i++ {
					indices = append(indices, i)
				}
				botStart := totalRows - botCount
				for i := 0; i < botCount; i++ {
					indices = append(indices, botStart+i)
				}
			}
		}
	} else {
		// Precedence 3: Default to all rows
		indices = make([]int, totalRows)
		for i := 0; i < totalRows; i++ {
			indices[i] = i
		}
	}

	// Apply Offset and Limit window
	if opts.Offset > 0 {
		if opts.Offset >= len(indices) {
			return []int{}, nil
		}
		indices = indices[opts.Offset:]
	}

	if opts.Limit != nil {
		lim := *opts.Limit
		if lim < len(indices) {
			indices = indices[:lim]
		}
	}

	return indices, nil
}

// ApplyPositional applies positional options (head, tail, index, offset, limit) to a MatchedRows collection.
func (m MatchedRows) ApplyPositional(opts PositionalOptions) (MatchedRows, error) {
	if len(m) == 0 {
		return MatchedRows{}, nil
	}

	targetIndices, err := ResolvePositionalIndices(len(m), opts)
	if err != nil {
		return nil, err
	}

	result := make(MatchedRows, len(targetIndices))
	for i, idx := range targetIndices {
		result[i] = m[idx]
	}
	return result, nil
}
