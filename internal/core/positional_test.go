package core

import (
	"reflect"
	"testing"
)

func intPtr(v int) *int {
	return &v
}

func TestParseIndexSpec(t *testing.T) {
	totalRows := 10

	tests := []struct {
		name      string
		spec      string
		totalRows int
		expected  []int
		wantErr   bool
	}{
		{
			name:      "single 0 index",
			spec:      "0",
			totalRows: totalRows,
			expected:  []int{0},
		},
		{
			name:      "single positive index",
			spec:      "5",
			totalRows: totalRows,
			expected:  []int{5},
		},
		{
			name:      "single negative index -1 (last row)",
			spec:      "-1",
			totalRows: totalRows,
			expected:  []int{9},
		},
		{
			name:      "single negative index -3",
			spec:      "-3",
			totalRows: totalRows,
			expected:  []int{7},
		},
		{
			name:      "comma-separated list",
			spec:      "0, 2, 4",
			totalRows: totalRows,
			expected:  []int{0, 2, 4},
		},
		{
			name:      "comma-separated with negative",
			spec:      "0, -1, -2",
			totalRows: totalRows,
			expected:  []int{0, 9, 8},
		},
		{
			name:      "JSON array",
			spec:      "[0, 3, 6]",
			totalRows: totalRows,
			expected:  []int{0, 3, 6},
		},
		{
			name:      "continuous range with dots",
			spec:      "2..5",
			totalRows: totalRows,
			expected:  []int{2, 3, 4, 5},
		},
		{
			name:      "continuous range with colons",
			spec:      "2:5",
			totalRows: totalRows,
			expected:  []int{2, 3, 4, 5},
		},
		{
			name:      "continuous range with negative start/end",
			spec:      "-4..-1",
			totalRows: totalRows,
			expected:  []int{6, 7, 8, 9},
		},
		{
			name:      "stepped range with dots",
			spec:      "0..8..2",
			totalRows: totalRows,
			expected:  []int{0, 2, 4, 6, 8},
		},
		{
			name:      "stepped range with colons",
			spec:      "1:7:2",
			totalRows: totalRows,
			expected:  []int{1, 3, 5, 7},
		},
		{
			name:      "descending range",
			spec:      "5..2",
			totalRows: totalRows,
			expected:  []int{5, 4, 3, 2},
		},
		{
			name:      "out of bounds positive",
			spec:      "10",
			totalRows: totalRows,
			wantErr:   true,
		},
		{
			name:      "out of bounds negative",
			spec:      "-11",
			totalRows: totalRows,
			wantErr:   true,
		},
		{
			name:      "empty table error",
			spec:      "0",
			totalRows: 0,
			wantErr:   true,
		},
		{
			name:      "invalid characters",
			spec:      "abc",
			totalRows: totalRows,
			wantErr:   true,
		},
		{
			name:      "invalid step <= 0",
			spec:      "0..5..0",
			totalRows: totalRows,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIndexSpec(tt.spec, tt.totalRows)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseIndexSpec(%q, %d) error = %v, wantErr %v", tt.spec, tt.totalRows, err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("ParseIndexSpec(%q, %d) = %v, want %v", tt.spec, tt.totalRows, got, tt.expected)
			}
		})
	}
}

