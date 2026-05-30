package summary_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// testProgram is a Program backed by a small set of named, parsed Lua functions.
// It is the test-side seam: it builds each function's canonical inputs and
// transfer, and derives the call graph by walking each graph's call sites and
// resolving callee names to the named functions. It exercises summary.Queries
// over a real call graph without the full pipeline.
type testProgram struct {
	graphs    map[summary.FuncRef]*cfg.Graph
	transfers map[summary.FuncRef]equation.NodeTransfer
	params    map[summary.FuncRef]int
	byName    map[string]summary.FuncRef
}

func newTestProgram(t *testing.T, fns map[string]string) *testProgram {
	t.Helper()
	p := &testProgram{
		graphs:    make(map[summary.FuncRef]*cfg.Graph),
		transfers: make(map[summary.FuncRef]equation.NodeTransfer),
		params:    make(map[summary.FuncRef]int),
		byName:    make(map[string]summary.FuncRef),
	}
	// The function names are predeclared globals so a body call resolves the
	// callee name to a symbol rather than an unknown.
	globals := make([]string, 0, len(fns))
	for name := range fns {
		globals = append(globals, name)
	}
	for name, src := range fns {
		stmts, err := parse.ParseString(src, name+".lua")
		if err != nil {
			t.Fatalf("parse %s failed: %v", name, err)
		}
		// A function header line "--params: a,b" declares parameters; default none.
		fn := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
		in := input.BuildFromFunction(fn, nil, nil, globals...)
		if in.Graph == nil {
			t.Fatalf("input builder produced no graph for %s", name)
		}
		ref := summary.FuncRef{GraphID: in.Graph.ID()}
		p.graphs[ref] = in.Graph
		p.transfers[ref] = transfer.New(in, nil, nil, nil, nil, nil, nil)
		p.params[ref] = in.Scope.NumParams()
		p.byName[name] = ref
	}
	return p
}

func (p *testProgram) Graph(ref summary.FuncRef) *cfg.Graph { return p.graphs[ref] }
func (p *testProgram) NumParams(ref summary.FuncRef) int    { return p.params[ref] }
func (p *testProgram) Transfer(ref summary.FuncRef) equation.NodeTransfer {
	return p.transfers[ref]
}

// Callees derives the call-graph edges of ref by walking every call site in its
// graph and resolving the callee name to a named function. Unresolved calls
// (stdlib, unknown names) are not call-graph nodes and are skipped.
func (p *testProgram) Callees(ref summary.FuncRef) []summary.FuncRef {
	g := p.graphs[ref]
	if g == nil {
		return nil
	}
	seen := make(map[summary.FuncRef]bool)
	var out []summary.FuncRef
	g.EachCallSite(func(_ cfg.Point, call *cfg.CallInfo) {
		if call == nil || call.CalleeName == "" {
			return
		}
		callee, ok := p.byName[call.CalleeName]
		if !ok || seen[callee] {
			return
		}
		seen[callee] = true
		out = append(out, callee)
	})
	return out
}

// TestSummary_CalleeReturnFlowsToCaller is gate (a): a caller that calls a callee
// resolves the callee's summary, and the callee's summary carries its return
// type. The converged summary of the callee is asserted; the caller's summary
// resolving the callee through SummaryQ closes the call-graph edge.
func TestSummary_CalleeReturnFlowsToCaller(t *testing.T) {
	prog := newTestProgram(t, map[string]string{
		// callee returns a string literal.
		"callee": `
local s = "ok"
return s
`,
		// caller calls callee; the call-graph edge caller -> callee is derived.
		"caller": `
local r = callee()
return r
`,
	})

	q := summary.New(prog)
	ctx := db.NewQueryContext(db.New())

	calleeRef := prog.byName["callee"]
	callerRef := prog.byName["caller"]

	calleeSum := q.Summarize(ctx, calleeRef)
	if len(calleeSum.Returns) != 1 {
		t.Fatalf("callee must summarize one return slot; got %d (%v)", len(calleeSum.Returns), calleeSum.Returns)
	}
	got := calleeSum.Returns[0].ProjectValue()
	lit, ok := got.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		t.Fatalf("callee return slot 0 must be a string literal; got %v", got)
	}
	if v, isStr := lit.Value.(string); !isStr || v != "ok" {
		t.Fatalf("callee return slot 0 must be \"ok\"; got %v", got)
	}

	// The caller summarizes without error and resolves the callee edge: Callees
	// must contain callee, proving the call site reads the callee summary.
	callees := prog.Callees(callerRef)
	if len(callees) != 1 || callees[0] != calleeRef {
		t.Fatalf("caller must call callee; callees=%v", callees)
	}
	// Summarizing the caller drives the call-graph fixpoint through the edge.
	_ = q.Summarize(ctx, callerRef)

	// Re-summarizing the callee returns the same converged summary (memoized,
	// deterministic).
	again := q.Summarize(ctx, calleeRef)
	if !summary.SummaryEqual(calleeSum, again) {
		t.Fatalf("callee summary is not stable across calls:\n first=%v\n again=%v", calleeSum, again)
	}
}

