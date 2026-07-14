package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/roman-pipik/rowlite/pkg/core"
)

var rootCmd = &cobra.Command{
	Use:   "rowlite",
	Short: "Rowlite is a flat Markdown database CLI and MCP server",
	Long:  `A zero-config, lightweight command line interface and MCP server to query and edit Markdown GFM tables.`,
}

var initCmd = &cobra.Command{
	Use:   "init <file>",
	Short: "Initialize table frontmatter constraints",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		columnsVal, _ := cmd.Flags().GetString("columns")
		uniqueVal, _ := cmd.Flags().GetString("unique")
		sortVal, _ := cmd.Flags().GetString("sort")
		auditInsertVal, _ := cmd.Flags().GetString("audit-insert")
		auditEditVal, _ := cmd.Flags().GetString("audit-edit")
		journalVal, _ := cmd.Flags().GetString("journal-log")

		var unique []string
		if uniqueVal != "" {
			unique = strings.Split(uniqueVal, ",")
			for i := range unique {
				unique[i] = strings.TrimSpace(unique[i])
			}
		}

		cfg := core.TableConfig{
			Rowlite: core.RowliteConfig{
				Unique:      unique,
				Sort:        sortVal,
				Audit: core.AuditConfig{
					Insert: auditInsertVal,
					Edit:   auditEditVal,
				},
				JournalLog:  journalVal,
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
			_, _, headers, _, err := core.ParseFrontmatterAndContent(filePath)
			if err != nil {
				return fmt.Errorf("failed to parse existing table structure: %v", err)
			}
			if len(headers) == 0 {
				return fmt.Errorf("existing file has no table headers. Please specify --columns to initialize a table")
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
				_, _, headers, _, err := core.ParseFrontmatterAndContent(filePath)
				if err == nil && len(headers) > 0 {
					var existingUserCols []string
					for _, h := range headers {
						if !core.IsAuditColumn(h, cfg) {
							existingUserCols = append(existingUserCols, h)
						}
					}
					if !reflect.DeepEqual(columns, existingUserCols) {
						return fmt.Errorf("specified columns %v do not match existing table structure %v. Run without --columns flag to preserve structure", columns, existingUserCols)
					}
				}
			}
		}

		return core.InitTable(filePath, columns, cfg)
	},
}

var listCmd = &cobra.Command{
	Use:   "list <file>",
	Short: "List matching rows from table",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		whereFlags, _ := cmd.Flags().GetStringSlice("where")

		var filters []core.Filter
		for _, wf := range whereFlags {
			flt, err := core.ParseFilter(wf)
			if err != nil {
				return err
			}
			filters = append(filters, flt)
		}

		_, rows, headers, _, err := core.ParseFrontmatterAndContent(filePath)
		if err != nil {
			return err
		}
		if len(headers) == 0 {
			return fmt.Errorf("no GFM table headers found in %s", filePath)
		}

		fmt.Println(core.FormatGFMLine(headers))
		seps := make([]string, len(headers))
		for i := range seps {
			seps[i] = "---"
		}
		fmt.Println(core.FormatGFMLine(seps))

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
				fmt.Println(core.FormatGFMLine(row))
			}
		}
		return nil
	},
}

var countCmd = &cobra.Command{
	Use:   "count <file>",
	Short: "Count matching rows from table",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		whereFlags, _ := cmd.Flags().GetStringSlice("where")

		var filters []core.Filter
		for _, wf := range whereFlags {
			flt, err := core.ParseFilter(wf)
			if err != nil {
				return err
			}
			filters = append(filters, flt)
		}

		_, rows, headers, _, err := core.ParseFrontmatterAndContent(filePath)
		if err != nil {
			return err
		}
		if len(headers) == 0 {
			return fmt.Errorf("no GFM table headers found in %s", filePath)
		}

		count := 0
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
				count++
			}
		}
		fmt.Println(count)
		return nil
	},
}