func TestResolvePositionalIndices(t *testing.T) {
	totalRows := 10

	t.Run("default all rows", func(t *testing.T) {
		indices, err := ResolvePositionalIndices(totalRows, PositionalOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		if !reflect.DeepEqual(indices, expected) {
			t.Errorf("got %v, want %v", indices, expected)
		}
	})

	t.Run("head only", func(t *testing.T) {
		indices, err := ResolvePositionalIndices(totalRows, PositionalOptions{Head: intPtr(3)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{0, 1, 2}
		if !reflect.DeepEqual(indices, expected) {
			t.Errorf("got %v, want %v", indices, expected)
		}
	})

	t.Run("head larger than table", func(t *testing.T) {
		indices, err := ResolvePositionalIndices(totalRows, PositionalOptions{Head: intPtr(20)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		if !reflect.DeepEqual(indices, expected) {
			t.Errorf("got %v, want %v", indices, expected)
		}
	})

	t.Run("tail only", func(t *testing.T) {
		indices, err := ResolvePositionalIndices(totalRows, PositionalOptions{Tail: intPtr(3)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{7, 8, 9}
		if !reflect.DeepEqual(indices, expected) {
			t.Errorf("got %v, want %v", indices, expected)
		}
	})

	t.Run("tail larger than table", func(t *testing.T) {
		indices, err := ResolvePositionalIndices(totalRows, PositionalOptions{Tail: intPtr(20)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		if !reflect.DeepEqual(indices, expected) {
			t.Errorf("got %v, want %v", indices, expected)
		}
	})

	t.Run("head and tail partition", func(t *testing.T) {
		indices, err := ResolvePositionalIndices(totalRows, PositionalOptions{Head: intPtr(2), Tail: intPtr(3)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{0, 1, 7, 8, 9}
		if !reflect.DeepEqual(indices, expected) {
			t.Errorf("got %v, want %v", indices, expected)
		}
	})

	t.Run("head and tail sum >= totalRows returns all", func(t *testing.T) {
		indices, err := ResolvePositionalIndices(totalRows, PositionalOptions{Head: intPtr(6), Tail: intPtr(5)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		if !reflect.DeepEqual(indices, expected) {
			t.Errorf("got %v, want %v", indices, expected)
		}
	})

	t.Run("index overrides head and tail", func(t *testing.T) {
		indices, err := ResolvePositionalIndices(totalRows, PositionalOptions{
			Head:  intPtr(5),
			Tail:  intPtr(5),
			Index: "1, 4",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{1, 4}
		if !reflect.DeepEqual(indices, expected) {
			t.Errorf("got %v, want %v", indices, expected)
		}
	})

	t.Run("offset and limit pagination", func(t *testing.T) {
		indices, err := ResolvePositionalIndices(totalRows, PositionalOptions{
			Offset: 3,
			Limit:  intPtr(4),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []int{3, 4, 5, 6}
		if !reflect.DeepEqual(indices, expected) {
			t.Errorf("got %v, want %v", indices, expected)
		}
	})

	t.Run("offset beyond length returns empty", func(t *testing.T) {
		indices, err := ResolvePositionalIndices(totalRows, PositionalOptions{
			Offset: 15,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(indices) != 0 {
			t.Errorf("got %v, want empty slice", indices)
		}
	})

	t.Run("negative offset error", func(t *testing.T) {
		_, err := ResolvePositionalIndices(totalRows, PositionalOptions{Offset: -1})
		if err == nil {
			t.Fatalf("expected error for negative offset")
		}
	})

	t.Run("zero limit error", func(t *testing.T) {
		_, err := ResolvePositionalIndices(totalRows, PositionalOptions{Limit: intPtr(0)})
		if err == nil {
			t.Fatalf("expected error for limit <= 0")
		}
	})
}

func TestApplyPositional(t *testing.T) {
	rows := MatchedRows{
		{Index: 0, Row: []string{"A"}},
		{Index: 1, Row: []string{"B"}},
		{Index: 2, Row: []string{"C"}},
		{Index: 3, Row: []string{"D"}},
		{Index: 4, Row: []string{"E"}},
	}

	res, err := rows.ApplyPositional(PositionalOptions{Head: intPtr(2)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(res))
	}
	if res[0].Index != 0 || res[0].Row[0] != "A" {
		t.Errorf("unexpected row 0: %+v", res[0])
	}
	if res[1].Index != 1 || res[1].Row[0] != "B" {
		t.Errorf("unexpected row 1: %+v", res[1])
	}
}
