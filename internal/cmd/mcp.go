package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/pipik-roman/rowlite/internal/core"
)

type cachedTableConfig struct {
	cfg     core.TableConfig
	modTime time.Time
}

// TableConfigCache provides thread-safe in-memory caching of TableConfig by (filePath, modTime)
type TableConfigCache struct {
	mu    sync.RWMutex
	items map[string]cachedTableConfig
}

func NewTableConfigCache() *TableConfigCache {
	return &TableConfigCache{
		items: make(map[string]cachedTableConfig),
	}
}

func (c *TableConfigCache) GetOrParse(ctx context.Context, filePath string, mode core.ContentMode) (core.TableConfig, [][]string, error) {
	stat, err := os.Stat(filePath)
	if err != nil {
		return core.TableConfig{}, nil, err
	}
	modTime := stat.ModTime()

	c.mu.RLock()
	cached, ok := c.items[filePath]
	c.mu.RUnlock()

	if ok && cached.modTime.Equal(modTime) {
		if mode == core.ContentModeNone {
			return cached.cfg, nil, nil
		}
	}

	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, filePath, mode)
	if err != nil {
		return core.TableConfig{}, nil, err
	}

	c.mu.Lock()
	c.items[filePath] = cachedTableConfig{
		cfg:     cfg,
		modTime: modTime,
	}
	c.mu.Unlock()

	return cfg, rows, nil
}

func (c *TableConfigCache) Invalidate(filePath string) {
	c.mu.Lock()
	delete(c.items, filePath)
	c.mu.Unlock()
}

func parseIndexArg(raw any) string {
	if raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case float64:
		return strconv.Itoa(int(v))
	case int:
		return strconv.Itoa(v)
	case []any:
		var parts []string
		for _, item := range v {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ",")
	case []int:
		var parts []string
		for _, item := range v {
			parts = append(parts, strconv.Itoa(item))
		}
		return strings.Join(parts, ",")
	case []string:
		return strings.Join(v, ",")
	default:
		return fmt.Sprint(v)
	}
}

func NewMcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start Rowlite stdio MCP server",
		Run: func(cmd *cobra.Command, args []string) {
			server := mcp.NewServer(&mcp.Implementation{
				Name:    "rowlite",
				Version: Version,
			}, nil)

			cache := NewTableConfigCache()

			type InitTableArgs struct {
				File          string   `json:"file" jsonschema:"path to the markdown file"`
				Columns       []string `json:"columns,omitempty" jsonschema:"list of column names in order (optional for existing tables)"`
				Unique        []string `json:"unique,omitempty" jsonschema:"columns that must be unique"`
				Sort          string   `json:"sort,omitempty" jsonschema:"sort rule (e.g. id:asc)"`
				AuditInsert   string   `json:"audit_insert,omitempty" jsonschema:"custom name for creation timestamp column"`
				AuditEdit     string   `json:"audit_edit,omitempty" jsonschema:"custom name for modification timestamp column"`
				JournalLog    string   `json:"journal_log,omitempty" jsonschema:"path to change journal log file"`
				AutoIncrement string   `json:"autoincrement,omitempty" jsonschema:"auto-increment column and optional step (e.g. id:10 or id). Identifiers are 1-based sequential integers."`
			}
			mcp.AddTool(server, &mcp.Tool{
				Name:        "rowlite_init",
				Description: "Initialize a new table structure or update frontmatter constraints in-place (supports unique keys, sort rules, audit_insert, audit_edit, journal_log, autoincrement)",
			}, func(ctx context.Context, req *mcp.CallToolRequest, args InitTableArgs) (*mcp.CallToolResult, any, error) {
				autoIncCol, autoIncStep := "", 0
				if args.AutoIncrement != "" {
					parts := strings.SplitN(args.AutoIncrement, ":", 2)
					autoIncCol = strings.TrimSpace(parts[0])
					autoIncStep = 1
					if len(parts) > 1 {
						if step, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && step > 0 {
							autoIncStep = step
						}
					}
				}

				cfg := core.TableConfig{
					Unique: args.Unique,
					Sort:   args.Sort,
					Audit: core.AuditConfig{
						Insert: args.AuditInsert,
						Edit:   args.AuditEdit,
					},
					JournalLog: args.JournalLog,
					AutoIncrement: core.AutoIncrementConfig{
						Column: autoIncCol,
						Step:   autoIncStep,
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
					cfgParsed, _, err := cache.GetOrParse(ctx, args.File, core.ContentModeNone)
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
						cfgParsed, _, err := cache.GetOrParse(ctx, args.File, core.ContentModeNone)
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
				File            string   `json:"file" jsonschema:"path to the markdown file"`
				Where           []string `json:"where,omitempty" jsonschema:"list of WHERE conditions (e.g. ['Priority == High', 'tags like beach'])"`
				Head            *int     `json:"head,omitempty" jsonschema:"extract the first N logical data rows from table start"`
				Tail            *int     `json:"tail,omitempty" jsonschema:"extract the last N logical data rows from table end"`
				Index           any      `json:"index,omitempty" jsonschema:"target 0-based row index (e.g. 0, -1), list ([0,2,4]), range ('2..10', '2:10'), or stepped slice ('0..20..2')"`
				Offset          int      `json:"offset,omitempty" jsonschema:"zero-based row offset for pagination"`
				Limit           *int     `json:"limit,omitempty" jsonschema:"maximum number of rows to return"`
				IncludeMetadata bool     `json:"include_metadata,omitempty" jsonschema:"when true, returns structured JSON metadata envelope with 0-based __row_index__, indices, total rows, and schema"`
			}
			mcp.AddTool(server, &mcp.Tool{
				Name:        "rowlite_list",
				Description: "List matching rows from the markdown GFM table using optional WHERE filters and positional selectors (head, tail, index, offset, limit)",
			}, func(ctx context.Context, req *mcp.CallToolRequest, args ListContentArgs) (*mcp.CallToolResult, any, error) {
				filters, err := core.ParseFilters(args.Where)
				if err != nil {
					return nil, nil, err
				}

				posOpts := core.PositionalOptions{
					Head:   args.Head,
					Tail:   args.Tail,
					Index:  parseIndexArg(args.Index),
					Offset: args.Offset,
					Limit:  args.Limit,
				}

				cfgParsed, rows, err := cache.GetOrParse(ctx, args.File, core.ContentModeAll)
				if err != nil {
					return nil, nil, err
				}
				headers := cfgParsed.Headers

				matched, err := core.SelectRowsStream(headers, rows, filters, posOpts)
				if err != nil {
					return nil, nil, err
				}

				if args.IncludeMetadata {
					dataSlice := make([]map[string]any, len(matched))
					for i, mr := range matched {
						obj := make(map[string]any)
						obj["__row_index__"] = mr.Index
						for colIdx, h := range headers {
							val := ""
							if colIdx < len(mr.Row) {
								val = mr.Row[colIdx]
							}
							obj[h] = val
						}
						dataSlice[i] = obj
					}

					metaResponse := map[string]any{
						"status": "success",
						"meta": map[string]any{
							"etag":              cfgParsed.ETag,
							"total_rows":        cfgParsed.RowCount,
							"returned_rows":     len(matched),
							"offset":            posOpts.Offset,
							"indices_retrieved": matched.Indices(),
						},
						"schema": map[string]any{
							"columns": headers,
						},
						"data": dataSlice,
					}
					jsonBytes, _ := json.MarshalIndent(metaResponse, "", "  ")
					return &mcp.CallToolResult{
						Content: []mcp.Content{
							&mcp.TextContent{Text: string(jsonBytes)},
						},
					}, nil, nil
				}

				var sb strings.Builder
				sb.WriteString(core.FormatGFMLine(headers))
				sb.WriteByte('\n')
				seps := make([]string, len(headers))
				for i := range seps {
					if i < len(cfgParsed.Alignments) && strings.TrimSpace(cfgParsed.Alignments[i]) != "" {
						seps[i] = cfgParsed.Alignments[i]
					} else {
						seps[i] = "---"
					}
				}
				sb.WriteString(core.FormatGFMLine(seps))
				sb.WriteByte('\n')

				for _, mr := range matched {
					sb.WriteString(core.FormatGFMLine(mr.Row))
					sb.WriteByte('\n')
				}

				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: sb.String()},
					},
				}, nil, nil
			})

			type CountContentArgs struct {
				File   string   `json:"file" jsonschema:"path to the markdown file"`
				Where  []string `json:"where,omitempty" jsonschema:"list of WHERE conditions (e.g. ['Priority == High', 'tags like beach'])"`
				Head   *int     `json:"head,omitempty" jsonschema:"count bounded to first N rows"`
				Tail   *int     `json:"tail,omitempty" jsonschema:"count bounded to last N rows"`
				Index  any      `json:"index,omitempty" jsonschema:"target 0-based row index, list, range, or stepped slice"`
				Offset int      `json:"offset,omitempty" jsonschema:"zero-based row offset for pagination"`
				Limit  *int     `json:"limit,omitempty" jsonschema:"maximum count limit"`
			}
			mcp.AddTool(server, &mcp.Tool{
				Name:        "rowlite_count",
				Description: "Count matching rows from the markdown GFM table using optional WHERE filters and positional selectors",
			}, func(ctx context.Context, req *mcp.CallToolRequest, args CountContentArgs) (*mcp.CallToolResult, any, error) {
				posOpts := core.PositionalOptions{
					Head:   args.Head,
					Tail:   args.Tail,
					Index:  parseIndexArg(args.Index),
					Offset: args.Offset,
					Limit:  args.Limit,
				}

				count, err := core.CountWithOptions(ctx, args.File, args.Where, posOpts)
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
				File       string            `json:"file" jsonschema:"path to the markdown file"`
				Row        map[string]string `json:"row" jsonschema:"JSON object mapping column names to cell values"`
				IfMatch    string            `json:"if_match,omitempty" jsonschema:"expected ETag checksum of the table before appending (for optimistic concurrency control)"`
				IfChecksum string            `json:"if_checksum,omitempty" jsonschema:"alias for if_match (expected table checksum)"`
			}
			mcp.AddTool(server, &mcp.Tool{
				Name:        "rowlite_append",
				Description: "Append a row to the GFM table (validates unique keys, applies auto-increment, updates audit timestamps, auto-sorts if configured, logs to change journal, and supports optimistic concurrency control via if_match)",
			}, func(ctx context.Context, req *mcp.CallToolRequest, args AppendRowArgs) (*mcp.CallToolResult, any, error) {
				expectedHash := args.IfMatch
				if expectedHash == "" {
					expectedHash = args.IfChecksum
				}
				err := core.AppendRowWithOptions(ctx, args.File, args.Row, expectedHash)
				if err != nil {
					var precondErr *core.PreconditionFailedError
					if errors.As(err, &precondErr) {
						errPayload := map[string]any{
							"status":        "error",
							"error_code":    "PRECONDITION_FAILED",
							"message":       fmt.Sprintf("Table content has changed since the provided ETag ('%s'). Current ETag is '%s'. Re-read table and retry.", precondErr.ExpectedETag, precondErr.CurrentETag),
							"expected_etag": precondErr.ExpectedETag,
							"current_etag":  precondErr.CurrentETag,
						}
						errBytes, _ := json.MarshalIndent(errPayload, "", "  ")
						return &mcp.CallToolResult{
							IsError: true,
							Content: []mcp.Content{
								&mcp.TextContent{Text: string(errBytes)},
							},
						}, nil, nil
					}
					return nil, nil, err
				}
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: "Row successfully appended"},
					},
				}, nil, nil
			})

			type UpdateRowsArgs struct {
				File       string            `json:"file" jsonschema:"path to the markdown file"`
				Where      []string          `json:"where,omitempty" jsonschema:"list of WHERE conditions (e.g. ['status == Pending', 'priority like high'])"`
				Head       *int              `json:"head,omitempty" jsonschema:"target first N matching rows"`
				Tail       *int              `json:"tail,omitempty" jsonschema:"target last N matching rows"`
				Index      any               `json:"index,omitempty" jsonschema:"target 0-based row index (e.g. 0, -1), list ([0,2,4]), range ('2..10'), or stepped slice ('0..20..2')"`
				Offset     int               `json:"offset,omitempty" jsonschema:"zero-based offset for pagination"`
				Limit      *int              `json:"limit,omitempty" jsonschema:"maximum number of rows to update"`
				Set        map[string]string `json:"set" jsonschema:"key-value map of column names to updated cell values"`
				IfMatch    string            `json:"if_match,omitempty" jsonschema:"expected ETag checksum of the table before updating (for optimistic concurrency control)"`
				IfChecksum string            `json:"if_checksum,omitempty" jsonschema:"alias for if_match (expected table checksum)"`
			}
			mcp.AddTool(server, &mcp.Tool{
				Name:        "rowlite_update",
				Description: "Update matching rows in a GFM table (validates unique keys, updates audit timestamps, auto-sorts if configured, logs to change journal, and supports optimistic concurrency control via if_match)",
			}, func(ctx context.Context, req *mcp.CallToolRequest, args UpdateRowsArgs) (*mcp.CallToolResult, any, error) {
				expectedHash := args.IfMatch
				if expectedHash == "" {
					expectedHash = args.IfChecksum
				}

				posOpts := core.PositionalOptions{
					Head:   args.Head,
					Tail:   args.Tail,
					Index:  parseIndexArg(args.Index),
					Offset: args.Offset,
					Limit:  args.Limit,
				}

				count, err := core.UpdateWithOptions(ctx, args.File, args.Where, posOpts, args.Set, expectedHash)
				if err != nil {
					var precondErr *core.PreconditionFailedError
					if errors.As(err, &precondErr) {
						errPayload := map[string]any{
							"status":        "error",
							"error_code":    "PRECONDITION_FAILED",
							"message":       fmt.Sprintf("Table content has changed since the provided ETag ('%s'). Current ETag is '%s'. Re-read table and retry.", precondErr.ExpectedETag, precondErr.CurrentETag),
							"expected_etag": precondErr.ExpectedETag,
							"current_etag":  precondErr.CurrentETag,
						}
						errBytes, _ := json.MarshalIndent(errPayload, "", "  ")
						return &mcp.CallToolResult{
							IsError: true,
							Content: []mcp.Content{
								&mcp.TextContent{Text: string(errBytes)},
							},
						}, nil, nil
					}
					return nil, nil, err
				}

				cache.Invalidate(args.File)

				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("Successfully updated %d row(s)", count)},
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
				Description: "Sort the markdown GFM table in-place using a sort rule or pre-configured frontmatter sort settings",
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
				Description: "Display table column headers, alignments, and frontmatter configurations (unique, sort, audit, journal_log, autoincrement)",
			}, func(ctx context.Context, req *mcp.CallToolRequest, args GetTableStructureArgs) (*mcp.CallToolResult, any, error) {
				cfg, _, err := cache.GetOrParse(ctx, args.File, core.ContentModeNone)
				if err != nil {
					return nil, nil, err
				}

				var sb strings.Builder
				if cfg.ETag != "" {
					sb.WriteString(fmt.Sprintf("ETag: %s\n", cfg.ETag))
				}
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
				if cfg.AutoIncrement.Column != "" {
					sb.WriteString(fmt.Sprintf("AutoIncrement Column: %s (step: %d)\n", cfg.AutoIncrement.Column, cfg.AutoIncrement.Step))
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
