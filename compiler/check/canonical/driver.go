// Package canonical wires the single-fixed-point type-flow engine into the
// Checker. The Driver runs that engine over a whole module as the Checker's only
// flow.
//
// The engine itself is built as a standalone leaf in the sub-packages:
//
//   - input  assembles one function's Inputs (CFG, raw evidence, scope facts);
//   - transfer is the real per-node value/condition/numeric transfer;
//   - equation solves point and demand cells for one function/context over the
//     generic worklist;
//   - summary evaluates the interprocedural summary cells through the db query
//     cycle. This is implementation decomposition of one conceptual product
//     equation system, not a driver-owned second pass.
//
// The Driver supplies the missing module context the engine's summary.Program
// interface needs: it walks the chunk graph plus every nested function and derives
// the call graph from each function's call sites. It then drives the
// interprocedural fixed point by summarizing every module function.
//
// The Driver runs the flow over a whole module and relies on value/numeric
// widening at loop headers for termination. It computes and memoizes
// per-function summaries, then bridges converged state to the diagnostic passes.
// A function whose body uses a deferred node kind carries that node's state
// forward unchanged: sound precision loss, never unsoundness, and still
// terminating.
package canonical

import (
	"fmt"
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/facts"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/canonical/provenance"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	canonicalsig "github.com/wippyai/go-lua/compiler/check/canonical/signature"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/domain/iteration"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/scope"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/core"
	"github.com/wippyai/go-lua/compiler/check/synth/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Config supplies the canonical driver's dependencies.
type Config struct {
	// Types resolves operator result types and other type operations. It is the
	// seam for the per-node transfer's operator resolution and the call-return
	// typing a later fidelity pass adds.
	Types core.TypeOps

	// GlobalTypes is the external value namespace of predeclared globals (print,
	// pairs, require, and any module-supplied globals). NewDriver normalizes it into
	// globalenv.TypeOverlay; the binder sees only deterministic names, and canonical
	// transfer/observation do not consult this map directly.
	GlobalTypes map[string]typ.Type

	// Stdlib is the base type scope: the predeclared globals and type aliases a
	// module sees. It is the base scope parameter-annotation resolution reads
	// against.
	Stdlib *scope.State

	// Manifests is the module manifest querier the annotation resolver reads for
	// imported type names. A nil querier still resolves primitive and structural
	// annotations.
	Manifests io.ManifestQuerier

	// MaxScopeDepth limits lexical scope nesting in the canonical scope walk.
	// A value <= 0 disables the limit.
	MaxScopeDepth int

	// EmitScopeDiag emits a warning when MaxScopeDepth truncates scope precision.
	EmitScopeDiag bool

	// ComputePasses are compatibility artifact producers over the canonical
	// graph/scopes bridge. They do not participate in the canonical fixed point.
	ComputePasses []api.ComputePass
}

// Driver runs the canonical type-flow engine over a module.
type Driver struct {
	cfg Config

	// resolver resolves declared type annotations (parameter and return types) in
	// the module's base scope. It is the single seam to the annotation/alias/import
	// machinery, shared by the input builder (parameter contracts) and the
	// diagnostic bridge (declared returns and the per-function declared-type map).
	resolver *resolve.Resolver

	// refs and summaries are the result of the last Run: every module function in
	// discovery order, and its converged interprocedural Summary. They are the
	// seam to the diagnostic bridge (component 11b), which reads the converged
	// per-function state rather than re-solving.
	refs      []summary.FuncRef
	summaries map[summary.FuncRef]summary.Summary

	// states is the converged intraprocedural FunctionState per function, the
	// per-point env the diagnostic bridge reads to derive point-local value types.
	states map[summary.FuncRef]state.FunctionState

	// globalTypes is the normalized source-global value namespace admitted from
	// Config.GlobalTypes. The raw string map is external configuration only.
	globalTypes globalenv.TypeOverlay

	// moduleScope is the base type-name scope every annotation resolves against: the
	// configured Stdlib scope enriched with the module's own `type X = ...`
	// definitions. Without it a named annotation referring to a module-local type
	// (a union alias, a record alias) resolves to an unresolved typ.Ref, which
	// blocks field-on-named-type and discriminant narrowing. It is populated per Run
	// from the module's TypeDef nodes, reusing scope.EnrichWithTypeDefs
	// machinery.
	moduleScope *scope.State

	// typedefCache single-sources a module type definition's resolution: a
	// `type X = ...` is resolved once and reused by both the module-wide scope
	// (buildModuleScope) and the per-point scopes (buildPointScopes), keyed by the
	// definition's AST TypeExpr node. Without it the two scope builders each mint a
	// fresh recursive family for the same source type, so a constructor's declared
	// return (resolved against the module scope) and the exported manifest type
	// (read from a point scope) carry distinct families and an imported
	// `local s: m.X = m.new()` mismatches the same source type. It is reset per Run.
	typedefCache map[ast.TypeExpr]typ.Type

	// pointScopes is the block-aware per-CFG-point type-name scope for every graph in
	// the module hierarchy, keyed by graph ID. Each point's scope carries exactly the
	// type definitions LEXICALLY VISIBLE there: a block-local `type X` inside an
	// if/do/loop body is in scope only at the points inside that block (the scope-exit
	// node pops it), and a `type X` is in scope only at points after its definition in
	// definition order (RPO). A nested function's points see the module-level types
	// its parent declared (the parent's block-aware exit scope is the nested function's
	// base) but not a sibling block's locals. It is the single per-point scope source
	// the diagnostic passes and local-declaration annotation resolution read, computed
	// once per Run by buildHierarchyScopes. Without it the flat module scope would make
	// a block-local or forward type spuriously visible (a not-visible/used-before-def
	// miss) and a shadowed type resolve to the wrong binding.
	pointScopes map[uint64]map[cfg.Point]*scope.State

	// scopeDepthExceeded records whether BuildPointScopes hit MaxScopeDepth for each
	// graph while building pointScopes. It feeds FuncResult.DepthLimitExceeded and
	// optional checker diagnostics without creating a concrete solver carrier.
	scopeDepthExceeded map[uint64]bool

	// activeProgram and activeCtx are the in-flight Run's program and db query
	// context. The per-node transfer's call typing (callTyper) reads them while the
	// intraprocedural fixpoint solves: the callee resolution needs the module-wide
	// function signatures (activeProgram), and the call pipeline needs the query
	// context. They are set before the summary loop and cleared after it, so a
	// transfer resolves calls against the fully built program.
	activeProgram *program
	activeCtx     *db.QueryContext

	// activeQueries is the in-flight Run's interprocedural summary query. The call
	// typing reads a callee's CURRENT summary through it during the solve under the
	// actual call-entry context. Those reads are the semantic call dependencies of
	// the product equation system; static Callees metadata is not used as a
	// bottom-context substitute. It is set before the summary loop and cleared
	// after it.
	activeQueries *summary.Queries

	// snapshotSummaryReads is set only while diagnostic exact observation is
	// running. The local observer still uses exact entry axes, but nested call
	// summaries are read from d.summaries instead of creating new exact Summary
	// cells after the fixed point has converged.
	snapshotSummaryReads  bool
	diagnosticOverlayRead func(summary.Key)

	// diagnosticContexts records summary contexts observed by the summary-owned
	// DiagnosticContextFrontier. The bridge uses these contexts, not an artificial
	// bottom/default context, when building per-function diagnostic state.
	diagnosticContexts map[summary.FuncRef][]summary.Key

	// diagnosticStates caches the exact observer states solved while discovering
	// diagnosticContexts so bridge materialization does not replay identical local
	// equation solves for the same summary.Key.
	diagnosticStates map[summary.Key]state.FunctionState

	// diagnosticSummaries is the exact-key summary overlay projected from
	// diagnostic observer states. Snapshot summary reads consult it before the
	// coarser per-function converged summary map, so exact diagnostic solves can
	// consume exact callee postconditions without creating new recursive summary
	// query cells after the main fixed point.
	diagnosticSummaries map[summary.Key]summary.Summary
}

// NewDriver constructs a canonical driver with the given configuration.
func NewDriver(cfg Config) *Driver {
	return &Driver{
		cfg:         cfg,
		globalTypes: globalenv.TypeOverlayFromMap(cfg.GlobalTypes),
		resolver:    resolve.New(resolve.Config{Manifests: cfg.Manifests}),
	}
}

// FuncRefs returns the module functions analyzed by the last Run, in discovery
// order (root first, then nested functions in CFG point order).
func (d *Driver) FuncRefs() []summary.FuncRef {
	return append([]summary.FuncRef(nil), d.refs...)
}

// SummaryFor returns the converged interprocedural Summary computed for ref by
// the last Run, and whether ref was analyzed.
func (d *Driver) SummaryFor(ref summary.FuncRef) (summary.Summary, bool) {
	s, ok := d.summaries[ref]
	return s, ok
}

func (d *Driver) summaryReader() summary.Reader {
	if d == nil {
		return summary.NewReader(nil, nil, nil)
	}
	if d.snapshotSummaryReads {
		return summary.NewReaderWithOverlayReads(nil, nil, d.summaries, d.diagnosticSummaries, d.diagnosticOverlayRead)
	}
	return summary.NewReader(d.activeQueries, d.activeCtx, d.summaries)
}

func (d *Driver) withSnapshotSummaryReads(run func()) {
	if d == nil || run == nil {
		return
	}
	prev := d.snapshotSummaryReads
	d.snapshotSummaryReads = true
	defer func() { d.snapshotSummaryReads = prev }()
	run()
}

// Run analyzes a module chunk with the flow engine.
//
// It first builds the module setup so the session sees the same graph
// hierarchy (root chunk function, bound globals, registered CFG hierarchy), then
// walks the hierarchy to enumerate every module function, builds the
// summary.Program over them, and drives the interprocedural summary fixed point
// by summarizing each function. The fixpoint converges by the engine's lattice
// widening; on a module where a join-only iteration does not terminate, this returns.
func (d *Driver) Run(sess api.AnalysisSession, chunk []ast.Stmt) {
	if sess == nil {
		return
	}

	root := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}
	sess.SetRootFuncNode(root)

	globals := d.globalTypes.Names()
	moduleBindings := bind.Bind(root, globals)
	if store := sess.StoreHandle(); store != nil {
		store.SetModuleBindings(moduleBindings)
	}

	rootGraph := sess.GetOrBuildCFG(root)
	if rootGraph == nil {
		return
	}
	sess.RegisterGraphHierarchy(rootGraph)

	// Rebuild the annotation resolver with this module's require() aliases: a
	// qualified annotation `m.T` (m bound by `local m = require("mod")`) resolves
	// the alias symbol `m` to its module path so `m.T` looks up the imported
	// manifest type rather than leaving an unresolved Ref. Without it a cross-module
	// type annotation never resolves to its recursive-family identity and an assign
	// of the imported constructor's result mismatches the same imported type.
	moduleAliases := topology.DiscoverModuleAliases(topology.ModuleAliasDiscoveryInput{
		Root:         rootGraph,
		GraphForFunc: sess.GetOrBuildCFG,
		AliasesForGraph: func(g *cfg.Graph) map[cfg.SymbolID]string {
			evidence := sess.EvidenceForGraph(g)
			return modules.AliasesFromAssignments(evidence.Assignments, g)
		},
	})
	d.resolver = resolve.New(resolve.Config{
		Manifests:      d.cfg.Manifests,
		ModuleBindings: moduleBindings,
		ModuleAliases:  moduleAliases,
	})

	// Single-source this Run's module type-definition resolution: the module scope
	// and the per-point scopes share one resolved family per `type X` so a
	// constructor's declared return and the exported manifest type are the same
	// recursive family.
	d.typedefCache = make(map[ast.TypeExpr]typ.Type)

	// Enrich the base scope with the module's own type definitions before any
	// annotation resolves: a named annotation referring to a module-local `type X`
	// must resolve to the defined type, not an unresolved typ.Ref. This feeds every
	// resolveType call below (parameter contracts, declared returns, function
	// signatures) through the same scope used for type definitions.
	d.moduleScope = d.buildModuleScope(sess, rootGraph)

	// Compute the block-aware per-point type-name scope for every graph in the
	// hierarchy: a block-local or forward `type X` is in scope only where Lua's
	// lexical rules make it visible, so a local-declaration annotation and the
	// diagnostic passes resolve a type name against the binding actually visible at
	// the point rather than the flat module scope.
	d.scopeDepthExceeded = make(map[uint64]bool)
	d.pointScopes = d.buildHierarchyScopes(sess, rootGraph)

	prog := d.buildProgram(sess, rootGraph, topology.ResolveModuleAliases(moduleAliases, d.cfg.Manifests))
	d.registerStoreGraphParents(sess, prog)
	queries := summary.New(prog)

	// Drive the canonical product equation system by demanding every module
	// summary. The summary solve query evaluates the per-context point/demand
	// cells and the Summary projection in one dependency cycle; a mutually
	// recursive or self-recursive cluster converges from the bottom seed via
	// Summary widening. Per-point diagnostic state is observed afterward by an
	// exact local solve over those converged dependencies.
	d.refs = prog.refs
	// The per-node transfer's call typing resolves callees against the fully built
	// program and runs the call pipeline against this run's query context. Expose
	// them for the solve below, then clear them when the run completes.
	d.activeProgram = prog
	d.activeCtx = sess.Context()
	d.activeQueries = queries
	d.diagnosticContexts = make(map[summary.FuncRef][]summary.Key)
	d.diagnosticStates = make(map[summary.Key]state.FunctionState)
	d.diagnosticSummaries = make(map[summary.Key]summary.Summary)
	defer func() { d.activeProgram = nil; d.activeCtx = nil; d.activeQueries = nil }()
	d.solvePass(sess, prog, queries)
	d.withSnapshotSummaryReads(func() {
		d.publishFunctionFacts(sess, prog)
		d.commitPublishedFunctionFacts(sess)
		d.bridgeResults(sess, prog, queries)
	})
}

// solvePass demands every module summary from the canonical product equation
// system, filling d.summaries / d.states from the converged query evaluation.
// The summary solve query is the implementation decomposition for the point,
// demand, entry, and summary cells; it is not a post-solve precision pass.
func (d *Driver) solvePass(sess api.AnalysisSession, prog *program, queries *summary.Queries) {
	d.summaries = make(map[summary.FuncRef]summary.Summary, len(prog.refs))
	d.states = make(map[summary.FuncRef]state.FunctionState, len(prog.refs))
	for _, ref := range prog.refs {
		d.summaries[ref] = queries.Summarize(sess.Context(), ref)
	}
	rootRef, _ := prog.refByFunc(sess.RootFuncNode())
	d.withSnapshotSummaryReads(func() {
		observed := summary.SelectPostWidenObservationRefs(summary.PostWidenObservationInput{
			Refs: prog.refs,
			Root: rootRef,
			Summary: func(ref summary.FuncRef) summary.Summary {
				return d.summaries[ref]
			},
			Graph: func(ref summary.FuncRef) *cfg.Graph {
				return prog.Graph(ref)
			},
			IsMethod: func(ref summary.FuncRef) bool {
				return prog.methodDef(ref) != nil
			},
			Nested: func(ref summary.FuncRef) []summary.FuncRef {
				return prog.funcTopology.NestedRefs(ref)
			},
			Parent: func(ref summary.FuncRef) (summary.FuncRef, bool) {
				return prog.funcTopology.ParentRef(ref)
			},
		})
		for _, ref := range observed {
			d.summaries[ref] = queries.ObservedSummary(sess.Context(), ref)
		}
	})
	if rootRef, ok := prog.refByFunc(sess.RootFuncNode()); ok {
		d.withSnapshotSummaryReads(func() {
			d.buildDiagnosticContexts(sess, prog, queries, rootRef)
		})
	}
	for _, ref := range prog.refs {
		// The converged per-point state is an exact observer over the already-solved
		// Summary dependencies, not a second interprocedural fixed point. Diagnostics
		// observe the same aggregate entry-value context that Summary.EntryValues uses,
		// so local helpers are checked under their solved caller-provided entry facts
		// instead of an artificial bottom/default call context.
		d.states[ref] = d.diagnosticState(sess, prog, queries, ref)
	}
}

func (d *Driver) registerStoreGraphParents(sess api.AnalysisSession, prog *program) {
	if d == nil || sess == nil || prog == nil {
		return
	}
	store := sess.StoreHandle()
	if store == nil {
		return
	}
	for _, ref := range prog.refs {
		g := prog.Graph(ref)
		if g == nil {
			continue
		}
		parent := d.returnScope(g)
		if parent == nil {
			parent = scope.New()
		}
		hash := parent.Hash()
		if hash == 0 {
			continue
		}
		store.SetParentScope(hash, parent)
		store.SetGraphParentHash(g.ID(), hash)
	}
}

