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
// per-function summaries, then projects the frozen artifact to diagnostics.
// A function whose body uses a deferred node kind carries that node's state
// forward unchanged: sound precision loss, never unsoundness, and still
// terminating.
package canonical

import (
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
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/compiler/check/domain/iteration"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
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

	// ComputePasses are compatibility artifact producers over canonical
	// graph/scopes projection. They do not participate in the canonical fixed point.
	ComputePasses []api.ComputePass
}

// Driver runs the canonical type-flow engine over a module.
type Driver struct {
	cfg Config

	// resolver resolves declared type annotations (parameter and return types) in
	// the module's base scope. It is the single seam to the annotation/alias/import
	// machinery, shared by the input builder (parameter contracts) and the
	// diagnostic projection (declared returns and the per-function declared-type map).
	resolver *resolve.Resolver

	// artifact is the frozen result of the latest canonical solve. Post-solve
	// projection/export/diagnostic phases receive this value explicitly instead of
	// consulting driver-owned phase flags.
	artifact canonicalSolveArtifact

	// diagnostics is the latest post-solve exact-observation artifact. It explains
	// contexts and exact states discovered from the frozen Summary snapshot; it is
	// not a semantic Summary producer.
	diagnostics diagnosticObservationArtifact

	// stats is the latest run's solve/projection/cache observability carrier. It
	// records counters only and is never read by transfer or summary logic to choose
	// semantic results.
	stats *summary.Stats

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

	// moduleAliases is the current run's normalized require-alias map. Annotation
	// resolution and solved export typedef projection share it so a qualified
	// typedef and a typeof-backed typedef see the same module namespace.
	moduleAliases  map[cfg.SymbolID]string
	moduleBindings *bind.BindingTable

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

	// activeSummaryReader is the explicit summary read capability for post-solve
	// observers. Live solve readers are rebuilt from activeQueries and the current
	// activeCtx because recursive query evaluation swaps activeCtx through
	// program.WithSolveContext.
	activeSummaryReader    summary.Reader
	activeSummaryReaderSet bool
}

type canonicalSolveArtifact struct {
	Refs      []summary.FuncRef
	States    map[summary.FuncRef]state.FunctionState
	Summaries map[summary.FuncRef]summary.Summary
	Snapshot  summary.CanonicalSummarySnapshot
}

type diagnosticObservationArtifact struct {
	Contexts       map[summary.FuncRef][]summary.Key
	States         map[summary.Key]state.FunctionState
	FunctionStates map[summary.FuncRef]state.FunctionState
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
	return append([]summary.FuncRef(nil), d.artifact.Refs...)
}

// SummaryFor returns the converged interprocedural Summary computed for ref by
// the last Run, and whether ref was analyzed.
func (d *Driver) SummaryFor(ref summary.FuncRef) (summary.Summary, bool) {
	s, ok := d.artifact.Summaries[ref]
	return s, ok
}

func (d *Driver) activeReader() summary.Reader {
	if d == nil {
		return summary.NewReader(nil, nil, nil)
	}
	if d.activeSummaryReaderSet {
		return d.activeSummaryReader
	}
	if d.activeQueries != nil && d.activeCtx != nil {
		return summary.NewReaderWithStats(d.activeQueries, d.activeCtx, d.artifact.Summaries, d.stats)
	}
	return d.activeSummaryReader
}