var appendCmd = &cobra.Command{
	Use:   "append <file> [data-string]",
	Short: "Append row values to table (supports JSON/CSV strings and stdin)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		var dataStr string

		if len(args) == 2 {
			dataStr = args[1]
		} else {
			rowJSON, _ := cmd.Flags().GetString("row")
			if rowJSON != "" {
				dataStr = rowJSON
			} else {
				// Read from stdin if piped
				stat, _ := os.Stdin.Stat()
				if (stat.Mode() & os.ModeCharDevice) == 0 {
					bytes, err := io.ReadAll(os.Stdin)
					if err == nil {
						dataStr = string(bytes)
					}
				}
			}
		}

		dataStr = strings.TrimSpace(dataStr)
		if dataStr == "" {
			return fmt.Errorf("data string content is required either as a positional argument, via --row flag, or piped to stdin")
		}

		cfg, _, headers, _, err := core.ParseFrontmatterAndContent(filePath)
		if err != nil {
			return err
		}

		parsedRows, err := core.ParseInputData(headers, cfg, dataStr)
		if err != nil {
			return err
		}

		return core.AppendRows(filePath, parsedRows)
	},
}

var sortCmd = &cobra.Command{
	Use:   "sort <file>",
	Short: "Sort table in-place",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		byRule, _ := cmd.Flags().GetString("by")
		trigger, _ := cmd.Flags().GetBool("trigger")

		return core.SortTableAction(filePath, byRule, trigger)
	},
}

