package lua

import (
	"strings"
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
)

// soundnessProbe records a program with its expected diagnostic state. Each
// fixed probe produces an error diagnostic; an unfixed probe produces none.
// An unexpected result fails this test so the expectation stays explicit.
type soundnessProbe struct {
	name  string
	fixed bool
	src   string
}

var soundnessProbes = []soundnessProbe{
	{
		// Array covariance is sound for reads and heap-tracked writes, but a
		// covariant alias of an opaque (parameter) array is not tracked, so a write
		// through the alias corrupts the original element type undetected.
		name:  "array-covariance-param-writeback",
		fixed: true,
		src: `local function f(a: {string}): string
			local b: {string | number} = a
			b[1] = 42
			local s: string = a[1]
			return s
		end return f`,
	},
	{
		name:  "map-isany-assign",
		fixed: true,
		src: `local function f(m: {[string]: string}): nil
			local n: {[string]: any} = m
			return nil
		end return f`,
	},
	{
		name:  "nonnil-assert-on-always-nil",
		fixed: true,
		src: `local function f(): string
			local x: nil = nil
			return x!
		end return f`,
	},
	{
		name:  "eq-false-then-edge-narrowing",
		fixed: true,
		src: `local function f(x: string | false): string
			if x == false then
				local s: string = x
				return s
			end
			return "y"
		end return f`,
	},
	{
		name:  "missing-required-field-call-arg",
		fixed: true,
		src: `local function g(o: {name: string}): number return 1 end
		local function f(): number
			return g({})
		end return f`,
	},
	{
		name:  "missing-required-field-return",
		fixed: true,
		src: `local function f(): {name: string}
			return {}
		end return f`,
	},
	{
		name:  "gmatch-iterator-returns-string",
		fixed: true,
		src: `local function f(s: string): number
			for w in s:gmatch("%a+") do
				local n: number = w
				return n
			end
			return 0
		end return f`,
	},
	{
		name:  "plain-arity-mismatch",
		fixed: true,
		src: `local function g(x: number, y: number): number return x end
		local function f(): number
			return g(1)
		end return f`,
	},
	{
		// Floor division of float operands is a float at runtime, not an integer.
		name:  "floor-division-float-operands",
		fixed: true,
		src: `local function f(a: number, b: number): integer
			local x: integer = a // b
			return x
		end return f`,
	},
	{
		// A same-module call that mutates a field invalidates the field's guard
		// narrowing: after clear(box) sets box.value = nil, box.value is nil again.
		name:  "interproc-mutation-vs-narrowing",
		fixed: true,
		src: `local function clear(b: { value: string? }) b.value = nil end
		local function f(box: { value: string? }): string
			if box.value then
				clear(box)
				local n: string = box.value
				return n
			end
			return "x"
		end return f`,
	},
	{
		// A guard-keyed assignment fact must not survive a closure call that mutates
		// the dependent local between guards.
		name:  "partitioned-assignment-closure-mutation",
		fixed: true,
		src: `local function f(ok: boolean): string
			local x: string?
			if ok then x = "ready" end
			local function clear() x = nil end
			clear()
			if ok then
				local s: string = x
				return s
			end
			return ""
		end return f`,
	},
	{
		// A return-position operator expression must be type-checked against the
		// return annotation: returning a number-typed `a // b` for an integer return
		// is rejected.
		name:  "return-operator-expr-checked",
		fixed: true,
		src: `local function f(a: number, b: number): integer
			return a // b
		end return f`,
	},
	{
		// An absent field of an empty table literal is nil, not an arbitrary top type;
		// it must not satisfy a declared (here function) type.
		name:  "empty-literal-absent-field",
		fixed: true,
		src: `local function f(): number
			local t = {}
			local g: fun(): number = t.run
			return g()
		end return f`,
	},
	{
		// A multi-assignment from a call that supplies fewer values leaves the
		// surplus targets nil, not their declared type.
		name:  "multi-return-undersupply",
		fixed: true,
		src: `local function one(): number return 1 end
		local function f(): number
			local a: number, b: number = one()
			return b
		end return f`,
	},
	{
		// A covariant alias mutated through a CLOSURE capture must not launder the
		// write-through: leak() writes wide.x (an alias of narrow), so narrow.x is a
		// string at runtime. Closure captured-variable writes are not in the call
		// outcome's parameter bindings, so the mutation is not propagated.
		name:  "closure-capture-covariance",
		fixed: true,
		src: `local function f(): number
			local narrow: { x: number } = { x = 1 }
			local wide: { x: number | string } = narrow
			local function leak() wide.x = "boom" end
			leak()
			local n: number = narrow.x
			return n
		end return f`,
	},
	{
		// Same laundering class via a CAST-created wider view. The cast is
		// runtime-validated (narrow conforms to the wider type at cast time), but it
		// still aliases narrow through a wider mutable view, so the later write
		// corrupts narrow.x. The operand must widen at the cast point.
		name:  "mutable-cast-widen",
		fixed: true,
		src: `local function f(): number
			local narrow: { x: number } = { x = 1 }
			local wide = narrow as { x: number | string }
			wide.x = "boom"
			local n: number = narrow.x
			return n
		end return f`,
	},
	{
		// Covariant alias via ordinary (non-declaration) assignment, mutated through a
		// closure: same exposure, different binding site.
		name:  "covariant-reassign-closure",
		fixed: true,
		src: `local function f(): number
			local narrow: { x: number } = { x = 1 }
			local wide: { x: number | string } = { x = 2 }
			wide = narrow
			local function leak() wide.x = "boom" end
			leak()
			local n: number = narrow.x
			return n
		end return f`,
	},
	{
		// narrow stored into a wider record-field slot, mutated through a closure via
		// the container. The store exposes narrow through holder.ref of the wider type.
		name:  "covariant-field-store-closure",
		fixed: true,
		src: `local function f(): number
			local narrow: { x: number } = { x = 1 }
			local holder: { ref: { x: number | string } } = { ref = narrow }
			local function leak() holder.ref.x = "boom" end
			leak()
			local n: number = narrow.x
			return n
		end return f`,
	},
	{
		// narrow stored into a wider array element, mutated through a closure.
		name:  "covariant-index-store-closure",
		fixed: true,
		src: `local function f(): number
			local narrow: { x: number } = { x = 1 }
			local sink: { { x: number | string } } = { narrow }
			local function leak() sink[1].x = "boom" end
			leak()
			local n: number = narrow.x
			return n
		end return f`,
	},
	{
		// A sub-object aliased to a wider local view, mutated through a closure.
		name:  "covariant-subobject-alias-closure",
		fixed: true,
		src: `local function f(): number
			local narrow: { inner: { x: number } } = { inner = { x = 1 } }
			local wideinner: { x: number | string } = narrow.inner
			local function leak() wideinner.x = "boom" end
			leak()
			local n: number = narrow.inner.x
			return n
		end return f`,
	},
	{
		// A callee that stores its argument into a wider sink it returns aliases
		// narrow into a wider mutable view across the call boundary.
		name:  "covariant-interproc-store",
		fixed: true,
		src: `local function box(o: { x: number | string }): { ref: { x: number | string } } return { ref = o } end
		local function f(): number
			local narrow: { x: number } = { x = 1 }
			local h = box(narrow)
			h.ref.x = "boom"
			local n: number = narrow.x
			return n
		end return f`,
	},
	{
		// Same class via a sink other than the return: the callee stores the argument
		// into another parameter's wider field, aliasing narrow through holder.ref.
		name:  "covariant-interproc-param-to-param",
		fixed: true,
		src: `local function link(dst: { ref: { x: number | string } }, o: { x: number | string }) dst.ref = o end
		local function f(): number
			local narrow: { x: number } = { x = 1 }
			local holder: { ref: { x: number | string } } = { ref = { x = 0 } }
			link(holder, narrow)
			holder.ref.x = "boom"
			local n: number = narrow.x
			return n
		end return f`,
	},
	{
		// Same class via a captured-upvalue sink: the callee stores the argument into
		// an outer container it captures, aliasing narrow through sink.ref.
		name:  "covariant-interproc-sink-store",
		fixed: true,
		src: `local sink: { ref: { x: number | string } } = { ref = { x = 0 } }
		local function stash(o: { x: number | string }) sink.ref = o end
		local function f(): number
			local narrow: { x: number } = { x = 1 }
			stash(narrow)
			sink.ref.x = "boom"
			local n: number = narrow.x
			return n
		end return f`,
	},
	{
		// A covariant map-value alias keeps the declared root witness across a
		// key write. Per-key reads use static-member facts, and invariant map
		// subtyping rejects the assignment.
		name:  "covariant-map-value-alias",
		fixed: true,
		src: `local function f(): number
			local narrow: { [string]: number } = {}
			narrow["k"] = 1
			local wide: { [string]: number | string } = narrow
			wide["k"] = "boom"
			local v = narrow["k"]
			if v then
				local n: number = v
				return n
			end
			return 0
		end return f`,
	},
	{
		// A concrete-valued map is invariant under aliasing because a wider alias
		// can write a value the narrow map forbids. Covariant widening applies
		// only to a fresh empty map with value Never.
		name:  "covariant-map-value-concrete",
		fixed: true,
		src: `local function f(narrow: { [string]: number }): number
			local wide: { [string]: number | string } = narrow
			wide["k"] = "boom"
			local v = narrow["k"]
			if v then
				local n: number = v
				return n
			end
			return 0
		end return f`,
	},
	{
		// A fresh literal passed to a callee that mutates it covariantly must not
		// launder the write-through: narrow.x becomes a string at runtime.
		name:  "fresh-literal-interproc-covariance",
		fixed: true,
		src: `local function corrupt(w: { x: number | string }) w.x = "boom" end
		local function f(): number
			local narrow: { x: number } = { x = 1 }
			corrupt(narrow)
			local n: number = narrow.x
			return n
		end return f`,
	},
}

