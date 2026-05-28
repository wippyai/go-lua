package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// These tests probe the numeric abstract domain's behavior on while-loops,
// which (unlike numeric for-loops) get their variable bounds only from the
// loop CONDITION as an edge constraint, not from a for-range. The question
// under test: does textbook Cousot widening at the loop header plus the
// condition edge constraint suffice to (a) terminate and (b) retain the
// bound needed to prove an array index is in range — WITHOUT any
// threshold-widening or cap machinery?
//
// If these fail (hang or false positive), there is a real gap and the
// numeric domain needs more than textbook Cousot widening. If they pass,
// the by-the-book design (Join hull + Cousot Widen at loop headers +
// condition edge constraints) is sufficient and no threshold widening is
// warranted.

// TestWhileLoop_BoundedConditionNarrowsArrayIndex: the loop condition
// `i <= 3` bounds i; indexing a length-3 array by i must be non-nil.
func TestWhileLoop_BoundedConditionNarrowsArrayIndex(t *testing.T) {
	source := `
		local data = {10, 20, 30}
		local i = 1
		while i <= 3 do
			local v: number = data[i]
			i = i + 1
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for bounded while-loop array indexing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWhileLoop_UnboundedTerminates: a while-loop whose variable grows with
// no numeric bound from the condition must still TERMINATE the analysis
// (Cousot widening to +inf) and type-check the result as a number.
func TestWhileLoop_UnboundedTerminates(t *testing.T) {
	source := `
		local function tick(): boolean
			return true
		end

		local x = 0
		while tick() do
			x = x + 1
		end
		local y: number = x
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for unbounded while-loop, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWhileLoop_StrictLessBoundNarrowsArrayIndex: strict `<` condition with
// the off-by-one. `i < 3` admits i in {1,2}; indexing a length-3 array stays
// in range.
func TestWhileLoop_StrictLessBoundNarrowsArrayIndex(t *testing.T) {
	source := `
		local data = {10, 20, 30}
		local i = 1
		while i < 3 do
			local v: number = data[i]
			i = i + 1
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for strict-bound while-loop array indexing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestRepeatLoop_BoundedConditionNarrowsArrayIndex: Lua repeat-until. The
// `until i > 3` exit condition means the body runs while i <= 3.
func TestRepeatLoop_BoundedConditionNarrowsArrayIndex(t *testing.T) {
	source := `
		local data = {10, 20, 30}
		local i = 1
		repeat
			local v: number = data[i]
			i = i + 1
		until i > 3
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for bounded repeat-loop array indexing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWhileLoop_OutOfBoundConditionStaysSound: SOUNDNESS probe. The loop
// condition `i <= 5` permits i ∈ {1..5} but the array has length 3, so
// data[i] for i ∈ {4,5} is out of range and must include nil. This proves
// the numeric domain is SOUND (does not over-narrow), not merely permissive:
// the checker MUST flag the nil-arithmetic / assignment.
func TestWhileLoop_OutOfBoundConditionStaysSound(t *testing.T) {
	source := `
		local data = {10, 20, 30}
		local i = 1
		while i <= 5 do
			local v: number = data[i]
			i = i + 1
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Errorf("expected an error: while-bound 5 exceeds array length 3, so data[i] can be nil and must NOT type-check as number")
	}
}

// TestWhileLoop_NestedUnboundedTerminates: termination stress. Two nested
// unbounded while-loops both incrementing. Without Cousot widening at the
// loop headers this diverges (infinite ascending interval chain). Must
// terminate and type-check.
func TestWhileLoop_NestedUnboundedTerminates(t *testing.T) {
	source := `
		local function cond1(): boolean return true end
		local function cond2(): boolean return false end

		local x = 0
		while cond1() do
			local y = 0
			while cond2() do
				y = y + 1
				x = x + y
			end
		end
		local total: number = x
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested unbounded while-loops, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWhileLoop_BoundedByArrayLength: the real-world pattern — loop bounded
// by the array's own length via `#arr`. Tests the symbolic length-reference
// machinery (lenRefs): i <= #arr must prove arr[i] in range.
func TestWhileLoop_BoundedByArrayLength(t *testing.T) {
	source := `
		local arr = {1, 2, 3, 4, 5}
		local i = 1
		while i <= #arr do
			local v: number = arr[i]
			i = i + 1
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for length-bounded while-loop, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWhileLoop_DecrementingStaysSound: decrementing loop. `i >= 1` bounds
// the lower end; i counts down from 3. data[i] for i in [1,3] is in range.
func TestWhileLoop_DecrementingStaysSound(t *testing.T) {
	source := `
		local data = {10, 20, 30}
		local i = 3
		while i >= 1 do
			local v: number = data[i]
			i = i - 1
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for decrementing while-loop, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWhileLoop_StepByTwoTerminates: non-unit step. i grows by 2; the loop
// must terminate (widening) and stay sound for indexing.
func TestWhileLoop_StepByTwoTerminates(t *testing.T) {
	source := `
		local data = {10, 20, 30, 40}
		local i = 1
		while i <= 4 do
			local v: number = data[i]
			i = i + 2
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for step-by-two while-loop, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWhileLoop_CompoundConditionNarrows: compound `and` condition. The
// numeric clause `i <= 3` must still narrow even alongside a boolean clause.
func TestWhileLoop_CompoundConditionNarrows(t *testing.T) {
	source := `
		local data = {10, 20, 30}
		local function active(): boolean return true end
		local i = 1
		while i <= 3 and active() do
			local v: number = data[i]
			i = i + 1
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for compound-condition while-loop, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWhileLoop_BreakBasedBoundStaysSound: `while true` with the bound
// enforced only by an inner `if i > 3 then break end`. The numeric bound on
// i inside the body after the break-guard comes from the negated branch
// condition (i <= 3), not the loop condition. Hard case — break-derived
// bounds. SOUNDNESS: indexing must be proven in range OR flagged; it must
// NOT silently admit an out-of-range nil as number.
func TestWhileLoop_BreakBasedBoundStaysSound(t *testing.T) {
	source := `
		local data = {10, 20, 30}
		local i = 1
		while true do
			if i > 3 then
				break
			end
			local v: number = data[i]
			i = i + 1
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for break-bounded while-loop (i <= 3 holds in body after the break-guard), got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWhileLoop_NestedSharedRowIndex: nested loops over a matrix. Outer
// bounds i by row count, inner bounds j by column count; both indexings must
// be proven in range. Stresses per-loop-header widening on nested headers.
func TestWhileLoop_NestedSharedRowIndex(t *testing.T) {
	source := `
		local matrix = {{1, 2}, {3, 4}}
		local i = 1
		while i <= 2 do
			local row = matrix[i]
			local j = 1
			while j <= 2 do
				local v: number = row[j]
				j = j + 1
			end
			i = i + 1
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested matrix while-loops, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestWhileLoop_AccumulatorThenIndex: build an array in a bounded loop, then
// index a literal position. Tests that loop-built array length facts survive
// the loop's widening for a post-loop literal index.
func TestWhileLoop_AccumulatorThenIndex(t *testing.T) {
	source := `
		local results: {number} = {}
		local i = 1
		while i <= 3 do
			results[i] = i * 10
			i = i + 1
		end
		local total = 0
		local k = 1
		while k <= #results do
			total = total + results[k]
			k = k + 1
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for accumulator-then-index while-loop, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
