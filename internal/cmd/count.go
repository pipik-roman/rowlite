package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pipik-roman/rowlite/internal/core"
)

func NewCountCmd() *cobra.Command {
	const whereFlagName = "where"
	var whereFlags []string
	var headFlag int
	var tailFlag int
	var indexFlag string
	var offsetFlag int
	var limitFlag int

	cmd := &cobra.Command{
		Use:   "count <file>",
		Short: "Count matching rows from table",
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

			count, err := core.CountWithOptions(cmd.Context(), filePath, whereFlags, posOpts)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), count)
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&whereFlags, whereFlagName, nil, "WHERE conditions (supported operators: ==, !=, like, regex)")
	cmd.Flags().IntVarP(&headFlag, "head", "H", 0, "Count bounded to first N rows")
	cmd.Flags().IntVarP(&tailFlag, "tail", "T", 0, "Count bounded to last N rows")
	cmd.Flags().StringVarP(&indexFlag, "index", "i", "", "Target 0-based logical row index, list (0,2,4), range (2..10, 2:10), or stepped slice (0..20..2)")
	cmd.Flags().IntVar(&offsetFlag, "offset", 0, "Zero-based row offset for pagination")
	cmd.Flags().IntVarP(&limitFlag, "limit", "l", 0, "Maximum count limit")

	return cmd
}
