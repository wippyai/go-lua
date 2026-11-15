package lua

import (
	"strings"
	"testing"
)

// Expectations come from the Lua 5.4 reference interpreter.
func TestTableConcatRange(t *testing.T) {
	cases := []struct {
		name, expr, want string
	}{
		{"whole", `table.concat({1, 2, 3}, ",")`, "1,2,3"},
		{"explicit range", `table.concat({1, 2, 3}, ",", 2, 3)`, "2,3"},
		{"single element", `table.concat({1, 2, 3}, ",", 2, 2)`, "2"},
		{"i greater than j", `table.concat({1, 2, 3}, ",", 3, 2)`, ""},
		{"i past length", `table.concat({1, 2, 3}, ",", 4, 3)`, ""},
		{"i past length without j", `table.concat({1, 2, 3}, ",", 4)`, ""},
		{"i greater than j beyond length", `table.concat({1, 2, 3}, ",", 9, 5)`, ""},
		{"empty table", `table.concat({}, ",")`, ""},
		{"index zero", `table.concat({[0] = "z", "a"}, ",", 0, 1)`, "z,a"},
		{"negative indices", `table.concat({[-1] = "m", [0] = "z"}, ",", -1, 0)`, "m,z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString("return " + tc.expr); err != nil {
				t.Fatal(err)
			}
			if got := L.Get(-1); got != LString(tc.want) {
				t.Fatalf("got %q, want %q", got.String(), tc.want)
			}
		})
	}
}

func TestTableConcatRangeBeyondLengthErrors(t *testing.T) {
	cases := []struct {
		name, expr string
		index      string
	}{
		{"j past length", `table.concat({1, 2, 3}, ",", 1, 5)`, "index 4"},
		{"i below one", `table.concat({1, 2, 3}, ",", 0)`, "index 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			err := L.DoString("return " + tc.expr)
			if err == nil {
				t.Fatalf("expected error, got %q", L.Get(-1).String())
			}
			if !strings.Contains(err.Error(), "invalid value") || !strings.Contains(err.Error(), tc.index) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
