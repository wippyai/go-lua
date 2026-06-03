package summary

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// FuncRef is kept as a summary-package alias for existing callers. The canonical
// identity itself lives in ref so module facts, transfer, and summary can share it
// without import cycles.
type FuncRef = ref.FuncRef

// Key identifies one interprocedural summary context. Entry is the captured-cell
// store visible at function entry; Refs is the function-identity path state that
// must travel with captured values; Values are caller-projected parameter product
// facts. All map-shaped components are interned as exact comparable keys.
type Key struct {
	Ref      FuncRef
	Entry    flow.CaptureCellsKey
	Refs     flow.FunctionRefsKey
	Closures flow.ClosureRefsKey
	Values   EntryValuesKey
}

// NewKey constructs the canonical summary key for ref and entry cells.
func NewKey(ref FuncRef, entry flow.CaptureCells) Key {
	return NewKeyWithRefs(ref, entry, flow.FunctionRefsDomain.Bottom())
}

// NewKeyWithRefs constructs the canonical summary key for ref, entry cells, and
// entry function identities.
func NewKeyWithRefs(ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs) Key {
	return NewKeyWithEntryValues(ref, entry, refs, nil)
}

// NewKeyWithEntryValues constructs the canonical summary key for ref, entry
// cells, entry function identities, and caller-projected parameter values.
func NewKeyWithEntryValues(ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs, values EntryValues) Key {
	return NewKeyWithEntryContext(ref, entry, refs, flow.ClosureRefsDomain.Bottom(), values)
}

// NewKeyWithEntryContext constructs the canonical summary key for every
// caller-provided entry component.
func NewKeyWithEntryContext(ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, values EntryValues) Key {
	return Key{
		Ref:      ref,
		Entry:    entry.Key(),
		Refs:     flow.FunctionRefsKeyOf(refs),
		Closures: flow.ClosureRefsKeyOf(closures),
		Values:   EntryValuesKeyOf(values),
	}
}

// Program is the call graph the summary fixpoint ranges over. It is the seam to
// the rest of the pipeline: it enumerates each function's inputs (graph,
// parameter count, per-node transfer) and the callees reachable from its body.
// This package owns neither CFG construction nor transfer assembly — a caller
// (the canonical driver, or a test) supplies them through this interface.
//
// Callees enumerates static body-call edges. It is graph metadata, not the
// semantic dependency itself: summary dependencies are created by transfer or
// projection code when it reads a callee Summary cell under the relevant entry
// context. A self-referential or mutually recursive cluster becomes cyclic
// through those semantic reads; the db cycle solves it with a bottom seed.
type Program interface {
	// Graph returns the control-flow graph for ref, or nil if ref is unknown.
	Graph(ref FuncRef) *cfg.Graph
	// NumParams returns ref's parameter count, the number of contract cells the
	// equation graph allocates.
	NumParams(ref FuncRef) int
	// Transfer returns the per-node transfer for ref's intraprocedural solve.
	Transfer(ref FuncRef) equation.NodeTransfer
	// Callees returns the functions ref's body calls as deterministic graph
	// metadata. Semantic dependencies are recorded by transfer/projection reads.
	Callees(ref FuncRef) []FuncRef
}

type declaredReturnsProvider interface {
	DeclaredReturns(ref FuncRef) []typ.Type
}

type returnCallTargetProvider interface {
	ReturnCallHasFiniteTarget(ref FuncRef, call *cfg.CallInfo) bool
}

type captureEntryProvider interface {
	CaptureEntries(ref FuncRef, captureExportsOf func(FuncRef) flow.CaptureCells) flow.CaptureCells
}

type captureEntryRefsProvider interface {
	CaptureEntryRefs(ref FuncRef, captureFunctionRefsOf func(FuncRef) flow.FunctionRefs) flow.FunctionRefs
}

type captureEntryClosureRefsProvider interface {
	CaptureEntryClosureRefs(ref FuncRef, captureClosureRefsOf func(FuncRef) flow.ClosureRefs) flow.ClosureRefs
}

// EntryValueDependencies exposes only the summary components needed to seed a
// callee's entry-value evidence. The solve query owns the actual Summary
// projection read; a provider must not inspect unrelated Summary axes or
// introduce driver-owned precision logic.
type EntryValueDependencies interface {
	CallEntryValues(dep FuncRef, callee FuncRef) EntryValues
	PrototypeSelf(dep FuncRef) flow.PrototypeSelf
}

type entryValueProvider interface {
	EntryValues(ref FuncRef, deps EntryValueDependencies) map[int]product.AbstractValue
}

