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
		fixed: false,
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
		name:  "cast-any-to-record-trusted",
		fixed: false,
		src: `local function f(x: any): string
			local r = x as {name: string}
			return r.name
		end return f`,
	},
	{
		name:  "cast-number-to-record-disjoint",
		fixed: false,
		src: `local function f(x: number): string
			local r = x as {name: string}
			return r.name
		end return f`,
	},
	{
		name:  "nonnil-assert-on-always-nil",
		fixed: false,
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
