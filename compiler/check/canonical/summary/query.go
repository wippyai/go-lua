package summary

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/types/db"
)

// FuncRef identifies one function in the call graph. It is the comparable db
// query key for both the intraprocedural solve and the interprocedural summary,
// mirroring the legacy api.FuncKey: a function is the unit the call-graph
// fixpoint ranges over.
//
// GraphID is the CFG's stable per-construction identity (cfg.Graph.ID). A
// distinct lexical instance of the same source — a nested function analyzed
// under a different captured environment — carries a distinct ParentHash, so two
// instances with identical bodies but different scopes summarize separately,
// exactly as the legacy FuncKey distinguishes them.
type FuncRef struct {
	GraphID    uint64
	ParentHash uint64
}

// Program is the call graph the summary fixpoint ranges over. It is the seam to
// the rest of the pipeline: it enumerates each function's inputs (graph,
// parameter count, per-node transfer) and the callees reachable from its body.
// This package owns neither CFG construction nor transfer assembly — a caller
// (the canonical driver, or a test) supplies them through this interface.
//
// Callees is what makes the summary system a least fixed point over the call
// graph: a function's summary depends on the summaries of the functions it
// calls, so SummaryQ resolves each callee's summary while computing its own.
// A self-referential or mutually recursive cluster makes the dependency cyclic;
// the db cycle solves it with a bottom seed.
type Program interface {
	// Graph returns the control-flow graph for ref, or nil if ref is unknown.
	Graph(ref FuncRef) *cfg.Graph
	// NumParams returns ref's parameter count, the number of contract cells the
	// equation graph allocates.
	NumParams(ref FuncRef) int
	// Transfer returns the per-node transfer for ref's intraprocedural solve.
	Transfer(ref FuncRef) equation.NodeTransfer
	// Callees returns the functions ref's body calls. The summary fixpoint reads
	// each callee's summary, recording the call-graph dependency.
	Callees(ref FuncRef) []FuncRef
}

// Queries bundles the two memoized db queries of the canonical interprocedural
// fixed point over one Program. Construct it with New; share it across the
// analysis so the call-graph cycle memoizes per function.
type Queries struct {
	prog Program

	// IntraQ memoizes the per-function intraprocedural solve: the converged
	// state.FunctionState the equation.Builder produces for one CFG. It is the
	// inner fixed point, surfaced for callers that need the full per-point state.
	IntraQ *db.Query[FuncRef, state.FunctionState]

	// SummaryQ is the call-graph least fixed point: each function's caller-facing
	// Summary, resolving callee summaries through itself. Mutual recursion is a db
	// query cycle solved with the SummaryDomain bottom seed and SummaryWiden.
	SummaryQ *db.Query[FuncRef, Summary]
}

// New builds the IntraQ/SummaryQ pair over prog.
//
// SummaryQ OWNS the intraprocedural compute (journal #353 / Codex C4): the db
// dependency stamp is revision-granular, so a separately memoized inner solve
// changing within one revision would not re-trigger the summary in that
// revision. By computing the FunctionState inside SummaryQ's compute — driving
// the equation.Builder and reading callee summaries in the same frame — the
// inner solve and the outer summary converge as one db cycle, with no reliance
// on a same-revision stamp. IntraQ shares that compute so a caller wanting the
// full per-point state reuses SummaryQ's work rather than re-solving.
func New(prog Program) *Queries {
	q := &Queries{prog: prog}

	q.IntraQ = db.NewQuery("CanonicalIntra", q.computeIntra, functionStateEqual)

	q.SummaryQ = db.NewQueryWithSeedAndWiden(
		"CanonicalSummary",
		q.computeSummary,
		SummaryEqual,
		// Bottom seed: a callee reached recursively before its summary exists
		// starts at the lattice bottom (no return, no contract). The fixpoint
		// climbs from there.
		func(*db.QueryContext, FuncRef) Summary { return SummaryDomain.Bottom() },
		// Widen accelerates a recursive summary that keeps growing, so the cycle
		// terminates by lattice height, not a depth cap.
		SummaryWiden,
	)

	return q
}

// Intra returns ref's converged intraprocedural FunctionState, memoized.
func (q *Queries) Intra(ctx *db.QueryContext, ref FuncRef) state.FunctionState {
	return q.IntraQ.Get(ctx, ref)
}

// Summarize returns ref's interprocedural Summary, driving the call-graph
// fixpoint as needed. This is the call-site lookup seam: a caller resolving a
// call to ref reads the converged callee summary here.
func (q *Queries) Summarize(ctx *db.QueryContext, ref FuncRef) Summary {
	return q.SummaryQ.Get(ctx, ref)
}

// computeIntra is IntraQ's compute: solve ref's intraprocedural fixed point.
func (q *Queries) computeIntra(_ *db.QueryContext, ref FuncRef) state.FunctionState {
	return q.solveIntra(ref)
}

// computeSummary is SummaryQ's compute and the call-graph fixpoint step.
//
// It (1) reads every callee's summary through SummaryQ, recording the call-graph
// dependency that makes a recursive cluster a db cycle, then (2) solves ref's own
// intraprocedural fixed point and projects it to a Summary. Reading the callee
// summaries first means a change to a callee's summary across the cycle's
// iterations re-triggers this function, and the db cycle iterates the cluster to
// its post-fixpoint. Because SummaryQ owns the intraproc compute, the inner solve
// re-runs whenever a dependency moves — no separate same-revision stamp needed.
func (q *Queries) computeSummary(ctx *db.QueryContext, ref FuncRef) Summary {
	for _, callee := range q.prog.Callees(ref) {
		// Record the dependency on each callee's summary. The current value feeds
		// the call-site resolution that a later transfer-fidelity pass consumes;
		// here it closes the call-graph edge so the cycle converges.
		_ = q.SummaryQ.Get(ctx, callee)
	}

	fs := q.solveIntra(ref)
	return Project(fs, q.prog.Graph(ref))
}

// solveIntra drives the equation.Builder for ref. It is the shared inner solve
// both queries use; it does not touch the db cache itself (its caller memoizes).
func (q *Queries) solveIntra(ref FuncRef) state.FunctionState {
	g := q.prog.Graph(ref)
	if g == nil {
		return state.FunctionStateDomain.Bottom()
	}
	tr := q.prog.Transfer(ref)
	if tr == nil {
		return state.FunctionStateDomain.Bottom()
	}
	return equation.NewBuilder(g, q.prog.NumParams(ref), tr).Solve()
}

// functionStateEqual is IntraQ's convergence/equality function.
func functionStateEqual(a, b state.FunctionState) bool {
	return state.FunctionStateDomain.Equal(a, b)
}