type callEntryValueProjector interface {
	ProjectCallEntryValues(ref FuncRef, fs state.FunctionState) CallEntryValues
}

type solveContextRunner interface {
	WithSolveContext(ctx *db.QueryContext, solve func() state.FunctionState) state.FunctionState
}

type entrySymbolValueProvider interface {
	EntrySymbolValues(ref FuncRef) map[cfg.SymbolID]product.AbstractValue
}

type paramNarrowProvider interface {
	LocalParamNarrows(ref FuncRef) []paramevidence.ParamNarrow
	DelegatedParamNarrowCalls(ref FuncRef) []paramevidence.DelegatedCall
	ResolveDelegatedCallee(ref FuncRef, call *ast.FuncCallExpr) (FuncRef, bool)
}

// Reader is the summary-owned observation boundary for a caller that may be
// inside the recursive summary solve or after convergence. During a solve it
// reads the live query cell under the requested entry context, recording the real
// dependency in the db cycle. After convergence it reads the immutable summary
// snapshot. Callers should not duplicate this live-vs-snapshot choice.
type Reader struct {
	queries   *Queries
	ctx       *db.QueryContext
	converged map[FuncRef]Summary
}

// NewReader constructs a summary observer for live query reads plus a converged
// fallback snapshot. A nil query/context makes the reader snapshot-only.
func NewReader(queries *Queries, ctx *db.QueryContext, converged map[FuncRef]Summary) Reader {
	return Reader{queries: queries, ctx: ctx, converged: converged}
}

// Live reports whether this reader is observing the active summary query cycle.
// When false, entry-context arguments are irrelevant because reads come from the
// converged snapshot.
func (r Reader) Live() bool {
	return r.queries != nil && r.ctx != nil
}

// Queries bundles the memoized db views used to evaluate the canonical product
// equation system over one Program. Construct it with New; share it across the
// analysis so call-graph cycles memoize per function/context.
type Queries struct {
	prog Program

	// solveQ evaluates the recursive caller-facing Summary cell. Its compute
	// performs the local point/demand solve needed to project that summary.
	// Diagnostic Intra reads are exact observers over the converged Summary state,
	// not a second memoized fixed point.
	solveQ *db.Query[Key, solveResult]

	// ParamNarrowQ evaluates context-free, caller-visible parameter-refinement
	// cells. These facts are expressed only over parameter placeholders, so they do
	// not vary with entry values/captures; transfer reads this cell directly instead
	// of forcing a full bottom-context Summary dependency.
	ParamNarrowQ *db.Query[FuncRef, []paramevidence.ParamNarrow]
}

type solveResult struct {
	State   state.FunctionState
	Summary Summary
}

// New builds the canonical summary query product over prog.
//
// The solve query owns the intraprocedural compute and the Summary projection
// together for the caller-facing Summary. That gives recursive summary reads,
// entry context, and projection one db cycle with no separate IntraQ cache. The
// db layer is revision-granular, so FunctionState is not a convergence axis here:
// caching it inside the recursive query can return same-revision stale point
// states after caller entry evidence moves. Intra therefore re-solves exactly over
// the converged Summary dependencies as an observer, not as a second fixed point.
func New(prog Program) *Queries {
	q := &Queries{prog: prog}

	q.solveQ = db.NewQueryWithSeedAndWiden(
		"CanonicalSummarySolve",
		q.computeSolve,
		solveResultEqual,
		func(*db.QueryContext, Key) solveResult {
			return solveResult{
				State:   state.FunctionStateDomain.Bottom(),
				Summary: SummaryDomain.Bottom(),
			}
		},
		solveResultWiden,
	)

	q.ParamNarrowQ = db.NewQueryWithSeedAndWiden(
		"CanonicalParamNarrows",
		q.computeParamNarrows,
		func(a, b []paramevidence.ParamNarrow) bool {
			return paramNarrowsDomain.Equal(a, b)
		},
		func(*db.QueryContext, FuncRef) []paramevidence.ParamNarrow {
			return paramNarrowsDomain.Bottom()
		},
		func(prev, next []paramevidence.ParamNarrow) []paramevidence.ParamNarrow {
			return paramNarrowsDomain.Widen(prev, next)
		},
	)

	return q
}

// Intra returns ref's converged intraprocedural FunctionState, memoized.
func (q *Queries) Intra(ctx *db.QueryContext, ref FuncRef) state.FunctionState {
	return q.IntraWithEntry(ctx, ref, flow.CaptureCellsDomain.Bottom())
}

