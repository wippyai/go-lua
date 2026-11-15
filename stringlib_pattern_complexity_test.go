package lua

import (
	"strings"
	"testing"
)

// A pattern whose search exceeds the matcher's backtrack budget raises an
// error from every pattern function instead of reporting no match.
func TestStringPatternTooComplexRaises(t *testing.T) {
	setup := `
local s = string.rep("a", 15)
local p = "^" .. string.rep("a*", 10) .. string.rep("a", 15) .. "$"
`
	cases := map[string]string{
		"find":   `return string.find(s, p)`,
		"match":  `return string.match(s, p)`,
		"gmatch": `for m in string.gmatch(s, p) do return m end`,
		"gsub":   `return string.gsub(s, p, "x")`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			err := L.DoString(setup + body)
			if err == nil {
				t.Fatalf("expected pattern too complex error, got %v", L.Get(-1))
			}
			if !strings.Contains(err.Error(), "pattern too complex") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
