package cmd

import (
	"github.com/spf13/cobra"
	"github.com/roman-pipik/rowlite/internal/core"
)

func NewRootCmd() *cobra.Command {
	var debugVal bool

	cmd := &cobra.Command{
		Use:     "rowlite",
		Version: "0.1.0",
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

	cmd.PersistentFlags().BoolVar(&debugVal, "debug", false, "print resolved TableConfig metadata")

	cmd.AddCommand(NewInitCmd())
	cmd.AddCommand(NewListCmd())
	cmd.AddCommand(NewCountCmd())
	cmd.AddCommand(NewAppendCmd())
	cmd.AddCommand(NewSortCmd())
	cmd.AddCommand(NewStructureCmd())
	cmd.AddCommand(NewMcpCmd())

	return cmd
}
