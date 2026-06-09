package summary

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// FuncRef is kept as a summary-package alias for existing callers. The canonical
// identity itself lives in ref so module facts, transfer, and summary can share it
// without import cycles.
type FuncRef = ref.FuncRef

// Key identifies one interprocedural summary context. References is the
// normalized callee-entry reference environment; Values are caller-projected
// parameter product facts. All map-shaped components are interned as exact
// comparable keys.
type Key struct {
	Ref        FuncRef
	References flow.ReferenceContextKey
	Values     EntryValuesKey
	Facts      EntryFactsKey
}

// NewDefaultKey constructs the key for a function entered without captured
// caller axes, preserving only caller-projected parameter values.
func NewDefaultKey(ref FuncRef, values EntryValues) Key {
	return NewKeyWithReferenceContext(
		ref,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
		values,
		flow.BoundaryFactsDomain.Top(),
	)
}

// NewKeyWithReferenceContext constructs the canonical summary key from the
// normalized callee-entry reference environment.
func NewKeyWithReferenceContext(ref FuncRef, references flow.ReferenceContext, values EntryValues, facts flow.BoundaryFacts) Key {
	return Key{
		Ref:        ref,
		References: flow.ReferenceContextKeyOf(references),
		Values:     entryValuesKeyOf(values),
		Facts:      entryFactsKeyOf(facts),
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

type captureEntryReferencesProvider interface {
	CaptureEntryReferences(ref FuncRef, captureReferencesOf func(FuncRef) flow.ReferenceContext) flow.ReferenceContext
}

// EntryPublicationDependencies exposes only caller-to-callee entry publication
// evidence. The solve query owns the actual Summary projection read; providers
// must not inspect unrelated Summary axes or introduce driver-owned precision.
type EntryPublicationDependencies interface {
	CallEntryPublication(dep FuncRef, callee FuncRef) CallEntryPublication
	PrototypeSelf(dep FuncRef) flow.PrototypeSelf
}

type entryValueProvider interface {
	EntryValues(ref FuncRef, deps EntryPublicationDependencies) map[int]product.AbstractValue
}

type entryFactProvider interface {
	EntryFacts(ref FuncRef, deps EntryPublicationDependencies) flow.BoundaryFacts
}

type entryValueMerger interface {
	MergeEntryValues(ref FuncRef, fixed, aggregate EntryValues) EntryValues
}

type callEntryPublicationProjector interface {
	ProjectCallEntryPublication(ref FuncRef, fs state.FunctionState) CallEntryPublications
}

type solveContextRunner interface {
	WithSolveContext(ctx *db.QueryContext, solve func() state.FunctionState) state.FunctionState
}

type entrySymbolValueProvider interface {
	EntrySymbolValues(ref FuncRef) map[cfg.SymbolID]product.AbstractValue
}

type returnPostconditionProvider interface {
	LocalReturnPostconditions(ref FuncRef) paramevidence.ReturnPostconditions
	DelegatedReturnPostconditionCalls(ref FuncRef) []paramevidence.DelegatedCall
	ResolveDelegatedCallee(ref FuncRef, call *ast.FuncCallExpr) (FuncRef, bool)
}

// Reader is the summary-owned observation boundary for a caller that may be
// inside the recursive summary solve or after convergence. During a solve it
// reads the live query cell under the requested entry context, recording the real
// dependency in the db cycle. After convergence it reads the immutable summary
// snapshot. Callers should not duplicate this live-vs-snapshot choice.
type Reader struct {
	queries  *Queries
	ctx      *db.QueryContext
	snapshot CanonicalSummarySnapshot
	stats    *Stats
}

// NewReader constructs a summary observer for live query reads plus a converged
// snapshot. A nil query/context makes the reader snapshot-only.
func NewReader(queries *Queries, ctx *db.QueryContext, converged map[FuncRef]Summary) Reader {
	return Reader{queries: queries, ctx: ctx, snapshot: BorrowCanonicalSummarySnapshot(converged, nil)}
}

// NewReaderWithStats constructs a summary reader that records live read and
// snapshot activity in stats. Stats are observational only and are never read to
// decide a summary result.
func NewReaderWithStats(queries *Queries, ctx *db.QueryContext, converged map[FuncRef]Summary, stats *Stats) Reader {
	return Reader{queries: queries, ctx: ctx, snapshot: BorrowCanonicalSummarySnapshot(converged, nil), stats: stats}
}

// NewSnapshotReader constructs a post-solve reader over a canonical summary
// snapshot. It never computes or records new Summary keys.
func NewSnapshotReader(snapshot CanonicalSummarySnapshot) Reader {
	return Reader{snapshot: snapshot}
}

// NewSnapshotReaderWithStats constructs a post-solve reader that records exact
// key hit/miss activity without changing snapshot semantics.
func NewSnapshotReaderWithStats(snapshot CanonicalSummarySnapshot, stats *Stats) Reader {
	return Reader{snapshot: snapshot, stats: stats}
}

// CanonicalSummarySnapshot is the immutable post-solve view of Summary facts
// already produced by the canonical query. By-key entries are exact Summary keys
// the engine demanded during the real solve; by-ref entries are the aggregate
// compatibility/export summaries.
type CanonicalSummarySnapshot struct {
	byRef map[FuncRef]Summary
	byKey map[Key]Summary
}

// BorrowCanonicalSummarySnapshot wraps existing maps without copying. It is for
// live readers and driver-owned post-solve storage where the driver controls
// mutation order.
func BorrowCanonicalSummarySnapshot(byRef map[FuncRef]Summary, byKey map[Key]Summary) CanonicalSummarySnapshot {
	return CanonicalSummarySnapshot{byRef: byRef, byKey: byKey}
}

// NewCanonicalSummarySnapshot copies maps into an immutable diagnostic/export
// snapshot.
func NewCanonicalSummarySnapshot(byRef map[FuncRef]Summary, byKey map[Key]Summary) CanonicalSummarySnapshot {
	return CanonicalSummarySnapshot{
		byRef: cloneSummaryByRef(byRef),
		byKey: cloneSummaryByKey(byKey),
	}
}

func (s CanonicalSummarySnapshot) ExactSummaryForKey(key Key) (Summary, bool) {
	sum, ok := s.byKey[key]
	return sum, ok
}

func (s CanonicalSummarySnapshot) SummaryForRef(ref FuncRef) (Summary, bool) {
	sum, ok := s.byRef[ref]
	return sum, ok
}

func (s CanonicalSummarySnapshot) HasExactKey(key Key) bool {
	if _, ok := s.byKey[key]; ok {
		return true
	}
	return false
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
	prog  Program
	stats *Stats

	// solveQ evaluates the recursive caller-facing Summary cell. Its compute
	// performs the local point/demand solve needed to project that summary.
	// Diagnostic Intra reads are exact observers over the converged Summary state,
	// not a second memoized fixed point.
	solveQ *db.Query[Key, solveResult]

	// ReturnPostconditionQ is the portable caller-visible normal-return proof
	// cell. It is the single interprocedural language for callee-proven argument
	// refinements after normal return.
	ReturnPostconditionQ *db.Query[FuncRef, paramevidence.ReturnPostconditions]

	// demanded records Summary keys the canonical query was asked to solve. A
	// post-solve snapshot may expose these exact keys to diagnostics, but
	// diagnostics do not add to the set and do not publish summaries.
	demanded map[Key]struct{}
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
	return NewWithStats(prog, nil)
}

// NewWithStats builds the canonical query product and records observability
// counters in stats. The stats object is write-only from semantic code.
func NewWithStats(prog Program, stats *Stats) *Queries {
	q := &Queries{prog: prog, stats: stats}

	q.solveQ = db.NewQueryWithSeedAndWiden(
		"CanonicalSummarySolve",
		q.computeSolve,
		func(a, b solveResult) bool {
			return SummaryDomain.Equal(a.Summary, b.Summary)
		},
		func(*db.QueryContext, Key) solveResult {
			return solveResult{
				State:   state.FunctionStateDomain.Bottom(),
				Summary: SummaryDomain.Bottom(),
			}
		},
		solveResultWiden,
	)

	q.ReturnPostconditionQ = db.NewQueryWithSeedAndWiden(
		"CanonicalReturnPostconditions",
		q.computeReturnPostconditions,
		func(a, b paramevidence.ReturnPostconditions) bool {
			return paramevidence.ReturnPostconditionsDomain.Equal(a, b)
		},
		func(*db.QueryContext, FuncRef) paramevidence.ReturnPostconditions {
			return paramevidence.ReturnPostconditionsDomain.Bottom()
		},
		func(prev, next paramevidence.ReturnPostconditions) paramevidence.ReturnPostconditions {
			return paramevidence.ReturnPostconditionsDomain.Widen(prev, next)
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
	if q != nil && q.stats != nil {
		q.stats.RecordObserveIntraWithKeyCall()
	}
	return q.solveIntra(ctx, key.Ref, q.entryReferences(ctx, key), q.entryValues(ctx, key), q.entryFacts(ctx, key), q.entrySymbolValues(key.Ref))
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
	if q != nil && q.stats != nil {
		q.stats.RecordSummarizeWithKeyCall()
	}
	return q.canonicalSummary(ctx, key)
}

// Summarize returns ref's caller-visible summary through this reader.
func (r Reader) Summarize(ref FuncRef) Summary {
	if r.queries != nil && r.ctx != nil {
		return r.queries.Summarize(r.ctx, ref)
	}
	if sum, ok := r.snapshot.SummaryForRef(ref); ok {
		return sum
	}
	return SummaryDomain.Bottom()
}

// SummarizeWithKey reads a summary through an already normalized entry key.
func (r Reader) SummarizeWithKey(key Key) Summary {
	if sum, ok := r.ExactSummaryForKey(key); ok {
		return sum
	}
	return SummaryDomain.Bottom()
}

// ExactSummaryForKey reads exactly the requested entry-context Summary. Live
// readers demand that key in the recursive query; snapshot readers return only
// keys the canonical solve already demanded.
func (r Reader) ExactSummaryForKey(key Key) (Summary, bool) {
	if r.queries != nil && r.ctx != nil {
		return r.queries.SummarizeWithKey(r.ctx, key), true
	}
	sum, ok := r.snapshot.ExactSummaryForKey(key)
	if r.stats != nil {
		r.stats.RecordSnapshotExactKeyRead(ok)
	}
	return sum, ok
}

// CanonicalSummarySnapshot returns a keyed snapshot for canonical Summary cells
// already demanded by the engine. It intentionally does not discover additional
// keys; callers may use it as a read-only post-solve diagnostic surface.
func (q *Queries) CanonicalSummarySnapshot(ctx *db.QueryContext, byRef map[FuncRef]Summary) CanonicalSummarySnapshot {
	if q == nil || ctx == nil || len(q.demanded) == 0 {
		return NewCanonicalSummarySnapshot(byRef, nil)
	}
	out := make(map[Key]Summary, len(q.demanded))
	for {
		var pending []Key
		for key := range q.demanded {
			if _, ok := out[key]; !ok {
				pending = append(pending, key)
			}
		}
		if len(pending) == 0 {
			break
		}
		for _, key := range pending {
			out[key] = q.solveQ.Get(ctx, key).Summary
		}
	}
	return NewCanonicalSummarySnapshot(byRef, out)
}

func (q *Queries) canonicalSummary(ctx *db.QueryContext, key Key) Summary {
	q.recordDemandedKey(key)
	return q.solveQ.Get(ctx, key).Summary
}

func (q *Queries) recordDemandedKey(key Key) {
	if q == nil {
		return
	}
	if q.demanded == nil {
		q.demanded = make(map[Key]struct{})
	}
	_, existed := q.demanded[key]
	q.demanded[key] = struct{}{}
	if q.stats != nil {
		q.stats.RecordSummaryKeyDemand(key, !existed)
	}
}

func cloneSummaryByRef(in map[FuncRef]Summary) map[FuncRef]Summary {
	if len(in) == 0 {
		return nil
	}
	out := make(map[FuncRef]Summary, len(in))
	for ref, sum := range in {
		out[ref] = sum
	}
	return out
}

func cloneSummaryByKey(in map[Key]Summary) map[Key]Summary {
	if len(in) == 0 {
		return nil
	}
	out := make(map[Key]Summary, len(in))
	for key, sum := range in {
		out[key] = sum
	}
	return out
}

// ReturnPostconditions reads ref's portable normal-return proof cell.
func (r Reader) ReturnPostconditions(ref FuncRef) paramevidence.ReturnPostconditions {
	if r.queries != nil && r.ctx != nil {
		return paramevidence.CloneReturnPostconditions(r.queries.ReturnPostconditions(r.ctx, ref))
	}
	if sum, ok := r.snapshot.SummaryForRef(ref); ok {
		return paramevidence.CloneReturnPostconditions(sum.Postconditions)
	}
	return paramevidence.ReturnPostconditionsDomain.Bottom()
}

func (q *Queries) intra(ctx *db.QueryContext, key Key) state.FunctionState {
	if q != nil && q.stats != nil {
		q.stats.RecordIntraObserverCall()
	}
	// First demand the recursive Summary cell so entry-value/capture dependencies
	// are at their converged approximation. The local state solve below is an exact
	// observer over those dependencies; it is intentionally not memoized as a
	// separate db fixed point.
	_ = q.canonicalSummary(ctx, key)
	return q.solveIntra(ctx, key.Ref, q.entryReferences(ctx, key), q.entryValues(ctx, key), q.entryFacts(ctx, key), q.entrySymbolValues(key.Ref))
}

// ReturnPostconditions returns ref's context-free portable normal-return proof
// cell. This is the summary-owned carrier callers/exporters should consume.
func (q *Queries) ReturnPostconditions(ctx *db.QueryContext, ref FuncRef) paramevidence.ReturnPostconditions {
	return q.ReturnPostconditionQ.Get(ctx, ref)
}

// computeSolve is solveQ's compute for one product cell.
//
// It solves ref's point/demand cells for this context and projects them to a
// Summary. Any callee summary needed by that solve is read by the transfer or by
// summary projection under the concrete entry context being modeled, so the db
// cycle records the real semantic dependency rather than an eager bottom-context
// call-graph edge.
func (q *Queries) computeSolve(ctx *db.QueryContext, key Key) solveResult {
	fs := q.solveIntra(ctx, key.Ref, q.entryReferences(ctx, key), q.entryValues(ctx, key), q.entryFacts(ctx, key), q.entrySymbolValues(key.Ref))
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
	sum.Postconditions = q.ReturnPostconditions(ctx, ref)
	if projector, ok := q.prog.(callEntryPublicationProjector); ok && projector != nil {
		sum.CallEntryPublication = projector.ProjectCallEntryPublication(ref, fs)
	}
	return sum
}

func (q *Queries) computeReturnPostconditions(ctx *db.QueryContext, ref FuncRef) paramevidence.ReturnPostconditions {
	provider, ok := q.prog.(returnPostconditionProvider)
	if !ok || provider == nil {
		return paramevidence.ReturnPostconditionsDomain.Bottom()
	}
	out := paramevidence.CloneReturnPostconditions(provider.LocalReturnPostconditions(ref))
	for _, dc := range provider.DelegatedReturnPostconditionCalls(ref) {
		calleeRef, ok := provider.ResolveDelegatedCallee(ref, dc.Call)
		if !ok {
			continue
		}
		out = paramevidence.ReturnPostconditionsDomain.Join(out, delegatedReturnPostconditions(q.ReturnPostconditions(ctx, calleeRef), dc))
	}
	return out
}

func delegatedReturnPostconditions(callee paramevidence.ReturnPostconditions, dc paramevidence.DelegatedCall) paramevidence.ReturnPostconditions {
	if !callee.HasConstraints() {
		return paramevidence.ReturnPostconditionsDomain.Bottom()
	}
	args := delegatedPlaceholderPaths(dc.ArgParams)
	out := paramevidence.ReturnPostconditionsFromCondition(callee.Substitute(args))
	for _, c := range callee.Condition().MustConstraints() {
		out = paramevidence.ReturnPostconditionsDomain.Join(out, delegatedConditionArgumentPostconditions(c, dc))
	}
	return out
}

func delegatedPlaceholderPaths(argParams []int) []constraint.Path {
	if len(argParams) == 0 {
		return nil
	}
	args := make([]constraint.Path, len(argParams))
	for calleeParam, callerParam := range argParams {
		if callerParam >= 0 {
			args[calleeParam] = constraint.ParamPath(callerParam)
		}
	}
	return args
}

func delegatedConditionArgumentPostconditions(c constraint.Constraint, dc paramevidence.DelegatedCall) paramevidence.ReturnPostconditions {
	pred, ok := constraint.SinglePathPredicate(c)
	if !ok || len(pred.Path.Segments) != 0 {
		return paramevidence.ReturnPostconditionsDomain.Bottom()
	}
	calleeArg := pred.Path.PlaceholderIndex()
	if calleeArg < 0 {
		return paramevidence.ReturnPostconditionsDomain.Bottom()
	}
	switch pred.Kind {
	case constraint.PathPredicateTruthy:
		return paramevidence.ReturnPostconditionsFromParamNarrows(delegatedArgumentEffects(dc.ArgTruthyEffects, calleeArg))
	case constraint.PathPredicateFalsy:
		return paramevidence.ReturnPostconditionsFromParamNarrows(delegatedArgumentEffects(dc.ArgFalsyEffects, calleeArg))
	default:
		return paramevidence.ReturnPostconditionsDomain.Bottom()
	}
}

func delegatedArgumentEffects(all [][]paramevidence.ParamNarrow, arg int) []paramevidence.ParamNarrow {
	if arg < 0 || arg >= len(all) || len(all[arg]) == 0 {
		return nil
	}
	return paramevidence.SortParamNarrows(all[arg])
}

func (q *Queries) entryReferences(ctx *db.QueryContext, key Key) flow.ReferenceContext {
	references := key.References.Context()
	if provider, ok := q.prog.(captureEntryReferencesProvider); ok {
		lexical := provider.CaptureEntryReferences(key.Ref, func(dep FuncRef) flow.ReferenceContext {
			sum := q.canonicalSummary(ctx, NewDefaultKey(dep, nil))
			return sum.CaptureReferences
		})
		references = flow.MergeReferenceContextWithFixed(references, lexical)
	}
	return references
}

func (q *Queries) entryValues(ctx *db.QueryContext, key Key) map[int]product.AbstractValue {
	values := key.Values.Values()
	provider, ok := q.prog.(entryValueProvider)
	if !ok || provider == nil {
		return values
	}
	provided := provider.EntryValues(key.Ref, NewReaderWithStats(q, ctx, nil, q.stats))
	if merger, ok := q.prog.(entryValueMerger); ok && merger != nil {
		return merger.MergeEntryValues(key.Ref, values, provided)
	}
	return (EntryValueContextMerge{
		Fixed:     values,
		Aggregate: provided,
	}).Values()
}

func keyUsesAggregateEntryProjection(key Key) bool {
	return len(key.Values.Values()) == 0 &&
		flow.ReferenceContextDomain.Equal(key.References.Context(), flow.ReferenceContextBottom()) &&
		flow.BoundaryFactsDomain.Equal(key.Facts.Facts(), flow.BoundaryFactsDomain.Top())
}

func (q *Queries) entryFacts(ctx *db.QueryContext, key Key) flow.BoundaryFacts {
	facts := key.Facts.Facts()
	if !keyUsesAggregateEntryProjection(key) {
		return facts
	}
	provider, ok := q.prog.(entryFactProvider)
	if !ok || provider == nil {
		return facts
	}
	return mergeEntryFacts(facts, provider.EntryFacts(key.Ref, NewReaderWithStats(q, ctx, nil, q.stats)))
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
func (q *Queries) solveIntra(ctx *db.QueryContext, ref FuncRef, references flow.ReferenceContext, entryValues map[int]product.AbstractValue, entryFacts flow.BoundaryFacts, entrySymbolValues map[cfg.SymbolID]product.AbstractValue) state.FunctionState {
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
			WithEntryReferences(references).
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

func solveResultWiden(prev, next solveResult) solveResult {
	return solveResult{
		State:   next.State,
		Summary: SummaryDomain.Widen(prev.Summary, next.Summary),
	}
}