func (d *Driver) publishFunctionFacts(sess api.AnalysisSession, prog *program) {
	if d == nil || sess == nil || prog == nil {
		return
	}
	store := sess.StoreHandle()
	if store == nil {
		return
	}
	reader := d.summaryReader()
	for _, ref := range prog.refs {
		symbols := prog.symbolsForRef(ref)
		if len(symbols) == 0 {
			continue
		}
		sum, ok := d.summaries[ref]
		if !ok {
			sum = reader.Summarize(ref)
		}
		returns := summary.ReturnTypes(sum)
		params := contractTypeVector(sum.Params, prog.NumParams(ref))
		publicParams := prog.publicPredicateParamVector(ref, params)
		sig := d.signatureForRef(prog, ref)
		refinement := paramevidence.FunctionRefinementFromParamNarrows(reader.ParamNarrows(ref), prog.facts.HasNoReturn(ref))
		for _, sym := range symbols {
			key, ok := store.ParentGraphKeyForSymbol(sym)
			if !ok {
				continue
			}
			builder := functionfact.NewBuilder()
			builder.AddSignature(sym, sig)
			builder.AddSummary(sym, returns)
			builder.AddNarrow(sym, returns)
			builder.AddBodyParams(sym, params)
			builder.AddPublicParams(sym, publicParams)
			builder.AddRefinement(sym, refinement)
			if facts := builder.Build(); len(facts) > 0 {
				store.MergeInterprocFactsNext(key, interprocdomain.FunctionFactsDelta(facts))
			}
		}
	}
}

func (d *Driver) commitPublishedFunctionFacts(sess api.AnalysisSession) {
	if d == nil || sess == nil {
		return
	}
	store := sess.StoreHandle()
	if store == nil {
		return
	}
	store.FixpointSwap()
}

func contractTypeVector(contracts paramevidence.Contracts, minLen int) []typ.Type {
	typesBySlot := paramevidence.ContractTypes(contracts)
	if len(typesBySlot) == 0 {
		return nil
	}
	n := minLen
	for slot := range typesBySlot {
		if slot >= 0 && slot+1 > n {
			n = slot + 1
		}
	}
	if n <= 0 {
		return nil
	}
	out := make([]typ.Type, n)
	any := false
	for slot, t := range typesBySlot {
		if slot < 0 || slot >= n || t == nil || typ.IsAbsentOrUnknown(t) {
			continue
		}
		out[slot] = t
		any = true
	}
	if !any {
		return nil
	}
	return out
}

func (p *program) publicPredicateContracts(ref summary.FuncRef, contracts paramevidence.Contracts) paramevidence.Contracts {
	slots := p.predicateInputSlots(ref)
	if len(slots) == 0 || len(contracts) == 0 {
		return contracts
	}
	out := make(paramevidence.Contracts, len(contracts))
	for slot, demand := range contracts {
		out[slot] = demand
	}
	for _, slot := range slots {
		delete(out, slot)
	}
	return out
}

func (p *program) publicPredicateParamVector(ref summary.FuncRef, params []typ.Type) []typ.Type {
	slots := p.predicateInputSlots(ref)
	if len(slots) == 0 || len(params) == 0 {
		return params
	}
	out := append([]typ.Type(nil), params...)
	for _, slot := range slots {
		if slot >= 0 && slot < len(out) {
			out[slot] = nil
		}
	}
	return out
}

func (p *program) predicateInputSlots(ref summary.FuncRef) []int {
	if p == nil {
		return nil
	}
	preds := p.facts.PredicateFacts()
	if len(preds) == 0 {
		return nil
	}
	var out []int
	g := p.Graph(ref)
	fn := p.funcExpr(ref)
	for _, pred := range preds {
		predRef, ok := p.refBySymbol(pred.FuncSym)
		if !ok || predRef != ref {
			continue
		}
		_, slot, ok := paramevidence.ParamSlotForSourceParam(g, fn, pred.ParamIndex)
		if !ok || slot < 0 {
			continue
		}
		out = append(out, slot)
	}
	return out
}

func (d *Driver) diagnosticState(sess api.AnalysisSession, prog *program, queries *summary.Queries, ref summary.FuncRef) state.FunctionState {
	if sess == nil || prog == nil || queries == nil {
		return state.FunctionStateDomain.Bottom()
	}
	contexts := d.diagnosticContexts[ref]
	if len(contexts) == 0 {
		reader := summary.NewReader(nil, nil, d.summaries)
		values := prog.EntryValues(ref, reader)
		if len(values) != 0 {
			return queries.IntraWithEntryValues(sess.Context(), ref, flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), values)
		}
		return queries.Intra(sess.Context(), ref)
	}
	out := state.FunctionStateDomain.Bottom()
	for _, key := range contexts {
		fs, ok := d.diagnosticStates[key]
		if !ok {
			fs = queries.IntraWithEntryContextFacts(sess.Context(), ref, key.Entry.Cells(), key.Refs.Refs(), key.Closures.Refs(), key.Values.Values(), key.Facts.Facts())
			if d.diagnosticStates == nil {
				d.diagnosticStates = make(map[summary.Key]state.FunctionState)
			}
			d.diagnosticStates[key] = fs
		}
		out = joinDiagnosticFunctionState(out, fs)
	}
	return out
}

func (d *Driver) buildDiagnosticContexts(sess api.AnalysisSession, prog *program, queries *summary.Queries, root summary.FuncRef) {
	if d == nil || sess == nil || prog == nil || queries == nil {
		return
	}
	result := summary.DiagnosticContextFrontier{
		Root:           root,
		Refs:           prog.refs,
		SummaryOverlay: d.diagnosticSummaries,
		ValidKey: func(key summary.Key) bool {
			return d.validDiagnosticContext(prog, key)
		},
		DefaultKey: func(ref summary.FuncRef) summary.Key {
			return d.defaultDiagnosticKey(prog, ref)
		},
		SolveWithDependencies: func(key summary.Key) (state.FunctionState, []summary.Key) {
			return d.observeDiagnosticIntraWithOverlayDeps(sess, queries, key)
		},
		ProjectSummary: func(key summary.Key, fs state.FunctionState) summary.Summary {
			return queries.ProjectStateSummary(sess.Context(), key.Ref, fs)
		},
		ProjectCalls: func(ref summary.FuncRef, fs state.FunctionState) []summary.Key {
			return d.projectDiagnosticCallContexts(prog, ref, fs)
		},
		ProjectClosures: func(ref summary.FuncRef, fs state.FunctionState) []summary.Key {
			return d.projectDiagnosticClosureContexts(prog, ref, fs)
		},
	}.Build()
	d.diagnosticContexts = result.Contexts
	d.diagnosticStates = result.States
	d.diagnosticSummaries = result.Summaries
}

func (d *Driver) observeDiagnosticIntraWithOverlayDeps(sess api.AnalysisSession, queries *summary.Queries, key summary.Key) (state.FunctionState, []summary.Key) {
	if d == nil {
		return state.FunctionStateDomain.Bottom(), nil
	}
	reads := make(map[summary.Key]struct{})
	prev := d.diagnosticOverlayRead
	d.diagnosticOverlayRead = func(read summary.Key) {
		reads[read] = struct{}{}
	}
	defer func() {
		d.diagnosticOverlayRead = prev
	}()
	fs := d.observeDiagnosticIntra(sess, queries, key)
	if len(reads) == 0 {
		return fs, nil
	}
	deps := make([]summary.Key, 0, len(reads))
	for dep := range reads {
		deps = append(deps, dep)
	}
	return fs, deps
}

func (d *Driver) observeDiagnosticIntra(sess api.AnalysisSession, queries *summary.Queries, key summary.Key) state.FunctionState {
	if d == nil || sess == nil || queries == nil {
		return state.FunctionStateDomain.Bottom()
	}
	return queries.ObserveIntraWithEntryContextFacts(sess.Context(), key.Ref, key.Entry.Cells(), key.Refs.Refs(), key.Closures.Refs(), key.Values.Values(), key.Facts.Facts())
}

func (d *Driver) validDiagnosticContext(prog *program, key summary.Key) bool {
	return d != nil && prog != nil && prog.Graph(key.Ref) != nil
}

func (d *Driver) defaultDiagnosticKey(prog *program, ref summary.FuncRef) summary.Key {
	if d == nil || prog == nil {
		return summary.NewKeyWithEntryContext(ref, flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom(), nil)
	}
	reader := summary.NewReader(nil, nil, d.summaries)
	values := prog.EntryValues(ref, reader)
	return summary.NewKeyWithEntryContext(ref, flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom(), values)
}

func (d *Driver) projectDiagnosticCallContexts(prog *program, ref summary.FuncRef, fs state.FunctionState) []summary.Key {
	if d == nil || prog == nil {
		return nil
	}
	return prog.ProjectCallEntryContextKeys(ref, fs)
}

func (d *Driver) projectDiagnosticClosureContexts(prog *program, ref summary.FuncRef, fs state.FunctionState) []summary.Key {
	if d == nil || prog == nil {
		return nil
	}
	return summary.ClosureEntryContextProjection{
		State: fs,
		ReferencePaths: func(callee summary.FuncRef) flow.ReferencePathProjection {
			return prog.referenceProjection(callee)
		},
	}.ProjectKeys()
}

func joinDiagnosticFunctionState(a, b state.FunctionState) state.FunctionState {
	out := state.FunctionStateDomain.Join(a, b)
	out.InPoints = joinDiagnosticInPoints(a.InPoints, b.InPoints)
	return out
}

func joinDiagnosticInPoints(a, b map[cfg.Point]flow.PointState) map[cfg.Point]flow.PointState {
	if len(a) == 0 {
		return cloneInPoints(b)
	}
	if len(b) == 0 {
		return cloneInPoints(a)
	}
	out := cloneInPoints(a)
	for p, ps := range b {
		out[p] = flow.PointStateDomain.Join(out[p], ps)
	}
	return out
}

func cloneInPoints(in map[cfg.Point]flow.PointState) map[cfg.Point]flow.PointState {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]flow.PointState, len(in))
	for p, ps := range in {
		out[p] = ps
	}
	return out
}

// bridgeResults populates the session's per-function results from converged flow
// state so the existing diagnostic passes (Checker.runPasses) run on solved
// facts.
//
// What it bridges from solved state vs. defaults is documented on the
// field-population helper (buildFuncResult). The defaulted fields are recorded
// transfer/bridge gaps, not fabricated facts. The bridge is scoped, so the diff a
// caller measures comes from transfer-fidelity worklist items: a diagnostic pass
// can no-op when the bridge defaults a fact it reads, or it can flag an unknown
// when the observation surface has not yet received the per-point value fact it
// needs.
func (d *Driver) bridgeResults(sess api.AnalysisSession, prog *program, queries *summary.Queries) {
	results := sess.ResultsMap()
	if results == nil {
		return
	}
	for _, ref := range d.refs {
		fn := prog.funcExpr(ref)
		if fn == nil {
			continue
		}
		result := d.buildFuncResult(sess, prog, queries, ref)
		results[fn] = result
		if fn == sess.RootFuncNode() {
			sess.SetRootResultValue(result)
		}
		d.emitScopeDepthDiagnostic(sess, fn, result)
	}
}

// buildFuncResult assembles one function's api.FuncResult from converged flow
// state in the shape the diagnostic passes consume.
//
// BRIDGED from the flow engine (sound inputs and computed facts):
//   - Graph: the function's CFG, the same graph the solve ranged over.
//   - Evidence: the raw graph-event trace (assignments, calls, returns, branches,
//     identifier uses). It is a sound INPUT the canonical input builder already
//     consumes, not a solved fact, so surfacing it to the passes fabricates
//     nothing. It backs the syntactic checks (control flow, identifier presence).
//   - GlobalTypes: the immutable value namespace of predeclared globals.
//
// SOLVER-SHAPED CARRIERS STILL NOT FABRICATED:
//   - Solved flow is exposed through FlowProjection (api.FlowOps), not by
//     constructing a concrete solver result it did not actually compute.
//   - FnRefinement is a summary projection, not a Solve/Narrow output.
//   - NarrowSynth is an observation facade over the same facts/resolver used by
//     diagnostics; it is not a second type checker.
//
// The canonical-computed value facts are projected only into carriers with the
// same semantics: per-point env facts seed the observation surface, return tuples
// become ReturnRelations/ReturnTypes, contextual call signatures become
// CallExpectedArgs, and solved body-demand parameter contracts become call-edge
// CallContracts. They are not forced into NarrowSynth, callable signatures, or
// FlowEvidence; those would fabricate solver-shaped structures or
// mutate immutable extraction evidence.
func (d *Driver) buildFuncResult(sess api.AnalysisSession, prog *program, queries *summary.Queries, ref summary.FuncRef) *api.FuncResult {
	g := prog.Graph(ref)

	var evidence api.FlowEvidence
	if store := sess.StoreHandle(); store != nil && g != nil {
		evidence = store.EvidenceForGraph(g)
	}
	callContracts := d.projectCallContracts(prog, ref, evidence)

	// Observation surface: project the converged FunctionState into the per-point /
	// per-symbol facts and the declared-type inputs the diagnostic passes query
	// through observation.Projector. The Projector reads those surfaces directly.
	//
	// The immutable annotation/global facts are computed once while assembling the
	// canonical input carrier. Clone them here before adding bridge-only function
	// binding facts so diagnostics cannot mutate the solver's input facts.
	facts := cloneFunctionFacts(prog.functionFacts[ref])
	funcSigs := prog.facts.FunctionBindingTypes(func(ref canonref.FuncRef) typ.Type {
		return d.signatureForRef(prog, ref)
	})
	// A named function (a module-level definition or a local-function binding) is a
	// defined identifier wherever it is referenced, but it is not a source type
	// annotation. Keep its signature in the binding overlay so DeclaredAt remains
	// annotation/global-only and EffectiveTypeAt can still type recursive or forward
	// function references.
	recordFunctionBindingTypes(&facts, funcSigs, g)
	recordCallbackEnvBindingTypes(&facts, prog.facts.CallbackEnv(ref))
	flowProjection := d.newCanonicalFacts(g, d.states[ref], facts, prog, queries, sess.Context(), evidence)
	literalSignatures := canonicalsig.LiteralSignatures(canonicalsig.LiteralInput{
		Graph:           g,
		Base:            d.baseScope(),
		ResolveType:     d.resolveType,
		InferredReturns: d.inferredReturnsForFunction,
		MethodFor: func(fn *ast.FunctionExpr) *cfg.FuncDefInfo {
			if ref, ok := prog.refByFunc(fn); ok {
				return prog.methodDef(ref)
			}
			return nil
		},
	})
	pointScopes := d.buildPointScopes(g)
	sourceSignature := d.declaredSignatureForRef(prog, ref)
	result := &api.FuncResult{
		Graph:              g,
		Evidence:           evidence,
		GlobalTypes:        d.globalTypes.ToMap(),
		GlobalTypeBindings: d.globalTypes,
		// The return check resolves the function's declared return annotation against
		// BaseScope; a generic function returning `T` must resolve `T` to its bounded
		// type parameter (the same scope its parameter annotations resolved in), or the
		// return type re-resolves to an unresolved typ.Ref and a sound `return x`
		// (x: T) mismatches it. A non-generic function's scope is the module base scope.
		BaseScope:                d.returnScope(g),
		Scopes:                   pointScopes,
		FlowInputs:               buildObservationInputs(g, facts),
		Facts:                    flowProjection,
		FlowProjection:           flowProjection,
		ReturnRelations:          d.summaryReader().ReturnRelations(ref),
		LiteralSignatures:        literalSignatures,
		LiteralSignatureProvider: api.LiteralSignatureLookupFromMap(literalSignatures),
		SourceSignature:          sourceSignature,
		PublicSeedSignature:      sourceSignature,
		TypeOps:                  d.cfg.Types,
		QueryContext: func() *db.QueryContext {
			if sess == nil {
				return nil
			}
			return sess.Context()
		}(),
		Extras:             d.runComputePasses(g, pointScopes),
		DepthLimitExceeded: d.scopeDepthExceededFor(g),
	}
	obs := observation.FromFuncResult(result, nil).WithProofValues()
	result.CallExpectedArgs = d.projectSolvedCallExpectedArgs(prog, ref, evidence)
	result.CallContracts = callContracts
	result.NarrowSynth = &returnSynth{
		driver: d,
		obs:    obs.TypeOf,
		ctx:    result.QueryContext,
	}
	result.FnRefinement = paramevidence.FunctionRefinementFromParamNarrows(d.summaryReader().ParamNarrows(ref), prog.facts.HasNoReturn(ref))
	return result
}

