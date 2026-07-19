package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/roman-pipik/rowlite/internal/core"
)

func NewListCmd() *cobra.Command {
	const whereFlagName = "where"
	var whereFlags []string
	cmd := &cobra.Command{
		Use:   "list <file>",
		Short: "List matching rows from table",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			var filters []core.Filter
			for _, wf := range whereFlags {
				flt, err := core.ParseFilter(wf)
				if err != nil {
					return err
				}
				filters = append(filters, flt)
			}

			cfg, rows, err := core.ParseFrontmatterAndContent(cmd.Context(), filePath, core.ContentModeAll)
			if err != nil {
				return err
			}
			headers := cfg.Headers
			if len(headers) == 0 {
				return fmt.Errorf("no gfm table headers found in %s", filePath)
			}

			fmt.Fprintln(cmd.OutOrStdout(), core.FormatGFMLine(headers))
			seps := make([]string, len(headers))
			for i := range seps {
				seps[i] = "---"
			}
			fmt.Fprintln(cmd.OutOrStdout(), core.FormatGFMLine(seps))

			for _, row := range rows {
				match := true
				for _, flt := range filters {
					m, err := core.EvaluateFilter(headers, row, flt)
					if err != nil {
						return err
					}
					if !m {
						match = false
						break
					}
				}
				if match {
					fmt.Fprintln(cmd.OutOrStdout(), core.FormatGFMLine(row))
				}
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&whereFlags, whereFlagName, nil, "WHERE conditions")

	return cmd
}
