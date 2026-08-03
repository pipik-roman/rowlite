package cmd

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipik-roman/rowlite/internal/core"
)

const (
	columnsFlag        = "columns"
	uniqueFlag         = "unique"
	sortFlag           = "sort"
	auditInsertFlag    = "audit-insert"
	auditEditFlag      = "audit-edit"
	journalLogFlag     = "journal-log"
	defaultAuditInsert = "created_at"
	defaultAuditEdit   = "updated_at"
)

func NewInitCmd() *cobra.Command {
	var (
		columnsVal     string
		uniqueVal      string
		sortVal        string
		auditInsertVal string
		auditEditVal   string
		journalVal     string
	)
	cmd := &cobra.Command{
		Use:   "init <file>",
		Short: "Initialize table frontmatter constraints",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			var unique []string
			if uniqueVal != "" {
				unique = strings.Split(uniqueVal, ",")
				for i := range unique {
					unique[i] = strings.TrimSpace(unique[i])
				}
			}

			cfg := core.TableConfig{
				Unique:      unique,
				Sort:        sortVal,
				Audit: core.AuditConfig{
					Insert: auditInsertVal,
					Edit:   auditEditVal,
				},
				JournalLog:  journalVal,
			}

			fileExists := false
			if _, err := os.Stat(filePath); err == nil {
				fileExists = true
			}

			var columns []string
			if columnsVal == "" {
				if !fileExists {
					return fmt.Errorf("--columns flag is required when creating a new table")
				}
				// Parse existing headers to get the columns list
				cfgParsed, _, err := core.ParseFrontmatterAndContent(cmd.Context(), filePath, core.ContentModeNone)
				if err != nil {
					return fmt.Errorf("failed to parse existing table structure: %w", err)
				}
				headers := cfgParsed.Headers
				if len(headers) == 0 {
					return fmt.Errorf("existing file has no table headers; please specify --columns to initialize a table")
				}
				// Filter out audit columns because InitTable will append them automatically
				for _, h := range headers {
					if !core.IsAuditColumn(h, cfg) {
						columns = append(columns, h)
					}
				}
			} else {
				columns = strings.Split(columnsVal, ",")
				for i := range columns {
					columns[i] = strings.TrimSpace(columns[i])
				}
				if fileExists {
					// Validate that columns match the existing table structure
					cfgParsed, _, err := core.ParseFrontmatterAndContent(cmd.Context(), filePath, core.ContentModeNone)
					if err == nil && len(cfgParsed.Headers) > 0 {
						var existingUserCols []string
						for _, h := range cfgParsed.Headers {
							if !core.IsAuditColumn(h, cfg) {
								existingUserCols = append(existingUserCols, h)
							}
						}
						if !reflect.DeepEqual(columns, existingUserCols) {
							return fmt.Errorf("specified columns %v do not match existing table structure %v; run without --columns flag to preserve structure", columns, existingUserCols)
						}
					}
				}
			}

			return core.InitTable(cmd.Context(), filePath, columns, cfg)
		},
	}

	cmd.Flags().StringVar(&columnsVal, columnsFlag, "", "ordered columns (comma-separated)")
	cmd.Flags().StringVar(&uniqueVal, uniqueFlag, "", "unique key columns (comma-separated)")
	cmd.Flags().StringVar(&sortVal, sortFlag, "", "frontmatter sort rule (e.g., id:asc)")
	cmd.Flags().StringVar(&auditInsertVal, auditInsertFlag, "", "enable creation timestamp auditing with custom column name")
	cmd.Flags().Lookup(auditInsertFlag).NoOptDefVal = defaultAuditInsert

	cmd.Flags().StringVar(&auditEditVal, auditEditFlag, "", "enable modification timestamp auditing with custom column name")
	cmd.Flags().Lookup(auditEditFlag).NoOptDefVal = defaultAuditEdit
	cmd.Flags().StringVar(&journalVal, journalLogFlag, "", "path to change journal log file")

	return cmd
}
