package cmd

import (
	"github.com/spf13/cobra"

	"github.com/roman-pipik/rowlite/internal/core"
)

const (
	byFlagName      = "by"
	triggerFlagName = "trigger"
)

func NewSortCmd() *cobra.Command {
	var (
		byRule  string
		trigger bool
	)
	cmd := &cobra.Command{
		Use:   "sort <file>",
		Short: "Sort table in-place",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			return core.SortTableAction(cmd.Context(), filePath, byRule, trigger)
		},
	}

	cmd.Flags().StringVar(&byRule, byFlagName, "", "override sort rule (e.g., id:desc)")
	cmd.Flags().BoolVar(&trigger, triggerFlagName, false, "trigger sorting by updating frontmatter permanently")

	return cmd
}
