package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func check(t *testing.T, name, source string) {
	t.Run(name, func(t *testing.T) {
		result := testutil.Check(source, testutil.WithStdlib())
		if result.HasError() {
			t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})
}

// Isolates dynamic-key map value-type widening: assigning true then false to a
// string-keyed map must infer value boolean, in straight-line and across a loop.
func TestMapValueWidensDynamicKey(t *testing.T) {
	pick := `
local function pick(t)
	for _, v in pairs(t) do return v end
	return ""
end
`
	check(t, "literal_key_straight", `
local pending = {}
pending["a"] = true
pending["a"] = false
`)
	check(t, "dynamic_key_straight", pick+`
local pending = {}
local keys = { "a", "b" }
for i = 1, #keys do pending[keys[i]] = true end
local k = pick(keys)
pending[k] = false
`)
	check(t, "dynamic_key_loop", pick+`
local pending = {}
local keys = { "a", "b" }
for i = 1, #keys do pending[keys[i]] = true end
while true do
	local k = pick(keys)
	if pending[k] then pending[k] = false end
	break
end
`)
}

// Isolates a table populated only inside a loop: the pre-loop empty {} joins with
// the back-edge map at the loop header. The empty table is the bottom of the table
// lattice (least information), so the join must yield the map, not collapse to {}.
func TestMapPopulatedOnlyInLoopStaysIndexable(t *testing.T) {
	check(t, "accumulator_in_while", `
local function pick(t)
	for _, v in pairs(t) do return v end
	return ""
end
local keys = { "a", "b" }
local exits = {}
while true do
	local k = pick(keys)
	exits[k] = 1
	if k == "" then break end
end
local x = exits["a"]
return x
`)
}

// Isolates returning a loop accumulator from an early return that is positioned
// before the accumulator write in source order. The return type is the loop-header
// phi of the pre-loop {} and the back-edge map; it must be the map so the caller
// can index the returned value.
func TestReturnedLoopAccumulatorStaysIndexable(t *testing.T) {
	check(t, "early_return_before_write", `
local function pick(t)
	for _, v in pairs(t) do return v end
	return ""
end
local function build(keys)
	local acc = {}
	while true do
		local done = false
		for _, v in pairs(acc) do
			if v then done = true break end
		end
		if done then
			return acc
		end
		local k = pick(keys)
		acc[k] = 1
	end
end
local r = build({ "a", "b" })
local x = r["a"]
return x
`)
}

// Faithful reduction of the ChannelSelect helper: two maps share a record-derived
// key, the accumulator write is nested in a guard that reads the other map, and the
// accumulator is returned from an early return. The returned accumulator must keep
// its index signature.
func TestGuardedDualMapAccumulatorReturn(t *testing.T) {
	check(t, "guarded_write_record_key", `
local function build(keys, ev)
	local pending = {}
	local acc = {}
	for i = 1, #keys do
		pending[keys[i]] = true
	end
	while true do
		local has = false
		for _, p in pairs(pending) do
			if p then has = true break end
		end
		if not has then
			return acc
		end
		if ev.kind == "x" and pending[ev.from] then
			acc[ev.from] = ev
			pending[ev.from] = false
		end
	end
end
local ev = { kind = "x", from = "a" }
local r = build({ "a", "b" }, ev)
if r == nil then return false end
local first = r["a"]
return first
`)
}
