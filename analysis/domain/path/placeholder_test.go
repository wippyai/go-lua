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

func TestReturnSlotIndexFromString(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	overflow := strconv.FormatInt(int64(maxInt), 10) + "0"

	tests := []struct {
		input string
		want  int
	}{
		{"ret[0]", 0},
		{"ret[42]", 42},
		{"ret[001]", 1},
		{"ret[-1]", -1},
		{"ret[+1]", -1},
		{"ret[]", -1},
		{"ret[1", -1},
		{"ret[1].field", -1},
		{"$1", -1},
		{"ret[" + overflow + "]", -1},
	}

	for _, tc := range tests {
		if got := ReturnSlotIndexFromString(tc.input); got != tc.want {
			t.Errorf("ReturnSlotIndexFromString(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestPathReturnSlotIndex(t *testing.T) {
	if got := (Path{Root: "ret[2]"}).ReturnSlotIndex(); got != 2 {
		t.Fatalf("ReturnSlotIndex() = %d, want 2", got)
	}
	if got := (Path{Root: "ret[2]", Symbol: 1}).ReturnSlotIndex(); got != -1 {
		t.Fatalf("symbol path ReturnSlotIndex() = %d, want -1", got)
	}
	if got := (Path{Root: "ret[2]"}).Field("value").ReturnSlotIndex(); got != 2 {
		t.Fatalf("return-slot member ReturnSlotIndex() = %d, want 2", got)
	}
}
