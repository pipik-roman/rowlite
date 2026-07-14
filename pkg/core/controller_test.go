package core

import (
	"reflect"
	"testing"
)

func TestParseFilter(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		want    Filter
		wantErr bool
	}{
		{
			name: "equality simple",
			expr: "status==TODO",
			want: Filter{Column: "status", Operator: "==", Value: "TODO"},
		},
		{
			name: "inequality spaces",
			expr: "priority != high",
			want: Filter{Column: "priority", Operator: "!=", Value: "high"},
		},
		{
			name: "like operator",
			expr: "name like task",
			want: Filter{Column: "name", Operator: "like", Value: "task"},
		},
		{
			name: "regex operator",
			expr: "id regex ^\\d+$",
			want: Filter{Column: "id", Operator: "regex", Value: "^\\d+$"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFilter(tt.expr)
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
		filter Filter
		want   bool
	}{
		{
			name:   "equality match",
			filter: Filter{Column: "priority", Operator: "==", Value: "high"},
			want:   true,
		},
		{
			name:   "equality no match",
			filter: Filter{Column: "priority", Operator: "==", Value: "medium"},
			want:   false,
		},
		{
			name:   "inequality match",
			filter: Filter{Column: "id", Operator: "!=", Value: "11"},
			want:   true,
		},
		{
			name:   "like match case insensitive",
			filter: Filter{Column: "name", Operator: "like", Value: "FIX"},
			want:   true,
		},
		{
			name:   "regex match",
			filter: Filter{Column: "id", Operator: "regex", Value: "^\\d+$"},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateFilter(headers, row, tt.filter)
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

	err := SortRows(headers, rows, "id:asc")
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

	err = SortRows(headers, rows, "value:desc")
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
}

func TestParseInputData(t *testing.T) {
	headers := []string{"id", "description"}

	// 1. Single JSON object
	got, err := ParseInputData(headers, `{"id":"1", "description":"test"}`)
	if err != nil {
		t.Fatalf("ParseInputData JSON object failed: %v", err)
	}
	want := []map[string]string{{"id": "1", "description": "test"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got = %v, want %v", got, want)
	}

	// 2. JSON array
	got, err = ParseInputData(headers, `[{"id":"1", "description":"test1"},{"id":"2", "description":"test2"}]`)
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
	got, err = ParseInputData(headers, `3,csv_test`)
	if err != nil {
		t.Fatalf("ParseInputData CSV failed: %v", err)
	}
	wantCSV := []map[string]string{{"id": "3", "description": "csv_test"}}
	if !reflect.DeepEqual(got, wantCSV) {
		t.Errorf("got = %v, want %v", got, wantCSV)
	}

	// 4. Multiline CSV
	got, err = ParseInputData(headers, "4,first\n5,second")
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
	_, err = ParseInputData(headers, "6,too,many,columns")
	if err == nil {
		t.Errorf("expected error on inconsistent CSV columns count")
	}
}