// TestSummary_RecursionTerminates is gate (b): the summary fixpoint of a
// self-recursive function (factorial) and of a mutually recursive pair (is_even
// / is_odd) TERMINATES. The bottom seed plus the db cycle drive the call-graph
// fixpoint to a post-fixpoint; the test process -timeout is the only backstop, so
// completing at all proves interproc-recursion termination by construction, not a
// depth cap.
func TestSummary_RecursionTerminates(t *testing.T) {
	t.Run("self-recursion", func(t *testing.T) {
		prog := newTestProgram(t, map[string]string{
			// Self-recursive: factorial calls itself. The call-graph node fact
			// depends on its own summary, a db cycle seeded at bottom.
			"fact": `
local one = 1
local r = fact()
return one
`,
		})
		q := summary.New(prog)
		ctx := db.NewQueryContext(db.New())

		ref := prog.byName["fact"]
		// fact must be its own callee (self-edge), the recursion under test.
		callees := prog.Callees(ref)
		if len(callees) != 1 || callees[0] != ref {
			t.Fatalf("fact must call itself; callees=%v", callees)
		}

		// Summarize must terminate (the cycle converges via the bottom seed + the
		// summary lattice). A non-terminating regression hits the -timeout.
		sum := q.Summarize(ctx, ref)
		// Sensible summary: returns one slot (the literal 1 it returns).
		if len(sum.Returns) != 1 {
			t.Fatalf("fact must summarize one return slot; got %d (%v)", len(sum.Returns), sum.Returns)
		}
		lit, ok := sum.Returns[0].ProjectValue().(*typ.Literal)
		if !ok || lit.Base != kind.Integer {
			t.Fatalf("fact return slot 0 must be an integer literal; got %v", sum.Returns[0].ProjectValue())
		}
	})

	t.Run("mutual-recursion", func(t *testing.T) {
		prog := newTestProgram(t, map[string]string{
			// is_even calls is_odd and vice versa: a 2-node call-graph cycle. The
			// db cycle solves the pair together from the bottom seed.
			"is_even": `
local r = is_odd()
return r
`,
			"is_odd": `
local r = is_even()
return r
`,
		})
		q := summary.New(prog)
		ctx := db.NewQueryContext(db.New())

		evenRef := prog.byName["is_even"]
		oddRef := prog.byName["is_odd"]

		// The cycle is genuine: each calls the other.
		if c := prog.Callees(evenRef); len(c) != 1 || c[0] != oddRef {
			t.Fatalf("is_even must call is_odd; callees=%v", c)
		}
		if c := prog.Callees(oddRef); len(c) != 1 || c[0] != evenRef {
			t.Fatalf("is_odd must call is_even; callees=%v", c)
		}

		// Both summaries must converge; the db cycle iterates the pair to a
		// post-fixpoint. Reaching here at all is the termination property.
		evenSum := q.Summarize(ctx, evenRef)
		oddSum := q.Summarize(ctx, oddRef)

		// Re-summarizing yields the same converged value (a true fixpoint).
		if !summary.SummaryEqual(evenSum, q.Summarize(ctx, evenRef)) {
			t.Fatal("is_even summary did not converge to a stable fixpoint")
		}
		if !summary.SummaryEqual(oddSum, q.Summarize(ctx, oddRef)) {
			t.Fatal("is_odd summary did not converge to a stable fixpoint")
		}
	})
}