func (d *Driver) projectSolvedCallExpectedArgs(prog *program, ref summary.FuncRef, evidence api.FlowEvidence) []api.CallExpectedArgEvidence {
	if d == nil || prog == nil || len(evidence.Calls) == 0 {
		return nil
	}
	g := prog.Graph(ref)
	tr, _ := prog.transfers[ref].(*transfer.Transfer)
	fs, ok := d.states[ref]
	if g == nil || tr == nil || !ok {
		return nil
	}
	var out []api.CallExpectedArgEvidence
	for i, ev := range evidence.Calls {
		info := ev.Info
		if info == nil || info.Call == nil || len(info.Call.Args) == 0 {
			continue
		}
		ps, ok := callEventPointState(fs, ev.Point)
		if !ok {
			continue
		}
		args := make([]typ.Type, len(info.Call.Args))
		any := false
		for argIdx := range info.Call.Args {
			expected := prog.expectedCallArgType(g, tr, ev.Point, info, &ps, argIdx)
			if expected == nil || typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
				continue
			}
			args[argIdx] = expected
			any = true
		}
		if !any {
			continue
		}
		if out == nil {
			out = make([]api.CallExpectedArgEvidence, len(evidence.Calls))
		}
		out[i] = api.NewCallExpectedArgEvidence(args)
	}
	return out
}

func callEventPointState(fs state.FunctionState, point cfg.Point) (flow.PointState, bool) {
	ps, ok := fs.Points[point]
	if ok {
		return ps, true
	}
	ps, ok = fs.InPoints[point]
	return ps, ok
}

func (d *Driver) projectCallContracts(prog *program, ref summary.FuncRef, evidence api.FlowEvidence) []api.CallContractEvidence {
	if d == nil || prog == nil || len(evidence.Calls) == 0 {
		return nil
	}
	g := prog.Graph(ref)
	tr, _ := prog.transfers[ref].(*transfer.Transfer)
	fs, ok := d.states[ref]
	if g == nil || tr == nil || !ok {
		return nil
	}
	ct := callTyper{d: d, g: g, ref: ref}
	var contracts []api.CallContractEvidence
	for i, ev := range evidence.Calls {
		if ev.Info == nil || ev.Info.Call == nil || len(ev.Info.Call.Args) == 0 {
			continue
		}
		ps, ok := fs.Points[ev.Point]
		if !ok {
			continue
		}
		ctx := tr.ProductCallContext(&ps, ev.Info.Call)
		demands := ct.CallArgDemands(ev.Info.Call, ctx)
		if len(demands) == 0 {
			continue
		}
		if contracts == nil {
			contracts = make([]api.CallContractEvidence, len(evidence.Calls))
		}
		contracts[i] = api.NewCallContractEvidence(demands)
	}
	return contracts
}

// program is the canonical driver's summary.Program: the module's call graph,
// with each function's inputs and per-node transfer assembled once. It is the
// concrete seam the summary fixpoint ranges over.
type program struct {
	driver       *Driver
	funcTopology topology.FunctionTopology
	inputs       map[summary.FuncRef]input.Inputs
	transfers    map[summary.FuncRef]equation.NodeTransfer
	params       map[summary.FuncRef]int

	// functionFacts are immutable annotation/global facts retained in the richer
	// bridge shape. Transfer, entry seeding, capture fallback, and diagnostics all
	// read this single carrier instead of parallel declared-type maps.
	functionFacts map[summary.FuncRef]functionFacts

	// refs is every module function in deterministic discovery order (root first,
	// then nested functions in CFG point order), so the driver summarizes them
	// reproducibly.
	refs []summary.FuncRef

	// declaredReturns records each function's annotation return tuple in the same
	// deterministic function-ref space as the summary fixed point. It is signature
	// input to Summary projection, not a driver-side body proof.
	declaredReturns map[summary.FuncRef][]typ.Type

	// facts are finite module-level semantic inputs derived after name resolution
	// and consumed by transfers during the canonical solve.
	facts facts.Module

	// callerRefsByCallee is the deterministic static reverse call graph. Aggregate
	// entry fallback uses it as a dependency index instead of scanning every module
	// summary while the summary solve is running.
	callerRefsByCallee map[summary.FuncRef][]summary.FuncRef

	// prototypePublishersBySym indexes solved summaries that publish a runtime self
	// value for a specific prototype symbol. A method receiver's fallback entry value
	// reads only publishers for its prototype, not every summary with any prototype
	// publication.
	prototypePublishersBySym map[cfg.SymbolID][]summary.FuncRef

	// referencePaths is the graph-derived vocabulary of function/closure identity
	// paths each callee can observe at entry.
	referencePaths map[summary.FuncRef]flow.ReferencePathProjection
}

func (p *program) Graph(ref summary.FuncRef) *cfg.Graph { return p.funcTopology.Graph(ref) }
func (p *program) NumParams(ref summary.FuncRef) int    { return p.params[ref] }
func (p *program) DeclaredReturns(ref summary.FuncRef) []typ.Type {
	return append([]typ.Type(nil), p.declaredReturns[ref]...)
}

func (p *program) refHasClosedDeclaredReturns(ref summary.FuncRef) bool {
	return declaredTupleClosed(p.declaredReturns[ref])
}

// refByFunc resolves a function literal to its FuncRef at the temporary AST
// boundary. Solver-facing identity remains FuncRef; AST pointers are not used as
// summary/query keys.
func (p *program) refByFunc(fn *ast.FunctionExpr) (summary.FuncRef, bool) {
	if p == nil || fn == nil {
		return summary.FuncRef{}, false
	}
	return p.funcTopology.RefForFunction(fn)
}

func (p *program) refBySymbol(sym cfg.SymbolID) (summary.FuncRef, bool) {
	if p == nil || sym == 0 {
		return summary.FuncRef{}, false
	}
	return p.funcTopology.RefForSymbol(sym)
}

func (p *program) symbolsForRef(ref summary.FuncRef) []cfg.SymbolID {
	if p == nil {
		return nil
	}
	return p.funcTopology.SymbolsForRef(ref)
}

func (p *program) refByGraph(g *cfg.Graph) (summary.FuncRef, bool) {
	if p == nil || g == nil {
		return summary.FuncRef{}, false
	}
	return p.funcTopology.RefForGraph(g)
}

func (p *program) funcExpr(ref summary.FuncRef) *ast.FunctionExpr {
	if p == nil {
		return nil
	}
	return p.funcTopology.Function(ref)
}

func (p *program) methodDef(ref summary.FuncRef) *cfg.FuncDefInfo {
	if p == nil {
		return nil
	}
	return p.funcTopology.MethodDef(ref)
}

func (p *program) Transfer(ref summary.FuncRef) equation.NodeTransfer {
	return p.transfers[ref]
}

func (p *program) WithSolveContext(ctx *db.QueryContext, solve func() state.FunctionState) state.FunctionState {
	if p == nil || p.driver == nil || solve == nil {
		return state.FunctionStateDomain.Bottom()
	}
	prev := p.driver.activeCtx
	p.driver.activeCtx = ctx
	defer func() { p.driver.activeCtx = prev }()
	return solve()
}

func (p *program) LocalParamNarrows(ref summary.FuncRef) []paramevidence.ParamNarrow {
	tr, ok := p.transfers[ref].(*transfer.Transfer)
	if !ok || tr == nil {
		return nil
	}
	return tr.ParamNarrowEffects()
}

func (p *program) DelegatedParamNarrowCalls(ref summary.FuncRef) []paramevidence.DelegatedCall {
	tr, ok := p.transfers[ref].(*transfer.Transfer)
	if !ok || tr == nil {
		return nil
	}
	return tr.ExitDominatingCalls()
}

func (p *program) ResolveDelegatedCallee(ref summary.FuncRef, call *ast.FuncCallExpr) (summary.FuncRef, bool) {
	if p == nil || p.driver == nil || call == nil {
		return summary.FuncRef{}, false
	}
	g := p.Graph(ref)
	if g == nil {
		return summary.FuncRef{}, false
	}
	return callTyper{d: p.driver, g: g}.resolveCalleeRef(call, p)
}

func (p *program) CaptureEntries(ref summary.FuncRef, captureExportsOf func(summary.FuncRef) flow.CaptureCells) flow.CaptureCells {
	g := p.Graph(ref)
	if g == nil || captureExportsOf == nil {
		return flow.CaptureCellsDomain.Bottom()
	}
	bindings := g.Bindings()
	fn := g.Func()
	if bindings == nil || fn == nil {
		return flow.CaptureCellsDomain.Bottom()
	}
	captured := bindings.CapturedSymbols(fn)
	if len(captured) == 0 {
		return flow.CaptureCellsDomain.Bottom()
	}
	deps := p.captureDependencyChain(ref)
	entries := make([]flow.CaptureCell, 0, len(captured))
	for _, sym := range captured {
		if !g.IsFreeSymbol(sym) {
			continue
		}
		if av, ok := p.captureEntryValue(ref, sym, deps, captureExportsOf); ok {
			entries = append(entries, flow.CaptureCell{Symbol: sym, Value: av})
		}
	}
	return flow.CaptureCellsOf(entries).ProjectPaths(p.referenceProjection(ref))
}

func (p *program) CaptureEntryRefs(ref summary.FuncRef, captureFunctionRefsOf func(summary.FuncRef) flow.FunctionRefs) flow.FunctionRefs {
	g := p.Graph(ref)
	if g == nil || captureFunctionRefsOf == nil {
		return flow.FunctionRefsDomain.Bottom()
	}
	bindings := g.Bindings()
	fn := g.Func()
	if bindings == nil || fn == nil {
		return flow.FunctionRefsDomain.Bottom()
	}
	captured := bindings.CapturedSymbols(fn)
	if len(captured) == 0 {
		return flow.FunctionRefsDomain.Bottom()
	}
	projection := p.referenceProjection(ref)
	if len(projection.Exact) == 0 && len(projection.Subtrees) == 0 {
		return flow.FunctionRefsDomain.Bottom()
	}
	out := flow.FunctionRefsDomain.Bottom()
	for _, dep := range p.captureDependencyChain(ref) {
		out = flow.FunctionRefsDomain.Join(out, flow.ProjectFunctionRefsByReferencePaths(captureFunctionRefsOf(dep), projection))
	}
	return out
}

func (p *program) CaptureEntryClosureRefs(ref summary.FuncRef, captureClosureRefsOf func(summary.FuncRef) flow.ClosureRefs) flow.ClosureRefs {
	g := p.Graph(ref)
	if g == nil || captureClosureRefsOf == nil {
		return flow.ClosureRefsDomain.Bottom()
	}
	projection := p.referenceProjection(ref)
	if len(projection.Exact) == 0 && len(projection.Subtrees) == 0 {
		return flow.ClosureRefsDomain.Bottom()
	}
	out := flow.ClosureRefsDomain.Bottom()
	for _, dep := range p.captureDependencyChain(ref) {
		out = flow.ClosureRefsDomain.Join(out, flow.ProjectClosureRefsByReferencePaths(captureClosureRefsOf(dep), projection))
	}
	return out
}