// IntraWithEntry returns ref's converged intraprocedural state under entry cells.
func (q *Queries) IntraWithEntry(ctx *db.QueryContext, ref FuncRef, entry flow.CaptureCells) state.FunctionState {
	return q.IntraWithEntryRefs(ctx, ref, entry, flow.FunctionRefsDomain.Bottom())
}

// IntraWithEntryRefs returns ref's converged intraprocedural state under entry
// cells and function identities.
func (q *Queries) IntraWithEntryRefs(ctx *db.QueryContext, ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs) state.FunctionState {
	return q.IntraWithEntryValues(ctx, ref, entry, refs, nil)
}

// IntraWithEntryValues returns ref's converged intraprocedural state under entry
// cells, function identities, and caller-projected parameter values.
func (q *Queries) IntraWithEntryValues(ctx *db.QueryContext, ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs, values EntryValues) state.FunctionState {
	return q.intra(ctx, NewKeyWithEntryValues(ref, entry, refs, values))
}

// IntraWithEntryContext returns ref's converged intraprocedural state under all
// caller-provided entry components.
func (q *Queries) IntraWithEntryContext(ctx *db.QueryContext, ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, values EntryValues) state.FunctionState {
	return q.intra(ctx, NewKeyWithEntryContext(ref, entry, refs, closures, values))
}

// Summarize returns ref's interprocedural Summary, driving the call-graph
// fixpoint as needed. This is the call-site lookup seam: a caller resolving a
// call to ref reads the converged callee summary here.
func (q *Queries) Summarize(ctx *db.QueryContext, ref FuncRef) Summary {
	return q.SummarizeWithEntry(ctx, ref, flow.CaptureCellsDomain.Bottom())
}

// SummarizeWithEntry returns ref's summary under a caller-provided entry cell
// store.
func (q *Queries) SummarizeWithEntry(ctx *db.QueryContext, ref FuncRef, entry flow.CaptureCells) Summary {
	return q.SummarizeWithEntryRefs(ctx, ref, entry, flow.FunctionRefsDomain.Bottom())
}

// SummarizeWithEntryRefs returns ref's summary under caller-provided entry cell
// and function-identity contexts.
func (q *Queries) SummarizeWithEntryRefs(ctx *db.QueryContext, ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs) Summary {
	return q.SummarizeWithEntryValues(ctx, ref, entry, refs, nil)
}

// SummarizeWithEntryValues returns ref's summary under caller-provided entry
// cells, function identities, and caller-projected parameter values.
func (q *Queries) SummarizeWithEntryValues(ctx *db.QueryContext, ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs, values EntryValues) Summary {
	return q.solveQ.Get(ctx, NewKeyWithEntryValues(ref, entry, refs, values)).Summary
}

// SummarizeWithEntryContext returns ref's summary under all caller-provided
// entry components.
func (q *Queries) SummarizeWithEntryContext(ctx *db.QueryContext, ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, values EntryValues) Summary {
	return q.solveQ.Get(ctx, NewKeyWithEntryContext(ref, entry, refs, closures, values)).Summary
}

// Summarize returns ref's caller-visible summary through this reader.
func (r Reader) Summarize(ref FuncRef) Summary {
	return r.SummarizeWithEntryContext(ref, flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom(), nil)
}

// SummarizeWithEntryValues reads ref's summary under entry cells, function
// identities, and caller-projected parameter values.
func (r Reader) SummarizeWithEntryValues(ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs, values EntryValues) Summary {
	return r.SummarizeWithEntryContext(ref, entry, refs, flow.ClosureRefsDomain.Bottom(), values)
}

// SummarizeWithEntryContext reads ref's summary under all caller-provided entry
// components, using the live summary query when one is active and the converged
// snapshot otherwise.
func (r Reader) SummarizeWithEntryContext(ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, values EntryValues) Summary {
	if r.queries != nil && r.ctx != nil {
		return r.queries.SummarizeWithEntryContext(r.ctx, ref, entry, refs, closures, values)
	}
	if sum, ok := r.converged[ref]; ok {
		return sum
	}
	return SummaryDomain.Bottom()
}

// ParamNarrows reads ref's portable parameter-refinement summary cell through
// the same live-or-converged observation boundary.
func (r Reader) ParamNarrows(ref FuncRef) []paramevidence.ParamNarrow {
	if r.queries != nil && r.ctx != nil {
		return r.queries.ParamNarrows(r.ctx, ref)
	}
	if sum, ok := r.converged[ref]; ok {
		return paramevidence.SortParamNarrows(sum.ParamNarrows)
	}
	return nil
}

