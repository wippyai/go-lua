package lua

import (
	"fmt"
	"testing"
)

// Expectations come from the Lua 5.4 reference interpreter: every count,
// byte value and position the string library returns is an integer.
func TestStringLibIntegerResults(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want []int64
	}{
		{"byte default", `string.byte("a")`, []int64{97}},
		{"byte index", `string.byte("abc", 2)`, []int64{98}},
		{"byte range", `string.byte("abc", 1, 3)`, []int64{97, 98, 99}},
		{"byte negative", `string.byte("abc", -1)`, []int64{99}},
		{"len", `string.len("hello")`, []int64{5}},
		{"find empty pattern", `string.find("abc", "")`, []int64{1, 0}},
		{"find plain", `string.find("hello", "ll", 1, true)`, []int64{3, 4}},
		{"find pattern", `string.find("hello", "l+")`, []int64{3, 4}},
		{"find position capture", `string.find("hello", "()ll()")`, []int64{3, 4, 3, 5}},
		{"match position capture", `string.match("hello", "()ll()")`, []int64{3, 5}},
		{"gmatch position capture", `(function() for p in string.gmatch("hello", "()l") do return p end end)()`, []int64{3}},
		{"gsub count", `select(2, string.gsub("hello", "l", "L"))`, []int64{2}},
		{"gsub count no match", `select(2, string.gsub("hello", "z", "Z"))`, []int64{0}},
		{"gsub function position capture", `(function() local r; string.gsub("hello", "()ll", function(p) r = p end); return r end)()`, []int64{3}},
		{"method byte", `("a"):byte()`, []int64{97}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString("return " + tc.expr); err != nil {
				t.Fatal(err)
			}
			if L.GetTop() != len(tc.want) {
				t.Fatalf("got %d results, want %d", L.GetTop(), len(tc.want))
			}
			for i, want := range tc.want {
				got := L.Get(i + 1)
				if got != LInteger(want) {
					t.Fatalf("result %d: got %v (%T), want %s", i+1, got, got, fmt.Sprint(LInteger(want)))
				}
			}
		})
	}
}

func TestStringLibPositionCaptureTableKey(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.DoString(`
local seen
local repl = setmetatable({}, {__index = function(_, k) seen = k; return "X" end})
local s = string.gsub("hello", "()ll", repl)
assert(s == "heXo", s)
assert(math.type(seen) == "integer", math.type(seen))
assert(seen == 3)
`); err != nil {
		t.Fatal(err)
	}
}
