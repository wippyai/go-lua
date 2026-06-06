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
	Facts    EntryFactsKey
}

// NewDefaultKey constructs the key for a function entered without captured
// caller axes, preserving only caller-projected parameter values.
func NewDefaultKey(ref FuncRef, values EntryValues) Key {
	return NewKeyWithEntryContextFacts(ref, flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom(), values, flow.BoundaryFactsDomain.Top())
}

// NewKeyWithEntryContextFacts constructs the canonical summary key for every
// caller-provided entry component, including parameter-relative path facts.
func NewKeyWithEntryContextFacts(ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, values EntryValues, facts flow.BoundaryFacts) Key {
	return Key{
		Ref:      ref,
		Entry:    entry.Key(),
		Refs:     flow.FunctionRefsKeyOf(refs),
		Closures: flow.ClosureRefsKeyOf(closures),
		Values:   entryValuesKeyOf(values),
		Facts:    entryFactsKeyOf(facts),
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

type entryValueMerger interface {
	MergeEntryValues(ref FuncRef, fixed, fallback EntryValues) EntryValues
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
	queries     *Queries
	ctx         *db.QueryContext
	converged   map[FuncRef]Summary
	overlay     map[Key]Summary
	overlayRead func(Key)
}

// NewReader constructs a summary observer for live query reads plus a converged
// fallback snapshot. A nil query/context makes the reader snapshot-only.
func NewReader(queries *Queries, ctx *db.QueryContext, converged map[FuncRef]Summary) Reader {
	return Reader{queries: queries, ctx: ctx, converged: converged}
}

// NewReaderWithOverlay constructs a summary observer with exact-key snapshot
// overrides. The overlay is used only when the reader is snapshot-backed; live
// readers still record semantic dependencies through the recursive summary query.
func NewReaderWithOverlay(queries *Queries, ctx *db.QueryContext, converged map[FuncRef]Summary, overlay map[Key]Summary) Reader {
	return Reader{queries: queries, ctx: ctx, converged: converged, overlay: overlay}
}

// NewReaderWithOverlayReads constructs a snapshot observer that reports every
// exact-overlay key it attempts to read. Diagnostic context discovery uses those
// read edges to refresh only observers whose exact callee overlay changed.
func NewReaderWithOverlayReads(queries *Queries, ctx *db.QueryContext, converged map[FuncRef]Summary, overlay map[Key]Summary, overlayRead func(Key)) Reader {
	return Reader{queries: queries, ctx: ctx, converged: converged, overlay: overlay, overlayRead: overlayRead}
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
	return q.IntraWithKey(ctx, NewDefaultKey(ref, nil))
}

// IntraWithKey returns the converged intraprocedural state for an already
// normalized summary key.
func (q *Queries) IntraWithKey(ctx *db.QueryContext, key Key) state.FunctionState {
	return q.intra(ctx, key)
}

// ObserveIntraWithKey observes the local point/demand solver for an already
// normalized exact entry key, without demanding a corresponding recursive
// Summary cell first.
func (q *Queries) ObserveIntraWithKey(ctx *db.QueryContext, key Key) state.FunctionState {
	return q.solveIntra(ctx, key.Ref, q.entryCells(ctx, key), q.entryRefs(ctx, key), q.entryClosureRefs(ctx, key), q.entryValues(ctx, key), q.entryFacts(key), q.entrySymbolValues(key.Ref))
}

// Summarize returns ref's interprocedural Summary, driving the call-graph
// fixpoint as needed. This is the call-site lookup seam: a caller resolving a
// call to ref reads the converged callee summary here.
func (q *Queries) Summarize(ctx *db.QueryContext, ref FuncRef) Summary {
	return q.SummarizeWithKey(ctx, NewDefaultKey(ref, nil))
}

// ObservedSummary returns the caller-visible summary after the recursive summary
// cell has converged and the local state has been re-observed over those
// dependencies. The recursive cell's Summary remains the lawful widened carrier
// used to terminate cycles; this projection is the post-widen narrowing surface
// consumed by snapshot readers and diagnostics.
func (q *Queries) ObservedSummary(ctx *db.QueryContext, ref FuncRef) Summary {
	fs := q.Intra(ctx, ref)
	return q.ProjectStateSummary(ctx, ref, fs)
}

// SummarizeWithKey returns the summary for an already normalized entry key.
func (q *Queries) SummarizeWithKey(ctx *db.QueryContext, key Key) Summary {
	return q.solveQ.Get(ctx, key).Summary
}

// Summarize returns ref's caller-visible summary through this reader.
func (r Reader) Summarize(ref FuncRef) Summary {
	return r.SummarizeWithKey(NewDefaultKey(ref, nil))
}

// SummarizeWithKey reads a summary through an already normalized entry key.
func (r Reader) SummarizeWithKey(key Key) Summary {
	if r.queries != nil && r.ctx != nil {
		return r.queries.SummarizeWithKey(r.ctx, key)
	}
	if r.overlay != nil {
		if r.overlayRead != nil {
			r.overlayRead(key)
		}
		if sum, ok := r.overlay[key]; ok {
			return sum
		}
	}
	if sum, ok := r.converged[key.Ref]; ok {
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
	return q.solveIntra(ctx, key.Ref, q.entryCells(ctx, key), q.entryRefs(ctx, key), q.entryClosureRefs(ctx, key), q.entryValues(ctx, key), q.entryFacts(key), q.entrySymbolValues(key.Ref))
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
	fs := q.solveIntra(ctx, key.Ref, q.entryCells(ctx, key), q.entryRefs(ctx, key), q.entryClosureRefs(ctx, key), q.entryValues(ctx, key), q.entryFacts(key), q.entrySymbolValues(key.Ref))
	sum := q.ProjectStateSummary(ctx, key.Ref, fs)
	return solveResult{State: fs, Summary: sum}
}

// ProjectStateSummary projects an already-solved local observer state into the
// same caller-visible Summary carrier used by the recursive summary cell. It is
// deliberately a summary-layer operation so diagnostic exact-context overlays do
// not reimplement projection policy in the driver.
func (q *Queries) ProjectStateSummary(ctx *db.QueryContext, ref FuncRef, fs state.FunctionState) Summary {
	sum := projectWithOptions(fs, q.prog.Graph(ref), projectOptions{
		DeclaredReturns:           q.declaredReturns(ref),
		ReturnCallHasFiniteTarget: q.returnCallHasFiniteTarget(ref),
	})
	sum.ParamNarrows = q.ParamNarrows(ctx, ref)
	if projector, ok := q.prog.(callEntryValueProjector); ok && projector != nil {
		sum.CallEntryValues = projector.ProjectCallEntryValues(ref, fs)
	}
	return sum
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
			return q.solveQ.Get(ctx, NewDefaultKey(dep, nil)).Summary.CaptureExports
		})
		entries = mergeCaptureCellsWithFixed(entries, lexical)
	}
	return entries
}

