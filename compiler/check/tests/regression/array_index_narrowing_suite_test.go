package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Full suite probing array/tuple index narrowing across the matrix of
// container shape × index kind × in/out-of-range. Each case asserts the
// SOUND-and-POWERFUL behavior: an index provably in range eliminates nil
// (powerful); an index NOT provably in range keeps nil (sound). Failures
// pinpoint gaps to fix by the book. Every case is a TDD lock.

// wantOK asserts no diagnostics (index proven in range → nil eliminated).
func wantOK(t *testing.T, name, src string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		r := testutil.Check(src, testutil.WithStdlib())
		if r.HasError() {
			t.Errorf("expected no errors (index proven in range), got: %v", testutil.ErrorMessages(r.Diagnostics))
		}
	})
}

// wantErr asserts a diagnostic (index NOT proven in range → nil retained →
// assignment to a non-optional type must fail). This is the soundness arm.
func wantErr(t *testing.T, name, src string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		r := testutil.Check(src, testutil.WithStdlib())
		if !r.HasError() {
			t.Errorf("expected an error (index NOT proven in range; nil must be retained), got none")
		}
	})
}

func TestArrayIndexNarrowing_LiteralIndex(t *testing.T) {
	wantOK(t, "in_range_first", `local d = {10,20,30}; local v: number = d[1]`)
	wantOK(t, "in_range_last", `local d = {10,20,30}; local v: number = d[3]`)
	wantErr(t, "out_of_range_high", `local d = {10,20,30}; local v: number = d[4]`)
	wantErr(t, "out_of_range_zero", `local d = {10,20,30}; local v: number = d[0]`)
}

func TestArrayIndexNarrowing_ForLoopVar(t *testing.T) {
	// for i = 1, 3 over a length-3 tuple: every index in range.
	wantOK(t, "for_exact_bound", `
		local d = {10,20,30}
		for i = 1, 3 do
			local v: number = d[i]
		end`)
	// for i = 1, 2: subset of range, still in range.
	wantOK(t, "for_under_bound", `
		local d = {10,20,30}
		for i = 1, 2 do
			local v: number = d[i]
		end`)
	// for i = 1, 5 over length-3 tuple: i can be 4,5 → out of range → nil.
	wantErr(t, "for_over_bound", `
		local d = {10,20,30}
		for i = 1, 5 do
			local v: number = d[i]
		end`)
}

func TestArrayIndexNarrowing_WhileLoopVar(t *testing.T) {
	// while i <= 3 over length-3 tuple: in range.
	wantOK(t, "while_exact_bound", `
		local d = {10,20,30}
		local i = 1
		while i <= 3 do
			local v: number = d[i]
			i = i + 1
		end`)
	// while i < 3: i in {1,2}, in range.
	wantOK(t, "while_strict_bound", `
		local d = {10,20,30}
		local i = 1
		while i < 3 do
			local v: number = d[i]
			i = i + 1
		end`)
	// while i <= 5 over length-3 tuple: i can be 4,5 → out of range → nil.
	wantErr(t, "while_over_bound", `
		local d = {10,20,30}
		local i = 1
		while i <= 5 do
			local v: number = d[i]
			i = i + 1
		end`)
}

func TestArrayIndexNarrowing_LengthRelative(t *testing.T) {
	// d[#d] is the last element; proven present when #d >= 1 (it is, literal).
	wantOK(t, "last_element", `local d = {10,20,30}; local v: number = d[#d]`)
	// while i <= #d: index bounded by the container's own length.
	wantOK(t, "while_length_relative", `
		local d = {10,20,30}
		local i = 1
		while i <= #d do
			local v: number = d[i]
			i = i + 1
		end`)
}

func TestArrayIndexNarrowing_TypedSequence(t *testing.T) {
	// A typed sequence {number} has UNKNOWN runtime length. A dynamic index
	// without a length proof must be nil-eligible (sound). Only a length
	// lower-bound (e.g. #seq guard) proves presence.
	wantErr(t, "seq_unbounded_index", `
		local seq: {number} = {1,2,3}
		local function idx(): integer return 2 end
		local v: number = seq[idx()]`)
	// seq[i] under i <= #seq: length-relative proof eliminates nil.
	wantOK(t, "seq_length_relative", `
		local seq: {number} = {1,2,3}
		local i = 1
		while i <= #seq do
			local v: number = seq[i]
			i = i + 1
		end`)
	// seq[i] under a CONSTANT bound i <= 3: a sequence's length is unknown,
	// so a constant bound does NOT prove the element is present (the sequence
	// could have fewer than 3 elements). Must keep nil → error.
	wantErr(t, "seq_constant_bound_no_length_proof", `
		local function build(): {number} return {1} end
		local seq = build()
		local i = 1
		while i <= 3 do
			local v: number = seq[i]
			i = i + 1
		end`)
}
