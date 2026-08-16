package core_test

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/pipik-roman/rowlite/internal/core"
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

func TestParseFilters(t *testing.T) {
	t.Run("empty filters", func(t *testing.T) {
		filters, err := core.ParseFilters(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(filters) != 0 {
			t.Errorf("expected 0 filters, got %d", len(filters))
		}
	})

	t.Run("valid multi filters", func(t *testing.T) {
		where := []string{"status == active", "priority != low", "tag like dev"}
		filters, err := core.ParseFilters(where)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(filters) != 3 {
			t.Fatalf("expected 3 filters, got %d", len(filters))
		}
		if filters[0].Column != "status" || filters[0].Operator != "==" || filters[0].Value != "active" {
			t.Errorf("unexpected filter 0: %+v", filters[0])
		}
		if filters[1].Column != "priority" || filters[1].Operator != "!=" || filters[1].Value != "low" {
			t.Errorf("unexpected filter 1: %+v", filters[1])
		}
		if filters[2].Column != "tag" || filters[2].Operator != "like" || filters[2].Value != "dev" {
			t.Errorf("unexpected filter 2: %+v", filters[2])
		}
	})

	t.Run("invalid filter error", func(t *testing.T) {
		_, err := core.ParseFilters([]string{"status == active", "invalid_clause"})
		if err == nil {
			t.Fatal("expected error on invalid clause, got nil")
		}
	})
}

func TestMatchesRow(t *testing.T) {
	headers := []string{"id", "name", "status"}
	row := []string{"10", "Deploy App", "active"}

	filters, err := core.ParseFilters([]string{"id == 10", "status == active"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	match, err := core.MatchesRow(headers, row, filters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !match {
		t.Error("expected row to match all filters")
	}

	nonMatchingFilters, _ := core.ParseFilters([]string{"id == 10", "status == done"})
	match, err = core.MatchesRow(headers, row, nonMatchingFilters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match {
		t.Error("expected row not to match filters")
	}
}

func TestSelectRowsAndHelpers(t *testing.T) {
	headers := []string{"id", "title", "category"}
	rows := [][]string{
		{"1", "Task A", "work"},
		{"2", "Task B", "personal"},
		{"3", "Task C", "work"},
		{"4", "Task D", "work"},
	}

	// 1. Select with no filters -> returns all rows preserving indices
	allMatches, err := core.SelectRows(headers, rows, nil)
	if err != nil {
		t.Fatalf("SelectRows error: %v", err)
	}
	if len(allMatches) != 4 {
		t.Fatalf("expected 4 matches, got %d", len(allMatches))
	}
	expectedIndices := []int{0, 1, 2, 3}
	if !reflect.DeepEqual(allMatches.Indices(), expectedIndices) {
		t.Errorf("Indices() = %v, want %v", allMatches.Indices(), expectedIndices)
	}
	if !reflect.DeepEqual(allMatches.Rows(), rows) {
		t.Errorf("Rows() = %v, want %v", allMatches.Rows(), rows)
	}

	// 2. Select with category == work
	filters, _ := core.ParseFilters([]string{"category == work"})
	workMatches, err := core.SelectRows(headers, rows, filters)
	if err != nil {
		t.Fatalf("SelectRows error: %v", err)
	}
	if len(workMatches) != 3 {
		t.Fatalf("expected 3 work matches, got %d", len(workMatches))
	}
	expectedWorkIndices := []int{0, 2, 3}
	if !reflect.DeepEqual(workMatches.Indices(), expectedWorkIndices) {
		t.Errorf("work Indices() = %v, want %v", workMatches.Indices(), expectedWorkIndices)
	}
	expectedWorkRows := [][]string{
		{"1", "Task A", "work"},
		{"3", "Task C", "work"},
		{"4", "Task D", "work"},
	}
	if !reflect.DeepEqual(workMatches.Rows(), expectedWorkRows) {
		t.Errorf("work Rows() = %v, want %v", workMatches.Rows(), expectedWorkRows)
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

func TestColumnResolution(t *testing.T) {
	headers := []string{"ID", "Title", "Item Name", "Tags", "Priority", "Status"}

	// 1. Test ResolveColumn logic
	tests := []struct {
		target      string
		wantIdx     int
		wantName    string
		expectErr   bool
		errContains string
	}{
		{"ID", 0, "ID", false, ""},
		{"id", 0, "ID", false, ""},
		{"tags", 3, "Tags", false, ""},
		{"item_name", 2, "Item Name", false, ""},
		{"item-name", 2, "Item Name", false, ""},
		{"itemname", 2, "Item Name", false, ""},
		{"tagss", -1, "", true, `column "tagss" not found. Did you mean "Tags"?`},
		{"priorityy", -1, "", true, `column "priorityy" not found. Did you mean "Priority"?`},
		{"abc", -1, "", true, `column "abc" not found. Available columns: [ID, Title, Item Name, Tags, Priority, Status]`},
	}

	for _, tt := range tests {
		idx, name, err := core.ResolveColumn(headers, tt.target)
		if tt.expectErr {
			if err == nil {
				t.Errorf("ResolveColumn(%q) expected error, got nil", tt.target)
			} else if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("ResolveColumn(%q) error %q does not contain %q", tt.target, err.Error(), tt.errContains)
			}
		} else {
			if err != nil {
				t.Errorf("ResolveColumn(%q) unexpected error: %v", tt.target, err)
			}
			if idx != tt.wantIdx || name != tt.wantName {
				t.Errorf("ResolveColumn(%q) got index %d, name %q; want index %d, name %q", tt.target, idx, name, tt.wantIdx, tt.wantName)
			}
		}
	}

	// 2. Test EvaluateFilter with case-insensitive/normalized column names
	row := []string{"1", "Task A", "Keyboard", "beach,coding", "high", "todo"}

	// Case-insensitive check
	fltLower, err := core.ParseFilter("tags==beach,coding")
	if err != nil {
		t.Fatalf("ParseFilter failed: %v", err)
	}
	match, err := core.EvaluateFilter(headers, row, fltLower)
	if err != nil || !match {
		t.Errorf("EvaluateFilter failed on case-insensitive column: err=%v, match=%v", err, match)
	}

	// Normalized check
	fltNorm, err := core.ParseFilter("item_name==Keyboard")
	if err != nil {
		t.Fatalf("ParseFilter failed: %v", err)
	}
	match, err = core.EvaluateFilter(headers, row, fltNorm)
	if err != nil || !match {
		t.Errorf("EvaluateFilter failed on normalized column: err=%v, match=%v", err, match)
	}
}

func TestAppendRowCaseInsensitiveAndNormalized(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_append_insensitive_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	columns := []string{"ID", "Item Name", "Tags"}
	cfg := core.TableConfig{}

	err = core.InitTable(context.Background(), tmpFile.Name(), columns, cfg)
	if err != nil {
		t.Fatalf("InitTable failed: %v", err)
	}

	// Append using case-insensitive and normalized JSON keys
	err = core.AppendRow(context.Background(), tmpFile.Name(), map[string]string{
		"id":        "1",
		"item_name": "Laptop",
		"tags":      "work,tech",
	})
	if err != nil {
		t.Fatalf("AppendRow with insensitive/normalized keys failed: %v", err)
	}

	_, rows, err := core.ParseFrontmatterAndContent(context.Background(), tmpFile.Name(), core.ContentModeAll)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent failed: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}

	if rows[0][0] != "1" || rows[0][1] != "Laptop" || rows[0][2] != "work,tech" {
		t.Errorf("Unexpected row values: %v", rows[0])
	}
}

func TestSortTableActionWithTrigger(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_sort_trigger_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	columns := []string{"category", "priority", "name"}
	cfg := core.TableConfig{}
	if err := core.InitTable(context.Background(), tmpFile.Name(), columns, cfg); err != nil {
		t.Fatalf("InitTable failed: %v", err)
	}

	rowsToAppend := []map[string]string{
		{"category": "Fruit", "priority": "1", "name": "Apple"},
		{"category": "Fruit", "priority": "2", "name": "Banana"},
		{"category": "Vegetable", "priority": "1", "name": "Carrot"},
	}
	for _, r := range rowsToAppend {
		if err := core.AppendRow(context.Background(), tmpFile.Name(), r); err != nil {
			t.Fatalf("AppendRow failed: %v", err)
		}
	}

	// Sort with trigger=true: category:asc, priority:desc
	err = core.SortTableAction(context.Background(), tmpFile.Name(), "category:asc,priority:desc", true)
	if err != nil {
		t.Fatalf("SortTableAction failed: %v", err)
	}

	cfgParsed, rows, err := core.ParseFrontmatterAndContent(context.Background(), tmpFile.Name(), core.ContentModeAll)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent failed: %v", err)
	}

	if cfgParsed.Sort != "category:asc,priority:desc" {
		t.Errorf("expected frontmatter sort to be updated to %q, got %q", "category:asc,priority:desc", cfgParsed.Sort)
	}

	wantSortedRows := [][]string{
		{"Fruit", "2", "Banana"},
		{"Fruit", "1", "Apple"},
		{"Vegetable", "1", "Carrot"},
	}

	if !reflect.DeepEqual(rows, wantSortedRows) {
		t.Errorf("rows not sorted properly with trigger=true: got %v, want %v", rows, wantSortedRows)
	}
}

func TestIsSortedBy(t *testing.T) {
	cfgWithRules := core.TableConfig{
		Headers: []string{"id", "title", "priority"},
		SortRules: []core.SortRule{
			{ColIdx: 0, Dir: "asc"},
			{ColIdx: 2, Dir: "desc"},
		},
	}

	if !cfgWithRules.IsSortedBy("id", "asc") {
		t.Error("expected IsSortedBy('id', 'asc') to be true")
	}
	if !cfgWithRules.IsSortedBy("id", "") {
		t.Error("expected IsSortedBy('id', '') with default asc to be true")
	}
	if cfgWithRules.IsSortedBy("id", "desc") {
		t.Error("expected IsSortedBy('id', 'desc') to be false")
	}
	if cfgWithRules.IsSortedBy("priority", "desc") {
		t.Error("expected IsSortedBy('priority', 'desc') to be false (not primary sort column)")
	}
}