var structureCmd = &cobra.Command{
	Use:   "structure <file>",
	Short: "Display table column headers and frontmatter constraints",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		cfg, _, headers, align, err := core.ParseFrontmatterAndContent(filePath)
		if err != nil {
			return err
		}
		if len(headers) == 0 {
			return fmt.Errorf("table headers not found in %s", filePath)
		}

		fmt.Printf("Columns: %s\n", strings.Join(headers, ", "))
		if len(align) > 0 {
			fmt.Printf("Alignments: %s\n", strings.Join(align, ", "))
		}
		if len(cfg.Rowlite.Unique) > 0 {
			fmt.Printf("Unique Keys: %s\n", strings.Join(cfg.Rowlite.Unique, ", "))
		}
		if cfg.Rowlite.Sort != "" {
			fmt.Printf("Sort Configuration: %s\n", cfg.Rowlite.Sort)
		}
		if cfg.Rowlite.Audit.Insert != "" {
			fmt.Printf("Audit Insert Column: %s\n", cfg.Rowlite.Audit.Insert)
		}
		if cfg.Rowlite.Audit.Edit != "" {
			fmt.Printf("Audit Edit Column: %s\n", cfg.Rowlite.Audit.Edit)
		}
		if cfg.Rowlite.JournalLog != "" {
			fmt.Printf("Journal Log: %s\n", cfg.Rowlite.JournalLog)
		}
		return nil
	},
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start Rowlite stdio MCP server",
	Run: func(cmd *cobra.Command, args []string) {
		server := mcp.NewServer(&mcp.Implementation{
			Name:    "rowlite",
			Version: "1.0.0",
		}, nil)

		type InitTableArgs struct {
			File        string   `json:"file" jsonschema:"path to the markdown file"`
			Columns     []string `json:"columns,omitempty" jsonschema:"list of column names in order (optional for existing tables)"`
			Unique      []string `json:"unique,omitempty" jsonschema:"columns that must be unique"`
			Sort        string   `json:"sort,omitempty" jsonschema:"sort rule (e.g. id:asc)"`
			AuditInsert string   `json:"audit_insert,omitempty" jsonschema:"custom name for creation timestamp column"`
			AuditEdit   string   `json:"audit_edit,omitempty" jsonschema:"custom name for modification timestamp column"`
			JournalLog  string   `json:"journal_log,omitempty" jsonschema:"path to change journal log file"`
		}
		mcp.AddTool(server, &mcp.Tool{
			Name:        "init_table",
			Description: "Initialize a new table structure or update frontmatter constraints in-place",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args InitTableArgs) (*mcp.CallToolResult, any, error) {
			cfg := core.TableConfig{
				Rowlite: core.RowliteConfig{
					Unique:      args.Unique,
					Sort:        args.Sort,
					Audit: core.AuditConfig{
						Insert: args.AuditInsert,
						Edit:   args.AuditEdit,
					},
					JournalLog:  args.JournalLog,
				},
			}

			fileExists := false
			if _, err := os.Stat(args.File); err == nil {
				fileExists = true
			}

			columns := args.Columns
			if len(columns) == 0 {
				if !fileExists {
					return nil, nil, fmt.Errorf("columns parameter is required when creating a new table")
				}
				_, _, headers, _, err := core.ParseFrontmatterAndContent(args.File)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to parse existing table structure: %v", err)
				}
				if len(headers) == 0 {
					return nil, nil, fmt.Errorf("existing file has no table headers. Please specify columns to initialize a table")
				}
				for _, h := range headers {
					if !core.IsAuditColumn(h, cfg) {
						columns = append(columns, h)
					}
				}
			} else {
				if fileExists {
					_, _, headers, _, err := core.ParseFrontmatterAndContent(args.File)
					if err == nil && len(headers) > 0 {
						var existingUserCols []string
						for _, h := range headers {
							if !core.IsAuditColumn(h, cfg) {
								existingUserCols = append(existingUserCols, h)
							}
						}
						if !reflect.DeepEqual(columns, existingUserCols) {
							return nil, nil, fmt.Errorf("specified columns %v do not match existing table structure %v. Omit columns parameter to preserve structure", columns, existingUserCols)
						}
					}
				}
			}

			err := core.InitTable(args.File, columns, cfg)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Table successfully initialized in %s", args.File)},
				},
			}, nil, nil
		})

		type ListContentArgs struct {
			File  string   `json:"file" jsonschema:"path to the markdown file"`
			Where []string `json:"where,omitempty" jsonschema:"list of WHERE conditions"`
		}
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_content",
			Description: "List matching rows from the markdown GFM table",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ListContentArgs) (*mcp.CallToolResult, any, error) {
			var filters []core.Filter
			for _, wf := range args.Where {
				flt, err := core.ParseFilter(wf)
				if err != nil {
					return nil, nil, err
				}
				filters = append(filters, flt)
			}

			_, rows, headers, _, err := core.ParseFrontmatterAndContent(args.File)
			if err != nil {
				return nil, nil, err
			}

			var sb strings.Builder
			sb.WriteString(core.FormatGFMLine(headers) + "\n")
			seps := make([]string, len(headers))
			for i := range seps {
				seps[i] = "---"
			}
			sb.WriteString(core.FormatGFMLine(seps) + "\n")

			for _, row := range rows {
				match := true
				for _, flt := range filters {
					m, err := core.EvaluateFilter(headers, row, flt)
					if err != nil {
						return nil, nil, err
					}
					if !m {
						match = false
						break
					}
				}
				if match {
					sb.WriteString(core.FormatGFMLine(row) + "\n")
				}
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: sb.String()},
				},
			}, nil, nil
		})

		type CountContentArgs struct {
			File  string   `json:"file" jsonschema:"path to the markdown file"`
			Where []string `json:"where,omitempty" jsonschema:"list of WHERE conditions"`
		}
		mcp.AddTool(server, &mcp.Tool{
			Name:        "count_content",
			Description: "Count matching rows from the markdown GFM table",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args CountContentArgs) (*mcp.CallToolResult, any, error) {
			var filters []core.Filter
			for _, wf := range args.Where {
				flt, err := core.ParseFilter(wf)
				if err != nil {
					return nil, nil, err
				}
				filters = append(filters, flt)
			}

			_, rows, headers, _, err := core.ParseFrontmatterAndContent(args.File)
			if err != nil {
				return nil, nil, err
			}

			count := 0
			for _, row := range rows {
				match := true
				for _, flt := range filters {
					m, err := core.EvaluateFilter(headers, row, flt)
					if err != nil {
						return nil, nil, err
					}
					if !m {
						match = false
						break
					}
				}
				if match {
					count++
				}
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("%d", count)},
				},
			}, nil, nil
		})

		type AppendRowArgs struct {
			File string            `json:"file" jsonschema:"path to the markdown file"`
			Row  map[string]string `json:"row" jsonschema:"JSON object mapping column names to cell values"`
		}
		mcp.AddTool(server, &mcp.Tool{
			Name:        "append_row",
			Description: "Append a row to the GFM table (validates unique and sort frontmatter rules)",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args AppendRowArgs) (*mcp.CallToolResult, any, error) {
			err := core.AppendRow(args.File, args.Row)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Row successfully appended"},
				},
			}, nil, nil
		})

		type SortTableArgs struct {
			File    string `json:"file" jsonschema:"path to the markdown file"`
			By      string `json:"by,omitempty" jsonschema:"sorting rule (e.g. id:desc)"`
			Trigger bool   `json:"trigger,omitempty" jsonschema:"if true, updates the frontmatter sort constraint to trigger auto-sort on future writes"`
		}
		mcp.AddTool(server, &mcp.Tool{
			Name:        "sort_table",
			Description: "Sort the markdown GFM table in-place",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args SortTableArgs) (*mcp.CallToolResult, any, error) {
			err := core.SortTableAction(args.File, args.By, args.Trigger)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Table successfully sorted"},
				},
			}, nil, nil
		})

		type GetTableStructureArgs struct {
			File string `json:"file" jsonschema:"path to the markdown file"`
		}
		mcp.AddTool(server, &mcp.Tool{
			Name:        "get_table_structure",
			Description: "Display table column headers and frontmatter configurations",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args GetTableStructureArgs) (*mcp.CallToolResult, any, error) {
			cfg, _, headers, align, err := core.ParseFrontmatterAndContent(args.File)
			if err != nil {
				return nil, nil, err
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Columns: %s\n", strings.Join(headers, ", ")))
			if len(align) > 0 {
				sb.WriteString(fmt.Sprintf("Alignments: %s\n", strings.Join(align, ", ")))
			}
			if len(cfg.Rowlite.Unique) > 0 {
				sb.WriteString(fmt.Sprintf("Unique Keys: %s\n", strings.Join(cfg.Rowlite.Unique, ", ")))
			}
			if cfg.Rowlite.Sort != "" {
				sb.WriteString(fmt.Sprintf("Sort Configuration: %s\n", cfg.Rowlite.Sort))
			}
			if cfg.Rowlite.Audit.Insert != "" {
				sb.WriteString(fmt.Sprintf("Audit Insert Column: %s\n", cfg.Rowlite.Audit.Insert))
			}
			if cfg.Rowlite.Audit.Edit != "" {
				sb.WriteString(fmt.Sprintf("Audit Edit Column: %s\n", cfg.Rowlite.Audit.Edit))
			}
			if cfg.Rowlite.JournalLog != "" {
				sb.WriteString(fmt.Sprintf("Journal Log: %s\n", cfg.Rowlite.JournalLog))
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: sb.String()},
				},
			}, nil, nil
		})

		if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			log.Printf("Server failed: %v", err)
		}
	},
}