func (q *Queries) entryRefs(ctx *db.QueryContext, key Key) flow.FunctionRefs {
	refs := key.Refs.Refs()
	if provider, ok := q.prog.(captureEntryRefsProvider); ok {
		lexical := provider.CaptureEntryRefs(key.Ref, func(dep FuncRef) flow.FunctionRefs {
			return q.solveQ.Get(ctx, NewDefaultKey(dep, nil)).Summary.CaptureFunctionRefs
		})
		refs = mergeFunctionRefsWithFixed(refs, lexical)
	}
	return refs
}

func (q *Queries) entryClosureRefs(ctx *db.QueryContext, key Key) flow.ClosureRefs {
	refs := key.Closures.Refs()
	if provider, ok := q.prog.(captureEntryClosureRefsProvider); ok {
		lexical := provider.CaptureEntryClosureRefs(key.Ref, func(dep FuncRef) flow.ClosureRefs {
			return q.solveQ.Get(ctx, NewDefaultKey(dep, nil)).Summary.CaptureClosureRefs
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
	if merger, ok := q.prog.(entryValueMerger); ok && merger != nil {
		return merger.MergeEntryValues(key.Ref, values, provided)
	}
	return (EntryValueContextMerge{
		Fixed:    values,
		Fallback: provided,
	}).Values()
}

func (q *Queries) entryFacts(key Key) flow.BoundaryFacts {
	return key.Facts.Facts()
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
		if current, ok := out.Value(entry.Symbol); ok {
			out = out.With(entry.Symbol, mergeCaptureCellValue(current, entry.Value))
			continue
		}
		out = out.With(entry.Symbol, entry.Value)
	}
	return flow.CaptureCellsDomain.Join(out, flow.CaptureCellsDomain.Bottom())
}

func mergeCaptureCellValue(fixed, fallback product.AbstractValue) product.AbstractValue {
	if fixed.IsZero() {
		return fallback
	}
	if fallback.IsZero() || product.Domain.Equal(fixed, fallback) {
		return fixed
	}
	fixedType := product.ProjectValueOrUnknown(fixed)
	fallbackType := product.ProjectValueOrUnknown(fallback)
	if typ.MorePrecise(fallbackType, fixedType) {
		return fallback
	}
	if typ.MorePrecise(fixedType, fallbackType) {
		return fixed
	}
	if fallback.Covers(fixed) {
		return fixed
	}
	if fixed.Covers(fallback) {
		return fallback
	}
	return fixed
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
func (q *Queries) solveIntra(ctx *db.QueryContext, ref FuncRef, entry flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, entryValues map[int]product.AbstractValue, entryFacts flow.BoundaryFacts, entrySymbolValues map[cfg.SymbolID]product.AbstractValue) state.FunctionState {
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
			WithEntryFacts(entryFacts).
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
	return SummaryDomain.Equal(a.Summary, b.Summary)
}

func solveResultWiden(prev, next solveResult) solveResult {
	return solveResult{
		State:   next.State,
		Summary: SummaryDomain.Widen(prev.Summary, next.Summary),
	}
}