func (p *program) EntryValues(ref summary.FuncRef, deps summary.EntryValueDependencies) map[int]product.AbstractValue {
	if deps == nil {
		return nil
	}
	receivers := p.facts.MethodReceivers(ref)
	prototypeReceivers := entryValuePrototypeReceivers(receivers)
	hasInferredSlots := p.hasInferredEntrySlot(ref)
	out := summary.AggregateEntryValues(summary.EntryValueAggregation{
		Callee:           ref,
		HasInferredSlots: hasInferredSlots,
		EachCallerEntryValues: func(yield func(summary.EntryValues)) {
			if !hasInferredSlots {
				return
			}
			for _, dep := range p.callerRefs(ref) {
				values := deps.CallEntryValues(dep, ref)
				if len(values) != 0 {
					yield(values)
				}
			}
		},
		PrototypeReceivers: prototypeReceivers,
		EachPrototypeSource: func(yield func(summary.EntryValuePrototypeSource)) {
			for _, dep := range p.prototypePublisherRefs(prototypeReceivers) {
				if protos := p.publishedPrototypes(dep); len(protos) > 0 {
					yield(summary.EntryValuePrototypeSource{
						Prototypes: protos,
						Self:       deps.PrototypeSelf(dep),
					})
				}
			}
		},
		SlotDeclared: func(slot int) bool {
			return p.paramSlotFixed(ref, slot)
		},
	})
	for _, seed := range p.facts.FunctionEntrySeeds(ref) {
		out = summary.JoinEntryValue(out, seed.Slot, product.FromType(seed.Type))
	}
	out = p.withPrototypeReceiverBaselines(ref, out, prototypeReceivers, deps)
	out = p.withPrototypeMethodSurfacesForRef(ref, out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// hasInferredEntrySlot is a query-dependency guard: functions whose parameters
// are all fixed declarations do not need aggregate caller entry evidence, so
// EntryValues must not read caller summaries and perturb the interprocedural
// cache/fixpoint. Refinable structural annotations (`{any}`, `any[]`, maps with
// dynamic interiors) are not fixed; EntrySeedEffect can compose caller evidence
// with them.
func (p *program) hasInferredEntrySlot(ref summary.FuncRef) bool {
	if p == nil {
		return false
	}
	g := p.Graph(ref)
	if g == nil {
		return false
	}
	for slot := range g.ParamSymbols() {
		if !p.paramSlotFixed(ref, slot) {
			return true
		}
	}
	return false
}

func (p *program) publishedPrototypes(ref summary.FuncRef) []cfg.SymbolID {
	sites := p.facts.SetMetatableSites(ref)
	if len(sites) == 0 {
		return nil
	}
	out := make([]cfg.SymbolID, 0, len(sites))
	for _, site := range sites {
		if site.PrototypeSym != 0 {
			out = append(out, site.PrototypeSym)
		}
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (p *program) ProjectCallEntryValues(ref summary.FuncRef, fs state.FunctionState) summary.CallEntryValues {
	projector, ok := p.callEntryProjector(ref)
	if !ok {
		return nil
	}
	return projector.valueProjection(fs).Project()
}

func (p *program) ProjectCallEntryContextKeys(ref summary.FuncRef, fs state.FunctionState) []summary.Key {
	projector, ok := p.callEntryProjector(ref)
	if !ok {
		return nil
	}
	return projector.contextProjection(fs).ProjectKeys()
}

func (p *program) callbackArgRefs(g *cfg.Graph, arg ast.Expr, rawSym cfg.SymbolID, in *flow.PointState) ([]summary.FuncRef, bool) {
	if p == nil || g == nil || arg == nil {
		return nil, false
	}
	resolver := (callTyper{d: p.driver, g: g}).targetResolver(p)
	var refs flow.FunctionRefs
	if in != nil {
		refs = in.FunctionRefs
	}
	return resolver.ResolveCallbackArgRefsOrSymbol(arg, refs, rawSym, p.refByFunc)
}

func (p *program) callEntryFunctionArgRefs(g *cfg.Graph, arg ast.Expr, in *flow.PointState) (flow.FunctionRefSet, bool) {
	got, ok := p.callbackArgRefs(g, arg, 0, in)
	return functionRefSetFromSummaryRefs(got, ok)
}

func (p *program) callEntryFunctionArgTreeRefs(g *cfg.Graph, tr *transfer.Transfer, arg ast.Expr, in *flow.PointState) (flow.FunctionRefs, bool) {
	if p == nil || p.driver == nil || tr == nil || in == nil {
		return flow.FunctionRefsDomain.Bottom(), false
	}
	call, ok := valueCallExpr(arg)
	if !ok {
		return flow.FunctionRefsDomain.Bottom(), false
	}
	returns := (callTyper{d: p.driver, g: g}).CallReturnFunctionRefsFromValues(call, tr.ProductCallContext(in, call))
	if len(returns) == 0 || flow.FunctionRefsDomain.Equal(returns[0], flow.FunctionRefsDomain.Bottom()) {
		return flow.FunctionRefsDomain.Bottom(), false
	}
	return returns[0], true
}

func (p *program) callEntryClosureArgRefs(g *cfg.Graph, arg ast.Expr, in *flow.PointState) (flow.ClosureRefSet, bool) {
	if p == nil || arg == nil || in == nil {
		return flow.ClosureRefSet{}, false
	}
	if fn, ok := arg.(*ast.FunctionExpr); ok && fn != nil {
		ref, ok := p.refByFunc(fn)
		if !ok {
			return flow.ClosureRefSet{}, false
		}
		captured := p.capturedSymbols(ref)
		projection := p.referenceProjection(ref)
		cells := captureCellsFromPoint(in, captured)
		cells = p.normalizeCapturedMethodReceiverCells(g, in, cells, captured)
		cells = cells.ProjectPaths(projection)
		return flow.ClosureRefSetOf(flow.ClosureRefOf(
			canonref.ToFlow(ref),
			cells,
			flow.ProjectFunctionRefsByReferencePaths(in.FunctionRefs, projection),
			flow.ProjectClosureRefsByReferencePaths(in.ClosureRefs, projection),
		)), true
	}
	resolver := (callTyper{d: p.driver, g: g}).targetResolver(p)
	return resolver.ResolveClosureRefSetAtExpr(arg, in.ClosureRefs)
}

func (p *program) callEntryClosureArgTreeRefs(g *cfg.Graph, tr *transfer.Transfer, arg ast.Expr, in *flow.PointState) (flow.ClosureRefs, bool) {
	if p == nil || p.driver == nil || tr == nil || in == nil {
		return flow.ClosureRefsDomain.Bottom(), false
	}
	call, ok := valueCallExpr(arg)
	if !ok {
		return flow.ClosureRefsDomain.Bottom(), false
	}
	returns := (callTyper{d: p.driver, g: g}).CallReturnClosureRefsFromValues(call, tr.ProductCallContext(in, call))
	if len(returns) == 0 || flow.ClosureRefsDomain.Equal(returns[0], flow.ClosureRefsDomain.Bottom()) {
		return flow.ClosureRefsDomain.Bottom(), false
	}
	return returns[0], true
}

func functionRefSetFromSummaryRefs(refs []summary.FuncRef, ok bool) (flow.FunctionRefSet, bool) {
	if !ok {
		return flow.FunctionRefSet{}, false
	}
	if len(refs) == 0 {
		return flow.FunctionRefSetTop(), true
	}
	flowRefs := make([]flow.FunctionRef, 0, len(refs))
	for _, ref := range refs {
		if ref == (summary.FuncRef{}) {
			continue
		}
		flowRefs = append(flowRefs, canonref.ToFlow(ref))
	}
	if len(flowRefs) == 0 {
		return flow.FunctionRefSet{}, false
	}
	return flow.FunctionRefSetOf(flowRefs...), true
}

func captureCellsFromPoint(in *flow.PointState, captured []cfg.SymbolID) flow.CaptureCells {
	if in == nil || len(captured) == 0 {
		return flow.CaptureCellsDomain.Bottom()
	}
	cells := in.Cells.Project(captured)
	for _, sym := range captured {
		if sym == 0 {
			continue
		}
		if _, ok := cells.Value(sym); ok {
			continue
		}
		if av, ok := flow.SymbolValue(*in, sym); ok && !av.IsZero() {
			cells = cells.With(sym, av)
		}
	}
	return cells
}

func (p *program) expectedCallArgType(g *cfg.Graph, tr *transfer.Transfer, point cfg.Point, info *cfg.CallInfo, in *flow.PointState, argIdx int) typ.Type {
	if p == nil || p.driver == nil || g == nil || tr == nil || info == nil || info.Call == nil || in == nil || argIdx < 0 {
		return nil
	}
	call := info.Call
	ctx := tr.ProductCallContext(in, call)
	ct := callTyper{d: p.driver, g: g}
	forceMethodReceiver := false
	if ref, ok := p.refByGraph(g); ok {
		forceMethodReceiver = callsite.ForceMethodReceiverAtPoint(g.Bindings(), g, p.inputs[ref].Evidence, point, call)
	}
	methodReceiver := ctx.SelfType
	if methodReceiver == nil || typ.IsAbsentOrUnknown(methodReceiver) {
		methodReceiver = ctx.ExprType(info.Receiver)
	}
	callee := typ.Type(nil)
	if !callsite.IsMethodCallInfo(info) {
		callee = ct.expectedCalleeTypeForCall(info.Callee, call, ctx)
	}
	expectedArgs := canonicalcall.ExpectedArgTypesForCall(canonicalcall.ExpectedArgsInput{
		Call:                call,
		ArgTypes:            canonicalcall.ShallowArgTypes(call.Args, ctx.ArgTypes(), ctx.ExprType),
		Resolver:            ct.callTypeResolver(ctx.ExprType),
		Ctx:                 p.driver.activeCtx,
		Query:               p.driver.cfg.Types,
		Callee:              callee,
		IsMethod:            callsite.IsMethodCallInfo(info),
		MethodName:          info.Method,
		MethodReceiverType:  methodReceiver,
		ForceMethodReceiver: forceMethodReceiver,
		ResolveTypeArg: func(expr ast.TypeExpr) typ.Type {
			return p.driver.resolveType(expr, p.driver.baseScope())
		},
	})
	if argIdx >= len(expectedArgs) {
		return nil
	}
	expected := expectedArgs[argIdx]
	if expected == nil || typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
		return nil
	}
	return expected
}

func (ct callTyper) expectedCalleeTypeForCall(expr ast.Expr, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) typ.Type {
	if ct.d != nil && ct.d.activeProgram != nil {
		if ref, ok := ct.targetResolver(ct.d.activeProgram).ResolveStaticCall(call); ok {
			if sig := ct.d.signatureForRef(ct.d.activeProgram, ref); sig != nil {
				return sig
			}
		}
	}
	if nested, ok := expr.(*ast.FuncCallExpr); ok && nested != nil {
		returns, ok := ct.CallReturnValues(nested, ctx.ForCall(nested))
		if ok && len(returns) > 0 {
			if t := product.ProjectValueOrUnknown(returns[0]); t != nil && !typ.IsAbsentOrUnknown(t) {
				return t
			}
		}
	}
	if fn := ct.callFunctionForDemand(call, ctx.ExprType); fn != nil {
		return fn
	}
	if expr != nil {
		if t := ctx.ExprType(expr); t != nil && !typ.IsAbsentOrUnknown(t) {
			return t
		}
	}
	return nil
}

func (p *program) EntrySymbolValues(ref summary.FuncRef) map[cfg.SymbolID]product.AbstractValue {
	var out map[cfg.SymbolID]product.AbstractValue
	add := func(sym cfg.SymbolID, t typ.Type) {
		if sym == 0 || t == nil || typ.IsAbsentOrUnknown(t) {
			return
		}
		if out == nil {
			out = make(map[cfg.SymbolID]product.AbstractValue)
		}
		seed := product.FromType(t)
		if prev, had := out[sym]; had {
			out[sym] = product.Domain.Join(prev, seed)
		} else {
			out[sym] = seed
		}
	}

	if g := p.Graph(ref); g != nil {
		if fnFacts, ok := p.functionFacts[ref]; ok && len(fnFacts.declared) > 0 {
			bindings := g.Bindings()
			if bindings != nil {
				for _, sym := range bindings.ReferencedGlobals() {
					if _, ok := fnFacts.declared[sym]; !ok {
						continue
					}
					add(sym, fnFacts.declared[sym])
				}
			}
		}
	}

	entries := p.facts.CallbackEnv(ref)
	for _, entry := range entries {
		add(entry.Symbol, entry.Type)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p *program) callerRefs(ref summary.FuncRef) []summary.FuncRef {
	if p == nil {
		return nil
	}
	if p.callerRefsByCallee != nil {
		return append([]summary.FuncRef(nil), p.callerRefsByCallee[ref]...)
	}
	var out []summary.FuncRef
	for _, caller := range p.refs {
		for _, callee := range p.Callees(caller) {
			if callee == ref {
				out = append(out, caller)
				break
			}
		}
	}
	return out
}

func (p *program) prototypePublisherRefs(receivers []summary.EntryValuePrototypeReceiver) []summary.FuncRef {
	if p == nil || len(receivers) == 0 || len(p.prototypePublishersBySym) == 0 {
		return nil
	}
	var out []summary.FuncRef
	seen := make(map[summary.FuncRef]bool)
	for _, receiver := range receivers {
		if receiver.Prototype == 0 {
			continue
		}
		for _, dep := range p.prototypePublishersBySym[receiver.Prototype] {
			if seen[dep] {
				continue
			}
			seen[dep] = true
			out = append(out, dep)
		}
	}
	canonref.SortFuncRefs(out)
	return out
}

func (p *program) paramSlotDeclared(ref summary.FuncRef, slot int) bool {
	t := p.paramSlotDeclaredType(ref, slot)
	return t != nil && !typ.IsAbsentOrUnknown(t)
}

// paramSlotFixed reports whether a parameter declaration is a closed runtime
// contract that should block caller-entry inference. An open generic binder
// (`x: T`, `x: {T}`) is declared but not fixed: exact call-entry values must
// still seed it so the single product fixpoint can solve `T` before callback
// entry and return projection read the parameter.
func (p *program) paramSlotFixed(ref summary.FuncRef, slot int) bool {
	t := p.paramSlotDeclaredType(ref, slot)
	return t != nil && !typ.IsAbsentOrUnknown(t) && !typ.ContainsFreeTypeParam(t) && !typ.IsRefinableAnnotation(t)
}

// declaredTupleClosed reports whether source return annotations are closed
// enough to own the caller-visible return tuple. Generic binder returns are not
// closed facts; selected-target summary projection must prefer solved product
// returns for `apply<T,U>(...): U`-style calls and use signature returns only as
// fallback.
func declaredTupleClosed(returns []typ.Type) bool {
	if len(returns) == 0 {
		return false
	}
	for _, ret := range returns {
		if ret == nil || typ.ContainsFreeTypeParam(ret) {
			return false
		}
	}
	return true
}

func (p *program) paramSlotDeclaredType(ref summary.FuncRef, slot int) typ.Type {
	g := p.Graph(ref)
	if g == nil || slot < 0 || slot >= len(g.ParamSymbols()) {
		return nil
	}
	sym := g.ParamSymbols()[slot]
	if sym == 0 {
		return nil
	}
	return p.declaredType(ref, sym)
}

func (p *program) paramSlotCount(ref summary.FuncRef) int {
	g := p.Graph(ref)
	if g == nil {
		return 0
	}
	return len(g.ParamSlotsReadOnly())
}

func (p *program) declaredType(ref summary.FuncRef, sym cfg.SymbolID) typ.Type {
	if p == nil || sym == 0 {
		return nil
	}
	facts, ok := p.functionFacts[ref]
	if !ok || facts.declared == nil {
		return nil
	}
	return facts.declared[sym]
}

func (p *program) CallEntryCells(ref summary.FuncRef, caller flow.CaptureCells) flow.CaptureCells {
	return caller.ProjectPaths(p.referenceProjection(ref))
}

func (p *program) CallEntryFunctionRefs(ref summary.FuncRef, caller flow.FunctionRefs) flow.FunctionRefs {
	return flow.ProjectFunctionRefsByReferencePaths(caller, p.referenceProjection(ref))
}

func (p *program) CallEntryClosureRefs(ref summary.FuncRef, caller flow.ClosureRefs) flow.ClosureRefs {
	return flow.ProjectClosureRefsByReferencePaths(caller, p.referenceProjection(ref))
}

func (p *program) referenceProjection(ref summary.FuncRef) flow.ReferencePathProjection {
	if p == nil {
		return flow.ReferencePathProjection{}
	}
	if projection, ok := p.referencePaths[ref]; ok {
		return projection
	}
	g := p.Graph(ref)
	if g == nil {
		return flow.ReferencePathProjection{}
	}
	projection := summary.ReferencePathProjectionForGraph(g)
	if p.referencePaths != nil {
		p.referencePaths[ref] = projection
	}
	return projection
}

func (p *program) paramPath(ref summary.FuncRef, slot int) (constraint.Path, bool) {
	g := p.Graph(ref)
	if g == nil || slot < 0 {
		return constraint.Path{}, false
	}
	params := g.ParamSymbols()
	if slot >= len(params) || params[slot] == 0 {
		return constraint.Path{}, false
	}
	return constraint.NewPath(params[slot], ""), true
}

func (p *program) capturedSymbols(ref summary.FuncRef) []cfg.SymbolID {
	g := p.Graph(ref)
	if g == nil || g.Bindings() == nil || g.Func() == nil {
		return nil
	}
	var out []cfg.SymbolID
	for _, sym := range g.Bindings().CapturedSymbols(g.Func()) {
		if g.IsFreeSymbol(sym) {
			out = append(out, sym)
		}
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (p *program) captureEntryValue(
	ref summary.FuncRef,
	sym cfg.SymbolID,
	deps []summary.FuncRef,
	captureExportsOf func(summary.FuncRef) flow.CaptureCells,
) (product.AbstractValue, bool) {
	if t, ok := p.facts.ModuleAliasType(sym); ok {
		return product.FromType(t), true
	}
	if t := p.declaredType(ref, sym); t != nil && !typ.IsAbsentOrUnknown(t) {
		return product.FromType(t), true
	}
	for _, dep := range deps {
		exports := captureExportsOf(dep)
		if av, ok := exports.Value(sym); ok && !av.IsZero() {
			return p.withCapturedPrototypeReceiverSurface(dep, sym, av), true
		}
		if t := p.declaredType(dep, sym); t != nil && !typ.IsAbsentOrUnknown(t) {
			return p.withCapturedPrototypeReceiverSurface(dep, sym, product.FromType(t)), true
		}
	}
	return product.AbstractValue{}, false
}

func (p *program) captureDependencyChain(ref summary.FuncRef) []summary.FuncRef {
	return p.funcTopology.ParentChain(ref)
}

// Callees derives ref's call-graph edges by walking every call site in its graph
// and resolving the callee name (or, for a method call with no static callee
// name, the method name) to a module function. Calls to stdlib, imported
// modules, or otherwise unresolved names are not call-graph nodes and are
// skipped: their return is the value-domain default, not a body to summarize.
func (p *program) Callees(ref summary.FuncRef) []summary.FuncRef {
	g := p.Graph(ref)
	if g == nil {
		return nil
	}
	seen := make(map[summary.FuncRef]bool)
	var out []summary.FuncRef
	ct := callTyper{d: p.driver, g: g}
	g.EachCallSite(func(_ cfg.Point, call *cfg.CallInfo) {
		if call == nil || call.Call == nil {
			return
		}
		// A self-edge (callee == ref) is kept: it is the recursion the summary
		// db cycle solves from the bottom seed, not an edge to elide.
		callee, ok := ct.resolveCalleeRef(call.Call, p)
		if !ok || seen[callee] {
			return
		}
		seen[callee] = true
		out = append(out, callee)
	})
	return out
}

// buildProgram walks the module's CFG hierarchy from the root graph, building one
// function entry per graph (root plus every nested function, transitively), and
// assembling the symbol/field identity maps used to derive call-graph edges.
func (d *Driver) buildProgram(sess api.AnalysisSession, rootGraph *cfg.Graph, moduleAliases []topology.ModuleAlias) *program {
	funcTopology := topology.DiscoverFunctions(topology.FunctionDiscoveryInput{
		Root:         rootGraph,
		GraphForFunc: sess.GetOrBuildCFG,
	})
	p := &program{
		driver:          d,
		funcTopology:    funcTopology,
		inputs:          make(map[summary.FuncRef]input.Inputs),
		transfers:       make(map[summary.FuncRef]equation.NodeTransfer),
		params:          make(map[summary.FuncRef]int),
		functionFacts:   make(map[summary.FuncRef]functionFacts),
		declaredReturns: make(map[summary.FuncRef][]typ.Type),
		referencePaths:  make(map[summary.FuncRef]flow.ReferencePathProjection),
		refs:            funcTopology.Refs(),
	}
	for _, ref := range p.refs {
		if g := p.Graph(ref); g != nil {
			p.referencePaths[ref] = summary.ReferencePathProjectionForGraph(g)
			d.addFunction(sess, p, ref, g)
		}
	}
	// Derive finite module facts after name resolution. Pre-transfer
	// facts feed transfer construction; the full fact set follows once transfers
	// can expose their body-local effects.
	prevActive := d.activeProgram
	d.activeProgram = p
	factProgram := facts.Program{
		Refs:          p.refs,
		ModuleAliases: moduleAliases,
		Graph: func(ref summary.FuncRef) *cfg.Graph {
			return p.Graph(ref)
		},
		Evidence: func(g *cfg.Graph) api.FlowEvidence {
			return sess.EvidenceForGraph(g)
		},
		ResolveCallee: func(g *cfg.Graph, call *ast.FuncCallExpr) (summary.FuncRef, bool) {
			ct := callTyper{d: d, g: g}
			return ct.resolveCalleeRef(call, p)
		},
		RefForFuncSymbol: func(sym cfg.SymbolID) (summary.FuncRef, bool) {
			return p.refBySymbol(sym)
		},
		DeclaredReturnTypes: func(ref summary.FuncRef) []typ.Type {
			return append([]typ.Type(nil), p.declaredReturns[ref]...)
		},
		NestedFuncRefs: func(ref summary.FuncRef) []summary.FuncRef {
			return p.funcTopology.NestedRefs(ref)
		},
		CallbackOverlaysForRef: func(ref summary.FuncRef) callbackenv.Overlays {
			return d.declaredCallbackOverlaysForRef(p, ref)
		},
		CalleeCallbackOverlays: func(g *cfg.Graph, call *ast.FuncCallExpr) callbackenv.Overlays {
			return d.staticCalleeCallbackOverlays(callTyper{d: d, g: g}, call)
		},
		TypeByName: func(name string) typ.Type {
			if name == "" {
				return nil
			}
			sc := d.baseScope()
			if sc == nil {
				return nil
			}
			t, ok := sc.LookupType(name)
			if !ok || t == nil || typ.IsAbsentOrUnknown(t) {
				return nil
			}
			return t
		},
		SetupExprType: func(g *cfg.Graph, expr ast.Expr, point cfg.Point) typ.Type {
			return d.callbackEnvSetupExprType(p, g, expr, point)
		},
	}
	p.facts = facts.BuildPreTransfer(factProgram)
	d.seedVariantFieldOrigins(p)
	for _, ref := range p.refs {
		p.transfers[ref] = d.buildTransfer(p, ref)
	}
	p.facts = facts.Build(factProgram)
	p.buildDependencyIndexes()
	d.activeProgram = prevActive
	return p
}

func (p *program) buildDependencyIndexes() {
	if p == nil {
		return
	}
	callers := make(map[summary.FuncRef][]summary.FuncRef)
	prototypePublishers := make(map[cfg.SymbolID][]summary.FuncRef)
	for _, caller := range p.refs {
		for _, callee := range p.Callees(caller) {
			callers[callee] = append(callers[callee], caller)
		}
		for _, proto := range p.publishedPrototypes(caller) {
			if proto == 0 {
				continue
			}
			prototypePublishers[proto] = append(prototypePublishers[proto], caller)
		}
	}
	for callee, refs := range callers {
		callers[callee] = canonref.UniqueSortedFuncRefs(refs)
	}
	for proto, refs := range prototypePublishers {
		prototypePublishers[proto] = canonref.UniqueSortedFuncRefs(refs)
	}
	p.callerRefsByCallee = callers
	p.prototypePublishersBySym = prototypePublishers
}

// addFunction registers one function's parameter count, canonical inputs, and
// bridge facts. Function identity and graph ownership come from FunctionTopology.
func (d *Driver) addFunction(sess api.AnalysisSession, p *program, ref summary.FuncRef, g *cfg.Graph) {
	if _, exists := p.inputs[ref]; exists {
		return
	}

	evidence := sess.EvidenceForGraph(g)
	in := input.Build(g, evidence, d.resolveType, d.typeParamScope(g.Func()))
	in.Scope.CellSymbols = g.CellBackedSymbols()
	p.params[ref] = in.Scope.NumParams()
	// The declared types of annotated parameters and annotated locals are the
	// narrowing base the transfer's edge narrowing widens to: a `local r: A|B = {...}`
	// narrows the declared union per edge, not the precise constructor value seeded in
	// the Env. The same resolution the diagnostic surface reads (buildFunctionFacts)
	// supplies them; it is annotation-only and does not depend on the solve.
	fnFacts := d.buildFunctionFacts(g, evidence)
	// A method/field-definition body's implicit `self` parameter has a declared type
	// only when the receiver is a type-namespace binding (`function T:m()`). Value
	// receivers remain runtime flow facts owned by PrototypeSelf.
	d.seedMethodSelf(&fnFacts, p, g)
	d.seedCapturedDeclaredTypes(&fnFacts, p, ref)
	p.functionFacts[ref] = fnFacts
	in.Scope.DeclaredTypes = fnFacts.declared
	p.inputs[ref] = in
	if fn := p.funcExpr(ref); fn != nil {
		p.declaredReturns[ref] = d.declaredReturnTypes(fn)
	}
}

func (d *Driver) seedCapturedDeclaredTypes(facts *functionFacts, p *program, ref summary.FuncRef) {
	if facts == nil || p == nil {
		return
	}
	captured := p.capturedSymbols(ref)
	if len(captured) == 0 {
		return
	}
	for _, sym := range captured {
		if sym == 0 {
			continue
		}
		if existing := facts.declared[sym]; existing != nil && !typ.IsAbsentOrUnknown(existing) {
			continue
		}
		for _, owner := range p.captureDependencyChain(ref) {
			ownerFacts, ok := p.functionFacts[owner]
			if !ok {
				continue
			}
			t := ownerFacts.declared[sym]
			if t == nil || typ.IsAbsentOrUnknown(t) {
				continue
			}
			if facts.declared == nil {
				facts.declared = make(map[cfg.SymbolID]typ.Type)
			}
			facts.declared[sym] = t
			if ownerFacts.annotated[sym] {
				if facts.annotated == nil {
					facts.annotated = make(map[cfg.SymbolID]bool)
				}
				facts.annotated[sym] = true
			}
			break
		}
	}
}

func (d *Driver) seedVariantFieldOrigins(p *program) {
	if d == nil || p == nil {
		return
	}
	for _, ref := range p.refs {
		in := p.inputs[ref]
		facts := provenance.Normalize(provenance.Input{
			Graph:       in.Graph,
			ConstValues: in.ConstValues,
			ReturnTransform: func(call *cfg.CallInfo, retIndex int) (effect.ReturnType, bool) {
				return d.returnTransformForCall(in.Graph, call, retIndex)
			},
		})
		in.VariantFieldOrigins = facts.VariantFieldOrigins
		p.inputs[ref] = in
	}
}

func (d *Driver) returnTransformForCall(g *cfg.Graph, call *cfg.CallInfo, retIndex int) (effect.ReturnType, bool) {
	if d == nil || g == nil || call == nil || call.Call == nil || retIndex < 0 {
		return nil, false
	}
	fnType := callTyper{d: d, g: g}.callTypeResolver(nil).ResolveCallee(call.Call.Func)
	spec := contract.ExtractSpec(fnType)
	if spec == nil {
		return nil, false
	}
	ret := spec.Effects.GetReturn(retIndex)
	if ret == nil || ret.Transform == nil {
		return nil, false
	}
	return ret.Transform, true
}

func (d *Driver) buildTransfer(p *program, ref summary.FuncRef) *transfer.Transfer {
	in := p.inputs[ref]
	g := p.Graph(ref)
	// A method body's implicit `self` is seeded here only for a named receiver type
	// (`function T:m()` where T is a type binding). A value receiver
	// (`function methods:m()`) is not a declared contract; split-pattern runtime self
	// flows through the PrototypeSelf product axis and EntryValues fallback.
	// A `expr :: T` cast asserts the operand has the annotated type. Resolve it
	// through the same annotation resolver the parameter and declared-local types
	// use, against the module base scope, so the transfer types a cast operand
	// (e.g. `pairs(cfg :: {[string]: string})`) by its asserted type.
	baseScope := d.baseScope()
	return transfer.New(in, transfer.Config{
		Ops:               opsResolver{d},
		FuncTyper:         funcTyper{d: d, prog: p},
		CallTyper:         callTyper{d: d, g: g, ref: ref},
		TypeChecks:        p.facts.TypeChecks(ref),
		SelfType:          d.methodSelfSeed(p, g),
		MethodReceivers:   p.facts.MethodReceivers(ref),
		SetMetatableSites: p.facts.SetMetatableSites(ref),
		MetatableIndexes:  p.facts.MetatableIndexes(),
		PrototypeMethods:  p.facts.PrototypeMethods(),
		PredicateFacts:    p.facts.PredicateFacts(),
		PredicateGuards:   p.facts.PredicateGuards(ref),
		CastType: func(expr ast.TypeExpr) typ.Type {
			return d.resolveType(expr, baseScope)
		},
		// A bare identifier naming a `type` used as a value (`M.AppError = AppError`)
		// resolves to that type's reified Meta, the same MetaForName rule the synth flow
		// applies, so the field carries the type value (with the built-in `:is` guard).
		TypeNameValue: func(name string) typ.Type {
			if meta := baseScope.MetaForName(name); meta != nil {
				return meta
			}
			return nil
		},
	})
}

// methodSelfSeed resolves a source-declared implicit `self` type for a method
// body's entry state. It applies only to a method/field definition whose receiver
// resolves in the type namespace (for example `function T:m()` with `type T = ...`).
// Value receivers are unannotated runtime facts and are seeded by the PrototypeSelf
// product axis, not by moduleCaptures.
func (d *Driver) methodSelfSeed(p *program, g *cfg.Graph) typ.Type {
	if g == nil {
		return nil
	}
	fn := g.Func()
	if fn == nil {
		return nil
	}
	ref, ok := p.refByFunc(fn)
	if !ok {
		return nil
	}
	info := p.methodDef(ref)
	if info == nil || info.Receiver == nil {
		return nil
	}
	bindings := g.Bindings()
	if bindings == nil || !phasecore.HasUnannotatedSelfParam(fn, bindings) {
		return nil
	}
	return d.namedReceiverType(info, d.baseScope())
}

// baseScope is the type-name scope annotation resolution reads against: the
// module scope (Stdlib enriched with the module's `type X` definitions) once Run
// has built it, falling back to the configured Stdlib before then.
func (d *Driver) baseScope() *scope.State {
	if d.moduleScope != nil {
		return d.moduleScope
	}
	return d.cfg.Stdlib
}

// returnScope is the scope the diagnostic return check resolves g's declared
// return annotation against: the function-context scope (the module base scope
// extended with bounded type parameters and function-local context such as
// varargs) when g is generic/variadic, else the plain module base scope.
func (d *Driver) returnScope(g *cfg.Graph) *scope.State {
	if g == nil {
		return d.baseScope()
	}
	return d.functionContextScopeOver(g.Func(), d.baseScope())
}

// buildModuleScope enriches the configured base scope with every type definition
// the module declares, so a named annotation referring to a module-local type
// resolves structurally. It walks the module's CFG hierarchy (root chunk plus
// every nested function) and applies scope.EnrichWithTypeDefs over each
// graph's TypeDef nodes, resolving each definition through the same annotation
// resolver the rest of the driver uses. Accumulating across the hierarchy makes a
// module-level alias visible to every function body,
// which seeds each function's base scope from the enclosing type namespace.
func (d *Driver) buildModuleScope(sess api.AnalysisSession, rootGraph *cfg.Graph) *scope.State {
	base := d.cfg.Stdlib
	if d.resolver == nil || rootGraph == nil {
		return base
	}
	defResolver := d.typeDefResolver()

	topology.WalkHierarchy(topology.HierarchyInput{
		Root:         rootGraph,
		GraphForFunc: sess.GetOrBuildCFG,
	}, func(node topology.HierarchyNode) {
		g := node.Graph
		base = scope.EnrichWithTypeDefs(g, base, defResolver)
	})
	return base
}

// buildPointScopes is the per-CFG-point scope the diagnostic and observation
// passes read. It returns g's block-aware per-point scopes (precomputed by
// buildHierarchyScopes): each point carries exactly the type definitions lexically
// visible there plus the function-local context (`self`, type params, varargs).
func (d *Driver) buildPointScopes(g *cfg.Graph) map[cfg.Point]*scope.State {
	if g == nil || d.resolver == nil {
		return nil
	}
	if d.pointScopes != nil {
		if scopes, ok := d.pointScopes[g.ID()]; ok {
			return scopes
		}
	}
	return d.computePointScopes(g, d.functionContextScopeOver(g.Func(), d.baseScope()))
}

// buildHierarchyScopes computes the block-aware per-point type-name scope for
// every graph in the module's CFG hierarchy, keyed by graph ID. It walks the
// hierarchy from the root chunk; each graph's points are scoped by the
// block-aware RPO walk (BuildPointScopes), which enters a child scope at every
// block/loop body, adds a `type X` to the scope active at its definition point,
// and POPS the block's locals on scope exit. A nested function's base scope is its
// parent's block-aware exit scope (the type namespace visible where the nested
// function is defined), extended with the nested function's own type parameters,
// so a module-level type stays visible inside the closure while a sibling block's
// local type does not. The root chunk's base is the configured stdlib scope.
func (d *Driver) buildHierarchyScopes(sess api.AnalysisSession, rootGraph *cfg.Graph) map[uint64]map[cfg.Point]*scope.State {
	if rootGraph == nil || d.resolver == nil {
		return nil
	}
	out := make(map[uint64]map[cfg.Point]*scope.State)

	// The root chunk's base is the configured stdlib scope (its predeclared globals
	// and imported type names), NOT the flat module scope: the module's own top-level
	// `type X` defs are re-introduced block-aware as BuildPointScopes encounters them in
	// RPO, so a block-local or forward type is not visible at points where Lua's
	// lexical rules exclude it. FunctionContextScope also carries function-local
	// context such as typed varargs for observation.
	rootBase := d.functionContextScopeOver(rootGraph.Func(), d.cfg.Stdlib)
	topology.WalkHierarchyWithState(topology.HierarchyStateInput[*scope.State]{
		Root:         rootGraph,
		RootState:    rootBase,
		GraphForFunc: sess.GetOrBuildCFG,
		ChildState: func(parent topology.HierarchyStateNode[*scope.State], nested cfg.NestedFunc, _ *cfg.Graph) *scope.State {
			// A nested function defined in this graph sees the type namespace visible at
			// its definition point. BuildPointScopes pops a block's locals on exit, so the
			// scope at the function-definition point carries the enclosing module-level
			// types but not a sibling block's locals.
			exitScope := parent.State
			scopes := out[parent.Graph.ID()]
			if scopes != nil {
				if s, ok := scopes[parent.Graph.Exit()]; ok && s != nil {
					exitScope = s
				}
			}
			childBase := exitScope
			if defScope := d.scopeAtNestedDef(scopes, parent.Graph, nested.Func); defScope != nil {
				childBase = defScope
			}
			return d.functionContextScopeOver(nested.Func, childBase)
		},
	}, func(node topology.HierarchyStateNode[*scope.State]) {
		scopes := d.computePointScopes(node.Graph, node.State)
		out[node.Graph.ID()] = scopes
	})
	return out
}

// scopeAtNestedDef returns the block-aware scope active at the point where the
// nested function fn is defined in parent graph g, so a closure defined inside a
// block sees that block's types while a closure defined at the module top level
// does not see a sibling block's locals. It returns nil when no definition point is
// found, so the caller falls back to the parent's exit scope.
func (d *Driver) scopeAtNestedDef(scopes map[cfg.Point]*scope.State, g *cfg.Graph, fn *ast.FunctionExpr) *scope.State {
	if scopes == nil || g == nil || fn == nil {
		return nil
	}
	var found *scope.State
	g.EachNode(func(p cfg.Point, _ cfg.NodeInfo) {
		if found != nil {
			return
		}
		if info := g.FuncDef(p); info != nil && info.FuncExpr == fn {
			found = scopes[p]
		}
	})
	return found
}

// computePointScopes runs the block-aware scope walk over g from base,
// resolving each `type X` through the Run's single-sourced typedef resolver so a
// per-point scope shares the same recursive family the module scope carries. The
// walk enters a child scope at every block body and pops it on exit, giving each
// point the type names lexically visible there.
func (d *Driver) computePointScopes(g *cfg.Graph, base *scope.State) map[cfg.Point]*scope.State {
	if g == nil || d.resolver == nil {
		return nil
	}
	defResolver := d.typeDefResolver()
	depthExceeded := false
	scopes := scope.BuildPointScopes(g, base, defResolver, scope.PointScopeOptions{
		MaxDepth:      d.cfg.MaxScopeDepth,
		DepthExceeded: &depthExceeded,
	})
	if depthExceeded {
		if d.scopeDepthExceeded == nil {
			d.scopeDepthExceeded = make(map[uint64]bool)
		}
		d.scopeDepthExceeded[g.ID()] = true
	}
	return scopes
}

func (d *Driver) scopeDepthExceededFor(g *cfg.Graph) bool {
	if d == nil || g == nil || d.scopeDepthExceeded == nil {
		return false
	}
	return d.scopeDepthExceeded[g.ID()]
}

func (d *Driver) runComputePasses(g *cfg.Graph, scopes map[cfg.Point]*scope.State) map[string]any {
	if d == nil || g == nil || len(d.cfg.ComputePasses) == 0 {
		return nil
	}
	extras := make(map[string]any, len(d.cfg.ComputePasses))
	for _, pass := range d.cfg.ComputePasses {
		if pass == nil || pass.Name() == "" {
			continue
		}
		extras[pass.Name()] = pass.Run(g, scopes)
	}
	if len(extras) == 0 {
		return nil
	}
	return extras
}

func (d *Driver) emitScopeDepthDiagnostic(sess api.AnalysisSession, fn *ast.FunctionExpr, result *api.FuncResult) {
	if d == nil || sess == nil || result == nil || !d.cfg.EmitScopeDiag || d.cfg.MaxScopeDepth <= 0 || !result.DepthLimitExceeded {
		return
	}
	scopeState := sess.ScopeDepthDiagState()
	if scopeState[fn] {
		return
	}
	pos := diag.Position{File: sess.Source()}
	span := diag.Span{}
	if fn != nil && fn.Line() > 0 {
		pos.Line = fn.Line()
		pos.Column = fn.Column()
		span.StartLine = fn.Line()
		span.StartCol = fn.Column()
		span.EndLine = fn.LastLine()
		span.EndCol = fn.LastColumn()
	}
	sess.AppendDiagnostics(diag.Diagnostic{
		Position: pos,
		Span:     span,
		Severity: diag.SeverityWarning,
		Message:  fmt.Sprintf("scope depth limit exceeded (max=%d); analysis may be incomplete", d.cfg.MaxScopeDepth),
	})
	scopeState[fn] = true
}

// typeDefResolver is the single-sourced TypeDef resolver for this Run: it resolves
// each `type X = ...` once (keyed by its AST TypeExpr node) and returns the cached
// family on every subsequent observation, so the module-wide scope and every
// per-point scope share one recursive family per source type. A generic typedef
// resolves per scope (it binds its type parameters against the enclosing scope), so
// only non-generic definitions are cached; a generic definition is resolved fresh.
func (d *Driver) typeDefResolver() scope.TypeDefResolver {
	return func(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *scope.State) typ.Type {
		if len(typeParams) > 0 || typeExpr == nil || d.typedefCache == nil {
			return d.resolver.ResolveTypeDef(name, typeExpr, typeParams, sc)
		}
		if cached, ok := d.typedefCache[typeExpr]; ok {
			return cached
		}
		resolved := d.resolver.ResolveTypeDef(name, typeExpr, typeParams, sc)
		d.typedefCache[typeExpr] = resolved
		return resolved
	}
}

// resolveType resolves a declared type annotation against the module base scope.
// It is the input.TypeResolver the canonical input builder reads for parameter
// annotations, so an annotated parameter seeds its declared product.AbstractValue
// at entry rather than the unconstrained unknown the engine would otherwise infer.
func (d *Driver) resolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	if d.resolver == nil || expr == nil {
		return nil
	}
	if sc == nil {
		sc = d.baseScope()
	}
	t := d.resolver.ResolveType(expr, sc)
	if typ.IsAbsentOrUnknown(t) {
		return nil
	}
	return t
}

// staticCalleeCallbackOverlays resolves only external/static callback overlay
// contracts for fact extraction. Module-local callees are handled by ResolveCallee
// and CallbackOverlaysForRef; this path intentionally avoids signatureForRef so
// immutable facts cannot pull inferred summary returns into pre-solve evidence.
func (d *Driver) staticCalleeCallbackOverlays(ct callTyper, call *ast.FuncCallExpr) callbackenv.Overlays {
	resolver := ct.callTypeResolver(nil)
	return canonicalcall.StaticCallbackOverlaysForCall(canonicalcall.StaticCallbackOverlayInput{
		Call:     call,
		Resolver: resolver,
	})
}

func (d *Driver) callbackEnvSetupExprType(_ *program, g *cfg.Graph, expr ast.Expr, point cfg.Point) typ.Type {
	if d == nil || expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.FunctionExpr:
		base := d.baseScope()
		if scopes := d.buildPointScopes(g); scopes != nil {
			if sc := scopes[point]; sc != nil {
				base = sc
			}
		}
		return canonicalsig.Build(canonicalsig.Input{
			Function:    e,
			Base:        base,
			ResolveType: d.resolveType,
			ReturnMode:  canonicalsig.ReturnDeclaredOnly,
		})
	case *ast.FuncCallExpr:
		if g == nil {
			return nil
		}
		callee := callTyper{d: d, g: g}.callTypeResolver(nil).ResolveCallee(e.Func)
		fn := unwrap.Function(callee)
		if fn == nil || len(fn.Returns) == 0 {
			return nil
		}
		return fn.Returns[0]
	default:
		return nil
	}
}

// callTyper adapts the driver to the transfer's CallTyper seam: the driver
// supplies program context (scope, type lookup, callee/receiver resolution,
// summary reads), while canonical/call owns the call-return and type-cast policy.
//
// Callee resolution priority: the live Env value the transfer resolved (a
// function-valued local), then the module-wide function signature of the callee
// symbol (a named or local function, even before its summary converges, via
// declared annotations), then the predeclared global's value type, then the field
// path read through TypeOps. A callee that resolves to no function yields no
// returns, so the transfer leaves the slot at the value-domain Top.
type callTyper struct {
	d   *Driver
	g   *cfg.Graph
	ref summary.FuncRef
}

func (ct callTyper) globalSymbolType(sym cfg.SymbolID) typ.Type {
	if sym == 0 || ct.d == nil || ct.d.activeProgram == nil || ct.g == nil {
		return nil
	}
	ref, ok := ct.d.activeProgram.refByGraph(ct.g)
	if !ok {
		return nil
	}
	facts, ok := ct.d.activeProgram.functionFacts[ref]
	if !ok || facts.declared == nil {
		for _, entry := range ct.d.activeProgram.facts.CallbackEnv(ref) {
			if entry.Symbol == sym {
				return entry.Type
			}
		}
		return nil
	}
	if t := facts.declared[sym]; t != nil {
		return t
	}
	for _, entry := range ct.d.activeProgram.facts.CallbackEnv(ref) {
		if entry.Symbol == sym {
			return entry.Type
		}
	}
	return nil
}

func (ct callTyper) globalNameType(name string) typ.Type {
	if name == "" || ct.d == nil {
		return nil
	}
	t, _ := ct.d.globalTypes.Type(name)
	return t
}

// opsResolver adapts the driver to the transfer's OperatorResolver seam: it routes
// arithmetic/relational operator typing through the shared TypeOps engine, the same
// resolver used by expression typing (core.QueryResolver). It holds the driver and reads
// the query context lazily at call time, because the transfers are built during
// buildProgram before the run's activeCtx is set. A run with no resolved context or
// query ops yields nil, so the transfer falls back to its structural numeric default.
type opsResolver struct{ d *Driver }

func (r opsResolver) resolver() *core.QueryResolver {
	if r.d == nil || r.d.cfg.Types == nil || r.d.activeCtx == nil {
		return nil
	}
	return core.NewQueryResolver(r.d.activeCtx, r.d.cfg.Types)
}

func (r opsResolver) BinaryOp(left typ.Type, op string, right typ.Type) typ.Type {
	res := r.resolver()
	if res == nil {
		return nil
	}
	return res.BinaryOp(left, op, right)
}

func (r opsResolver) UnaryOp(op string, operand typ.Type) typ.Type {
	res := r.resolver()
	if res == nil {
		return nil
	}
	return res.UnaryOp(op, operand)
}

// CallReturns types call's Lua return vector. argTypes are the value-domain types
// the transfer resolved for the arguments (typ.Unknown for an undetermined slot);
// exprType resolves an expression against the live point Env for callee/receiver
// resolution.
func (ct callTyper) CallReturns(call *ast.FuncCallExpr, argTypes []typ.Type, exprType func(ast.Expr) typ.Type, cells flow.CaptureCells, refs flow.FunctionRefs) ([]typ.Type, bool) {
	d := ct.d
	if call == nil || d == nil || d.cfg.Types == nil || d.activeProgram == nil {
		return nil, false
	}
	return canonicalcall.InferReturnTypes(ct.callReturnInput(call, argTypes, exprType, cells, refs, flow.ClosureRefsDomain.Bottom(), nil))
}

func (ct callTyper) callReturnInput(call *ast.FuncCallExpr, argTypes []typ.Type, exprType func(ast.Expr) typ.Type, cells flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, methodReceiverType typ.Type) canonicalcall.ReturnInput {
	d := ct.d
	resolver := ct.callTypeResolver(exprType)
	argTypes = ct.refineFunctionArgTypes(call, argTypes, exprType, cells, refs, closures, methodReceiverType)
	return canonicalcall.ReturnInput{
		Call:               call,
		ArgTypes:           argTypes,
		Env:                ct.callInterceptEnv(exprType),
		Ctx:                d.activeCtx,
		Query:              d.cfg.Types,
		MethodReceiverType: methodReceiverType,
		SummaryReturns: func(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) []typ.Type {
			return ct.moduleCallSummaryReturns(call, exprType, cells, refs)
		},
		Resolver: resolver,
		ResolveTypeArg: func(expr ast.TypeExpr) typ.Type {
			return d.resolveType(expr, d.baseScope())
		},
	}
}

func (ct callTyper) refineFunctionArgTypes(call *ast.FuncCallExpr, argTypes []typ.Type, exprType func(ast.Expr) typ.Type, cells flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, methodReceiverType typ.Type) []typ.Type {
	d := ct.d
	if d == nil || d.activeProgram == nil || call == nil || len(call.Args) == 0 {
		return argTypes
	}
	resolver := ct.targetResolver(d.activeProgram)
	callbackRefs := make(map[ast.Expr][]summary.FuncRef)
	for _, arg := range call.Args {
		argRefs, ok := resolver.ResolveCallbackArgRefs(arg, refs, d.activeProgram.refByFunc)
		if !ok || len(argRefs) == 0 {
			continue
		}
		callbackRefs[arg] = argRefs
	}
	if len(callbackRefs) == 0 {
		return argTypes
	}
	projector := newCallableProjector(d, d.activeProgram, d.activeQueries, d.activeCtx)
	expectedArgs := canonicalcall.ExpectedArgTypesForCall(canonicalcall.ExpectedArgsInput{
		Call:     call,
		ArgTypes: argTypes,
		CallbackArg: func(arg ast.Expr) bool {
			_, ok := callbackRefs[arg]
			return ok
		},
		Resolver:           ct.callTypeResolver(exprType),
		Ctx:                d.activeCtx,
		Query:              d.cfg.Types,
		MethodReceiverType: methodReceiverType,
		ResolveTypeArg: func(expr ast.TypeExpr) typ.Type {
			return d.resolveType(expr, d.baseScope())
		},
	})
	return canonicalcall.RefineCallbackArgTypes(canonicalcall.CallbackArgRefinementInput{
		Call:         call,
		ArgTypes:     argTypes,
		ExpectedArgs: expectedArgs,
		CallbackRefs: func(arg ast.Expr) ([]summary.FuncRef, bool) {
			argRefs, ok := callbackRefs[arg]
			return argRefs, ok
		},
		FunctionType: func(ref summary.FuncRef) typ.Type {
			return projector.FunctionTypeByRef(canonref.ToFlow(ref), cells, refs, closures)
		},
		ContextualFunction: func(ref summary.FuncRef, values summary.EntryValues) typ.Type {
			return ct.functionTypeByRefWithEntryValues(projector, ref, cells, refs, closures, values)
		},
	})
}

func (ct callTyper) functionTypeByRefWithEntryValues(projector callableProjector, ref summary.FuncRef, cells flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, values summary.EntryValues) typ.Type {
	d := ct.d
	if d == nil || d.activeProgram == nil || len(values) == 0 {
		return nil
	}
	sig := d.signatureForRef(d.activeProgram, ref)
	if sig == nil {
		return nil
	}
	entryCells := d.activeProgram.CallEntryCells(ref, cells)
	entryRefs := d.activeProgram.CallEntryFunctionRefs(ref, refs)
	entryClosures := d.activeProgram.CallEntryClosureRefs(ref, closures)
	sum := projector.reader.SummarizeWithEntryContext(ref, entryCells, entryRefs, entryClosures, values)
	return summary.FunctionSignatureWithEntryParamsAndProjectedReturns(sig, d.refHasDeclaredReturns(d.activeProgram, ref), sum, values)
}

func (ct callTyper) CallArgDemands(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) []callobligation.Obligation {
	return canonicalcall.CallArgDemandsForCall(canonicalcall.CallArgDemandsInput{
		Call: call,
		SummaryDemands: func(call *ast.FuncCallExpr) ([]callobligation.Obligation, bool) {
			return ct.moduleCallArgDemands(call, ctx)
		},
		FunctionShape: func(call *ast.FuncCallExpr) *typ.Function {
			return ct.callFunctionForDemand(call, ctx.ExprType)
		},
		SelfType: func(*ast.FuncCallExpr) typ.Type {
			return ctx.SelfType
		},
	})
}

func (ct callTyper) moduleCallArgDemands(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) ([]callobligation.Obligation, bool) {
	d := ct.d
	if d == nil || d.activeProgram == nil || call == nil || len(call.Args) == 0 {
		return nil, false
	}
	prog := d.activeProgram
	outcome := ct.summaryOnlyProductCallOutcome(call, ctx)
	if !outcome.HasTargets() {
		return nil, false
	}
	targets := outcome.Targets()
	currentRef, hasCurrentRef := ct.currentRef()
	projectionTargets := make([]paramevidence.CallArgDemandTarget, 0, len(targets))
	for _, target := range targets {
		ref := target.Ref
		if hasCurrentRef && ref == currentRef {
			continue
		}
		localRef := ref
		localFn := prog.funcExpr(localRef)
		projectionTargets = append(projectionTargets, paramevidence.CallArgDemandTarget{
			Graph:     prog.Graph(localRef),
			Function:  localFn,
			Contracts: prog.publicPredicateContracts(localRef, target.Summary.Params),
			DeclaredSlotType: func(slot int) typ.Type {
				return prog.paramSlotDeclaredType(localRef, slot)
			},
			EntrySlotType: func(slot int) typ.Type {
				return ct.callEntrySlotType(localRef, call, ctx.RuntimeArgValues, target.EntryValues, slot)
			},
			SourceParamAnnotated: func(sourceParam int) bool {
				return paramevidence.SourceParamAnnotated(localFn, sourceParam)
			},
		})
	}
	if len(projectionTargets) == 0 {
		return nil, false
	}
	return canonicalcall.DemandsForCallTargets(call, projectionTargets), true
}

func (ct callTyper) currentRef() (summary.FuncRef, bool) {
	if ct.ref != (summary.FuncRef{}) {
		return ct.ref, true
	}
	if ct.d == nil || ct.d.activeProgram == nil || ct.g == nil {
		return summary.FuncRef{}, false
	}
	return ct.d.activeProgram.refByGraph(ct.g)
}

func (ct callTyper) summaryForCallEntryContext(entry canonicalcall.EntryContext) summary.Summary {
	d := ct.d
	if d == nil {
		return summary.SummaryDomain.Bottom()
	}
	return d.summaryReader().SummarizeWithEntryContextFacts(
		entry.Ref(),
		entry.CaptureCells(),
		entry.FunctionRefs(),
		entry.ClosureRefs(),
		entry.EntryValues(),
		entry.EntryFacts(),
	)
}

func (ct callTyper) callFunctionForDemand(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) *typ.Function {
	if ct.d == nil || call == nil || ct.d.activeProgram == nil {
		return nil
	}
	resolver := ct.callTypeResolver(exprType)
	return canonicalcall.FunctionForDemand(canonicalcall.DemandFunctionInput{
		Call: call,
		SummaryFunction: func(call *ast.FuncCallExpr) *typ.Function {
			if ref, ok := ct.resolveCalleeRef(call, ct.d.activeProgram); ok && ct.d.activeQueries != nil && ct.d.activeCtx != nil {
				return ct.d.signatureForRef(ct.d.activeProgram, ref)
			}
			return nil
		},
		Resolver: resolver,
	})
}

// CallReturnValues is the product-carrier call-return path. It preserves product
// axes owned by the canonical fixed point (for example gradual-top evidence and
// callee summary return values) and falls back to the type-only CallReturns seam
// only at external boundaries that still expose typ.Type.
func (ct callTyper) CallReturnValues(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) ([]product.AbstractValue, bool) {
	d := ct.d
	if call == nil || d == nil || d.activeProgram == nil {
		return nil, false
	}
	argTypes := ctx.ArgTypes()
	exprType := ctx.ExprType
	outcome := ct.callOutcomeForProductCall(call, ctx)
	summaryReturns := outcome.InferredReturnValues()

	return canonicalcall.InferReturnValues(canonicalcall.ReturnValueInput{
		Call:                 call,
		Env:                  ct.callInterceptEnv(exprType),
		TypePolicyAvailable:  d.cfg.Types != nil,
		PendingInput:         ctx.PendingInput,
		BlockDynamicFallback: outcome.HasTargets() && !outcome.HasInformativeReturnValues(),
		SummaryReturnValues: func(call *ast.FuncCallExpr) []product.AbstractValue {
			return summaryReturns
		},
		ExprValue: ctx.ExprValue,
		TypeFallback: func() ([]typ.Type, bool) {
			return canonicalcall.InferReturnTypes(ct.callReturnInput(call, argTypes, exprType, ctx.Cells, ctx.FunctionRefs, ctx.ClosureRefs, ctx.SelfType))
		},
	})
}

func (ct callTyper) callEntryFunctionArgRefs(arg ast.Expr, refs flow.FunctionRefs) (flow.FunctionRefSet, bool) {
	d := ct.d
	if d == nil || d.activeProgram == nil {
		return flow.FunctionRefSet{}, false
	}
	resolver := ct.targetResolver(d.activeProgram)
	got, ok := resolver.ResolveCallbackArgRefs(arg, refs, d.activeProgram.refByFunc)
	return functionRefSetFromSummaryRefs(got, ok)
}

func (ct callTyper) callEntryFunctionArgTreeRefs(arg ast.Expr, ctx transfer.ProductCallContext) (flow.FunctionRefs, bool) {
	if ct.d == nil || ct.d.activeProgram == nil {
		return flow.FunctionRefsDomain.Bottom(), false
	}
	call, ok := valueCallExpr(arg)
	if !ok {
		return flow.FunctionRefsDomain.Bottom(), false
	}
	returns := ct.CallReturnFunctionRefsFromValues(call, ctx.ForCall(call))
	if len(returns) == 0 || flow.FunctionRefsDomain.Equal(returns[0], flow.FunctionRefsDomain.Bottom()) {
		return flow.FunctionRefsDomain.Bottom(), false
	}
	return returns[0], true
}

func (ct callTyper) callEntryClosureArgRefs(arg ast.Expr, ctx transfer.ProductCallContext) (flow.ClosureRefSet, bool) {
	d := ct.d
	if d == nil || d.activeProgram == nil || arg == nil {
		return flow.ClosureRefSet{}, false
	}
	if fn, ok := arg.(*ast.FunctionExpr); ok && fn != nil {
		ref, ok := d.activeProgram.refByFunc(fn)
		if !ok {
			return flow.ClosureRefSet{}, false
		}
		captured := d.activeProgram.capturedSymbols(ref)
		projection := d.activeProgram.referenceProjection(ref)
		cells := ctx.Cells.Project(captured)
		cells = d.activeProgram.normalizeCapturedMethodReceiverCellsFromCells(ct.g, cells, captured)
		cells = cells.ProjectPaths(projection)
		return flow.ClosureRefSetOf(flow.ClosureRefOf(
			canonref.ToFlow(ref),
			cells,
			flow.ProjectFunctionRefsByReferencePaths(ctx.FunctionRefs, projection),
			flow.ProjectClosureRefsByReferencePaths(ctx.ClosureRefs, projection),
		)), true
	}
	resolver := ct.targetResolver(d.activeProgram)
	return resolver.ResolveClosureRefSetAtExpr(arg, ctx.ClosureRefs)
}

func (ct callTyper) callEntryClosureArgTreeRefs(arg ast.Expr, ctx transfer.ProductCallContext) (flow.ClosureRefs, bool) {
	if ct.d == nil || ct.d.activeProgram == nil {
		return flow.ClosureRefsDomain.Bottom(), false
	}
	call, ok := valueCallExpr(arg)
	if !ok {
		return flow.ClosureRefsDomain.Bottom(), false
	}
	returns := ct.CallReturnClosureRefsFromValues(call, ctx.ForCall(call))
	if len(returns) == 0 || flow.ClosureRefsDomain.Equal(returns[0], flow.ClosureRefsDomain.Bottom()) {
		return flow.ClosureRefsDomain.Bottom(), false
	}
	return returns[0], true
}

func valueCallExpr(expr ast.Expr) (*ast.FuncCallExpr, bool) {
	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		return e, e != nil
	case *ast.CastExpr:
		return valueCallExpr(e.Expr)
	default:
		return nil, false
	}
}

func (ct callTyper) moduleCallSummaryReturns(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type, cells flow.CaptureCells, refs flow.FunctionRefs) []typ.Type {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return nil
	}
	return ct.callOutcomeForTypedCall(call, exprType, cells, refs).InferredReturnTypes()
}

func (ct callTyper) moduleCallSummaryReturnValues(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) []product.AbstractValue {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return nil
	}
	// Keep the normalized product context intact through summary projection.
	// Selected-target signature returns close generics from ctx.ArgValues and
	// expression projections; rebuilding a partial context from runtime args
	// alone loses that evidence and degrades precise calls to unknown.
	return ct.callOutcomeForProductCall(call, ctx).InferredReturnValues()
}

func (ct callTyper) CallReturnFunctionRefs(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type, cells flow.CaptureCells, refs flow.FunctionRefs) []flow.FunctionRefs {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return nil
	}
	return ct.callOutcomeForTypedCall(call, exprType, cells, refs).ReturnFunctionRefs()
}