func init() {
	initCmd.Flags().String("columns", "", "ordered columns (comma-separated)")
	initCmd.Flags().String("unique", "", "unique key columns (comma-separated)")
	initCmd.Flags().String("sort", "", "frontmatter sort rule (e.g., id:asc)")
	initCmd.Flags().String("audit-insert", "", "enable creation timestamp auditing with custom column name")
	initCmd.Flags().Lookup("audit-insert").NoOptDefVal = "created_at"

	initCmd.Flags().String("audit-edit", "", "enable modification timestamp auditing with custom column name")
	initCmd.Flags().Lookup("audit-edit").NoOptDefVal = "updated_at"
	initCmd.Flags().String("journal-log", "", "path to change journal log file")

	listCmd.Flags().StringSlice("where", nil, "WHERE conditions")
	countCmd.Flags().StringSlice("where", nil, "WHERE conditions")

	appendCmd.Flags().String("row", "", "JSON row object to append")

	sortCmd.Flags().String("by", "", "override sort rule (e.g., id:desc)")
	sortCmd.Flags().Bool("trigger", false, "trigger sorting by updating frontmatter permanently")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(countCmd)
	rootCmd.AddCommand(appendCmd)
	rootCmd.AddCommand(sortCmd)
	rootCmd.AddCommand(structureCmd)
	rootCmd.AddCommand(mcpCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
