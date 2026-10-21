package lua

import (
	"fmt"
	"testing"
)

func TestBinaryConstantFoldingMatchesRuntime(t *testing.T) {
	for _, tc := range []struct{ lhs, op, rhs string }{
		{"1", "+", "2"},
		{"1", "-", "2"},
		{"3", "*", "4"},
		{"(1 + 2)", "*", "(3 + 4)"},
		{"9007199254740993", "+", "2"},
		{"9223372036854775807", "+", "1"},
		{"0", "-", "9223372036854775807"},
		{"9223372036854775807", "*", "2"},
		{"1.0", "+", "2"},
		{"1", "-", "2.0"},
		{"1.5", "*", "2"},
		{"6", "/", "2"},
		{"7", "%", "3"},
		{"2", "^", "3"},
		{"7", "//", "3"},
		{"7.5", "//", "2.5"},
		{"(0 - 7)", "//", "3"},
		{"9007199254740993", "//", "1"},
		{"7", "&", "3"},
		{"9007199254740993", "&", "1"},
		{"7.0", "&", "3"},
		{"4", "|", "3"},
		{"7", "~", "3"},
		{"3", "<<", "2"},
		{"8", ">>", "2"},
		{"8", "<<", "(0 - 2)"},
		{"3", ">>", "(0 - 2)"},
		{"(0 - 1)", ">>", "64"},
		{"(0 - 1)", "<<", "(0 - 64)"},
	} {
		t.Run(tc.lhs+tc.op+tc.rhs, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			code := fmt.Sprintf(`
local a, b = %s, %s
local folded = %s %s %s
local runtime = a %s b
assert(math.type(folded) == math.type(runtime), "constant folding changes numeric subtype")
assert(folded == runtime, "constant folding changes numeric value")
`, tc.lhs, tc.rhs, tc.lhs, tc.op, tc.rhs, tc.op)
			if err := L.DoString(code); err != nil {
				t.Fatal(err)
			}
		})
	}
}
