package cmd

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pipik-roman/rowlite/internal/core"
)

const (
	columnsFlag        = "columns"
	uniqueFlag         = "unique"
	sortFlag           = "sort"
	auditFlag          = "audit"
	auditInsertFlag    = "audit-insert"
	auditEditFlag      = "audit-edit"
	journalLogFlag     = "journal-log"
	autoincrementFlag  = "autoincrement"
	defaultAuditInsert = "created_at"
	defaultAuditEdit   = "updated_at"
)

func NewInitCmd() *cobra.Command {
	var (
		columnsVal       string
		uniqueVal        string
		sortVal          string
		auditVal         bool
		auditInsertVal   string
		auditEditVal     string
		journalVal       string
		autoincrementVal string
	)
	cmd := &cobra.Command{
		Use:   "init <file>",
		Short: "Initialize table frontmatter constraints and headers",
		Long: `Create a new markdown table file or update frontmatter constraints in-place.
Supports column definitions, autoincrement integer IDs, automatic creation/modification timestamps, unique keys, sort rules, and change journaling.`,
		Example: `  # Initialize a new table with column schema, autoincrement ID, and audit timestamps
  rowlite init tasks.md --columns "id,task,status,priority" --autoincrement "id" --audit

  # Initialize with custom autoincrement step, unique keys, and sort rule
  rowlite init tasks.md --columns "id,task,status" --autoincrement "id:10" --unique "id" --sort "id:asc" --audit

  # Update constraints on an existing markdown table in-place
  rowlite init existing_table.md --autoincrement "id" --audit

  # Shorthand flag aliases are also available (-c, -a, -i, -e, or combined -ie)
  rowlite init tasks.md -c id,task,status -a id -ie`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			if auditVal {
				if auditInsertVal == "" {
					auditInsertVal = defaultAuditInsert
				}
				if auditEditVal == "" {
					auditEditVal = defaultAuditEdit
				}
			}

			var unique []string
			if uniqueVal != "" {
				unique = strings.Split(uniqueVal, ",")
				for i := range unique {
					unique[i] = strings.TrimSpace(unique[i])
				}
			}

			autoIncCol, autoIncStep := "", 0
			if autoincrementVal != "" {
				parts := strings.SplitN(autoincrementVal, ":", 2)
				autoIncCol = strings.TrimSpace(parts[0])
				autoIncStep = 1
				if len(parts) > 1 {
					if step, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && step > 0 {
						autoIncStep = step
					}
				}
			}

			cfg := core.TableConfig{
				Unique: unique,
				Sort:   sortVal,
				Audit: core.AuditConfig{
					Insert: auditInsertVal,
					Edit:   auditEditVal,
				},
				JournalLog: journalVal,
				AutoIncrement: core.AutoIncrementConfig{
					Column: autoIncCol,
					Step:   autoIncStep,
				},
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

			if err := core.InitTable(cmd.Context(), filePath, columns, cfg); err != nil {
				return err
			}
			if isVerboseOrDebug(cmd) {
				fmt.Fprintf(cmd.OutOrStdout(), "Successfully initialized table in %s\n", filePath)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&columnsVal, columnsFlag, "c", "", "ordered columns (comma-separated)")
	cmd.Flags().StringVarP(&uniqueVal, uniqueFlag, "u", "", "unique key columns (comma-separated)")
	cmd.Flags().StringVarP(&sortVal, sortFlag, "s", "", "frontmatter sort rule (e.g., id:asc)")
	cmd.Flags().BoolVar(&auditVal, auditFlag, false, "enable both creation (created_at) and edit (updated_at) timestamp auditing")
	cmd.Flags().StringVarP(&auditInsertVal, auditInsertFlag, "i", "", "enable creation timestamp auditing with optional custom column name")
	cmd.Flags().Lookup(auditInsertFlag).NoOptDefVal = defaultAuditInsert

	cmd.Flags().StringVarP(&auditEditVal, auditEditFlag, "e", "", "enable modification timestamp auditing with optional custom column name")
	cmd.Flags().Lookup(auditEditFlag).NoOptDefVal = defaultAuditEdit
	cmd.Flags().StringVarP(&journalVal, journalLogFlag, "j", "", "path to change journal log file (logs 4-column change history: timestamp, operation, row_key, details)")
	cmd.Flags().StringVarP(&autoincrementVal, autoincrementFlag, "a", "", "auto-increment column and optional step (e.g. id:10 or id; generates 1-based sequential integer IDs)")
	cmd.Flags().BoolP("verbose", "v", false, "print verbose confirmation message on success")

	return cmd
}
