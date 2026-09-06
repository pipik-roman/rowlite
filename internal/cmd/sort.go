package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pipik-roman/rowlite/internal/core"
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
		Long: `Sort table in-place using a sort rule or pre-configured frontmatter sort settings.

If --by is omitted, rowlite uses the frontmatter "sort" rule configured in the table.
Multiple columns can be specified as a comma-separated list (e.g. "category:asc,priority:desc").`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			if err := core.SortTableAction(cmd.Context(), filePath, byRule, trigger); err != nil {
				return err
			}
			if isVerboseOrDebug(cmd) {
				fmt.Fprintf(cmd.OutOrStdout(), "Successfully sorted table in %s\n", filePath)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&byRule, byFlagName, "", "comma-separated sort rule override (e.g., \"category:asc,priority:desc\"); if omitted, frontmatter sort rule is used")
	cmd.Flags().BoolVar(&trigger, triggerFlagName, false, "update frontmatter sort constraint permanently to trigger auto-sort on future writes")
	cmd.Flags().BoolP("verbose", "v", false, "print verbose confirmation message on success")

	return cmd
}