func (ct callTyper) CallReturnFunctionRefsFromValues(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) []flow.FunctionRefs {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return nil
	}
	return ct.summaryOnlyProductCallOutcome(call, ctx).ReturnFunctionRefs()
}

func (ct callTyper) CallReturnClosureRefsFromValues(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) []flow.ClosureRefs {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return nil
	}
	return ct.summaryOnlyProductCallOutcome(call, ctx).ReturnClosureRefs()
}

// ReturnRelations resolves the callee's caller-visible return relations through
// the same interprocedural summary fixed point CallReturns uses for return values.
// A recursive seed (Summary bottom) is not consumed as proof; the relation appears
// only after the callee summary/projector proves it.
func (ct callTyper) ReturnRelations(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type, cells flow.CaptureCells, refs flow.FunctionRefs) flow.ReturnRelations {
	d := ct.d
	if d == nil || call == nil {
		return flow.ReturnRelationsDomain.Top()
	}
	var outcome canonicalcall.CallOutcome
	if d.activeProgram != nil {
		outcome = ct.callOutcomeForTypedCall(call, exprType, cells, refs)
	}
	return outcome.ReturnRelations(call, ct.callTypeResolver(exprType), exprType != nil)
}

func (ct callTyper) ReturnRelationsFromValues(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.ReturnRelations {
	d := ct.d
	if d == nil || call == nil {
		return flow.ReturnRelationsDomain.Top()
	}
	var outcome canonicalcall.CallOutcome
	if d.activeProgram != nil {
		outcome = ct.callOutcomeForProductCall(call, ctx)
	}
	return outcome.ReturnRelations(call, ct.callTypeResolver(ctx.ExprType), ctx.ExprValue != nil)
}

// CellEffects resolves the callee's caller-visible capture-cell transformer
// through the same interprocedural summary fixed point as return values and
// return relations. Imported or unresolved callees have no module-local cell
// effect in this domain.
func (ct callTyper) CellEffects(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type, cells flow.CaptureCells, refs flow.FunctionRefs) flow.CaptureEffects {
	projector, ok := ct.cellEffectProjector()
	if !ok || call == nil {
		return flow.CaptureEffectsDomain.Bottom()
	}
	outcome := ct.callOutcomeForTypedCall(call, exprType, cells, refs)
	return projector.typedCallEffects(outcome, call, exprType, cells, refs)
}

func (ct callTyper) CellEffectsFromValues(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.CaptureEffects {
	projector, ok := ct.cellEffectProjector()
	if !ok || call == nil {
		return flow.CaptureEffectsDomain.Bottom()
	}
	outcome := ct.summaryOnlyProductCallOutcome(call, ctx)
	return projector.productCallEffects(outcome, call, ctx)
}

func (ct callTyper) ReceiverEffectsFromValues(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.ReceiverEffects {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return flow.ReceiverEffectsDomain.Bottom()
	}
	effects := ct.summaryOnlyProductCallOutcome(call, ctx).ReceiverEffects()
	return effects
}

func (ct callTyper) BoundaryFactsFromValues(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.BoundaryFacts {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	facts := ct.summaryOnlyProductCallOutcome(call, ctx).BoundaryFacts()
	return facts
}

func (ct callTyper) ContainerElementUnionsFromValues(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) []effect.ContainerElementUnion {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return nil
	}
	return canonicalcall.ContainerElementUnionsForCall(canonicalcall.ContainerElementUnionInput{
		Call: call,
		SummarySignature: func(call *ast.FuncCallExpr) typ.Type {
			if ref, ok := ct.resolveCalleeRef(call, d.activeProgram); ok {
				return d.signatureForRef(d.activeProgram, ref)
			}
			return nil
		},
		Resolver: ct.callTypeResolver(ctx.ExprType),
	})
}

func (ct callTyper) FunctionValueByRef(ref flow.FunctionRef, cells flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs) (typ.Type, bool) {
	d := ct.d
	if d == nil || d.activeProgram == nil {
		return nil, false
	}
	projector := newCallableProjector(d, d.activeProgram, d.activeQueries, d.activeCtx)
	sig := projector.FunctionTypeByRef(ref, cells, refs, closures)
	if typ.IsAbsentOrUnknown(sig) {
		return nil, false
	}
	return sig, true
}

func (ct callTyper) FunctionValueAtPath(path constraint.Path, cells flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs) (typ.Type, bool) {
	d := ct.d
	if d == nil || d.activeProgram == nil || path.IsEmpty() {
		return nil, false
	}
	projector := newCallableProjector(d, d.activeProgram, d.activeQueries, d.activeCtx)
	sig := projector.TypeAt(flow.PointState{
		Cells:        cells,
		FunctionRefs: refs,
		ClosureRefs:  closures,
	}, path)
	if typ.IsAbsentOrUnknown(sig) {
		return nil, false
	}
	return sig, true
}

// IterVars types a generic-for loop's iteration variables from its iterator
// expression. The driver resolves the callee and source expression; the
// iteration domain owns Iterator-effect classification and variable projection.
// An iterator with no iteration effect (and not the ipairs/pairs builtin) yields
// no types, so the transfer leaves the variables untyped.
func (ct callTyper) IterVars(iter *ast.FuncCallExpr, count int, exprType func(ast.Expr) typ.Type) ([]typ.Type, bool) {
	proj, ok := ct.IterVarProjection(iter, count, exprType)
	if !ok || proj.Empty {
		return nil, false
	}
	return proj.Types, true
}

func (ct callTyper) IterVarProjection(iter *ast.FuncCallExpr, count int, exprType func(ast.Expr) typ.Type) (iteration.VarProjection, bool) {
	if iter == nil || count <= 0 || iter.Method != "" || exprType == nil {
		return iteration.VarProjection{}, false
	}
	kind, srcIdx, ok := ct.iteratorKind(iter)
	if !ok || srcIdx < 0 || srcIdx >= len(iter.Args) {
		return iteration.VarProjection{}, false
	}
	return iteration.ProjectVarTypes(kind, count, exprType(iter.Args[srcIdx]))
}

// iteratorKind resolves a generic-for iterator's iteration kind and iterated source
// parameter index. It prefers the Iterator effect on the iterator function's contract
// spec (so a user-defined or stdlib iterator with a declared iteration effect types
// its loop variables), falling back to the ipairs/pairs builtin recognition on a
// predeclared global, the documented builtin iteration forms.
func (ct callTyper) iteratorKind(iter *ast.FuncCallExpr) (effect.IteratorKind, int, bool) {
	fnType := ct.callTypeResolver(func(e ast.Expr) typ.Type {
		// Resolve only the callee through the standard callee resolution; the source
		// argument is typed by the caller's exprType, so a bare exprType here suffices.
		return typ.Unknown
	}).ResolveCallee(iter.Func)
	builtin := iteration.BuiltinName(iter.Func, ct.bindings())
	return iteration.Kind(fnType, builtin, len(iter.Args))
}

// KeyedIterSource reports whether iter is a keyed (pairs-style) iteration and, if
// so, returns the iterated source-argument expression. It reuses iteratorKind so
// the keyed/indexed decision is the contract-spec iteration effect (or the pairs/
// ipairs builtin), not a name match: only a keyed iteration's first loop variable
// is a key of the source, so only that case yields a source for KeyOf production.
func (ct callTyper) KeyedIterSource(iter *ast.FuncCallExpr) (ast.Expr, bool) {
	return iteration.KeyedSource(iter, ct.iteratorKind)
}

func (ct callTyper) IndexedIterSource(iter *ast.FuncCallExpr) (constraint.Path, bool) {
	bindings := ct.bindings()
	if ct.g == nil || bindings == nil {
		return constraint.Path{}, false
	}
	return iteration.IndexedSourcePath(iter, bindings, ct.iteratorKind)
}

func (ct callTyper) KeysCollectorContainer(call *cfg.CallInfo, retIndex int) (constraint.Path, bool) {
	if call == nil || call.Call == nil || ct.g == nil {
		return constraint.Path{}, false
	}
	prog := ct.d.activeProgram
	if prog == nil {
		return constraint.Path{}, false
	}
	ref, ok := ct.resolveCalleeRef(call.Call, prog)
	if !ok {
		return constraint.Path{}, false
	}
	kc, ok := prog.facts.KeysCollector(ref)
	if !ok || kc.ReturnIndex != retIndex {
		return constraint.Path{}, false
	}
	return iteration.ContainerPath(callsite.RuntimeArgAt(call, kc.ParamIndex), ct.bindings())
}

func (ct callTyper) callTypeResolver(exprType func(ast.Expr) typ.Type) canonicalcall.TypeResolver {
	var query core.TypeOps
	var queryCtx *db.QueryContext
	if ct.d != nil {
		query = ct.d.cfg.Types
		queryCtx = ct.d.activeCtx
	}
	return canonicalcall.TypeResolver{
		Bindings: ct.bindings(),
		ExprType: exprType,
		Ctx:      queryCtx,
		Query:    query,
		Static: canonicalcall.StaticTypeLookup{
			FuncBySymbol: func(sym cfg.SymbolID) (typ.Type, bool) {
				if sym == 0 || ct.d == nil || ct.d.activeProgram == nil {
					return nil, false
				}
				ref, ok := ct.d.activeProgram.funcRef(sym)
				if !ok {
					return nil, false
				}
				sig := ct.d.signatureForRef(ct.d.activeProgram, ref)
				return sig, sig != nil
			},
			FieldFunc: func(sym cfg.SymbolID, field fieldkey.Key) (typ.Type, bool) {
				if sym == 0 || ct.d == nil || ct.d.activeProgram == nil {
					return nil, false
				}
				ref, ok := ct.d.activeProgram.fieldFuncRef(sym, field)
				if !ok {
					return nil, false
				}
				sig := ct.d.signatureForRef(ct.d.activeProgram, ref)
				return sig, sig != nil
			},
			ImportedBase: func(sym cfg.SymbolID) (typ.Type, bool) {
				if sym == 0 || ct.d == nil || ct.d.activeProgram == nil {
					return nil, false
				}
				return ct.d.activeProgram.facts.ModuleAliasType(sym)
			},
			GlobalBySymbol: func(sym cfg.SymbolID) (typ.Type, bool) {
				t := ct.globalSymbolType(sym)
				return t, t != nil
			},
			GlobalByName: func(name string) (typ.Type, bool) {
				t := ct.globalNameType(name)
				return t, t != nil
			},
		},
	}
}

func (ct callTyper) callInterceptEnv(exprType func(ast.Expr) typ.Type) canonicalcall.InterceptEnv {
	var bindings *bind.BindingTable
	if ct.g != nil {
		bindings = ct.g.Bindings()
	}
	if ct.d == nil {
		return canonicalcall.InterceptEnv{Bindings: bindings, ExprType: exprType}
	}
	return canonicalcall.InterceptEnv{
		Scope:     ct.d.baseScope(),
		Manifests: ct.d.cfg.Manifests,
		Bindings:  bindings,
		ExprType:  exprType,
		TypeLookup: func(name string) typ.Type {
			return ct.globalNameType(name)
		},
	}
}

// TypeCastTarget reports whether call is a type-cast/assertion call `T(arg)`.
func (ct callTyper) TypeCastTarget(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) (typ.Type, bool) {
	return canonicalcall.InferTypeCastTarget(call, ct.callInterceptEnv(exprType))
}

// ParamNarrows resolves the callee of call to a module function and returns its
// parameter-narrowing effects. It resolves module-local callees through the same
// symbol/prototype/field identity path as call typing, so wrappers narrow arguments
// without a source-name fallback. A callee that does not resolve to a module
// function is an imported callee: its body-proven refinement rides its imported
// signature (the manifest function summary the module export published), so the
// effects are recovered from that signature's FunctionRefinement instead.
func (ct callTyper) ParamNarrows(call *ast.FuncCallExpr) []transfer.ParamNarrow {
	if call == nil {
		return nil
	}
	prog := ct.d.activeProgram
	if prog == nil {
		return nil
	}
	return canonicalcall.ParamNarrowsForCall(canonicalcall.ParamNarrowsInput{
		Call: call,
		SummaryNarrows: func(call *ast.FuncCallExpr) ([]paramevidence.ParamNarrow, bool) {
			ref, ok := ct.resolveCalleeRef(call, prog)
			if !ok {
				return nil, false
			}
			return ct.d.summaryReader().ParamNarrows(ref), true
		},
		Resolver: ct.callTypeResolver(nil),
	})
}

// IsNoReturn reports whether call's selected callees are all module functions the
// program proved never return normally. A statement call terminates the caller's
// flow only when every selected target is no-return.
func (ct callTyper) IsNoReturn(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) bool {
	if call == nil {
		return false
	}
	d := ct.d
	if d == nil {
		return false
	}
	prog := d.activeProgram
	if prog == nil {
		return false
	}
	return ct.summaryOnlyProductCallOutcome(call, ctx).NeverReturns(func(ref summary.FuncRef) bool {
		return prog.facts.HasNoReturn(ref)
	})
}

// resolveCalleeRef resolves call's callee to its module FuncRef. Method calls
// resolve by receiver/prototype symbol plus method field; non-method calls resolve
// by identifier symbol or static field path. It returns false when no module function
// is named.
func (ct callTyper) resolveCalleeRef(call *ast.FuncCallExpr, prog *program) (summary.FuncRef, bool) {
	refs := ct.resolveCalleeRefs(call, prog, nil)
	if len(refs) == 1 {
		return refs[0], true
	}
	return summary.FuncRef{}, false
}

func (ct callTyper) resolveCalleeRefs(call *ast.FuncCallExpr, prog *program, refs flow.FunctionRefs) []summary.FuncRef {
	return ct.resolveCallTargets(call, prog, refs, nil).DirectRefs()
}

func (ct callTyper) exprPath(expr ast.Expr) (constraint.Path, bool) {
	bindings := ct.bindings()
	if bindings == nil || expr == nil {
		return constraint.Path{}, false
	}
	path := flowpath.FromExprWithBindings(expr, nil, bindings)
	if path.IsEmpty() || path.Symbol == 0 {
		return constraint.Path{}, false
	}
	return path, true
}

func (p *program) fieldFuncRef(sym cfg.SymbolID, field fieldkey.Key) (summary.FuncRef, bool) {
	if p == nil || sym == 0 || field == (fieldkey.Key{}) {
		return summary.FuncRef{}, false
	}
	return p.facts.FieldFuncRef(sym, field)
}

func (p *program) funcRef(sym cfg.SymbolID) (summary.FuncRef, bool) {
	if p == nil || sym == 0 {
		return summary.FuncRef{}, false
	}
	if ref, ok := p.facts.FunctionRef(sym); ok {
		return ref, true
	}
	return p.refBySymbol(sym)
}

func (p *program) ReturnCallHasFiniteTarget(caller summary.FuncRef, call *cfg.CallInfo) bool {
	if p == nil || call == nil {
		return false
	}
	if call.Method != "" {
		method, ok := fieldkey.FromName(call.Method)
		if !ok {
			return false
		}
		if call.CalleePath.Symbol != 0 {
			if _, ok := p.fieldFuncRef(call.CalleePath.Symbol, method); ok {
				return true
			}
		}
		if g := p.Graph(caller); g != nil && call.ReceiverSymbol != 0 {
			if _, ok := p.selfMethodFuncRef(g, call.ReceiverSymbol, method); ok {
				return true
			}
		}
		return false
	}
	if call.CalleeSymbol != 0 {
		if _, ok := p.funcRef(call.CalleeSymbol); ok {
			return true
		}
	}
	if sym, field, ok := directFieldFuncPath(call.CalleePath); ok {
		if _, ok := p.fieldFuncRef(sym, field); ok {
			return true
		}
	}
	return false
}

func directFieldFuncPath(path constraint.Path) (cfg.SymbolID, fieldkey.Key, bool) {
	if path.Symbol == 0 || len(path.Segments) != 1 {
		return 0, fieldkey.Key{}, false
	}
	field, ok := fieldkey.FromSegment(path.Segments[0])
	if !ok {
		return 0, fieldkey.Key{}, false
	}
	return path.Symbol, field, true
}

func (p *program) selfMethodFuncRef(g *cfg.Graph, selfSym cfg.SymbolID, method fieldkey.Key) (summary.FuncRef, bool) {
	if p == nil || g == nil || selfSym == 0 || method == (fieldkey.Key{}) {
		return summary.FuncRef{}, false
	}
	current, ok := p.refByGraph(g)
	if !ok {
		return summary.FuncRef{}, false
	}
	params := g.ParamSymbols()
	for _, receiver := range p.facts.MethodReceivers(current) {
		if receiver.PrototypeSym == 0 || receiver.SelfSlot < 0 || receiver.SelfSlot >= len(params) {
			continue
		}
		if params[receiver.SelfSlot] != selfSym {
			continue
		}
		if ref, ok := p.fieldFuncRef(receiver.PrototypeSym, method); ok {
			return ref, true
		}
	}
	return summary.FuncRef{}, false
}

// bindings returns the calling graph's binding table, or nil when unavailable.
func (ct callTyper) bindings() *bind.BindingTable {
	if ct.g == nil {
		return nil
	}
	return ct.g.Bindings()
}

// funcTyper adapts the driver to the transfer's FuncTyper seam. It resolves a
// function literal's signature in the lexical scope where the literal is defined,
// not the flat module scope: a table-field callback inside `make<T>()` that
// declares `(): T` must store the enclosing generic TypeParam in the product
// value, not an unresolved `typ.Ref("T")`. The inferred lookup reads the same
// summary query signatureForRef uses (the converged returns, or the in-flight
// query's current returns during the solve), which the call-graph fixpoint
// widens, so it is stable inside the solve rather than a second driver pass.
type funcTyper struct {
	d    *Driver
	prog *program
}

func (ft funcTyper) FuncRef(fn *ast.FunctionExpr) (flow.FunctionRef, bool) {
	if ft.d == nil || ft.d.activeProgram == nil || fn == nil {
		return flow.FunctionRef{}, false
	}
	ref, ok := ft.d.activeProgram.refByFunc(fn)
	if !ok {
		return flow.FunctionRef{}, false
	}
	return canonref.ToFlow(ref), true
}

func (ft funcTyper) MethodFuncRef(info *cfg.FuncDefInfo) (flow.FunctionRef, bool) {
	if info == nil || info.FuncExpr == nil {
		return flow.FunctionRef{}, false
	}
	return ft.FuncRef(info.FuncExpr)
}

func (ft funcTyper) CapturedSymbols(ref flow.FunctionRef) []cfg.SymbolID {
	if ft.d == nil || ft.d.activeProgram == nil {
		return nil
	}
	return ft.d.activeProgram.capturedSymbols(canonref.FromFlow(ref))
}

func (ft funcTyper) ReferenceProjection(ref flow.FunctionRef) flow.ReferencePathProjection {
	if ft.d == nil || ft.d.activeProgram == nil {
		return flow.ReferencePathProjection{}
	}
	return ft.d.activeProgram.referenceProjection(canonref.FromFlow(ref))
}

// FuncType builds fn's signature from its declared parameter and return
// annotations, splicing the inferred summary return when fn declares none. The result
// is the structural callable a function-valued table field carries.
func (ft funcTyper) FuncType(fn *ast.FunctionExpr) *typ.Function {
	return ft.build(fn, nil)
}

func (ft funcTyper) build(fn *ast.FunctionExpr, method *cfg.FuncDefInfo) *typ.Function {
	if ft.d == nil || fn == nil {
		return nil
	}
	base := ft.literalBaseScope(fn)
	if method != nil {
		return canonicalsig.Build(canonicalsig.Input{
			Method:          method,
			Base:            base,
			ResolveType:     ft.d.resolveType,
			InferredReturns: ft.d.inferredReturnsForFunction,
			ReturnMode:      canonicalsig.ReturnDeclaredThenInferred,
		})
	}
	return canonicalsig.Build(canonicalsig.Input{
		Function:        fn,
		Base:            base,
		ResolveType:     ft.d.resolveType,
		InferredReturns: ft.d.inferredReturnsForFunction,
		ReturnMode:      canonicalsig.ReturnDeclaredThenInferred,
	})
}

func (ft funcTyper) literalBaseScope(fn *ast.FunctionExpr) *scope.State {
	if ft.d == nil {
		return nil
	}
	base := ft.d.baseScope()
	prog := ft.prog
	if prog == nil {
		prog = ft.d.activeProgram
	}
	if prog == nil || fn == nil {
		return base
	}
	ref, ok := prog.refByFunc(fn)
	if !ok {
		return base
	}
	parent, ok := prog.funcTopology.ParentRef(ref)
	if !ok {
		return base
	}
	parentGraph := prog.Graph(parent)
	if parentGraph == nil {
		return base
	}
	if ft.d.pointScopes != nil {
		if scopes := ft.d.pointScopes[parentGraph.ID()]; scopes != nil {
			if defScope := ft.d.scopeAtNestedDef(scopes, parentGraph, fn); defScope != nil {
				return defScope
			}
		}
	}
	return ft.d.functionContextScopeOver(parentGraph.Func(), base)
}

// inferredReturnTypes is ref's inferred return tuple from the converged summary, or
// its current return tuple from the in-flight summary query during the solve.
// It is the inferred half supplied to canonical signature construction.
func (d *Driver) inferredReturnTypes(ref summary.FuncRef) []typ.Type {
	return d.summaryReader().ReturnTypes(ref)
}

// MethodFuncType builds a method definition's signature with the implicit leading
// `self` parameter typed as the receiver's class. The receiver name `T` in
// `function T:m()` binds the instance contract in the type namespace, so self is
// the named type `T`, and the declared parameter/return annotations follow. This
// is the callable the class field `T.m` holds.
func (ft funcTyper) MethodFuncType(info *cfg.FuncDefInfo) *typ.Function {
	if info == nil {
		return nil
	}
	return ft.build(info.FuncExpr, info)
}

// genericScope registers fn's generic type parameters (<T, U>) on builder and
// returns the resolution scope in which the parameter/return annotations resolve
// those names to type-parameter references. Reusing the type-param scope
// (typ.NewTypeParam + scope.WithTypeParams) makes a generic function's signature
// carry TypeParams, so the call pipeline infers the type arguments from the call's
// argument types (identity(42) instantiates T=number). A non-generic function
// resolves against the module base scope unchanged.
func (d *Driver) genericScope(builder *typ.FunctionBuilder, fn *ast.FunctionExpr) *scope.State {
	return d.genericScopeOver(builder, fn, d.baseScope())
}

// genericScopeOver is genericScope over an explicit base scope, so a nested
// function literal resolves its annotations in the ENCLOSING generic function's
// type-param scope: a table-field function `count = function(self: Collection<T>)`
// inside `M.new<T>()` resolves the captured `T` to the outer function's bounded
// type parameter rather than an unresolved typ.Ref, matching the declared
// `c: Collection<T>` the literal is checked against.
func (d *Driver) genericScopeOver(builder *typ.FunctionBuilder, fn *ast.FunctionExpr, base *scope.State) *scope.State {
	if base == nil {
		base = d.baseScope()
	}
	return canonicalsig.GenericScope(builder, canonicalsig.ScopeInput{
		Function:    fn,
		Base:        base,
		ResolveType: d.resolveType,
	})
}

func (d *Driver) functionContextScopeOver(fn *ast.FunctionExpr, base *scope.State) *scope.State {
	if base == nil {
		base = d.baseScope()
	}
	return canonicalsig.FunctionContextScope(canonicalsig.ScopeInput{
		Function:    fn,
		Base:        base,
		ResolveType: d.resolveType,
	})
}

// typeParamScope is the resolution scope a generic function's own body annotations
// (its parameter and return types) resolve against: the module base scope extended
// with fn's type parameters bound to their bounded typ.TypeParam. It is the same
// scope genericScope builds for the signature, without the signature builder, so a
// `function f<T: Printable>(x: T): T` body resolves `T` to the bounded type
// parameter rather than an unresolved typ.Ref. A non-generic function resolves
// against the base scope unchanged.
func (d *Driver) typeParamScope(fn *ast.FunctionExpr) *scope.State {
	return canonicalsig.TypeParamScope(canonicalsig.ScopeInput{
		Function:    fn,
		Base:        d.baseScope(),
		ResolveType: d.resolveType,
	})
}

func (d *Driver) declaredReturnTypes(fn *ast.FunctionExpr) []typ.Type {
	return canonicalsig.ReturnTypes(canonicalsig.ReturnInput{
		Function:    fn,
		Scope:       d.typeParamScope(fn),
		ResolveType: d.resolveType,
		Mode:        canonicalsig.ReturnDeclaredOnly,
	})
}

func (d *Driver) declaredCallbackOverlaysForRef(prog *program, ref summary.FuncRef) callbackenv.Overlays {
	return callbackenv.OverlaysFromFunction(d.declaredSignatureForRef(prog, ref))
}

// declaredSignatureForRef builds ref's source-declared callable shape only. It
// does not consult return summaries; fact extraction uses it solely as a boundary
// format for declared contract specs.
func (d *Driver) declaredSignatureForRef(prog *program, ref summary.FuncRef) *typ.Function {
	return d.signatureForRefWithMode(prog, ref, canonicalsig.ReturnDeclaredOnly, nil)
}

func (d *Driver) refHasDeclaredReturns(prog *program, ref summary.FuncRef) bool {
	if d == nil || prog == nil {
		return false
	}
	if info := prog.methodDef(ref); info != nil && info.FuncExpr != nil {
		return len(info.FuncExpr.ReturnTypes) > 0
	}
	fn := prog.funcExpr(ref)
	return fn != nil && len(fn.ReturnTypes) > 0
}

// signatureForRef builds the canonical function signature of ref: its declared
// parameter types (resolved annotations; an unannotated parameter is an optional
// gradual `any`) and its return tuple. It is the type a caller observes for the
// function the ref analyzes.
func (d *Driver) signatureForRef(prog *program, ref summary.FuncRef) *typ.Function {
	return d.signatureForRefWithMode(prog, ref, canonicalsig.ReturnDeclaredThenInferred, func(*ast.FunctionExpr) []typ.Type {
		return d.summaryReader().ReturnTypes(ref)
	})
}

func (d *Driver) signatureForRefWithMode(prog *program, ref summary.FuncRef, mode canonicalsig.ReturnMode, inferred func(*ast.FunctionExpr) []typ.Type) *typ.Function {
	if d == nil || prog == nil {
		return nil
	}
	typer := funcTyper{d: d, prog: prog}
	if info := prog.methodDef(ref); info != nil && info.FuncExpr != nil {
		return canonicalsig.Build(canonicalsig.Input{
			Method:          info,
			Base:            typer.literalBaseScope(info.FuncExpr),
			ResolveType:     d.resolveType,
			InferredReturns: inferred,
			ReturnMode:      mode,
		})
	}
	fn := prog.funcExpr(ref)
	if fn == nil {
		return nil
	}
	return canonicalsig.Build(canonicalsig.Input{
		Function:        fn,
		Base:            typer.literalBaseScope(fn),
		ResolveType:     d.resolveType,
		InferredReturns: inferred,
		ReturnMode:      mode,
	})
}

func (d *Driver) inferredReturnsForFunction(fn *ast.FunctionExpr) []typ.Type {
	if d == nil || d.activeProgram == nil || fn == nil {
		return nil
	}
	ref, ok := d.activeProgram.refByFunc(fn)
	if !ok {
		return nil
	}
	return d.inferredReturnTypes(ref)
}

// compile-time assertion: the program implements summary.Program.
var _ summary.Program = (*program)(nil)
