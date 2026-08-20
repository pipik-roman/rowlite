package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipik-roman/rowlite/internal/core"
)

const (
	rowFlagName     = "row"
	ifMatchFlagName = "if-match"
)

func NewAppendCmd() *cobra.Command {
	var rowFlag string
	var ifMatchFlag string
	var ifChecksumFlag string
	var checksumFlag string
	cmd := &cobra.Command{
		Use:   "append <file> [data-string]",
		Short: "Append row values to table (supports JSON/CSV strings and stdin)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			var dataStr string

			if len(args) == 2 {
				dataStr = args[1]
			} else {
				if rowFlag != "" {
					dataStr = rowFlag
				} else {
					// Read from stdin if piped
					stat, _ := os.Stdin.Stat()
					if (stat.Mode() & os.ModeCharDevice) == 0 {
						bytes, err := io.ReadAll(os.Stdin)
						if err == nil {
							dataStr = string(bytes)
						}
					}
				}
			}

			dataStr = strings.TrimSpace(dataStr)
			if dataStr == "" {
				return fmt.Errorf("data string content is required either as a positional argument, via --row flag, or piped to stdin")
			}

			expectedHash := ifMatchFlag
			if expectedHash == "" {
				if ifChecksumFlag != "" {
					expectedHash = ifChecksumFlag
				} else if checksumFlag != "" {
					expectedHash = checksumFlag
				}
			}

			return core.AppendWithOptions(cmd.Context(), filePath, dataStr, expectedHash)
		},
	}

	cmd.Flags().StringVar(&rowFlag, rowFlagName, "", "JSON row object to append")
	cmd.Flags().StringVar(&ifMatchFlag, ifMatchFlagName, "", "Precondition table checksum (ETag) for optimistic concurrency control")
	cmd.Flags().StringVar(&ifChecksumFlag, "if-checksum", "", "Alias for --if-match (precondition table checksum)")
	cmd.Flags().StringVar(&checksumFlag, "checksum", "", "Alias for --if-match (precondition table checksum)")

	return cmd
}
