package core

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// RowliteTable holds parsed GFM table cells and column metadata (full-memory model)
type RowliteTable struct {
	Headers    []string
	Rows       [][]string
	Alignments []string
}

// SplitGFMLine parses a GFM table row line, splitting columns by unescaped pipe symbols.
func SplitGFMLine(line string) ([]string, error) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty line")
	}
	if trimmed[0] != '|' || trimmed[len(trimmed)-1] != '|' {
		return nil, fmt.Errorf("GFM table row must start and end with '|'")
	}

	var cells []string
	var currentCell bytes.Buffer

	for i := 1; i < len(trimmed)-1; i++ {
		c := trimmed[i]
		if c == '\\' {
			// Check if the next character is a pipe or a backslash
			if i+1 < len(trimmed)-1 && (trimmed[i+1] == '|' || trimmed[i+1] == '\\') {
				currentCell.WriteByte(trimmed[i+1])
				i++
				continue
			}
		}
		if c == '|' {
			cells = append(cells, strings.TrimSpace(currentCell.String()))
			currentCell.Reset()
		} else {
			currentCell.WriteByte(c)
		}
	}
	cells = append(cells, strings.TrimSpace(currentCell.String()))

	return cells, nil
}

// FormatGFMLine formats a list of cell strings into a standard GFM pipe-separated line.
func FormatGFMLine(cells []string) string {
	var buf bytes.Buffer
	buf.WriteByte('|')
	for _, cell := range cells {
		buf.WriteByte(' ')
		for i := 0; i < len(cell); i++ {
			c := cell[i]
			if c == '|' {
				buf.WriteString(`\|`)
			} else if c == '\\' {
				buf.WriteString(`\\`)
			} else {
				buf.WriteByte(c)
			}
		}
		buf.WriteString(" |")
	}
	return buf.String()
}

// IsSeparatorLine checks if a parsed set of GFM cells corresponds to a table header separator.
func IsSeparatorLine(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		trimmed := strings.TrimSpace(cell)
		if len(trimmed) == 0 {
			return false
		}
		hasDash := false
		for i := 0; i < len(trimmed); i++ {
			c := trimmed[i]
			if c == '-' {
				hasDash = true
			} else if c == ':' {
				// Alignment marker, allowed
			} else {
				return false
			}
		}
		if !hasDash {
			return false
		}
	}
	return true
}

// ScanTableHeaders reads a file line-by-line and returns:
// - headers: list of header names
// - alignments: raw alignment cells
// - headerLineNum: 1-based line number of the header row (row 0 of table)
// - error: any parsing error or table-not-found error
func ScanTableHeaders(filePath string) (headers []string, alignments []string, headerLineNum int, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, 0, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	lineNum := 0
	var lastCells []string
	var lastLineNum int

	for {
		lineNum++
		lineBytes, isPrefix, readErr := reader.ReadLine()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, nil, 0, readErr
		}
		var lineBuf bytes.Buffer
		lineBuf.Write(lineBytes)
		for isPrefix {
			var moreBytes []byte
			moreBytes, isPrefix, readErr = reader.ReadLine()
			if readErr != nil && readErr != io.EOF {
				return nil, nil, 0, readErr
			}
			lineBuf.Write(moreBytes)
		}
		line := lineBuf.String()
		trimmed := strings.TrimSpace(line)

		if len(trimmed) > 0 && trimmed[0] == '|' && strings.Count(trimmed, "|") >= 2 {
			cells, parseErr := SplitGFMLine(line)
			if parseErr == nil {
				if lastCells != nil && IsSeparatorLine(cells) {
					if len(cells) != len(lastCells) {
						return nil, nil, 0, fmt.Errorf("separator row column count (%d) mismatch with header row column count (%d)", len(cells), len(lastCells))
					}
					return lastCells, cells, lastLineNum, nil
				}
				lastCells = cells
				lastLineNum = lineNum
			} else {
				lastCells = nil
				lastLineNum = 0
			}
		} else {
			lastCells = nil
			lastLineNum = 0
		}
	}

	return nil, nil, 0, fmt.Errorf("no GFM table found in %s", filePath)
}
