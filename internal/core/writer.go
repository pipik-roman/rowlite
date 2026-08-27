package core

import (
	"bufio"
	"context"
	"os"
	"strings"
)

// AtomicWriteTable writes frontmatter config, headers, separator, and rows atomically using a temp file.
// It preserves unmodified surrounding content (non-table text and other frontmatter keys) if the table exists.
func AtomicWriteTable(ctx context.Context, filePath string, cfg TableConfig, rows [][]string) error {
	var beforeLines []string
	var afterLines []string
	hasExistingTable := false

	if _, err := os.Stat(filePath); err == nil {
		_, _, headerLineNum, scanErr := ScanTableHeaders(ctx, filePath)
		if scanErr == nil {
			// Partition the file into beforeLines and afterLines
			file, err := os.Open(filePath)
			if err == nil {
				scanner := bufio.NewScanner(file)
				lineNum := 0
				tableEnded := false

				for scanner.Scan() {
					lineNum++
					line := scanner.Text()

					if lineNum < headerLineNum {
						beforeLines = append(beforeLines, line)
						continue
					}

					if !tableEnded {
						if lineNum == headerLineNum || lineNum == headerLineNum+1 {
							// skip headers and separator
							continue
						}
						// For lines after separator, check if they are table rows
						trimmed := strings.TrimSpace(line)
						if isTableRow(trimmed) {
							// skip table rows
							continue
						}
						tableEnded = true
					}

					afterLines = append(afterLines, line)
				}
				file.Close() // Close file handle immediately to prevent sharing violation on Windows
				if err := scanner.Err(); err == nil {
					hasExistingTable = true
				}
			}
		}
	}

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

	if hasExistingTable {
		// Write the original unmodified content before the table
		for _, line := range beforeLines {
			_, err = writer.WriteString(line)
			if err == nil {
				_, err = writer.WriteString("\n")
			}
			if err != nil {
				return err
			}
		}
	} else {
		// Write config frontmatter from scratch if no existing table/file
		frontmatterStr := SerializeConfig(cfg)
		if frontmatterStr != "" {
			_, err = writer.WriteString(frontmatterStr)
			if err != nil {
				return err
			}
		}
	}

	// Write table headers
	_, err = writer.WriteString(FormatGFMLine(cfg.Headers))
	if err == nil {
		_, err = writer.WriteString("\n")
	}
	if err != nil {
		return err
	}

	// Write table alignments
	_, err = writer.WriteString(FormatGFMLine(cfg.Alignments))
	if err == nil {
		_, err = writer.WriteString("\n")
	}
	if err != nil {
		return err
	}

	// Write table rows
	for _, row := range rows {
		_, err = writer.WriteString(FormatGFMLine(row))
		if err == nil {
			_, err = writer.WriteString("\n")
		}
		if err != nil {
			return err
		}
	}

	if hasExistingTable {
		// Write the original unmodified content after the table
		for _, line := range afterLines {
			_, err = writer.WriteString(line)
			if err == nil {
				_, err = writer.WriteString("\n")
			}
			if err != nil {
				return err
			}
		}
	}

	err = writer.Flush()
	if err != nil {
		return err
	}
	tempFile.Close()

	return os.Rename(tempPath, filePath)
}
