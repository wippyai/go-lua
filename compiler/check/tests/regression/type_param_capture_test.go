package regression

import "testing"

// A generic callee's type parameter and a caller's type parameter of the same
// name are distinct: instantiating the callee keeps the caller's parameter.
func TestCalleeTypeParamKeepsCallerTypeParam(t *testing.T) {
	for _, tt := range []struct {
		name string
		code string
		want string
	}{
		{"stdlib sort of records holding a bounded caller T", `
local function sorted<T: number>(items: {T}): {{v: T}}
	local out: {{v: T}} = {}
	for _, it in ipairs(items) do
		table.insert(out, { v = it })
	end
	table.sort(out, function(a, b) return a.v < b.v end)
	table.sort(out)
	return out
end
local r = sorted({ 2, 1 })
local n: number = r[1].v
`, ""},
		{"stdlib sort of a bounded caller T list", `
local function top<T: number>(items: {T}): T?
	table.sort(items, function(a, b) return a > b end)
	return items[1]
end
local n: number? = top({ 3, 1 })
`, ""},
		{"stdlib bounded generic applied to a bounded caller T", `
local function clamp<T: number>(x: T, lo: T): T
	return math.max(x, lo)
end
local n: integer = clamp(3, 1)
`, ""},
		{"user generic callee inside a bounded user generic", `
local function each<T>(xs: {T}, f: (T) -> ())
	for _, x in ipairs(xs) do f(x) end
end
local function total<T: number>(xs: {T}): number
	local s = 0
	each(xs, function(x: T) s = s + x end)
	return s
end
local n: number = total({ 1, 2 })
`, ""},
		{"caller T stays distinct from callee T in the result", `
local function wrap<T>(x: T): {T}
	return { x }
end
local function first<T: string>(x: T): T
	local list = wrap(x)
	return list[1]
end
local s: string = first("a")
local n: number = first("a")
`, "cannot assign"},
	} {
		t.Run(tt.name, func(t *testing.T) { checkBothModes(t, tt.code, tt.want) })
	}
}