func (q *Queries) intra(ctx *db.QueryContext, key Key) state.FunctionState {
	// First demand the recursive Summary cell so entry-value/capture dependencies
	// are at their converged approximation. The local state solve below is an exact
	// observer over those dependencies; it is intentionally not memoized as a
	// separate db fixed point.
	_ = q.solveQ.Get(ctx, key).Summary
	return q.solveIntra(ctx, key.Ref, q.entryCells(ctx, key), q.entryRefs(ctx, key), q.entryClosureRefs(ctx, key), q.entryValues(ctx, key), q.entrySymbolValues(key.Ref))
}

// ParamNarrows returns ref's context-free parameter-refinement cell: portable
// facts proved about source parameters on every normal return.
func (q *Queries) ParamNarrows(ctx *db.QueryContext, ref FuncRef) []paramevidence.ParamNarrow {
	return q.ParamNarrowQ.Get(ctx, ref)
}

// computeSolve is solveQ's compute for one product cell.
//
// It solves ref's point/demand cells for this context and projects them to a
// Summary. Any callee summary needed by that solve is read by the transfer or by
// summary projection under the concrete entry context being modeled, so the db
// cycle records the real semantic dependency rather than an eager bottom-context
// call-graph edge.
func (q *Queries) computeSolve(ctx *db.QueryContext, key Key) solveResult {
	fs := q.solveIntra(ctx, key.Ref, q.entryCells(ctx, key), q.entryRefs(ctx, key), q.entryClosureRefs(ctx, key), q.entryValues(ctx, key), q.entrySymbolValues(key.Ref))
	sum := ProjectWithOptions(fs, q.prog.Graph(key.Ref), ProjectOptions{
		DeclaredReturns:           q.declaredReturns(key.Ref),
		ReturnCallHasFiniteTarget: q.returnCallHasFiniteTarget(key.Ref),
	})
	sum.ParamNarrows = q.ParamNarrows(ctx, key.Ref)
	if projector, ok := q.prog.(callEntryValueProjector); ok && projector != nil {
		sum.CallEntryValues = projector.ProjectCallEntryValues(key.Ref, fs)
	}
	return solveResult{State: fs, Summary: sum}
}

func (q *Queries) computeParamNarrows(ctx *db.QueryContext, ref FuncRef) []paramevidence.ParamNarrow {
	provider, ok := q.prog.(paramNarrowProvider)
	if !ok || provider == nil {
		return nil
	}
	out := paramevidence.SortParamNarrows(provider.LocalParamNarrows(ref))
	for _, dc := range provider.DelegatedParamNarrowCalls(ref) {
		calleeRef, ok := provider.ResolveDelegatedCallee(ref, dc.Call)
		if !ok {
			continue
		}
		for _, ce := range q.ParamNarrows(ctx, calleeRef) {
			for _, inherited := range delegatedParamNarrows(ce, dc) {
				out = paramNarrowSetLattice{}.Join(out, []paramevidence.ParamNarrow{inherited})
			}
		}
	}
	return paramevidence.SortParamNarrows(out)
}

func delegatedParamNarrows(callee paramevidence.ParamNarrow, dc paramevidence.DelegatedCall) []paramevidence.ParamNarrow {
	if callee.IsParamEquality() {
		left, ok := delegatedParamIndex(callee.Param, dc.ArgParams)
		if !ok {
			return nil
		}
		right, ok := delegatedParamIndex(callee.EqParam, dc.ArgParams)
		if !ok || left == right {
			return nil
		}
		return []paramevidence.ParamNarrow{{Param: left, EqParam: right}}
	}
	if callee.IsParamInequality() {
		left, ok := delegatedParamIndex(callee.Param, dc.ArgParams)
		if !ok {
			return nil
		}
		right, ok := delegatedParamIndex(callee.EqParam, dc.ArgParams)
		if !ok || left == right {
			return nil
		}
		return []paramevidence.ParamNarrow{{Param: left, EqParam: right, NotEqual: true}}
	}
	if callee.CondArg {
		return delegatedConditionArgNarrows(callee, dc)
	}
	if len(callee.Segments) != 0 || callee.EqParam >= 0 || callee.CastType != nil {
		return nil
	}
	switch callee.Check {
	case cfg.CheckTruthy, cfg.CheckNotNil, cfg.CheckNil, cfg.CheckFalsy, cfg.CheckTypeEqual, cfg.CheckTypeNot:
	default:
		return nil
	}
	callerParam, ok := delegatedParamIndex(callee.Param, dc.ArgParams)
	if !ok {
		return nil
	}
	return []paramevidence.ParamNarrow{{
		Param:   callerParam,
		Check:   callee.Check,
		TypeKey: callee.TypeKey,
		EqParam: -1,
	}}
}

