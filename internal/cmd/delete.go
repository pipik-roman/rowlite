package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pipik-roman/rowlite/internal/core"
)

func NewDeleteCmd() *cobra.Command {
	var whereFlags []string
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
		Use:   "delete <file>",
		Short: "Delete matching rows from table (supports --where, positional slicing, and OCC)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

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

			count, err := core.DeleteWithOptions(cmd.Context(), filePath, whereFlags, posOpts, expectedHash, allFlag, dryRunFlag)
			if err != nil {
				return err
			}

			if dryRunFlag {
				fmt.Fprintf(cmd.OutOrStdout(), "Dry run: would delete %d row(s) from %s\n", count, filePath)
				return nil
			}

			if isVerboseOrDebug(cmd) {
				fmt.Fprintf(cmd.OutOrStdout(), "Successfully deleted %d row(s) from %s\n", count, filePath)
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&whereFlags, "where", nil, "WHERE filter conditions (e.g. 'status == Closed', 'tags like archive')")
	cmd.Flags().IntVarP(&headFlag, "head", "H", 0, "Delete only first N matching rows")
	cmd.Flags().IntVarP(&tailFlag, "tail", "T", 0, "Delete only last N matching rows")
	cmd.Flags().StringVarP(&indexFlag, "index", "i", "", "Target 0-based row index (e.g. 0, -1), list (0,2,4), range (2..10), or stepped slice (0..20..2)")
	cmd.Flags().IntVar(&offsetFlag, "offset", 0, "Zero-based row offset for pagination")
	cmd.Flags().IntVarP(&limitFlag, "limit", "l", 0, "Maximum number of matching rows to delete")
	cmd.Flags().StringVar(&ifMatchFlag, "if-match", "", "Precondition table checksum (ETag) for optimistic concurrency control")
	cmd.Flags().StringVar(&ifChecksumFlag, "if-checksum", "", "Alias for --if-match (precondition table checksum)")
	cmd.Flags().StringVar(&checksumFlag, "checksum", "", "Alias for --if-match (precondition table checksum)")
	cmd.Flags().BoolVar(&allFlag, "all", false, "Allow deleting all rows when no filter or positional selector is provided")
	cmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Preview the number of matching rows to delete without modifying the file")
	cmd.Flags().BoolP("verbose", "v", false, "Print verbose confirmation message on success")

	return cmd
}
