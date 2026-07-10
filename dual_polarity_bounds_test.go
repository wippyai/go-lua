package lua

import (
	"strings"
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
)

// TestDualPolarityBoundsNarrowing pins that a negated bounds guard (the
// `if oob then bail end` form) establishes the in-range and numeric-floor proofs
// on its FALSE edge, so an array read on that edge narrows from T? to T. The
// soundness cases verify it does not over-narrow: an upper bound without a floor,
// or a floor without an upper bound, must still report the read may be nil.
func TestDualPolarityBoundsNarrowing(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr bool
	}{
		{
			// Negated guard, both bounds on the false edge -> narrows in the else.
			name: "negated-both-bounds-narrows",
			src: `local function f(arr: {number}, i: integer): number
				if i < 1 or i > #arr then
					return 0
				else
					local x: number = arr[i]
					return x
				end
			end return f`,
			wantErr: false,
		},
		{
			// The canonical `if oob then error() end` guard: error() is no-return,
			// so the continuation is the false edge carrying both bounds.
			name: "error-guard-continuation-narrows",
			src: `local function f(arr: {number}, i: integer): number
				if i < 1 or i > #arr then error("oob") end
				local x: number = arr[i]
				return x
			end return f`,
			wantErr: false,
		},
		{
			// error()-no-return must not break a stub function's declared return:
			// callers still see the declared type, not unknown.
			name: "noreturn-stub-preserves-declared-return",
			src: `type Box = { value: number }
			local function make(): Box error("nyi") end
			local function f(): number
				local b = make()
				return b.value
			end return f`,
			wantErr: false,
		},
		{
			// Soundness: error-guard with only the upper bound (no floor) must still
			// report may-be-nil on the continuation.
			name: "error-guard-upper-only-still-errors",
			src: `local function f(arr: {number}, i: integer): number
				if i > #arr then error("oob") end
				local x: number = arr[i]
				return x
			end return f`,
			wantErr: true,
		},
		{
			// Positive guard still narrows in the then-branch (unchanged behavior).
			name: "positive-both-bounds-narrows",
			src: `local function f(arr: {number}, i: integer): number
				if i >= 1 and i <= #arr then
					local x: number = arr[i]
					return x
				end
				return 0
			end return f`,
			wantErr: false,
		},
		{
			// Soundness: upper bound only (no floor) -> i could be < 1 -> may be nil.
			name: "negated-upper-only-still-errors",
			src: `local function f(arr: {number}, i: integer): number
				if i > #arr then
					return 0
				else
					local x: number = arr[i]
					return x
				end
			end return f`,
			wantErr: true,
		},
		{
			// Soundness: floor only (no upper bound) -> i could exceed #arr -> may be nil.
			name: "negated-floor-only-still-errors",
			src: `local function f(arr: {number}, i: integer): number
				if i < 1 then
					return 0
				else
					local x: number = arr[i]
					return x
				end
			end return f`,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := testutil.Check(tc.src, testutil.WithStdlib())
			var msgs []string
			for _, d := range result.Diagnostics {
				msgs = append(msgs, d.Message)
			}
			gotErr := len(result.Diagnostics) > 0
			if gotErr != tc.wantErr {
				t.Fatalf("wantErr=%v gotErr=%v: %s", tc.wantErr, gotErr, strings.Join(msgs, " | "))
			}
		})
	}
}

// TestInterprocBoundsFloorPropagation pins that normal-return numeric floors
// cross same-module call boundaries. The callee's in-range proof already carried
// the upper bound; this covers the lower-bound lane so callers can narrow
// arr[i] after a helper performs the bounds check.
func TestInterprocBoundsFloorPropagation(t *testing.T) {
	cases := map[string]string{
		// Callee bounds-checks both bounds, caller reads the argument.
		"interproc-both-bounds": `local function bound(arr: {number}, i: integer)
			if i < 1 or i > #arr then error("oob") end
		end
		local function use(arr: {number}, i: integer): number
			bound(arr, i)
			local x: number = arr[i]
			return x
		end return use`,
		// Callee supplies the floor, caller supplies the local in-range bound.
		"interproc-floor-from-callee": `local function bound(arr: {number}, i: integer)
			if i < 1 then error("oob") end
		end
		local function use(arr: {number}, i: integer): number
			if i > #arr then return 0 end
			bound(arr, i)
			local x: number = arr[i]
			return x
		end return use`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			result := testutil.Check(src, testutil.WithStdlib())
			if len(result.Diagnostics) != 0 {
				var msgs []string
				for _, d := range result.Diagnostics {
					msgs = append(msgs, d.Message)
				}
				t.Fatalf("interproc numeric floor did not propagate for %s: %s", name, strings.Join(msgs, " | "))
			}
		})
	}
}

func TestInterprocBoundsFloorDoesNotPropagateAfterParameterReassignment(t *testing.T) {
	src := `local function bound(arr: {number}, i: integer)
		if i < 1 then error("oob") end
		i = 2
	end
	local function use(arr: {number}, i: integer): number
		if i > #arr then return 0 end
		bound(arr, i)
		local x: number = arr[i]
		return x
	end return use`

	result := testutil.Check(src, testutil.WithStdlib())
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected may-be-nil diagnostic: callee-local reassignment must not prove caller index floor")
	}
}
