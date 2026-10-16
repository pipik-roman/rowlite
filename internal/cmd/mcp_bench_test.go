package cmd_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pipik-roman/rowlite/internal/cmd"
)

func fetchMCPTools(tb testing.TB) []*mcp.Tool {
	tb.Helper()
	ctx := context.Background()

	server := cmd.NewMcpServer()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "schema-bench-client",
		Version: "1.0.0",
	}, nil)

	t1, t2 := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, t1, nil)
	if err != nil {
		tb.Fatalf("server.Connect failed: %v", err)
	}
	defer serverSession.Close()

	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		tb.Fatalf("client.Connect failed: %v", err)
	}
	defer clientSession.Close()

	res, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		tb.Fatalf("ListTools failed: %v", err)
	}
	return res.Tools
}

func countSchemaProperties(schema any) int {
	data, err := json.Marshal(schema)
	if err != nil {
		return 0
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return 0
	}
	props, ok := m["properties"].(map[string]any)
	if !ok {
		return 0
	}
	return len(props)
}

// TestMCPSchemaSizeBudget enforces the AI-First Minimal MCP Schema Principle,
// ensuring total and per-tool MCP JSON schemas do not regress into CLI flag bloat.
func TestMCPSchemaSizeBudget(t *testing.T) {
	tools := fetchMCPTools(t)
	if len(tools) != 8 {
		t.Fatalf("expected 8 MCP tools, got %d", len(tools))
	}

	totalBytes := 0
	totalProps := 0

	for _, tool := range tools {
		raw, err := json.Marshal(tool)
		if err != nil {
			t.Fatalf("failed to marshal tool %s: %v", tool.Name, err)
		}
		bLen := len(raw)
		props := countSchemaProperties(tool.InputSchema)

		totalBytes += bLen
		totalProps += props

		if props > 4 {
			t.Errorf("tool %s exposes %d properties (max budget: 4)", tool.Name, props)
		}
		if bLen > 450 {
			t.Errorf("tool %s serialized schema is %d bytes (max budget: 450 bytes)", tool.Name, bLen)
		}
	}

	if totalProps > 22 {
		t.Errorf("total MCP properties across all tools is %d (max budget: 22, legacy was 54)", totalProps)
	}
	if totalBytes > 2500 {
		t.Errorf("total serialized MCP schema is %d bytes (max budget: 2500 bytes, legacy was ~9100 bytes)", totalBytes)
	}
}

// BenchmarkMCPSchemaSize measures total and per-tool MCP schema footprint (bytes, properties, estimated tokens).
func BenchmarkMCPSchemaSize(b *testing.B) {
	tools := fetchMCPTools(b)

	b.Run("TotalToolset", func(b *testing.B) {
		var totalBytes int
		var totalProps int
		for b.Loop() {
			totalBytes = 0
			totalProps = 0
			for _, tool := range tools {
				raw, _ := json.Marshal(tool)
				totalBytes += len(raw)
				totalProps += countSchemaProperties(tool.InputSchema)
			}
		}
		b.ReportMetric(float64(totalBytes), "schema_bytes")
		b.ReportMetric(float64(totalProps), "properties")
		b.ReportMetric(float64(totalBytes)/4.0, "est_tokens")
	})

	for _, tool := range tools {
		b.Run(tool.Name, func(b *testing.B) {
			var toolBytes int
			var props int
			for b.Loop() {
				raw, _ := json.Marshal(tool)
				toolBytes = len(raw)
				props = countSchemaProperties(tool.InputSchema)
			}
			b.ReportMetric(float64(toolBytes), "schema_bytes")
			b.ReportMetric(float64(props), "properties")
			b.ReportMetric(float64(toolBytes)/4.0, "est_tokens")
		})
	}
}
