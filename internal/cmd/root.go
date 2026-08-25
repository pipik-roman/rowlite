package cmd

import (
	"github.com/spf13/cobra"
	"github.com/pipik-roman/rowlite/internal/core"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func NewRootCmd() *cobra.Command {
	var debugVal bool

	cmd := &cobra.Command{
		Use:     "rowlite",
		Version: Version,
		Short:   "Rowlite is a flat Markdown database CLI and MCP server",
		Long:    `A zero-config, lightweight command line interface and MCP server to query and edit Markdown GFM tables.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			debugFlag, err := cmd.Flags().GetBool("debug")
			if err == nil {
				cmd.SetContext(core.WithDebug(cmd.Context(), debugFlag))
			}
			return nil
		},
	}

	cmd.PersistentFlags().BoolVar(&debugVal, "debug", false, "print detailed table scanning diagnostics and resolved table configuration metadata to stderr")

	cmd.AddCommand(NewInitCmd())
	cmd.AddCommand(NewListCmd())
	cmd.AddCommand(NewCountCmd())
	cmd.AddCommand(NewAppendCmd())
	cmd.AddCommand(NewUpdateCmd())
	cmd.AddCommand(NewDeleteCmd())
	cmd.AddCommand(NewSortCmd())
	cmd.AddCommand(NewStructureCmd())
	cmd.AddCommand(NewMcpCmd())
	cmd.AddCommand(NewSetupMCPCmd())

	return cmd
}