func delegatedConditionArgNarrows(callee paramevidence.ParamNarrow, dc paramevidence.DelegatedCall) []paramevidence.ParamNarrow {
	if callee.Param < 0 {
		return nil
	}
	var effects []paramevidence.ParamNarrow
	switch callee.Check {
	case cfg.CheckTruthy:
		effects = delegatedArgEffects(dc.ArgTruthyEffects, callee.Param)
	case cfg.CheckFalsy:
		effects = delegatedArgEffects(dc.ArgFalsyEffects, callee.Param)
	default:
		return nil
	}
	return paramevidence.SortParamNarrows(effects)
}

func delegatedArgEffects(all [][]paramevidence.ParamNarrow, arg int) []paramevidence.ParamNarrow {
	if arg < 0 || arg >= len(all) || len(all[arg]) == 0 {
		return nil
	}
	return paramevidence.SortParamNarrows(all[arg])
}

func delegatedParamIndex(calleeParam int, argParams []int) (int, bool) {
	if calleeParam < 0 || calleeParam >= len(argParams) {
		return 0, false
	}
	callerParam := argParams[calleeParam]
	if callerParam < 0 {
		return 0, false
	}
	return callerParam, true
}

func (q *Queries) entryCells(ctx *db.QueryContext, key Key) flow.CaptureCells {
	entries := key.Entry.Cells()
	if provider, ok := q.prog.(captureEntryProvider); ok {
		lexical := provider.CaptureEntries(key.Ref, func(dep FuncRef) flow.CaptureCells {
			return q.solveQ.Get(ctx, NewKey(dep, flow.CaptureCellsDomain.Bottom())).Summary.CaptureExports
		})
		entries = mergeCaptureCellsWithFixed(entries, lexical)
	}
	return entries
}

func (q *Queries) entryRefs(ctx *db.QueryContext, key Key) flow.FunctionRefs {
	refs := key.Refs.Refs()
	if provider, ok := q.prog.(captureEntryRefsProvider); ok {
		lexical := provider.CaptureEntryRefs(key.Ref, func(dep FuncRef) flow.FunctionRefs {
			return q.solveQ.Get(ctx, NewKey(dep, flow.CaptureCellsDomain.Bottom())).Summary.CaptureFunctionRefs
		})
		refs = mergeFunctionRefsWithFixed(refs, lexical)
	}
	return refs
}

func (q *Queries) entryClosureRefs(ctx *db.QueryContext, key Key) flow.ClosureRefs {
	refs := key.Closures.Refs()
	if provider, ok := q.prog.(captureEntryClosureRefsProvider); ok {
		lexical := provider.CaptureEntryClosureRefs(key.Ref, func(dep FuncRef) flow.ClosureRefs {
			return q.solveQ.Get(ctx, NewKey(dep, flow.CaptureCellsDomain.Bottom())).Summary.CaptureClosureRefs
		})
		refs = mergeClosureRefsWithFixed(refs, lexical)
	}
	return refs
}

func (q *Queries) entryValues(ctx *db.QueryContext, key Key) map[int]product.AbstractValue {
	values := key.Values.Values()
	provider, ok := q.prog.(entryValueProvider)
	if !ok || provider == nil {
		return values
	}
	provided := provider.EntryValues(key.Ref, NewReader(q, ctx, nil))
	return MergeEntryValuesWithFixed(values, provided)
}

func mergeCaptureCellsWithFixed(fixed, fallback flow.CaptureCells) flow.CaptureCells {
	if fixed.IsTop() || fallback.IsTop() {
		if fixed.IsTop() {
			return fixed
		}
		return fallback
	}
	if len(fixed.Entries()) == 0 {
		return flow.CaptureCellsDomain.Join(fallback, flow.CaptureCellsDomain.Bottom())
	}
	out := fixed
	for _, entry := range fallback.Entries() {
		if _, ok := out.Value(entry.Symbol); ok {
			continue
		}
		out = out.With(entry.Symbol, entry.Value)
	}
	return flow.CaptureCellsDomain.Join(out, flow.CaptureCellsDomain.Bottom())
}

