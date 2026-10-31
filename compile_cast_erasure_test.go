// SPDX-License-Identifier: MPL-2.0

package lua

import (
	"testing"
)

// A type cast (`::`) and a non-nil assertion (`!`) are erased at runtime: the
// value they wrap must reach its consumer unchanged. Short-circuit operators
// finish by patching their jumps to the pc that holds the result, so the
// register-propagation peephole must not pop that instruction. Popping it
// leaves the already-resolved jump skipping the consuming instruction, and a
// table field, an assignment target or an arithmetic operand silently loses
// the value.
func TestCastErasureKeepsShortCircuitResult(t *testing.T) {
	const prelude = `
		local function id(value) return value end
		local present, fallback = "A", "B"
		local absent = nil
	`

	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name:     "constructor field or cast truthy lhs",
			code:     `local t = {k = (id(present) or fallback) :: string, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "A,1",
		},
		{
			name:     "constructor field or cast falsy lhs",
			code:     `local t = {k = (id(absent) or fallback) :: string, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "B,1",
		},
		{
			name:     "constructor field and cast truthy lhs",
			code:     `local t = {k = (id(present) and fallback) :: string, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "B,1",
		},
		{
			name:     "constructor field and cast falsy lhs",
			code:     `local t = {k = (id(absent) and fallback) :: string?, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "nil,1",
		},
		{
			name:     "constructor field comparison cast",
			code:     `local t = {k = (id(1) < 2) :: boolean, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "true,1",
		},
		{
			name:     "constructor field comparison cast false",
			code:     `local t = {k = (id(2) < 1) :: boolean, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "false,1",
		},
		{
			name:     "constructor field unparenthesized or cast",
			code:     `local t = {k = id(absent) or fallback :: string, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "B,1",
		},
		{
			name:     "constructor field unparenthesized and cast",
			code:     `local t = {k = id(present) and fallback :: string, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "B,1",
		},
		{
			name:     "constructor field unparenthesized comparison cast",
			code:     `local t = {k = id(1) < 2 :: number, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "true,1",
		},
		{
			name:     "constructor field non-nil assertion over or",
			code:     `local t = {k = (id(present) or fallback)!, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "A,1",
		},
		{
			name:     "constructor field double cast",
			code:     `local t = {k = ((id(present) or fallback) :: string) :: string, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "A,1",
		},
		{
			name:     "constructor field bracket key or cast",
			code:     `local t = {["k"] = (id(present) or fallback) :: string, n = 1} return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "A,1",
		},
		{
			name:     "positional item or cast",
			code:     `local t = {(id(present) or fallback) :: string, 1} return tostring(t[1]) .. "," .. tostring(t[2])`,
			expected: "A,1",
		},
		{
			name:     "positional item and cast",
			code:     `local t = {(id(present) and fallback) :: string, 1} return tostring(t[1]) .. "," .. tostring(t[2])`,
			expected: "B,1",
		},
		{
			name:     "positional item comparison cast",
			code:     `local t = {(id(1) < 2) :: boolean, 1} return tostring(t[1]) .. "," .. tostring(t[2])`,
			expected: "true,1",
		},
		{
			name:     "positional item unparenthesized or cast",
			code:     `local t = {id(absent) or fallback :: string, 1} return tostring(t[1]) .. "," .. tostring(t[2])`,
			expected: "B,1",
		},
		{
			name:     "call argument or cast",
			code:     `return tostring(id((id(present) or fallback) :: string))`,
			expected: "A",
		},
		{
			name:     "call argument and cast",
			code:     `return tostring(id((id(absent) and fallback) :: string?))`,
			expected: "nil",
		},
		{
			name:     "call argument comparison cast",
			code:     `return tostring(id((id(1) < 2) :: boolean))`,
			expected: "true",
		},
		{
			name: "call argument or cast followed by another argument",
			code: `local function pair(x, y) return tostring(x) .. "," .. tostring(y) end
				return pair((id(present) or fallback) :: string, 1)`,
			expected: "A,1",
		},
		{
			name:     "call argument unparenthesized or cast",
			code:     `return tostring(id(id(absent) or fallback :: string))`,
			expected: "B",
		},
		{
			name:     "field assignment or cast",
			code:     `local t = {} t.k = (id(present) or fallback) :: string t.n = 1 return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "A,1",
		},
		{
			name:     "index assignment and cast",
			code:     `local t = {} t["k"] = (id(present) and fallback) :: string t.n = 1 return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "B,1",
		},
		{
			name:     "index assignment comparison cast",
			code:     `local t = {} t.k = (id(1) < 2) :: boolean t.n = 1 return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "true,1",
		},
		{
			name:     "arithmetic operand or cast",
			code:     `return tostring(((id(1) or 2) :: number) + 1)`,
			expected: "2",
		},
		{
			name:     "arithmetic operand and cast",
			code:     `return tostring(((id(1) and 2) :: number) * 3)`,
			expected: "6",
		},
		{
			name:     "concat operand or cast",
			code:     `return ((id(present) or fallback) :: string) .. "!"`,
			expected: "A!",
		},
		{
			name:     "index key or cast",
			code:     `local t = {A = 7} return tostring(t[(id(present) or fallback) :: string])`,
			expected: "7",
		},
		{
			name:     "local assignment or cast",
			code:     `local v = (id(present) or fallback) :: string return tostring(v)`,
			expected: "A",
		},
		{
			name:     "condition or cast",
			code:     `if ((id(absent) or fallback) :: string) == "B" then return "yes" end return "no"`,
			expected: "yes",
		},
		{
			name:     "nested constructor field or cast",
			code:     `local t = {inner = {k = (id(present) or fallback) :: string, n = 1}} return tostring(t.inner.k) .. "," .. tostring(t.inner.n)`,
			expected: "A,1",
		},
		{
			name: "union cast on constructor field",
			code: `local row: {[string]: unknown} = {execution_state = "a"}
				local function text(value: unknown): string?
					if type(value) ~= "string" then return nil end
					return value
				end
				local t = {k = (text(row.execution_state) or "b") :: string, n = 1}
				return tostring(t.k) .. "," .. tostring(t.n)`,
			expected: "a,1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.DoString(prelude + "\n" + tt.code); err != nil {
				t.Fatalf("runtime error: %v", err)
			}
			got := L.Get(-1)
			if got.String() != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, got.String())
			}
		})
	}
}
