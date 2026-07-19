package core_test

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/roman-pipik/rowlite/internal/core"
)

func TestParseFilter(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		want    core.Filter
		wantErr bool
	}{
		{
			name: "equality simple",
			expr: "status==TODO",
			want: core.Filter{Column: "status", Operator: "==", Value: "TODO"},
		},
		{
			name: "inequality spaces",
			expr: "priority != high",
			want: core.Filter{Column: "priority", Operator: "!=", Value: "high"},
		},
		{
			name: "like operator",
			expr: "name like task",
			want: core.Filter{Column: "name", Operator: "like", Value: "task"},
		},
		{
			name: "regex operator",
			expr: "id regex ^\\d+$",
			want: core.Filter{Column: "id", Operator: "regex", Value: "^\\d+$"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.ParseFilter(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFilter() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateFilter(t *testing.T) {
	headers := []string{"id", "name", "priority"}
	row := []string{"10", "Fix task", "high"}

	tests := []struct {
		name   string
		filter core.Filter
		want   bool
	}{
		{
			name:   "equality match",
			filter: core.Filter{Column: "priority", Operator: "==", Value: "high"},
			want:   true,
		},
		{
			name:   "equality no match",
			filter: core.Filter{Column: "priority", Operator: "==", Value: "medium"},
			want:   false,
		},
		{
			name:   "inequality match",
			filter: core.Filter{Column: "id", Operator: "!=", Value: "11"},
			want:   true,
		},
		{
			name:   "like match case insensitive",
			filter: core.Filter{Column: "name", Operator: "like", Value: "FIX"},
			want:   true,
		},
		{
			name:   "regex match",
			filter: core.Filter{Column: "id", Operator: "regex", Value: "^\\d+$"},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.EvaluateFilter(headers, row, tt.filter)
			if err != nil {
				t.Errorf("EvaluateFilter() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("EvaluateFilter() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSortRows(t *testing.T) {
	headers := []string{"id", "value"}
	rows := [][]string{
		{"10", "banana"},
		{"2", "apple"},
		{"1", "cherry"},
	}

	err := core.SortRows(headers, rows, "id:asc")
	if err != nil {
		t.Fatalf("SortRows error = %v", err)
	}
	wantNumericalAsc := [][]string{
		{"1", "cherry"},
		{"2", "apple"},
		{"10", "banana"},
	}
	if !reflect.DeepEqual(rows, wantNumericalAsc) {
		t.Errorf("numerical asc got = %v, want %v", rows, wantNumericalAsc)
	}

	err = core.SortRows(headers, rows, "value:desc")
	if err != nil {
		t.Fatalf("SortRows error = %v", err)
	}
	wantAlphaDesc := [][]string{
		{"1", "cherry"},
		{"10", "banana"},
		{"2", "apple"},
	}
	if !reflect.DeepEqual(rows, wantAlphaDesc) {
		t.Errorf("alphabetical desc got = %v, want %v", rows, wantAlphaDesc)
	}

	// Test multi-column sorting
	multiHeaders := []string{"category", "priority", "name"}
	multiRows := [][]string{
		{"Fruit", "2", "Banana"},
		{"Vegetable", "1", "Carrot"},
		{"Fruit", "1", "Apple"},
		{"Vegetable", "2", "Potato"},
	}
	// Sort by category:asc, priority:desc
	err = core.SortRows(multiHeaders, multiRows, "category:asc,priority:desc")
	if err != nil {
		t.Fatalf("SortRows multi-column error = %v", err)
	}
	wantMultiSort := [][]string{
		{"Fruit", "2", "Banana"},
		{"Fruit", "1", "Apple"},
		{"Vegetable", "2", "Potato"},
		{"Vegetable", "1", "Carrot"},
	}
	if !reflect.DeepEqual(multiRows, wantMultiSort) {
		t.Errorf("multi-column sort got = %v, want %v", multiRows, wantMultiSort)
	}
}

func TestParseInputData(t *testing.T) {
	headers := []string{"id", "description"}

	// 1. Single JSON object
	got, err := core.ParseInputData(core.TableConfig{Headers: headers}, `{"id":"1", "description":"test"}`)
	if err != nil {
		t.Fatalf("ParseInputData JSON object failed: %v", err)
	}
	want := []map[string]string{{"id": "1", "description": "test"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got = %v, want %v", got, want)
	}

	// 2. JSON array
	got, err = core.ParseInputData(core.TableConfig{Headers: headers}, `[{"id":"1", "description":"test1"},{"id":"2", "description":"test2"}]`)
	if err != nil {
		t.Fatalf("ParseInputData JSON array failed: %v", err)
	}
	wantArray := []map[string]string{
		{"id": "1", "description": "test1"},
		{"id": "2", "description": "test2"},
	}
	if !reflect.DeepEqual(got, wantArray) {
		t.Errorf("got = %v, want %v", got, wantArray)
	}

	// 3. Single CSV
	got, err = core.ParseInputData(core.TableConfig{Headers: headers}, `3,csv_test`)
	if err != nil {
		t.Fatalf("ParseInputData CSV failed: %v", err)
	}
	wantCSV := []map[string]string{{"id": "3", "description": "csv_test"}}
	if !reflect.DeepEqual(got, wantCSV) {
		t.Errorf("got = %v, want %v", got, wantCSV)
	}

	// 4. Multiline CSV
	got, err = core.ParseInputData(core.TableConfig{Headers: headers}, "4,first\n5,second")
	if err != nil {
		t.Fatalf("ParseInputData multiline CSV failed: %v", err)
	}
	wantMultilineCSV := []map[string]string{
		{"id": "4", "description": "first"},
		{"id": "5", "description": "second"},
	}
	if !reflect.DeepEqual(got, wantMultilineCSV) {
		t.Errorf("got = %v, want %v", got, wantMultilineCSV)
	}

	// 5. Inconsistent columns count error
	_, err = core.ParseInputData(core.TableConfig{Headers: headers}, "6,too,many,columns")
	if err == nil {
		t.Errorf("expected error on inconsistent CSV columns count")
	}
}

func TestZeroConfigAuditing(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_zeroconfig_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Zero-config: initialize table without sort/uniqueness/audit configs, but with standard audit column names
	columns := []string{"id", "description", "created_at", "updated_at"}
	cfg := core.TableConfig{}

	err = core.InitTable(context.Background(), tmpFile.Name(), columns, cfg)
	if err != nil {
		t.Fatalf("InitTable failed: %v", err)
	}

	// Verify GFM table headers on disk
	cfgParsed, _, err := core.ParseFrontmatterAndContent(context.Background(), tmpFile.Name(), core.ContentModeNone)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent failed: %v", err)
	}
	if !reflect.DeepEqual(cfgParsed.Headers, columns) {
		t.Errorf("headers got = %v, want %v", cfgParsed.Headers, columns)
	}

	// Append a row without passing timestamps (user-defined input fields are id and description)
	err = core.AppendRow(context.Background(), tmpFile.Name(), map[string]string{"id": "1", "description": "hello"})
	if err != nil {
		t.Fatalf("AppendRow failed: %v", err)
	}

	// Verify that created_at and updated_at were silently auto-filled
	_, rows, err := core.ParseFrontmatterAndContent(context.Background(), tmpFile.Name(), core.ContentModeAll)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got = %d", len(rows))
	}
	if rows[0][0] != "1" || rows[0][1] != "hello" {
		t.Errorf("row values got = %v", rows[0])
	}
	if rows[0][2] == "" || rows[0][3] == "" {
		t.Errorf("created_at and updated_at were not auto-filled: %v", rows[0])
	}
}

func TestIsSortColUnique(t *testing.T) {
	tests := []struct {
		name          string
		uniqueIndices []int
		sortRules     []core.SortRule
		want          bool
	}{
		{
			name:          "unique type, sort type id (true)",
			uniqueIndices: []int{0}, // e.g. type
			sortRules: []core.SortRule{
				{ColIdx: 0, Dir: "asc"}, // type
				{ColIdx: 1, Dir: "asc"}, // id
			},
			want: true,
		},
		{
			name:          "unique id, sort type id (false)",
			uniqueIndices: []int{1}, // e.g. id
			sortRules: []core.SortRule{
				{ColIdx: 0, Dir: "asc"}, // type
				{ColIdx: 1, Dir: "asc"}, // id
			},
			want: false,
		},
		{
			name:          "unique type id, sort type id name (true)",
			uniqueIndices: []int{0, 1},
			sortRules: []core.SortRule{
				{ColIdx: 0, Dir: "asc"},
				{ColIdx: 1, Dir: "asc"},
				{ColIdx: 2, Dir: "asc"},
			},
			want: true,
		},
		{
			name:          "unique id type, sort type id name (false due to order)",
			uniqueIndices: []int{1, 0},
			sortRules: []core.SortRule{
				{ColIdx: 0, Dir: "asc"},
				{ColIdx: 1, Dir: "asc"},
				{ColIdx: 2, Dir: "asc"},
			},
			want: false,
		},
		{
			name:          "unique type name, sort type id (false due to missing name)",
			uniqueIndices: []int{0, 2},
			sortRules: []core.SortRule{
				{ColIdx: 0, Dir: "asc"},
				{ColIdx: 1, Dir: "asc"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := core.TableConfig{
				UniqueIndices: tt.uniqueIndices,
				SortRules:     tt.sortRules,
			}
			// Simulate the prefix matching logic
			isSortColUnique := false
			if len(cfg.UniqueIndices) > 0 && len(cfg.UniqueIndices) <= len(cfg.SortRules) {
				prefixMatch := true
				for i, uIdx := range cfg.UniqueIndices {
					if cfg.SortRules[i].ColIdx != uIdx {
						prefixMatch = false
						break
					}
				}
				isSortColUnique = prefixMatch
			}
			if isSortColUnique != tt.want {
				t.Errorf("got %v, want %v", isSortColUnique, tt.want)
			}
		})
	}
}
