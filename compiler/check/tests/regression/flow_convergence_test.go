package regression

import (
	"runtime/debug"
	"testing"
	"time"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// These tests lock the flow-solver fixpoint against non-convergence. A
// loop-carried table slot whose phi merges a stable nil edge with a back-edge
// that oscillates between a concrete element type and the opaque unknown type
// forms a limit cycle: each iteration the slot flips and the value interner
// grows without bound. A correct abstract interpreter must reach a fixed point
// (monotone-ascending merge, widening on the infinite-height type lattice), so
// the analysis must TERMINATE and the sound result must type-check.
//
// checkConverges runs the checker under a short deadline with a heap cap, so a
// regression that reintroduces the divergence fails fast with a clear message
// instead of exhausting machine memory.
func checkConverges(t *testing.T, name, src string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		prev := debug.SetMemoryLimit(1 << 30)
		defer debug.SetMemoryLimit(prev)

		done := make(chan *testutil.Result, 1)
		go func() {
			done <- testutil.Check(src, testutil.WithStdlib())
		}()

		select {
		case r := <-done:
			if r.HasError() {
				t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(r.Diagnostics))
			}
		case <-time.After(8 * time.Second):
			t.Fatalf("non-convergence: checker did not terminate within 8s — flow-solver fixpoint failed to converge (limit cycle / missing widening)")
		}
	})
}

// TestFlowConvergence_GroupedBucketLoop locks the minimal bucket-grouping
// pattern: groups[key] is read, defaulted to a fresh table, then appended to.
// The slot's phi merges nil (first iteration) with the loop-carried table; the
// back-edge type lags through unknown, and a non-monotone join would descend
// the slot every other pass, spinning the worklist forever.
func TestFlowConvergence_GroupedBucketLoop(t *testing.T) {
	checkConverges(t, "string_key", `
		local function group_entries(entries)
			local groups = {}
			for _, entry in ipairs(entries) do
				local key = "default"
				groups[key] = groups[key] or {}
				table.insert(groups[key], entry)
			end
			return groups
		end
		local _ = group_entries({1, 2, 3, 4, 5})
	`)

	// table.insert on the directly-indexed slot, no intermediate key local.
	checkConverges(t, "literal_key", `
		local function group_entries(entries)
			local groups = {}
			for _, entry in ipairs(entries) do
				groups["default"] = groups["default"] or {}
				table.insert(groups["default"], entry)
			end
			return groups
		end
		local _ = group_entries({1, 2, 3})
	`)
}

// TestFlowConvergence_TypedGroupedBucketLoop adds an explicit map annotation so
// the carried element type is concrete from the start; the analysis must still
// converge and accept the sound program.
func TestFlowConvergence_TypedGroupedBucketLoop(t *testing.T) {
	checkConverges(t, "typed_map", `
		local function group_by(entries: {any})
			local groups: {[string]: {any}} = {}
			for _, entry in ipairs(entries) do
				local suite = "default"
				groups[suite] = groups[suite] or {}
				table.insert(groups[suite], entry)
			end
			return groups
		end
		local _ = group_by({1, 2, 3})
	`)
}

// TestFlowConvergence_IndependentBranchChain locks the acyclic DNF-explosion
// pattern: a straight-line chain of independent guarded reads, each introducing
// a fresh discriminant. With an exact-DNF path condition that never forgets a
// dead discriminant, the disjuncts cross-multiply (2^N) and normalization runs
// O(disjuncts^2), a practical non-termination. A condition domain that projects
// out dead SSA versions keeps the path condition bounded by the live vocabulary,
// so the analysis stays fast.
func TestFlowConvergence_IndependentBranchChain(t *testing.T) {
	checkConverges(t, "truthiness_guard_chain", `
		local function pick(x: {
			a: string?, b: string?, c: string?, d: string?, e: string?,
			f: string?, g: string?, h: string?, i: string?, j: string?,
			k: string?, l: string?, m: string?, n: string?, o: string?,
			p: string?, q: string?, r: string?, s: string?, t: string?
		}): {string}
			local out: {string} = {}
			if x.a then table.insert(out, x.a) end
			if x.b then table.insert(out, x.b) end
			if x.c then table.insert(out, x.c) end
			if x.d then table.insert(out, x.d) end
			if x.e then table.insert(out, x.e) end
			if x.f then table.insert(out, x.f) end
			if x.g then table.insert(out, x.g) end
			if x.h then table.insert(out, x.h) end
			if x.i then table.insert(out, x.i) end
			if x.j then table.insert(out, x.j) end
			if x.k then table.insert(out, x.k) end
			if x.l then table.insert(out, x.l) end
			if x.m then table.insert(out, x.m) end
			if x.n then table.insert(out, x.n) end
			if x.o then table.insert(out, x.o) end
			if x.p then table.insert(out, x.p) end
			if x.q then table.insert(out, x.q) end
			if x.r then table.insert(out, x.r) end
			if x.s then table.insert(out, x.s) end
			if x.t then table.insert(out, x.t) end
			return out
		end
		local _ = pick
	`)
}

// TestFlowConvergence_TypeTestGuardChain probes the residual class the presence-only
// projection does NOT forget: a straight-line chain of independent type-test
// (HasType) discriminant guards over distinct dead fields. If projection only
// drops positive presence guards, these type-tests accumulate and the DNF still
// cross-multiplies. A complete relevance projection must bound this too.
func TestFlowConvergence_TypeTestGuardChain(t *testing.T) {
	checkConverges(t, "type_test_chain", `
		local function pick(x: {
			a: any, b: any, c: any, d: any, e: any,
			f: any, g: any, h: any, i: any, j: any,
			k: any, l: any, m: any, n: any, o: any,
			p: any, q: any, r: any, s: any, t: any
		}): {string}
			local out: {string} = {}
			if type(x.a) == "string" then table.insert(out, x.a) end
			if type(x.b) == "string" then table.insert(out, x.b) end
			if type(x.c) == "string" then table.insert(out, x.c) end
			if type(x.d) == "string" then table.insert(out, x.d) end
			if type(x.e) == "string" then table.insert(out, x.e) end
			if type(x.f) == "string" then table.insert(out, x.f) end
			if type(x.g) == "string" then table.insert(out, x.g) end
			if type(x.h) == "string" then table.insert(out, x.h) end
			if type(x.i) == "string" then table.insert(out, x.i) end
			if type(x.j) == "string" then table.insert(out, x.j) end
			if type(x.k) == "string" then table.insert(out, x.k) end
			if type(x.l) == "string" then table.insert(out, x.l) end
			if type(x.m) == "string" then table.insert(out, x.m) end
			if type(x.n) == "string" then table.insert(out, x.n) end
			if type(x.o) == "string" then table.insert(out, x.o) end
			if type(x.p) == "string" then table.insert(out, x.p) end
			if type(x.q) == "string" then table.insert(out, x.q) end
			if type(x.r) == "string" then table.insert(out, x.r) end
			if type(x.s) == "string" then table.insert(out, x.s) end
			if type(x.t) == "string" then table.insert(out, x.t) end
			return out
		end
		local _ = pick
	`)
}

// TestFlowConvergence_FunctionReturnAssignedToLocal locks the second known
// divergent pattern: a function return flowing into a loop-carried local whose
// element type lags through unknown across the back-edge.
func TestFlowConvergence_FunctionReturnAssignedToLocal(t *testing.T) {
	checkConverges(t, "return_into_carry", `
		local function make(): {any}
			return {}
		end
		local acc = make()
		for i = 1, 5 do
			acc = acc or make()
			table.insert(acc, i)
		end
		local _ = acc
	`)
}