func (d *Driver) withActiveSummaryReader(reader summary.Reader, run func()) {
	if d == nil || run == nil {
		return
	}
	prev := d.activeSummaryReader
	prevSet := d.activeSummaryReaderSet
	d.activeSummaryReader = reader
	d.activeSummaryReaderSet = true
	defer func() {
		d.activeSummaryReader = prev
		d.activeSummaryReaderSet = prevSet
	}()
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
	d.moduleBindings = moduleBindings
	if store := sess.CanonicalStoreHandle(); store != nil {
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
	d.moduleAliases = moduleAliases
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
	d.stats = summary.NewStats()
	queries := summary.NewWithStats(prog, d.stats)

	// Drive the canonical product equation system by demanding every module
	// summary. The summary solve query evaluates the per-context point/demand
	// cells and the Summary projection in one dependency cycle; a mutually
	// recursive or self-recursive cluster converges from the bottom seed via
	// Summary widening. Per-point diagnostic state is observed afterward by an
	// exact local solve over those converged dependencies.
	// The per-node transfer's call typing resolves callees against the fully built
	// program and runs the call pipeline against this run's query context. Expose
	// them for the solve below, then clear them when the run completes.
	d.activeProgram = prog
	d.activeCtx = sess.Context()
	d.activeQueries = queries
	d.artifact = canonicalSolveArtifact{}
	d.diagnostics = diagnosticObservationArtifact{}
	defer func() {
		d.activeProgram = nil
		d.activeCtx = nil
		d.activeQueries = nil
		d.activeSummaryReader = summary.Reader{}
		d.activeSummaryReaderSet = false
		d.moduleAliases = nil
		d.moduleBindings = nil
	}()
	artifact := d.solvePass(sess, prog, queries)
	d.artifact = artifact
	snapshotReader := summary.NewSnapshotReaderWithStats(artifact.Snapshot, d.stats)
	var diagnostics diagnosticObservationArtifact
	d.withActiveSummaryReader(snapshotReader, func() {
		diagnostics = d.buildDiagnosticObservationArtifact(sess, prog, queries, artifact)
		diagnostics.FunctionStates = d.diagnosticFunctionStates(sess, prog, queries, artifact, &diagnostics)
	})
	d.artifact = artifact
	d.diagnostics = diagnostics
	d.withActiveSummaryReader(snapshotReader, func() {
		d.installFunctionFactProjection(sess, prog, artifact)
		d.projectPublicResults(sess, prog, artifact, diagnostics)
	})
}

func (d *Driver) registerStoreGraphParents(sess api.AnalysisSession, prog *program) {
	if d == nil || sess == nil || prog == nil {
		return
	}
	store := sess.CanonicalStoreHandle()
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

// projectPublicResults populates the session's per-function public results from
// the frozen solve artifact so diagnostic passes run on solved facts.
//
// buildPublicResult documents which fields are direct projections, immutable
// inputs, or intentionally empty. Empty fields are not fabricated precision; a
// diagnostic pass may no-op or report unknown when the public observation surface
// lacks the fact it needs.
func (d *Driver) projectPublicResults(sess api.AnalysisSession, prog *program, artifact canonicalSolveArtifact, diagnostics diagnosticObservationArtifact) {
	results := sess.ResultsMap()
	if results == nil {
		return
	}
	for _, ref := range artifact.Refs {
		fn := prog.funcExpr(ref)
		if fn == nil {
			continue
		}
		result := d.buildPublicResult(sess, prog, artifact, diagnostics, ref)
		results[fn] = result
		if fn == sess.RootFuncNode() {
			sess.SetRootResultValue(result)
		}
		d.emitScopeDepthDiagnostic(sess, fn, result)
	}
}

// buildPublicResult assembles one function's api.FuncResult from the frozen solve
// artifact in the shape the diagnostic passes consume.
//
// Directly projected from canonical inputs and solved facts:
//   - Graph: the function's CFG, the same graph the solve ranged over.
//   - Evidence: the raw graph-event trace (assignments, calls, returns, branches,
//     identifier uses). It is a sound INPUT the canonical input builder already
//     consumes, not a solved fact, so surfacing it to the passes fabricates
//     nothing. It backs the syntactic checks (control flow, identifier presence).
//   - GlobalTypes: the immutable value namespace of predeclared globals.
//
// Solver-shaped carriers not fabricated:
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
func (d *Driver) buildPublicResult(sess api.AnalysisSession, prog *program, artifact canonicalSolveArtifact, diagnostics diagnosticObservationArtifact, ref summary.FuncRef) *api.FuncResult {
	return d.publicResultProjection(sess, prog, artifact, diagnostics, ref).build()
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

	// observationContexts are immutable source/annotation contexts retained for
	// transfer, entry seeding, capture fallback, and diagnostics. All phases read
	// this single carrier instead of parallel declared-type maps.
	observationContexts map[summary.FuncRef]functionObservationContext

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
	// entry projection uses it as a dependency index instead of scanning every module
	// summary while the summary solve is running.
	callerRefsByCallee map[summary.FuncRef][]summary.FuncRef

	// prototypePublishersBySym indexes solved summaries that publish a runtime self
	// value for a specific prototype symbol. A method receiver's fallback entry value
	// reads only publishers for its prototype, not every summary with any prototype
	// publication.
	prototypePublishersBySym map[cfg.SymbolID][]summary.FuncRef

	// prototypeSurfaceMethodsBySym and prototypeMetatablesBySym are immutable
	// program-level method-surface facts. Diagnostic entry projection uses declared
	// method signatures, so it can read these cached surfaces instead of rebuilding
	// the same prototype record/metatable during every observed context.
	prototypeSurfaceMethodsBySym map[cfg.SymbolID][]prototypeSurfaceMethod
	prototypeMetatablesBySym     map[cfg.SymbolID]product.AbstractValue

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

func (p *program) LocalReturnPostconditions(ref summary.FuncRef) paramevidence.ReturnPostconditions {
	tr, ok := p.transfers[ref].(*transfer.Transfer)
	if !ok || tr == nil {
		return paramevidence.ReturnPostconditionsDomain.Bottom()
	}
	return paramevidence.ReturnPostconditionsFromParamNarrows(tr.ParamNarrowEffects())
}

func (p *program) DelegatedReturnPostconditionCalls(ref summary.FuncRef) []paramevidence.DelegatedCall {
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
	obsCtx, ok := p.observationContexts[ref]
	if !ok || obsCtx.declared == nil {
		return nil
	}
	return obsCtx.declared[sym]
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
		driver:              d,
		funcTopology:        funcTopology,
		inputs:              make(map[summary.FuncRef]input.Inputs),
		transfers:           make(map[summary.FuncRef]equation.NodeTransfer),
		params:              make(map[summary.FuncRef]int),
		observationContexts: make(map[summary.FuncRef]functionObservationContext),
		declaredReturns:     make(map[summary.FuncRef][]typ.Type),
		referencePaths:      make(map[summary.FuncRef]flow.ReferencePathProjection),
		refs:                funcTopology.Refs(),
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
	factProgram := d.factProgramProjection(sess, p, moduleAliases).programView()
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
	p.buildPrototypeSurfaceCaches()
}

// addFunction registers one function's parameter count, canonical inputs, and
// projection facts. Function identity and graph ownership come from FunctionTopology.
func (d *Driver) addFunction(sess api.AnalysisSession, p *program, ref summary.FuncRef, g *cfg.Graph) {
	if _, exists := p.inputs[ref]; exists {
		return
	}

	evidence := sess.EvidenceForGraph(g)
	in := input.Build(g, evidence, d.resolveType, d.typeParamScope(g.Func()))
	in.Scope.CellSymbols = functionsymbols.OwnedCapturedByNested(g)
	p.params[ref] = in.Scope.NumParams()
	// The declared types of annotated parameters and annotated locals are the
	// narrowing base the transfer's edge narrowing widens to: a `local r: A|B = {...}`
	// narrows the declared union per edge, not the precise constructor value seeded in
	// the Env. The same resolution the diagnostic surface reads
	// (buildFunctionObservationContext) supplies them; it is annotation-only and
	// does not depend on the solve.
	obsCtx := d.buildFunctionObservationContext(g, evidence)
	// A method/field-definition body's implicit `self` parameter has a declared type
	// only when the receiver is a type-namespace binding (`function T:m()`). Value
	// receivers remain runtime flow facts owned by PrototypeSelf.
	d.seedMethodSelf(&obsCtx, p, g)
	d.seedCapturedDeclaredTypes(&obsCtx, p, ref)
	p.observationContexts[ref] = obsCtx
	in.Scope.DeclaredTypes = obsCtx.declared
	p.inputs[ref] = in
	if fn := p.funcExpr(ref); fn != nil {
		p.declaredReturns[ref] = d.declaredReturnTypes(fn)
	}
}

func (d *Driver) seedCapturedDeclaredTypes(obsCtx *functionObservationContext, p *program, ref summary.FuncRef) {
	if obsCtx == nil || p == nil {
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
		if existing := obsCtx.declared[sym]; existing != nil && !typ.IsAbsentOrUnknown(existing) {
			continue
		}
		for _, owner := range p.captureDependencyChain(ref) {
			ownerCtx, ok := p.observationContexts[owner]
			if !ok {
				continue
			}
			t := ownerCtx.declared[sym]
			if t == nil || typ.IsAbsentOrUnknown(t) {
				continue
			}
			if obsCtx.declared == nil {
				obsCtx.declared = make(map[cfg.SymbolID]typ.Type)
			}
			obsCtx.declared[sym] = t
			if ownerCtx.annotated.Contains(sym) {
				obsCtx.annotated.Add(sym)
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
		in.VariantCaseFieldProjections = facts.VariantCaseFieldProjections
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
	proj := d.transferConfigProjection(p, ref)
	return transfer.New(in, proj.config())
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
	obsCtx, ok := ct.d.activeProgram.observationContexts[ref]
	if !ok || obsCtx.declared == nil {
		for _, entry := range ct.d.activeProgram.facts.CallbackEnv(ref) {
			if entry.Symbol == sym {
				return entry.Type
			}
		}
		return nil
	}
	if t := obsCtx.declared[sym]; t != nil {
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
	key := entry.Key()
	reader := d.activeReader()
	if reader.Live() {
		return reader.SummarizeWithKey(key)
	}
	if sum, ok := reader.ExactSummaryForKey(key); ok {
		return sum
	}
	// Snapshot exact-key absence is an explicit precision downgrade, not an
	// exact-context fallback. The aggregate ref summary is still canonical
	// post-solve evidence and carries context-free facts such as postconditions.
	return reader.Summarize(key.Ref)
}

// ProductCallFromValues is the product-carrier call path. It preserves product
// axes owned by the canonical fixed point (for example gradual-top evidence and
// callee summary return values), projects caller-visible effects from the same
// selected outcome, and keeps type-only signature fallback inside this boundary.
func (ct callTyper) ProductCallFromValues(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) transfer.ProductCallResult {
	frame, ok := ct.callBoundaryFrame(call, ctx, productCallOutcomeOptions{})
	if !ok {
		return transfer.EmptyProductCallResult()
	}
	return frame.productResult()
}

func (ct callTyper) IterVarProjection(iter *ast.FuncCallExpr, count int, exprType func(ast.Expr) typ.Type) (flow.IteratorVarProjection, bool) {
	if iter == nil || count <= 0 || iter.Method != "" || exprType == nil {
		return flow.IteratorVarProjection{}, false
	}
	kind, srcIdx, ok := ct.iteratorKind(iter)
	if !ok || srcIdx < 0 || srcIdx >= len(iter.Args) {
		return flow.IteratorVarProjection{}, false
	}
	source := iter.Args[srcIdx]
	sourceType := exprType(source)
	if ct.rejectImplicitSelfDynamicIteratorSource(source, sourceType) {
		return flow.IteratorVarProjection{}, false
	}
	return flow.ProjectIteratorVarTypes(kind, count, sourceType)
}

func (ct callTyper) rejectImplicitSelfDynamicIteratorSource(source ast.Expr, sourceType typ.Type) bool {
	if !typ.IsAny(unwrap.Underlying(sourceType)) || ct.g == nil {
		return false
	}
	path, ok := ct.exprPath(source)
	if !ok || len(path.Segments) == 0 {
		return false
	}
	for _, slot := range ct.g.ParamSlotsReadOnly() {
		if slot.Symbol == path.Symbol {
			return slot.IsImplicitSelf
		}
	}
	return false
}

// iteratorKind resolves a generic-for iterator's iteration kind and iterated source
// parameter index. It prefers the Iterator effect on the iterator function's contract
// spec (so a user-defined or stdlib iterator with a declared iteration effect types
// its loop variables), falling back to the ipairs/pairs builtin recognition on a
// predeclared global, the documented builtin iteration forms.
func (ct callTyper) iteratorKind(iter *ast.FuncCallExpr) (flow.IteratorKind, int, bool) {
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
	return ct.callInterceptEnv(exprType).TypeCastTarget(call)
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
	return ct.resolveCallTargets(
		call,
		prog,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), refs, flow.ClosureRefsDomain.Bottom()),
	).DirectRefs()
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

// FuncType builds fn's declared callable shape for transfer-owned function
// values. It must not read inferred summary returns: transfer runs inside the
// summary fixed point, and inferred returns are owned by Summary projection and
// post-solve callable observation.
func (ft funcTyper) FuncType(fn *ast.FunctionExpr) *typ.Function {
	return ft.build(fn, nil)
}

func (ft funcTyper) build(fn *ast.FunctionExpr, method *cfg.FuncDefInfo) *typ.Function {
	if ft.d == nil || fn == nil {
		return nil
	}
	base := ft.literalBaseScope(fn)
	if method != nil {
		return canonicalsig.Input{
			Method:      method,
			Base:        base,
			ResolveType: ft.d.resolveType,
			ReturnMode:  canonicalsig.ReturnDeclaredOnly,
		}.Build()
	}
	return canonicalsig.Input{
		Function:    fn,
		Base:        base,
		ResolveType: ft.d.resolveType,
		ReturnMode:  canonicalsig.ReturnDeclaredOnly,
	}.Build()
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
	return d.activeReader().ReturnTypes(ref)
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
	return canonicalsig.ScopeInput{
		Function:    fn,
		Base:        base,
		ResolveType: d.resolveType,
	}.Generic(builder)
}

func (d *Driver) functionContextScopeOver(fn *ast.FunctionExpr, base *scope.State) *scope.State {
	if base == nil {
		base = d.baseScope()
	}
	return canonicalsig.ScopeInput{
		Function:    fn,
		Base:        base,
		ResolveType: d.resolveType,
	}.FunctionContext()
}

// typeParamScope is the resolution scope a generic function's own body annotations
// (its parameter and return types) resolve against: the module base scope extended
// with fn's type parameters bound to their bounded typ.TypeParam. It is the same
// scope genericScope builds for the signature, without the signature builder, so a
// `function f<T: Printable>(x: T): T` body resolves `T` to the bounded type
// parameter rather than an unresolved typ.Ref. A non-generic function resolves
// against the base scope unchanged.
func (d *Driver) typeParamScope(fn *ast.FunctionExpr) *scope.State {
	return canonicalsig.ScopeInput{
		Function:    fn,
		Base:        d.baseScope(),
		ResolveType: d.resolveType,
	}.TypeParams()
}

func (d *Driver) declaredReturnTypes(fn *ast.FunctionExpr) []typ.Type {
	return canonicalsig.ReturnInput{
		Function:    fn,
		Scope:       d.typeParamScope(fn),
		ResolveType: d.resolveType,
		Mode:        canonicalsig.ReturnDeclaredOnly,
	}.Types()
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
		return d.activeReader().ReturnTypes(ref)
	})
}

func (d *Driver) signatureForRefWithMode(prog *program, ref summary.FuncRef, mode canonicalsig.ReturnMode, inferred func(*ast.FunctionExpr) []typ.Type) *typ.Function {
	if d == nil || prog == nil {
		return nil
	}
	typer := funcTyper{d: d, prog: prog}
	if info := prog.methodDef(ref); info != nil && info.FuncExpr != nil {
		return canonicalsig.Input{
			Method:          info,
			Base:            typer.literalBaseScope(info.FuncExpr),
			ResolveType:     d.resolveType,
			InferredReturns: inferred,
			ReturnMode:      mode,
		}.Build()
	}
	fn := prog.funcExpr(ref)
	if fn == nil {
		return nil
	}
	return canonicalsig.Input{
		Function:        fn,
		Base:            typer.literalBaseScope(fn),
		ResolveType:     d.resolveType,
		InferredReturns: inferred,
		ReturnMode:      mode,
	}.Build()
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
