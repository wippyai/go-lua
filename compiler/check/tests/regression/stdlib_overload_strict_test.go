package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// checkBothModes expects the same outcome in gradual and strict mode.
func checkBothModes(t *testing.T, code string, want string) {
	t.Helper()
	checkModes(t, code, want, want)
}

// checkModes expects no error when the mode's want is empty, otherwise exactly
// one error containing it.
func checkModes(t *testing.T, code string, wantGradual, wantStrict string) {
	t.Helper()
	for _, strict := range []bool{false, true} {
		name, want := "gradual", wantGradual
		if strict {
			name, want = "strict", wantStrict
		}
		t.Run(name, func(t *testing.T) {
			result := testutil.Check(code, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
			messages := testutil.ErrorMessages(result.Diagnostics)
			if want == "" {
				if len(messages) != 0 {
					t.Fatalf("unexpected errors: %v", messages)
				}
				return
			}
			if len(messages) != 1 || !strings.Contains(messages[0], want) {
				t.Fatalf("expected %q, got %v", want, messages)
			}
		})
	}
}

func TestGenericOverloadKeepsArgumentType(t *testing.T) {
	for _, tt := range []struct {
		name string
		code string
		want string
	}{
		{"remove from asserted any list", `
local M = { MAX_UNDO = 10 }
function M.record(t: any, forward_events: any, inverse_events: any)
	local undo_stack = t.undo :: { any }
	undo_stack[#undo_stack + 1] = { forward = forward_events or {}, inverse = inverse_events or {} }
	while #undo_stack > M.MAX_UNDO do table.remove(undo_stack, 1) end
end
return M
`, ""},
		{"remove from any list", `
local function f(list: { any })
	table.remove(list, 1)
	table.remove(list)
end
`, ""},
		{"remove keeps element type", `
local function f(list: { string }): string?
	return table.remove(list, 1)
end
`, ""},
		{"remove rejects non list", `table.remove("x", 1)`, "argument 1:"},
		{"insert into any list", `
local function f(list: { any }, v: any)
	table.insert(list, v)
	table.insert(list, 1, v)
end
`, ""},
		{"random with any bounds", `
local function f(m: integer, n: integer): number
	return math.random() + math.random(m) + math.random(m, n)
end
`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) { checkBothModes(t, tt.code, tt.want) })
	}
}

func TestIndexedWriteKeepsLocalType(t *testing.T) {
	for _, tt := range []struct {
		name string
		code string
	}{
		{"asserted list append", `
local function f(u: any)
	local s = u :: { any }
	s[#s + 1] = 1
	local x: { any } = s
end
`},
		{"alias of annotated param", `
local function f(p: { any })
	local s = p
	s[#s + 1] = 1
	local x: { any } = s
end
`},
		{"call result", `
local function g(): { any } return {} end
local s = g()
s[#s + 1] = 1
local x: { any } = s
`},
		{"typed element append", `
local function f(u: any)
	local s = u :: { number }
	s[#s + 1] = 1
	local x: { number } = s
end
`},
	} {
		t.Run(tt.name, func(t *testing.T) { checkBothModes(t, tt.code, "") })
	}
}

// An any element is consistent with a typed comparator in gradual mode; strict
// mode treats it as unknown, which the comparator cannot accept.
func TestSortTypedComparatorOnAnyList(t *testing.T) {
	checkModes(t, `
local function f(list: { any })
	table.sort(list, function(a: string, b: string): boolean return a < b end)
end
`, "", "argument 2:")
}

func TestSortComparatorMatchesElementType(t *testing.T) {
	for _, tt := range []struct {
		name string
		code string
		want string
	}{
		{"typed record comparator", `
type Map = { name: string }
local function f(list: { Map })
	table.sort(list, function(a: Map, b: Map): boolean return a.name < b.name end)
end
`, ""},
		{"untyped comparator", `
local list = { 3, 1, 2 }
table.sort(list, function(a, b) return a < b end)
`, ""},
		{"string comparator", `
local function f(list: { string })
	table.sort(list, function(a: string, b: string): boolean return a < b end)
	table.sort(list)
end
`, ""},
		{"any list untyped comparator", `
local function f(list: { any })
	table.sort(list, function(a, b) return a.order < b.order end)
end
`, ""},
		{"mismatched comparator", `
local function f(list: { number })
	table.sort(list, function(a: string, b: string): boolean return a < b end)
end
`, "unsatisfiable bounds: number is not subtype of never"},
	} {
		t.Run(tt.name, func(t *testing.T) { checkBothModes(t, tt.code, tt.want) })
	}
}

// table.sort orders the values at integer keys 1..#list: it takes every
// integer-keyed table, arrays included, and still rejects a possibly nil list.
func TestSortIntegerKeyedTable(t *testing.T) {
	for _, tt := range []struct {
		name string
		code string
		want string
	}{
		{"integer-keyed map with comparator", `
local function f(m: {[integer]: {[string]: any}})
	table.sort(m, function(a, b) return a.x < b.x end)
end
`, ""},
		{"integer-keyed map without comparator", `
local function f(m: {[integer]: {[string]: any}})
	table.sort(m)
end
`, ""},
		{"integer-keyed map with typed comparator", `
type Entry = { name: string }
local function f(m: {[integer]: Entry})
	table.sort(m, function(a: Entry, b: Entry): boolean return a.name < b.name end)
end
`, ""},
		{"array", `
local function f(list: {string})
	table.sort(list, function(a: string, b: string): boolean return a < b end)
	table.sort(list)
end
`, ""},
		{"possibly nil integer-keyed map", `
local function f(m: {[integer]: {[string]: any}}?)
	table.sort(m)
end
`, "argument 1:"},
		{"possibly nil array", `
local function f(list: {string}?)
	table.sort(list)
end
`, "argument 1:"},
		{"string-keyed map", `
local function f(m: {[string]: number})
	table.sort(m)
end
`, "argument 1:"},
		{"integer-keyed map with mismatched comparator", `
local function f(m: {[integer]: number})
	table.sort(m, function(a: string, b: string): boolean return a < b end)
end
`, "unsatisfiable bounds: number is not subtype of never"},
		{"literal list of records with differing fields", `
local entries = {
	{ id = "a:explicit", kind = "function.lua", meta = { type = "bootloader", order = 10 } },
	{ id = "b:default", kind = "function.lua", meta = { type = "bootloader" } },
}
table.sort(entries, function(a, b)
	local a_order = a.meta and a.meta.order or 999
	local b_order = b.meta and b.meta.order or 999
	if a_order ~= b_order then return a_order < b_order end
	return a.id < b.id
end)
`, ""},
		{"literal list of numbers and strings", `
local items = { 1, "two", 3 }
table.sort(items, function(a, b) return tostring(a) < tostring(b) end)
`, ""},
		{"literal list of numbers and strings with number comparator", `
local items = { 1, "two", 3 }
table.sort(items, function(a: number, b: number): boolean return a < b end)
`, "unsatisfiable bounds: integer | string is not subtype of"},
	} {
		t.Run(tt.name, func(t *testing.T) { checkBothModes(t, tt.code, tt.want) })
	}
}

// A record is a map from its field names to the join of its field types.
func TestGenericMapParamFromRecord(t *testing.T) {
	checkBothModes(t, `
local function values<V>(t: {[string]: V}): {V}
	local out: {V} = {}
	for _, v in pairs(t) do out[#out + 1] = v end
	return out
end
local vs = values({ a = 1, b = "x" })
local s: string = vs[1]
`, "cannot assign integer | string to string")
}