func TestSoundnessProbes(t *testing.T) {
	for _, p := range soundnessProbes {
		result := testutil.Check(p.src, testutil.WithStdlib())
		var msgs []string
		for _, d := range result.Diagnostics {
			msgs = append(msgs, d.Message)
		}
		errored := len(result.Diagnostics) > 0
		switch {
		case errored == p.fixed:
			if p.fixed {
				t.Logf("guarded %-40s errors: %s", p.name, strings.Join(msgs, " | "))
			} else {
				t.Logf("open    %-40s", p.name)
			}
		case p.fixed && !errored:
			t.Errorf("expected diagnostic for %s", p.name)
		default:
			t.Errorf("unexpected diagnostic for %s", p.name)
		}
	}
}

func TestSoundnessDecomposableIdentityCompareProbe(t *testing.T) {
	result := testutil.Check(`
local function f(): boolean
	local left = { a = 1 }
	local right = { a = 1 }
	return left == right
end
return f
`, testutil.WithStdlib())
	plan := result.PlacementPlan()
	for _, entry := range plan.Entries {
		if entry.Decomposable {
			t.Fatalf("identity-compared allocation was marked decomposable: %#v", entry)
		}
	}
}

func TestSoundnessFrameLocalAfterSelectProbe(t *testing.T) {
	result := testutil.Check(`
local channel = require("channel")

type Message = { value: integer }

local function route(ch: Channel<Message>): integer
	local scratch = { value = 1 }
	local selected = channel.select {
		ch:case_receive(),
	}
	local out: integer = scratch.value
	if selected.ok then
		return out
	end
	return out
end

local out: integer = route(nil :: Channel<Message>)
`, testutil.WithStdlib(), testutil.WithManifest("channel", testutil.ChannelManifest()), testutil.WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want clean select probe", result.Diagnostics)
	}
	plan := result.PlacementPlan()
	for _, entry := range plan.Entries {
		if entry.FrameLocal {
			t.Fatalf("allocation reachable after select was marked frame-local: %#v", entry)
		}
	}
}

func TestSoundnessHoistableLoadConditionalAliasWriteProbe(t *testing.T) {
	result := testutil.Check(`
type Config = { limit: number }
local config: Config = { limit = 3 }
local alias = config
local total = 0
local i = 0
while i < 3 do
	total = total + config.limit
	if i == 1 then
		alias.limit = 9
	end
	i = i + 1
end
return total
`, testutil.WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want clean alias-write probe", result.Diagnostics)
	}
	if loads := result.PlacementPlan().HoistableLoads; len(loads) != 0 {
		t.Fatalf("conditional alias write exported hoistable-load licenses: %#v", loads)
	}
}
