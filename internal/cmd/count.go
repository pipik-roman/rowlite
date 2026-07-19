package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/roman-pipik/rowlite/internal/core"
)

func NewCountCmd() *cobra.Command {
	const whereFlagName = "where"
	var whereFlags []string
	cmd := &cobra.Command{
		Use:   "count <file>",
		Short: "Count matching rows from table",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			count, err := core.Count(cmd.Context(), filePath, whereFlags)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), count)
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&whereFlags, whereFlagName, nil, "WHERE conditions")

	return cmd
}
