package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipik-roman/rowlite/internal/core"
)

func NewUpdateCmd() *cobra.Command {
	var whereFlags []string
	var setFlags []string
	var dataFlag string
	var headFlag int
	var tailFlag int
	var indexFlag string
	var offsetFlag int
	var limitFlag int
	var ifMatchFlag string
	var ifChecksumFlag string
	var checksumFlag string
	var allFlag bool
	var dryRunFlag bool

	cmd := &cobra.Command{
		Use:   "update <file> [data-string]",
		Short: "Update matching rows in table (supports --where, --set, --data, and positional slicing)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			var updates map[string]string
			var err error

			// 1. Determine updates from args[1], --data, --set, or stdin
			if len(args) == 2 {
				updates, err = core.ParseUpdateData(args[1])
				if err != nil {
					return err
				}
			} else if dataFlag != "" {
				updates, err = core.ParseUpdateData(dataFlag)
				if err != nil {
					return err
				}
			} else if len(setFlags) > 0 {
				updates, err = core.ParseSetExpressions(setFlags)
				if err != nil {
					return err
				}
			} else {
				// Check if piped from stdin
				stat, _ := os.Stdin.Stat()
				if (stat.Mode() & os.ModeCharDevice) == 0 {
					bytes, readErr := io.ReadAll(os.Stdin)
					if readErr == nil && len(strings.TrimSpace(string(bytes))) > 0 {
						updates, err = core.ParseUpdateData(string(bytes))
						if err != nil {
							return err
						}
					}
				}
			}

			if len(updates) == 0 {
				return fmt.Errorf("no update assignments provided; specify via --set col=val, --data '{\"col\":\"val\"}', positional argument, or stdin")
			}

			var posOpts core.PositionalOptions
			if cmd.Flags().Changed("head") {
				posOpts.Head = &headFlag
			}
			if cmd.Flags().Changed("tail") {
				posOpts.Tail = &tailFlag
			}
			if cmd.Flags().Changed("index") {
				posOpts.Index = indexFlag
			}
			if cmd.Flags().Changed("offset") {
				posOpts.Offset = offsetFlag
			}
			if cmd.Flags().Changed("limit") {
				posOpts.Limit = &limitFlag
			}

			expectedHash := ifMatchFlag
			if expectedHash == "" {
				if ifChecksumFlag != "" {
					expectedHash = ifChecksumFlag
				} else if checksumFlag != "" {
					expectedHash = checksumFlag
				}
			}

			count, err := core.UpdateWithOptions(cmd.Context(), filePath, whereFlags, posOpts, updates, expectedHash, allFlag, dryRunFlag)
			if err != nil {
				return err
			}

			if dryRunFlag {
				fmt.Fprintf(cmd.OutOrStdout(), "Dry run: would update %d row(s) in %s\n", count, filePath)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Successfully updated %d row(s) in %s\n", count, filePath)
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&whereFlags, "where", nil, "WHERE filter conditions (e.g. 'status == Pending', 'tags like beach')")
	cmd.Flags().StringSliceVarP(&setFlags, "set", "s", nil, "Column update assignments (e.g. --set status=Done --set 'priority=High')")
	cmd.Flags().StringVarP(&dataFlag, "data", "d", "", "JSON or key=val update string (e.g. '{\"status\":\"Done\"}')")
	cmd.Flags().IntVarP(&headFlag, "head", "H", 0, "Update only first N matching rows")
	cmd.Flags().IntVarP(&tailFlag, "tail", "T", 0, "Update only last N matching rows")
	cmd.Flags().StringVarP(&indexFlag, "index", "i", "", "Target 0-based row index (e.g. 0, -1), list (0,2,4), range (2..10), or stepped slice (0..20..2)")
	cmd.Flags().IntVar(&offsetFlag, "offset", 0, "Zero-based row offset for pagination")
	cmd.Flags().IntVarP(&limitFlag, "limit", "l", 0, "Maximum number of matching rows to update")
	cmd.Flags().StringVar(&ifMatchFlag, "if-match", "", "Precondition table checksum (ETag) for optimistic concurrency control")
	cmd.Flags().StringVar(&ifChecksumFlag, "if-checksum", "", "Alias for --if-match (precondition table checksum)")
	cmd.Flags().StringVar(&checksumFlag, "checksum", "", "Alias for --if-match (precondition table checksum)")
	cmd.Flags().BoolVar(&allFlag, "all", false, "Allow updating all rows when no filter or positional selector is provided")
	cmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Preview the number of matching rows to update without modifying the file")

	return cmd
}
