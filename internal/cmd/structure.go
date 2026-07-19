package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/roman-pipik/rowlite/internal/core"
)

func NewStructureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "structure <file>",
		Short: "Display table column headers and frontmatter constraints",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			cfg, _, err := core.ParseFrontmatterAndContent(cmd.Context(), filePath, core.ContentModeNone)
			if err != nil {
				return err
			}
			headers := cfg.Headers
			align := cfg.Alignments
			if len(headers) == 0 {
				return fmt.Errorf("table headers not found in %s", filePath)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Columns: %s\n", strings.Join(headers, ", "))
			if len(align) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Alignments: %s\n", strings.Join(align, ", "))
			}
			if len(cfg.Unique) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Unique Keys: %s\n", strings.Join(cfg.Unique, ", "))
			}
			if cfg.Sort != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Sort Configuration: %s\n", cfg.Sort)
			}
			if cfg.Audit.Insert != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Audit Insert Column: %s\n", cfg.Audit.Insert)
			}
			if cfg.Audit.Edit != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Audit Edit Column: %s\n", cfg.Audit.Edit)
			}
			if cfg.JournalLog != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Journal Log: %s\n", cfg.JournalLog)
			}
			return nil
		},
	}

	return cmd
}
