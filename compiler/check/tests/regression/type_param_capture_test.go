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

// A generic call's result never carries the callee's own type parameters: a
// type argument with pending evidence waits for it, and a failed inference
// reports once without calling the uninstantiated signature.
func TestGenericCallResultHasNoCalleeTypeParam(t *testing.T) {
	for _, tt := range []struct {
		name string
		code string
		want string
	}{
		{"bounded generic over a local assigned from a dynamic call", `
local function timeline(nodes: {{created_at: number, updated_at: number?}}, flow_start: number, to_ms: (any) -> any)
	local entries = {}
	for _, n in ipairs(nodes) do
		local start_ms = to_ms(n.created_at)
		local end_ms = to_ms(n.updated_at or n.created_at)
		table.insert(entries, {
			node = n,
			start_ms = start_ms,
			duration_ms = math.max(0, end_ms - start_ms),
			rel_ms = math.max(0, start_ms - flow_start),
		})
	end
	table.sort(entries, function(a, b) return a.duration_ms > b.duration_ms end)
	table.sort(entries, function(a, b) return a.start_ms < b.start_ms end)
	return entries
end
`, ""},
		{"record field from a bounded generic over a dynamic local", `
local function f(g: () -> any)
	local s = g()
	local e = { d = math.max(0, s) }
	local z: string = e
end
`, "cannot assign {d: any} to string"},
		{"expected type contradicting the arguments", `
local function identity<T>(x: T): T
	return x
end
local s: string = identity(42)
`, "cannot assign integer to string"},
		{"failed inference", `
local m = math.max("a", 1)
local z: string = m
`, "infer: type argument integer | string does not satisfy constraint number"},
	} {
		t.Run(tt.name, func(t *testing.T) { checkBothModes(t, tt.code, tt.want) })
	}
}
