package path

import (
	"strconv"
	"testing"
)

func TestPlaceholderIndexFromString(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	overflow := strconv.FormatInt(int64(maxInt), 10) + "0"

	tests := []struct {
		input string
		want  int
	}{
		{"$0", 0},
		{"$42", 42},
		{"$001", 1},
		{"$-1", -1},
		{"$+1", -1},
		{"$", -1},
		{"x", -1},
		{"$" + overflow, -1},
	}

	for _, tc := range tests {
		if got := PlaceholderIndexFromString(tc.input); got != tc.want {
			t.Errorf("PlaceholderIndexFromString(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}
