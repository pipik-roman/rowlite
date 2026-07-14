package core

import (
	"bufio"
	"io"
	"os"
)

// AppendRowDirectly appends a row string directly to the end of the file.
// It verifies the file ends with a newline first.
func AppendRowDirectly(filePath string, row []string) error {
	formatted := FormatGFMLine(row)

	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if stat.Size() > 0 {
		_, err = file.Seek(-1, io.SeekEnd)
		if err != nil {
			return err
		}
		buf := make([]byte, 1)
		_, err = file.Read(buf)
		if err != nil {
			return err
		}
		if buf[0] != '\n' {
			_, err = file.WriteString("\n")
			if err != nil {
				return err
			}
		}
	}

	_, err = file.WriteString(formatted + "\n")
	return err
}

// AtomicWriteTable writes headers, separator, and rows atomically using a temp file.
func AtomicWriteTable(filePath string, headers []string, alignments []string, rows [][]string) error {
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

	_, err = writer.WriteString(FormatGFMLine(headers) + "\n")
	if err != nil {
		return err
	}

	_, err = writer.WriteString(FormatGFMLine(alignments) + "\n")
	if err != nil {
		return err
	}

	for _, row := range rows {
		_, err = writer.WriteString(FormatGFMLine(row) + "\n")
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
