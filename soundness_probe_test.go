package lua

import (
	"strings"
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
)

// soundnessProbe is a program whose soundness depends on the checker REJECTING
// it: each program must produce at least one error diagnostic. fixed records the
// current behavior: a fixed probe must error (the hole is closed and guarded); an
// unfixed probe is a known, still-open soundness hole pinned here so that closing
// it flips this test, prompting the flag to be set to true.
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
		// A concrete cast must not PROVE a separate obligation: laundering an any
		// through `as T` to satisfy a parameter contract is rejected.
		name:  "cast-any-laundered-to-param",
		fixed: true,
		src: `local function need(x: {name: string}): number return 1 end
		local function f(y: any): number
			return need(y as {name: string})
		end return f`,
	},
	{
		// Same, with a disjoint concrete source.
		name:  "cast-disjoint-laundered-to-param",
		fixed: true,
		src: `local function need(x: {name: string}): number return 1 end
		local function f(y: number): number
			return need(y as {name: string})
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
				t.Logf("GUARDED %-40s errors: %s", p.name, strings.Join(msgs, " | "))
			} else {
				t.Logf("OPEN    %-40s (known soundness hole, still pinned)", p.name)
			}
		case p.fixed && !errored:
			t.Errorf("REGRESSION %-40s no longer errors (was a guarded fix)", p.name)
		default:
			t.Errorf("NOW SOUND  %-40s now errors; set fixed:true to guard it", p.name)
		}
	}
}