func mergeFunctionRefsWithFixed(fixed, fallback flow.FunctionRefs) flow.FunctionRefs {
	if flow.FunctionRefsDomain.Equal(fixed, flow.FunctionRefsDomain.Top()) ||
		flow.FunctionRefsDomain.Equal(fallback, flow.FunctionRefsDomain.Top()) {
		if flow.FunctionRefsDomain.Equal(fixed, flow.FunctionRefsDomain.Top()) {
			return fixed
		}
		return fallback
	}
	if len(fixed) == 0 {
		return flow.FunctionRefsDomain.Join(fallback, flow.FunctionRefsDomain.Bottom())
	}
	out := flow.FunctionRefsDomain.Join(fixed, flow.FunctionRefsDomain.Bottom())
	for path, set := range fallback {
		if set.IsBottom() {
			continue
		}
		if _, ok := flow.FunctionRefAt(out, path); ok {
			continue
		}
		out = flow.WithFunctionRef(out, path, set)
	}
	return flow.FunctionRefsDomain.Join(out, flow.FunctionRefsDomain.Bottom())
}

func mergeClosureRefsWithFixed(fixed, fallback flow.ClosureRefs) flow.ClosureRefs {
	if flow.ClosureRefsDomain.Equal(fixed, flow.ClosureRefsDomain.Top()) ||
		flow.ClosureRefsDomain.Equal(fallback, flow.ClosureRefsDomain.Top()) {
		if flow.ClosureRefsDomain.Equal(fixed, flow.ClosureRefsDomain.Top()) {
			return fixed
		}
		return fallback
	}
	if len(fixed) == 0 {
		return flow.ClosureRefsDomain.Join(fallback, flow.ClosureRefsDomain.Bottom())
	}
	out := flow.ClosureRefsDomain.Join(fixed, flow.ClosureRefsDomain.Bottom())
	for path, set := range fallback {
		if set.IsBottom() {
			continue
		}
		if _, ok := flow.ClosureRefAt(out, path); ok {
			continue
		}
		out = flow.WithClosureRef(out, path, set)
	}
	return flow.ClosureRefsDomain.Join(out, flow.ClosureRefsDomain.Bottom())
}

func (q *Queries) entrySymbolValues(ref FuncRef) map[cfg.SymbolID]product.AbstractValue {
	provider, ok := q.prog.(entrySymbolValueProvider)
	if !ok || provider == nil {
		return nil
	}
	return provider.EntrySymbolValues(ref)
}

// solveIntra drives the equation.Builder for ref/context. It evaluates the
// point/demand cell subgraph used by the summary cell; it does not touch the db
// cache itself (its caller memoizes).
func (q *Queries) solveIntra(ctx *db.QueryContext, ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, entryValues map[int]product.AbstractValue, entrySymbolValues map[cfg.SymbolID]product.AbstractValue) state.FunctionState {
	g := q.prog.Graph(ref)
	if g == nil {
		return state.FunctionStateDomain.Bottom()
	}
	tr := q.prog.Transfer(ref)
	if tr == nil {
		return state.FunctionStateDomain.Bottom()
	}
	solve := func() state.FunctionState {
		return equation.NewBuilder(g, q.prog.NumParams(ref), tr).
			WithEntryCells(entry).
			WithEntryFunctionRefs(refs).
			WithEntryClosureRefs(closures).
			WithEntryValues(entryValues).
			WithEntrySymbolValues(entrySymbolValues).
			Solve()
	}
	if runner, ok := q.prog.(solveContextRunner); ok && runner != nil {
		return runner.WithSolveContext(ctx, solve)
	}
	return solve()
}

func (q *Queries) declaredReturns(ref FuncRef) []typ.Type {
	prog, ok := q.prog.(declaredReturnsProvider)
	if !ok || prog == nil {
		return nil
	}
	return prog.DeclaredReturns(ref)
}

func (q *Queries) returnCallHasFiniteTarget(ref FuncRef) func(*cfg.CallInfo) bool {
	prog, ok := q.prog.(returnCallTargetProvider)
	if !ok || prog == nil {
		return nil
	}
	return func(call *cfg.CallInfo) bool {
		return prog.ReturnCallHasFiniteTarget(ref, call)
	}
}

func solveResultEqual(a, b solveResult) bool {
	return SummaryEqual(a.Summary, b.Summary)
}

func solveResultWiden(prev, next solveResult) solveResult {
	return solveResult{
		State:   next.State,
		Summary: SummaryWiden(prev.Summary, next.Summary),
	}
}
