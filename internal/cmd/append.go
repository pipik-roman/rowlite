package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/roman-pipik/rowlite/internal/core"
)

const rowFlagName = "row"

func NewAppendCmd() *cobra.Command {
	var rowFlag string
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

			return core.Append(cmd.Context(), filePath, dataStr)
		},
	}

	cmd.Flags().StringVar(&rowFlag, rowFlagName, "", "JSON row object to append")

	return cmd
}
