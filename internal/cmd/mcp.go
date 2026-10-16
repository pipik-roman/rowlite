package cmd

import (
	"context"
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

// NewMcpServer constructs the Rowlite MCP server with minimal AI-first tool schemas
// and shared server-level instructions to avoid per-tool description bloat.
func NewMcpServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "rowlite",
		Version: Version,
	}, &mcp.ServerOptions{
		Instructions: "Markdown GFM table engine. Filter syntax for 'where': 'Col == val' (operators: ==, !=, like, regex). Sort syntax: 'Col:asc' or 'Col:desc'.",
	})

	cache := NewTableConfigCache()

	type InitTableArgs struct {
		File    string   `json:"file"`
		Columns []string `json:"columns,omitempty"`
		Unique  []string `json:"unique,omitempty"`
		Sort    string   `json:"sort,omitempty" jsonschema:"e.g. id:asc"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rowlite_init",
		Description: "Initialize table columns and constraints",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args InitTableArgs) (*mcp.CallToolResult, any, error) {
		cfg := core.TableConfig{
			Unique: args.Unique,
			Sort:   args.Sort,
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
		} else if fileExists {
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

		if err := core.InitTable(ctx, args.File, columns, cfg); err != nil {
			return nil, nil, err
		}
		cache.Invalidate(args.File)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Table successfully initialized in %s", args.File)},
			},
		}, nil, nil
	})

	type ListContentArgs struct {
		File   string   `json:"file"`
		Where  []string `json:"where,omitempty" jsonschema:"e.g. ['Status == active', 'Name like foo']"`
		Limit  *int     `json:"limit,omitempty"`
		Offset int      `json:"offset,omitempty"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rowlite_list",
		Description: "Query rows from a Markdown table",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ListContentArgs) (*mcp.CallToolResult, any, error) {
		filters, err := core.ParseFilters(args.Where)
		if err != nil {
			return nil, nil, err
		}

		posOpts := core.PositionalOptions{
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
		File  string   `json:"file"`
		Where []string `json:"where,omitempty"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rowlite_count",
		Description: "Count matching rows in a Markdown table",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CountContentArgs) (*mcp.CallToolResult, any, error) {
		count, err := core.CountWithOptions(ctx, args.File, args.Where, core.PositionalOptions{})
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
		File string            `json:"file"`
		Row  map[string]string `json:"row"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rowlite_append",
		Description: "Append a row to a Markdown table",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AppendRowArgs) (*mcp.CallToolResult, any, error) {
		if err := core.AppendRowWithOptions(ctx, args.File, args.Row, ""); err != nil {
			return nil, nil, err
		}
		cache.Invalidate(args.File)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Row successfully appended"},
			},
		}, nil, nil
	})

	type UpdateRowsArgs struct {
		File  string            `json:"file"`
		Where []string          `json:"where,omitempty"`
		Index any               `json:"index,omitempty" jsonschema:"0-based index or range (e.g. 0, '2..5')"`
		Set   map[string]string `json:"set"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rowlite_update",
		Description: "Update matching rows in a Markdown table",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args UpdateRowsArgs) (*mcp.CallToolResult, any, error) {
		posOpts := core.PositionalOptions{
			Index: parseIndexArg(args.Index),
		}

		count, err := core.UpdateWithOptions(ctx, args.File, args.Where, posOpts, args.Set, "", false, false)
		if err != nil {
			return nil, nil, err
		}

		cache.Invalidate(args.File)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Successfully updated %d row(s)", count)},
			},
		}, nil, nil
	})

	type DeleteRowsArgs struct {
		File  string   `json:"file"`
		Where []string `json:"where,omitempty"`
		Index any      `json:"index,omitempty" jsonschema:"0-based index or range (e.g. 0, '2..5')"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rowlite_delete",
		Description: "Delete matching rows from a Markdown table",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DeleteRowsArgs) (*mcp.CallToolResult, any, error) {
		posOpts := core.PositionalOptions{
			Index: parseIndexArg(args.Index),
		}

		count, err := core.DeleteWithOptions(ctx, args.File, args.Where, posOpts, "", false, false)
		if err != nil {
			return nil, nil, err
		}

		cache.Invalidate(args.File)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Successfully deleted %d row(s)", count)},
			},
		}, nil, nil
	})

	type SortTableArgs struct {
		File string `json:"file"`
		By   string `json:"by,omitempty" jsonschema:"e.g. id:desc"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rowlite_sort",
		Description: "Sort table rows in-place",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SortTableArgs) (*mcp.CallToolResult, any, error) {
		if err := core.SortTableAction(ctx, args.File, args.By, false); err != nil {
			return nil, nil, err
		}
		cache.Invalidate(args.File)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Table successfully sorted"},
			},
		}, nil, nil
	})

	type GetTableStructureArgs struct {
		File string `json:"file"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rowlite_get_structure",
		Description: "Inspect table columns and constraints",
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

	return server
}

func NewMcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start Rowlite stdio MCP server",
		Run: func(cmd *cobra.Command, args []string) {
			server := NewMcpServer()
			if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
				log.Printf("Server failed: %v", err)
			}
		},
	}

	return cmd
}

