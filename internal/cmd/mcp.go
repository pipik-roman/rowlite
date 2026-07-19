package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/roman-pipik/rowlite/internal/core"
)

func NewMcpCmd() *cobra.Command {
	cmd := &cobra.Command{
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
				Name:        "rowlite_init",
				Description: "Initialize a new table structure or update frontmatter constraints in-place",
			}, func(ctx context.Context, req *mcp.CallToolRequest, args InitTableArgs) (*mcp.CallToolResult, any, error) {
				cfg := core.TableConfig{
					Unique:      args.Unique,
					Sort:        args.Sort,
					Audit: core.AuditConfig{
						Insert: args.AuditInsert,
						Edit:   args.AuditEdit,
					},
					JournalLog:  args.JournalLog,
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
					cfgParsed, _, err := core.ParseFrontmatterAndContent(ctx, args.File, core.ContentModeNone)
					if err != nil {
						return nil, nil, fmt.Errorf("failed to parse existing table structure: %w", err)
					}
					if len(cfgParsed.Headers) == 0 {
						return nil, nil, fmt.Errorf("existing file has no table headers; please specify columns to initialize a table")
					}
					for _, h := range cfgParsed.Headers {
						if !core.IsAuditColumn(h, cfg) {
							columns = append(columns, h)
						}
					}
				} else {
					if fileExists {
						cfgParsed, _, err := core.ParseFrontmatterAndContent(ctx, args.File, core.ContentModeNone)
						if err == nil && len(cfgParsed.Headers) > 0 {
							var existingUserCols []string
							for _, h := range cfgParsed.Headers {
								if !core.IsAuditColumn(h, cfg) {
									existingUserCols = append(existingUserCols, h)
								}
							}
							if !reflect.DeepEqual(columns, existingUserCols) {
								return nil, nil, fmt.Errorf("specified columns %v do not match existing table structure %v; omit columns parameter to preserve structure", columns, existingUserCols)
							}
						}
					}
				}

				err := core.InitTable(ctx, args.File, columns, cfg)
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
				Name:        "rowlite_list",
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

				cfgParsed, rows, err := core.ParseFrontmatterAndContent(ctx, args.File, core.ContentModeAll)
				if err != nil {
					return nil, nil, err
				}
				headers := cfgParsed.Headers
				if err != nil {
					return nil, nil, err
				}

				var sb strings.Builder
				sb.WriteString(core.FormatGFMLine(headers))
				sb.WriteByte('\n')
				seps := make([]string, len(headers))
				for i := range seps {
					seps[i] = "---"
				}
				sb.WriteString(core.FormatGFMLine(seps))
				sb.WriteByte('\n')

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
						sb.WriteString(core.FormatGFMLine(row))
						sb.WriteByte('\n')
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
				Name:        "rowlite_count",
				Description: "Count matching rows from the markdown GFM table",
			}, func(ctx context.Context, req *mcp.CallToolRequest, args CountContentArgs) (*mcp.CallToolResult, any, error) {
				count, err := core.Count(ctx, args.File, args.Where)
				if err != nil {
					return nil, nil, err
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
				Name:        "rowlite_append",
				Description: "Append a row to the GFM table (validates unique and sort frontmatter rules)",
			}, func(ctx context.Context, req *mcp.CallToolRequest, args AppendRowArgs) (*mcp.CallToolResult, any, error) {
				err := core.AppendRow(ctx, args.File, args.Row)
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
				Name:        "rowlite_sort",
				Description: "Sort the markdown GFM table in-place",
			}, func(ctx context.Context, req *mcp.CallToolRequest, args SortTableArgs) (*mcp.CallToolResult, any, error) {
				err := core.SortTableAction(ctx, args.File, args.By, args.Trigger)
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
				Name:        "rowlite_get_structure",
				Description: "Display table column headers and frontmatter configurations",
			}, func(ctx context.Context, req *mcp.CallToolRequest, args GetTableStructureArgs) (*mcp.CallToolResult, any, error) {
				cfg, _, err := core.ParseFrontmatterAndContent(ctx, args.File, core.ContentModeNone)
				if err != nil {
					return nil, nil, err
				}

				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("Columns: %s\n", strings.Join(cfg.Headers, ", ")))
				if len(cfg.Alignments) > 0 {
					sb.WriteString(fmt.Sprintf("Alignments: %s\n", strings.Join(cfg.Alignments, ", ")))
				}
				if len(cfg.Unique) > 0 {
					sb.WriteString(fmt.Sprintf("Unique Keys: %s\n", strings.Join(cfg.Unique, ", ")))
				}
				if cfg.Sort != "" {
					sb.WriteString(fmt.Sprintf("Sort Configuration: %s\n", cfg.Sort))
				}
				if cfg.Audit.Insert != "" {
					sb.WriteString(fmt.Sprintf("Audit Insert Column: %s\n", cfg.Audit.Insert))
				}
				if cfg.Audit.Edit != "" {
					sb.WriteString(fmt.Sprintf("Audit Edit Column: %s\n", cfg.Audit.Edit))
				}
				if cfg.JournalLog != "" {
					sb.WriteString(fmt.Sprintf("Journal Log: %s\n", cfg.JournalLog))
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

	return cmd
}
