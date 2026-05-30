// Package canonical wires the single-fixed-point type-flow engine into the
// Checker. It is the cutover seam (DAG component 11): the Driver runs the
// canonical engine over a whole module, behind the Checker's WithCanonicalFlow
// opt-in, while the legacy pipeline stays the default.
//
// The engine itself is built as a standalone leaf in the sub-packages:
//
//   - input  assembles one function's Inputs (CFG, raw evidence, scope facts);
//   - transfer is the real per-node value/condition/numeric transfer;
//   - equation solves one function's intraprocedural FunctionState fixed point
//     over the generic worklist (the inner of the two locked fixed points);
//   - summary computes the interprocedural Summary fixed point over the call
//     graph through the db query cycle (the outer fixed point).
//
// The Driver supplies the missing module context the engine's summary.Program
// interface needs: it walks the module's CFG hierarchy the same way the legacy
// driver does (the chunk graph plus every nested function), and derives the call
// graph from each function's call sites. It then drives the interprocedural
// fixed point by summarizing every module function.
//
// SCOPE (component 11a): the Driver RUNS the canonical flow over a whole module
// and proves it TERMINATES (the deadlock fixtures that hang the legacy runSCC
// converge here via the value/numeric widening at loop headers). It computes and
// memoizes the per-function summaries; it does NOT yet bridge the converged
// state to the diagnostic passes — that is component 11b. A function whose body
// uses a node kind the per-node transfer defers carries that node's state forward
// unchanged: sound (precision loss, never unsoundness) and still terminating.
package canonical

import (
	"os"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/keyscoll"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/modules"
	checkphase "github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/intercept"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/resolve"
	"github.com/wippyai/go-lua/compiler/check/synth/transform"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Config supplies the canonical driver's dependencies. It mirrors the subset of
// the legacy pipeline config the canonical engine consumes.
type Config struct {
	// Types resolves operator result types and other type operations. It is the
	// seam for the per-node transfer's operator resolution and the call-return
	// typing a later fidelity pass adds.
	Types core.TypeOps

	// GlobalTypes is the value namespace of predeclared globals (print, pairs,
	// require, and any module-supplied globals). The binder seeds these names so a
	// body reference to one resolves to a global rather than an undefined symbol,
	// exactly as the legacy driver does.
	GlobalTypes map[string]typ.Type

	// Stdlib is the base type scope: the predeclared globals and type aliases a
	// module sees. It is the base scope parameter-annotation resolution reads
	// against.
	Stdlib *scope.State

	// Manifests is the module manifest querier the annotation resolver reads for
	// imported type names. It is the same db the legacy runner resolves against; a
	// nil querier still resolves primitive and structural annotations.
	Manifests io.ManifestQuerier
}

// Driver runs the canonical type-flow engine over a module. It is the canonical
// counterpart of pipeline.Driver and satisfies the same module-driver seam:
// Run(api.AnalysisSession, []ast.Stmt).
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

	// returnMethodWrites is the method fields a nested closure installs on a
	// function's returned table, per return-tuple slot. The return projection rebuilds
	// d.summaries every solve pass, so this cache (computed from the graphs, stable
	// across re-solves) is layered onto the returned slot by ReturnTypes so a caller of
	// `make()` sees the closure-installed methods on the result.
	returnMethodWrites map[summary.FuncRef]map[int][]closureMethodWrite

	// funcExprs maps each ref to the function literal it analyzes, so the bridge
	// stores results into the session keyed by *ast.FunctionExpr, the same key the
	// diagnostic passes range over.
	funcExprs map[summary.FuncRef]*ast.FunctionExpr

	// moduleCaptures is the module-wide type of every capturable symbol, the
	// fallback a nested function's observation surface reads for a variable captured
	// from an enclosing scope (a free variable not declared in the closure body).
	moduleCaptures map[cfg.SymbolID]typ.Type

	// moduleAliasTypes maps a require() alias symbol to its module export type. It is
	// a STATIC fact (the manifest export, no solve needed), built before the solve so
	// the per-node call resolution can type a `time.now()` call whose base `time` is a
	// captured require alias inside a nested function, where the transfer Env tracks
	// no value for the captured free variable.
	moduleAliasTypes map[cfg.SymbolID]typ.Type

	// moduleScope is the base type-name scope every annotation resolves against: the
	// configured Stdlib scope enriched with the module's own `type X = ...`
	// definitions. Without it a named annotation referring to a module-local type
	// (a union alias, a record alias) resolves to an unresolved typ.Ref, which
	// blocks field-on-named-type and discriminant narrowing. It is populated per Run
	// from the module's TypeDef nodes, reusing the legacy scope.EnrichWithTypeDefs
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

	// activeProgram and activeCtx are the in-flight Run's program and db query
	// context. The per-node transfer's call typing (callTyper) reads them while the
	// intraprocedural fixpoint solves: the callee resolution needs the module-wide
	// function signatures (activeProgram), and the call pipeline needs the query
	// context. They are set before the summary loop and cleared after it, so a
	// transfer built in phase 1 resolves calls against the fully built program.
	activeProgram *program
	activeCtx     *db.QueryContext

	// activeQueries is the in-flight Run's interprocedural summary query. The call
	// typing reads a callee's CURRENT return summary through it during the solve so
	// an intra-module callee with no declared return resolves to its inferred return
	// tuple as the call-graph fixpoint converges (the db cycle records the callee
	// dependency, the same way computeSummary closes its Callees edges). It is set
	// before the summary loop and cleared after it.
	activeQueries *summary.Queries

	// derivingContracts guards the body-proven parameter-contract derivation against
	// re-entering the contract narrowing. The derivation resolves a body callee's
	// signature (signatureForRef), which itself applies constrainUnannotatedParams; a
	// callee that is another in-module function would recurse without this guard. The
	// derivation needs only the callee's DECLARED parameter types (an annotated
	// parameter the narrowing never touches), so the base signature suffices while the
	// guard is set.
	derivingContracts bool
}

// NewDriver constructs a canonical driver with the given configuration.
func NewDriver(cfg Config) *Driver {
	return &Driver{
		cfg:      cfg,
		resolver: resolve.New(resolve.Config{Manifests: cfg.Manifests}),
	}
}

// FuncRefs returns the module functions analyzed by the last Run, in discovery
// order (root first, then nested functions in CFG point order).
func (d *Driver) FuncRefs() []summary.FuncRef { return d.refs }

// SummaryFor returns the converged interprocedural Summary computed for ref by
// the last Run, and whether ref was analyzed.
func (d *Driver) SummaryFor(ref summary.FuncRef) (summary.Summary, bool) {
	s, ok := d.summaries[ref]
	return s, ok
}

// FuncExprFor returns the function literal ref analyzes, the key its bridged
// result is stored under in the session.
func (d *Driver) FuncExprFor(ref summary.FuncRef) (*ast.FunctionExpr, bool) {
	fn, ok := d.funcExprs[ref]
	return fn, ok
}

// ReturnTypes is ref's canonical-computed return tuple as concrete types: slot i
// is the value-domain projection (with nilability) of the i-th returned value's
// converged AbstractValue. A slot the transfer does not pin projects to the sound
// over-approximation (value-domain Top -> unknown). It is the canonical fact the
// bridge does not yet route into a legacy diagnostic field; exposing it here keeps
// it measurable for the transfer-fidelity worklist without fabricating a legacy
// structure.
func (d *Driver) ReturnTypes(ref summary.FuncRef) []typ.Type {
	s, ok := d.summaries[ref]
	if !ok || len(s.Returns) == 0 {
		return nil
	}
	out := make([]typ.Type, len(s.Returns))
	for i, av := range s.Returns {
		out[i] = projectValue(av)
	}
	return d.applyReturnMethodWrites(ref, out)
}

// ParamTypes is ref's canonical-computed parameter contracts as concrete types,
// keyed by parameter index: the value-domain projection of the obligation the body
// imposed on each parameter. An absent index is an unconstrained parameter.
func (d *Driver) ParamTypes(ref summary.FuncRef) map[int]typ.Type {
	s, ok := d.summaries[ref]
	if !ok || len(s.Params) == 0 {
		return nil
	}
	out := make(map[int]typ.Type, len(s.Params))
	for i, av := range s.Params {
		out[i] = projectValue(av)
	}
	return out
}

// PointType returns the canonical-computed value type of symbol sym at CFG point
// p in ref's converged state, and whether the env holds a value for it. It reads
// the per-point env the bridge defaults the legacy Facts/FlowSolution structures
// from; a later transfer-fidelity pass routes this into those structures.
func (d *Driver) PointType(ref summary.FuncRef, p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
	fs, ok := d.states[ref]
	if !ok {
		return nil, false
	}
	ps, ok := fs.Points[p]
	if !ok {
		return nil, false
	}
	av, ok := ps.Env[symKey(sym)]
	if !ok || av.IsZero() {
		return nil, false
	}
	return projectValue(av), true
}

// Run analyzes a module chunk with the canonical engine.
//
// It first reproduces the legacy module setup so the session sees the same graph
// hierarchy (root chunk function, bound globals, registered CFG hierarchy), then
// walks the hierarchy to enumerate every module function, builds the
// summary.Program over them, and drives the interprocedural summary fixed point
// by summarizing each function. The fixpoint converges by the engine's lattice
// widening; on a module the legacy flow deadlocks, this returns.
func (d *Driver) Run(sess api.AnalysisSession, chunk []ast.Stmt) {
	if sess == nil {
		return
	}

	root := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}
	sess.SetRootFuncNode(root)

	globals := collectGlobalNames(d.cfg.GlobalTypes)
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
	d.resolver = resolve.New(resolve.Config{
		Manifests:      d.cfg.Manifests,
		ModuleBindings: moduleBindings,
		ModuleAliases:  d.buildModuleAliases(sess, rootGraph),
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
	// signatures) through the same scope the legacy flow puts type defs in.
	d.moduleScope = d.buildModuleScope(sess, rootGraph)

	// Compute the block-aware per-point type-name scope for every graph in the
	// hierarchy: a block-local or forward `type X` is in scope only where Lua's
	// lexical rules make it visible, so a local-declaration annotation and the
	// diagnostic passes resolve a type name against the binding actually visible at
	// the point rather than the flat module scope.
	d.pointScopes = d.buildHierarchyScopes(sess, rootGraph)

	// Resolve each require() alias to its module export type before the solve, so a
	// captured module value (`time` read as `time.now()` inside a nested function)
	// types its calls even where the transfer Env tracks no value for the capture.
	d.moduleAliasTypes = d.buildModuleAliasTypes(sess, rootGraph)

	prog := d.buildProgram(sess, rootGraph)
	queries := summary.New(prog)

	// Drive the interprocedural fixed point: summarizing every module function
	// runs each function's intraprocedural solve (the inner fixed point) and
	// resolves its callees' summaries through the db cycle (the outer fixed
	// point). A mutually recursive or self-recursive cluster is a db query cycle
	// the engine converges from the bottom seed via SummaryWiden.
	d.refs = prog.refs
	d.funcExprs = prog.funcExprs
	// The per-node transfer's call typing resolves callees against the fully built
	// program and runs the call pipeline against this run's query context. Expose
	// them for the solve below, then clear them when the run completes.
	d.activeProgram = prog
	d.activeCtx = sess.Context()
	d.activeQueries = queries
	defer func() { d.activeProgram = nil; d.activeCtx = nil; d.activeQueries = nil }()
	d.solvePass(sess, prog, queries)

	// A nested function reads a free variable captured from an enclosing scope (an
	// upvalue like a builder's `renderer` captured into the closure it returns).
	// The first pass solves with no capture value seeded, so the capture's module-
	// wide converged type is unknown to the closure body and a record built from it
	// collapses to unknown. Now that the first pass has converged, the module-wide
	// capture map is known: seed it into every transfer's capture resolver and re-
	// solve, so the closure body sees the captured value's type. Each re-solve uses
	// a fresh query memo (the captures move the inputs); the loop iterates until the
	// capture map stabilizes, bounded so a non-converging chain still terminates.
	// The capture types are the same module-wide values the observation surface
	// trusts, so this never makes a value more precise than the converged solve.
	// A split-pattern OOP method's implicit `self` is the receiver prototype, which
	// is only enriched with its instance data fields by enrichPrototypeReceivers at
	// the end of the first solve's buildModuleCaptures. Re-seed self from that
	// enriched record (alongside the capture resolvers) so the re-solve types
	// self.field reads against the receiver's proven fields. A named-type receiver was
	// already seeded at construction; this only adds the previously-unresolvable value
	// receivers. The self seed shifts the capture map (the methods read more concrete
	// fields), so it iterates in the same bounded fixpoint as capture seeding.
	prevCaptures := d.moduleCaptures
	for pass := 0; pass < captureRefinePasses; pass++ {
		if len(prevCaptures) == 0 {
			break
		}
		seededCaptures := d.seedCaptureResolvers(prog)
		seededSelf := d.seedMethodSelfFromCaptures(prog)
		seededNarrowBases := d.seedCaptureNarrowBases(prog)
		if !seededCaptures && !seededSelf && !seededNarrowBases {
			break
		}
		queries = summary.New(prog)
		d.activeQueries = queries
		d.solvePass(sess, prog, queries)
		if captureMapEqual(prevCaptures, d.moduleCaptures) {
			break
		}
		prevCaptures = d.moduleCaptures
	}

	// The (value, err) inverse-correlation binds were computed at program-build time
	// from the syntactically certain return forms only: a non-literal value/error
	// return (`return u.email, nil`) was indeterminate, so its callee did not yet
	// prove the pattern. Now that the solve has converged, recompute the binds with
	// the per-return value types the callee proved; a callee whose return slots are
	// non-optional value / nil error (the inverse pattern) now proves it. Re-solve
	// once when any bind changed so the converged state the bridge reads reflects the
	// newly correlated narrowing. The recompute is monotone (it only adds binds a
	// determinate return type proves), so one extra pass suffices.
	if d.recomputeSiblingNilBinds(prog) {
		queries = summary.New(prog)
		d.activeQueries = queries
		d.solvePass(sess, prog, queries)
		// The sibling re-solve refined a callee's converged return (a `local v, err =
		// f()` value narrowed non-nil), which flows into the prototype record a method
		// receiver's `self` is seeded from. The capture-refine loop seeded `self` and the
		// capture resolvers from the PRE-recompute captures, so a sibling method's body
		// (`self:inputs()`) still reads the stale prototype. Re-seed `self` and the
		// capture resolvers from the refreshed captures and re-solve once when either
		// shifted, so a body read of a method whose return the recompute refined sees the
		// converged signature.
		if d.seedMethodSelfFromCaptures(prog) || d.seedCaptureResolvers(prog) {
			queries = summary.New(prog)
			d.activeQueries = queries
			d.solvePass(sess, prog, queries)
		}
	}

	// A local function that returns a freshly-built table whose method fields are
	// installed inside a nested closure (`local function init() obj.m = function...
	// end; init(); return obj`) loses those methods: the field write runs in the
	// closure's graph against the captured `obj`, whose mutation never flows back to
	// the owning function's converged `obj`. Compute the closure-installed method
	// fields per returned slot (from the graphs, now that every closure body is typed),
	// then re-solve once so a caller's `local x = make()` sees the enriched return
	// (ReturnTypes layers the cached method fields onto the projected return slot).
	d.flowBackClosureMethodWrites(prog)
	if len(d.returnMethodWrites) > 0 {
		queries = summary.New(prog)
		d.activeQueries = queries
		d.solvePass(sess, prog, queries)
	}

	d.bridgeResults(sess, prog)
}

// recomputeSiblingNilBinds re-derives every function's (value, err) correlation
// binds using the converged per-return value types and re-installs them on the owning
// transfer, returning whether any transfer's bind set changed. It is the post-solve
// counterpart of the program-build-time bind computation: a callee whose value/error
// return slots are non-literal (a field access, a call) classifies by the type the
// solve proved, which is unavailable before the solve.
func (d *Driver) recomputeSiblingNilBinds(p *program) bool {
	changed := false
	for ref, g := range p.graphs {
		tr, ok := p.transfers[ref].(*transfer.Transfer)
		if !ok || tr == nil {
			continue
		}
		binds := d.siblingNilBinds(p, g)
		if siblingNilBindsEqual(p.siblingNils[ref], binds) {
			continue
		}
		tr.SetSiblingNils(binds)
		p.siblingNils[ref] = binds
		changed = true
	}
	return changed
}

// siblingNilBindsEqual reports whether two sibling-nil bind slices carry the same
// (error symbol -> value symbols) correlations, the change test the post-solve
// recompute iterates against. Order-insensitive on the value symbols within a bind,
// since the bind producer appends them in graph-traversal order.
func siblingNilBindsEqual(a, b []transfer.SiblingNilBind) bool {
	if len(a) != len(b) {
		return false
	}
	index := func(binds []transfer.SiblingNilBind) map[cfg.SymbolID]map[cfg.SymbolID]bool {
		out := make(map[cfg.SymbolID]map[cfg.SymbolID]bool, len(binds))
		for _, bind := range binds {
			set := out[bind.ErrSym]
			if set == nil {
				set = make(map[cfg.SymbolID]bool, len(bind.ValueSyms))
				out[bind.ErrSym] = set
			}
			for _, vs := range bind.ValueSyms {
				set[vs] = true
			}
		}
		return out
	}
	ai, bi := index(a), index(b)
	if len(ai) != len(bi) {
		return false
	}
	for err, aset := range ai {
		bset, ok := bi[err]
		if !ok || len(aset) != len(bset) {
			return false
		}
		for vs := range aset {
			if !bset[vs] {
				return false
			}
		}
	}
	return true
}

// captureRefinePasses bounds the capture-seeding re-solve loop. A closure-captured
// upvalue's type can feed another closure's capture, so seeding shifts the capture
// map and the loop re-solves until the map stabilizes (captureMapEqual). The bound
// is the termination guarantee: it caps the loop whether or not the chain reaches a
// fixed point, so a pathological case never re-solves unboundedly.
const captureRefinePasses = 4

// solvePass drives the interprocedural fixed point over prog with queries, filling
// d.summaries / d.states from the converged solve and computing the module-wide
// capture map. Summarizing every function runs its intraprocedural solve (the
// inner fixed point) and resolves its callees' summaries through the db cycle (the
// outer fixed point); a recursive cluster converges from the bottom seed via
// SummaryWiden.
func (d *Driver) solvePass(sess api.AnalysisSession, prog *program, queries *summary.Queries) {
	d.summaries = make(map[summary.FuncRef]summary.Summary, len(prog.refs))
	d.states = make(map[summary.FuncRef]state.FunctionState, len(prog.refs))
	for _, ref := range prog.refs {
		d.summaries[ref] = queries.Summarize(sess.Context(), ref)
		// The converged per-point state shares SummaryQ's cache entry (IntraQ
		// reuses the same compute), so this reads the already-solved fixed point
		// rather than re-solving it.
		d.states[ref] = queries.Intra(sess.Context(), ref)
	}
	d.moduleCaptures = d.buildModuleCaptures(sess, prog)
}

// seedCaptureResolvers installs the module-wide capture map as each transfer's
// free-variable resolver, so the next solve pass sees a captured upvalue's
// converged type. It returns false when no transfer carries the canonical type and
// nothing changes (no further pass would differ).
func (d *Driver) seedCaptureResolvers(prog *program) bool {
	if len(d.moduleCaptures) == 0 {
		return false
	}
	captures := d.moduleCaptures
	resolve := func(sym cfg.SymbolID) (typ.Type, bool) {
		t, ok := captures[sym]
		if !ok || t == nil || typ.IsAbsentOrUnknown(t) {
			return nil, false
		}
		return t, true
	}
	seeded := false
	for _, tr := range prog.transfers {
		ct, ok := tr.(*transfer.Transfer)
		if !ok || ct == nil {
			continue
		}
		ct.SetCaptureResolver(resolve)
		seeded = true
	}
	return seeded
}

// seedCaptureNarrowBases makes a captured OPTIONAL value narrowable by a body guard.
// A closure that reads a free variable captured from an enclosing scope (a builder's
// `decorator: Decorator?`, a module-level `_services: Services?`) gets no Env slot for
// it: seedEntry seeds only parameters, so a `if decorator then decorator() end` guard
// finds no tracked value and the read stays optional, spuriously raising the
// optional-call / optional-return diagnostic.
//
// The transfer's per-edge guard narrowing (narrowBase) already refines a symbol with
// no Env value when the symbol carries a DECLARED narrowing base: it narrows the
// declared type on the guard edge and writes the result into Env, so a guarded read
// observes the non-nil refinement. This adds each genuine captured optional's
// converged module-wide type to the owning transfer's narrowing-base map (the same
// instance the transfer aliases, retained in p.declaredTypes), so the EXISTING
// narrowing refines the capture exactly as it refines an annotated local.
//
// It is sound: the base is added only as a narrowing source, never as an entry-Env
// value, so an UNGUARDED captured-optional read finds no Env refinement and stays
// optional (its diagnostic still fires). Only a narrowable optional (a type the
// truthy/not-nil guard can shrink) is seeded; a non-optional or already-declared
// symbol is left untouched. It returns whether any base was added.
func (d *Driver) seedCaptureNarrowBases(prog *program) bool {
	if len(d.moduleCaptures) == 0 {
		return false
	}
	added := false
	for ref, g := range prog.graphs {
		if g == nil {
			continue
		}
		base := prog.declaredTypes[ref]
		if base == nil {
			continue
		}
		for sym, t := range d.moduleCaptures {
			if sym == 0 || t == nil || typ.IsAbsentOrUnknown(t) {
				continue
			}
			if _, exists := base[sym]; exists {
				continue
			}
			if !isCapturedInGraph(g, sym) {
				continue
			}
			if _, optional := typ.SplitNilableFieldType(t); !optional {
				continue
			}
			base[sym] = t
			added = true
		}
	}
	return added
}

// isCapturedInGraph reports whether sym is a free variable g reads from an enclosing
// scope: a symbol g neither takes as a parameter nor declares as a local. It mirrors
// the transfer's isCapturedFreeVar classification (a SymbolLocal/SymbolParam is the
// body's own, never a capture), so the narrowing-base seed targets the same symbols
// the capture resolver feeds.
func isCapturedInGraph(g *cfg.Graph, sym cfg.SymbolID) bool {
	if g == nil || sym == 0 {
		return false
	}
	for _, ps := range g.ParamSymbols() {
		if ps == sym {
			return false
		}
	}
	if k, ok := g.SymbolKind(sym); ok && (k == cfg.SymbolLocal || k == cfg.SymbolParam) {
		return false
	}
	return true
}

// seedMethodSelfFromCaptures re-seeds each method body's implicit `self` with the
// receiver record now that the module-wide capture map (and the enriched
// split-pattern prototype it carries) is known. A value-receiver method (the
// split-pattern OOP class: methods live on a bare prototype table sealed onto the
// instance with setmetatable) has no resolvable receiver at program-build time, so
// transfer.New left its self slot unseeded and the first solve typed self.field as
// unknown. enrichPrototypeReceivers ran at the end of the first solve's
// buildModuleCaptures, joining the instance data fields onto the prototype record;
// receiverType now resolves the receiver to that enriched record. Seeding it into
// slot 0 (the implicit self slot, which carries no declared annotation) lets the
// re-solve track self.field reads against the receiver's proven fields and methods.
//
// The self type is injected as the slot-0 inferred-parameter value: seedEntry pins
// an unannotated parameter slot from inferredParamBySlot when declaredParamBySlot has
// no entry, which is exactly the implicit-self case. The function's other inferred
// params are preserved by rebuilding the full slot map. A named-type receiver was
// already seeded at construction (declaredParamBySlot[0]); re-seeding it as inferred
// is a no-op there because declaredParamBySlot wins in seedEntry, so this is safe to
// run for every method. It returns whether any transfer's self seed changed.
func (d *Driver) seedMethodSelfFromCaptures(prog *program) bool {
	if prog == nil || len(prog.methodDefs) == 0 {
		return false
	}
	changed := false
	for ref, g := range prog.graphs {
		if g == nil {
			continue
		}
		selfType := d.methodSelfSeed(prog, g)
		if selfType == nil || typ.IsAbsentOrUnknown(selfType) {
			continue
		}
		tr, ok := prog.transfers[ref].(*transfer.Transfer)
		if !ok || tr == nil {
			continue
		}
		bySlot := d.inferredParamsBySlot(prog, ref)
		if existing, ok := bySlot[0]; ok && existing != nil && existing.String() == selfType.String() {
			continue
		}
		merged := make(map[int]typ.Type, len(bySlot)+1)
		for slot, t := range bySlot {
			merged[slot] = t
		}
		merged[0] = selfType
		tr.SetInferredParams(merged)
		changed = true
	}
	return changed
}

// captureMapEqual reports whether two module-capture maps carry the same symbol
// types, the fixpoint test the capture-seeding loop iterates against. A type
// changes the map when its string form differs (the canonical structural identity
// the rest of the engine compares by).
func captureMapEqual(a, b map[cfg.SymbolID]typ.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for sym, ta := range a {
		tb, ok := b[sym]
		if !ok || ta == nil || tb == nil {
			if (ta == nil) != (tb == nil) || !ok {
				return false
			}
			continue
		}
		if ta.String() != tb.String() {
			return false
		}
	}
	return true
}

// bridgeResults populates the session's per-function results from the converged
// canonical state, so the SAME legacy diagnostic passes (Checker.runPasses) run on
// canonical-computed facts. This is the diagnostic bridge (component 11b): it
// maximizes parity by reusing the diagnostic layer unchanged, so any diagnostic
// difference between the flows reflects ONLY fact divergence (transfer fidelity),
// not a diagnostic-format difference.
//
// What it bridges from the canonical state vs. defaults is documented on the
// field-population helper (buildFuncResult). The defaulted fields are recorded
// transfer/bridge gaps, not fabricated facts. The bridge is scoped, so the diff a
// caller measures has two shapes, both transfer-fidelity worklist items:
//
//   - legacy-only diagnostics: a check the canonical flow cannot yet make because
//     the bridge defaults the fact it reads (the solved-phase pass no-ops);
//   - canonical-only diagnostics: a value-operand pass that runs on the bridged
//     Graph+Evidence reads the empty observation surface as the value-domain
//     unknown and flags it, where the legacy solved facts resolved the concrete
//     type. These resolve as the bridge routes the per-point value facts (return
//     and declared-parameter typing) into the observation surface.
func (d *Driver) bridgeResults(sess api.AnalysisSession, prog *program) {
	results := sess.ResultsMap()
	if results == nil {
		return
	}
	// d.moduleCaptures is already the converged map from the final solve pass; the
	// observation surface and result bridge read it directly.
	for _, ref := range d.refs {
		fn := prog.funcExprs[ref]
		if fn == nil {
			continue
		}
		result := d.buildFuncResult(sess, prog, ref)
		results[fn] = result
		if fn == sess.RootFuncNode() {
			sess.SetRootResultValue(result)
		}
	}
}

// buildModuleCaptures is the module-wide type of every symbol any function may
// capture from an enclosing scope: a symbol's declared (annotated) type, or, when
// it has none, its converged value at its defining function's reachable points.
// A nested function reads a captured variable under the SAME module-global symbol
// id (the session shares one binding table across the hierarchy), so a read inside
// a closure resolves the captured variable's enclosing-scope type from this map.
func (d *Driver) buildModuleCaptures(sess api.AnalysisSession, prog *program) map[cfg.SymbolID]typ.Type {
	out := make(map[cfg.SymbolID]typ.Type)
	for _, ref := range d.refs {
		g := prog.graphs[ref]
		if g == nil {
			continue
		}
		// Declared types (parameters and annotated locals) are the authoritative
		// capture type; they take precedence over an inferred value.
		facts := d.buildFunctionFacts(g, sess.EvidenceForGraph(g))
		for sym, t := range facts.declared {
			if t != nil {
				out[sym] = t
			}
		}
		// Inferred values: a captured local with no annotation takes the value it
		// holds at the defining function's EXIT — the final binding a closure
		// captures, after every field write has accumulated (a class table built up
		// by `function T:m()` definitions carries all its methods at exit, whereas a
		// join over every point would keep only the fields common to all points and
		// drop the methods written late). A symbol not live at exit (a temporary that
		// goes out of scope before the exit) falls back to the join over the points
		// where it is live. A declared symbol is not overwritten.
		fs, ok := d.states[ref]
		if !ok {
			continue
		}
		liveAtExit := make(map[cfg.SymbolID]bool)
		if exitPS, ok := fs.Points[g.Exit()]; ok {
			for key, av := range exitPS.Env {
				sym, ok := symFromKey(key)
				if !ok || av.IsZero() {
					continue
				}
				if _, declared := out[sym]; declared {
					continue
				}
				t := projectValue(av)
				if t == nil || typ.IsUnknown(t) {
					continue
				}
				out[sym] = t
				liveAtExit[sym] = true
			}
		}
		for _, ps := range fs.Points {
			for key, av := range ps.Env {
				sym, ok := symFromKey(key)
				if !ok || av.IsZero() || liveAtExit[sym] {
					continue
				}
				if _, declared := out[sym]; declared {
					continue
				}
				t := projectValue(av)
				if t == nil || typ.IsUnknown(t) {
					continue
				}
				if prev, seen := out[sym]; seen && prev != nil {
					out[sym] = product.Domain.Join(product.FromType(prev), av).ProjectValue()
				} else {
					out[sym] = t
				}
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	zzDumpCaptures(out)
	if os.Getenv("ZCAP") != "" {
		var b *bind.BindingTable
		for _, g := range prog.graphs {
			if g != nil && g.Bindings() != nil {
				b = g.Bindings()
				break
			}
		}
		if b != nil {
			for sym, t := range out {
				if t == nil {
					continue
				}
				zcap("capture sym=%d name=%q type=%s", uint64(sym), b.Name(sym), t.String())
			}
		}
	}
	d.enrichPrototypeReceivers(sess, prog, out)
	return out
}

// instanceData is the data fields collected for one OOP class across its
// setmetatable construction sites, keyed by the prototype receiver symbol.
type instanceData struct {
	fields map[string]*fieldAcc
	count  int
}

// enrichPrototypeReceivers ties a split-pattern OOP class's data fields onto its
// method-prototype receiver. In the split pattern (local methods = {}; local mt =
// {__index = methods}; an instance literal sealed with setmetatable(instance, mt))
// the methods are defined on a bare prototype table while the data fields live on
// the instance. The method receiver `self` resolves to the prototype's converged
// value (the method surface only), so a method body's self.dataField read wrongly
// reports the field absent. At runtime self IS the instance, which sees both the
// data fields and (through __index) the prototype methods.
//
// This pass reconstructs the instance contract from the module's setmetatable call
// sites. The metatable's __index field references the prototype symbol directly in
// the metatable literal ({__index = methods}); that symbol is the method receiver.
// The pass maps each metatable to its __index source symbol (the prototype symbol),
// then at each setmetatable(table, mt) call resolves the table argument's data
// fields and groups them under the prototype symbol the metatable's __index names.
// Each prototype's captured value is then enriched with its class's data fields. A
// data field observed on every construction is required; one seen on only some
// constructions is optional. The prototype's own method fields are preserved, so
// self.method() and self.dataField both resolve.
func (d *Driver) enrichPrototypeReceivers(sess api.AnalysisSession, prog *program, out map[cfg.SymbolID]typ.Type) {
	if len(out) == 0 || prog == nil {
		return
	}
	// metatableIndexSym maps a metatable symbol to the symbol its __index field
	// references in the metatable literal: the class's method prototype.
	metatableIndexSym := d.collectMetatableIndexSymbols(sess, prog)
	if len(metatableIndexSym) == 0 {
		return
	}
	byProto := make(map[cfg.SymbolID]*instanceData)
	for _, ref := range d.refs {
		g := prog.graphs[ref]
		if g == nil {
			continue
		}
		fs, hasState := d.states[ref]
		if !hasState {
			continue
		}
		bindings := g.Bindings()
		if bindings == nil {
			continue
		}
		evidence := sess.EvidenceForGraph(g)
		for _, call := range evidence.Calls {
			info := call.Info
			if info == nil || info.CalleeName != "setmetatable" || len(info.Args) < 2 {
				continue
			}
			metaSym := identSymbol(info.Args[1], bindings)
			protoSym, ok := metatableIndexSym[metaSym]
			if !ok || protoSym == 0 {
				continue
			}
			ps, ok := fs.Points[call.Point]
			if !ok {
				continue
			}
			tableRec, ok := unwrap.Alias(exprValueAt(info.Args[0], g, ps, out)).(*typ.Record)
			if !ok {
				continue
			}
			entry := byProto[protoSym]
			if entry == nil {
				entry = &instanceData{fields: make(map[string]*fieldAcc)}
				byProto[protoSym] = entry
			}
			entry.count++
			for _, f := range tableRec.Fields {
				if f.Name == metaIndexField || isFunctionField(f.Type) {
					continue
				}
				fa := entry.fields[f.Name]
				if fa == nil {
					fa = &fieldAcc{typ: f.Type, optional: f.Optional}
					entry.fields[f.Name] = fa
				} else {
					fa.typ = product.Domain.Join(product.FromType(fa.typ), product.FromType(f.Type)).ProjectValue()
					fa.optional = fa.optional || f.Optional
				}
				fa.count++
			}
		}
	}
	for protoSym, entry := range byProto {
		// A method body's `self.field = value` write widens the field beyond its
		// construction-site initial value: a field initialized `= nil` and reassigned
		// `= T` in a method is `T?`, not the narrow `nil` the literal alone records, so
		// a later `return self.field` reads the union rather than a spurious nil. The
		// write join runs before mergeInstanceFields so the accumulated type carries
		// into the receiver record.
		d.joinSelfFieldWrites(prog, protoSym, entry)
		if len(entry.fields) == 0 {
			continue
		}
		rec, ok := out[protoSym].(*typ.Record)
		if !ok {
			continue
		}
		out[protoSym] = mergeInstanceFields(rec, entry.fields, entry.count)
	}
}

// joinSelfFieldWrites widens a class's instance data fields with the values its
// method bodies write to `self`. A split-pattern class initializes a field at the
// construction site (recorded in entry.fields) but a method commonly reassigns it
// (`self._cache = computed`); reading only the literal types the field at its
// initial value, so a `return self.field` after the write mistypes. This scans every
// method body whose receiver is protoSym for a `self.<name> = value` write and joins
// the written value's converged type into the field accumulator: a field already
// present (a construction field) widens to the union of its init and written types; a
// field written but never constructed is added optional (a method may not run before
// the field is first read). The written value is read from the assignment point's
// converged in-state, the same state the observation surface trusts, so this never
// invents a type more precise than the solve proved.
func (d *Driver) joinSelfFieldWrites(prog *program, protoSym cfg.SymbolID, entry *instanceData) {
	if entry == nil || protoSym == 0 {
		return
	}
	for ref, g := range prog.graphs {
		if g == nil {
			continue
		}
		fn := g.Func()
		if fn == nil {
			continue
		}
		info, ok := prog.methodDefs[fn]
		if !ok || info == nil || info.ReceiverSymbol != protoSym {
			continue
		}
		bindings := g.Bindings()
		if bindings == nil || !phasecore.HasUnannotatedSelfParam(fn, bindings) {
			continue
		}
		params := g.ParamSymbols()
		if len(params) == 0 || params[0] == 0 || bindings.Name(params[0]) != "self" {
			continue
		}
		selfSym := params[0]
		fs, hasState := d.states[ref]
		if !hasState {
			continue
		}
		g.EachAssign(func(p cfg.Point, ai *cfg.AssignInfo) {
			if ai == nil {
				return
			}
			for i := range ai.Targets {
				target := ai.Targets[i]
				if target.Kind != cfg.TargetField || target.BaseSymbol != selfSym || len(target.FieldPath) != 1 {
					continue
				}
				name := target.FieldPath[0]
				if name == "" {
					continue
				}
				ps, ok := fs.Points[p]
				if !ok {
					continue
				}
				vt := exprValueAt(ai.SourceAt(i), g, ps, d.moduleCaptures)
				if vt == nil || typ.IsAbsentOrUnknown(vt) {
					continue
				}
				if fa := entry.fields[name]; fa != nil {
					fa.typ = product.Domain.Join(product.FromType(fa.typ), product.FromType(vt)).ProjectValue()
				} else {
					entry.fields[name] = &fieldAcc{typ: vt, optional: true}
				}
			}
		})
	}
}

// collectMetatableIndexSymbols maps every metatable symbol to the symbol its
// __index field references. It scans the module's table-literal assignments
// (local mt = {__index = methods}) and records the metatable symbol -> __index
// source symbol edge, the static link from a class metatable to its method
// prototype. The shared module binding table gives both symbols the same id in
// every body, so a setmetatable call in any function resolves the prototype its
// metatable names.
func (d *Driver) collectMetatableIndexSymbols(sess api.AnalysisSession, prog *program) map[cfg.SymbolID]cfg.SymbolID {
	out := make(map[cfg.SymbolID]cfg.SymbolID)
	for _, ref := range d.refs {
		g := prog.graphs[ref]
		if g == nil {
			continue
		}
		bindings := g.Bindings()
		if bindings == nil {
			continue
		}
		evidence := sess.EvidenceForGraph(g)
		for _, assign := range evidence.Assignments {
			info := assign.Info
			if info == nil {
				continue
			}
			for i, target := range info.Targets {
				if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
					continue
				}
				src := info.SourceAt(i)
				tbl, ok := src.(*ast.TableExpr)
				if !ok {
					continue
				}
				idxSym := indexFieldSourceSymbol(tbl, bindings)
				if idxSym != 0 {
					out[target.Symbol] = idxSym
				}
			}
		}
	}
	return out
}

// closureMethodWrite is one `<base>.<name> = function(...)` write the driver
// attributes back onto the returned table: the field name, the function value, and
// whether the writing closure is proven to run synchronously before the owning
// function returns (present) or only may run (optional).
type closureMethodWrite struct {
	name    string
	fn      *typ.Function
	present bool
}

// closureInvocation classifies how a method-installing closure is reachable from the
// owning function, the soundness pivot for attributing its writes to the returned
// table. Higher ordinals dominate when a closure is reachable several ways.
type closureInvocation uint8

const (
	// invocationNever: the closure is defined but never invoked nor passed as a
	// value anywhere, so at runtime its writes never run. Its method is genuinely
	// absent and must not be attributed (a method call on it must still error).
	invocationNever closureInvocation = iota
	// invocationConditional: the closure's binding is called only on a path that does
	// not dominate the exit, so a return on the other path runs without it. Its method
	// is attributed OPTIONAL (the value carries the call's nilability).
	invocationConditional
	// invocationInstalled: the closure is unconditionally invoked or registered to run
	// — a callback passed to a runtime (coroutine.spawn / an event registration) or a
	// statement call dominating the exit. Either way it is the object's initializer and
	// its method fields are PRESENT on the returned table. A registered callback is the
	// canonical "wire up the methods" form; only a never-referenced closure (the dead
	// case) is excluded.
	invocationInstalled
)

// flowBackClosureMethodWrites enriches each function's converged return tuple with
// the method fields a nested closure installs on a returned table. The canonical
// transfer types `obj.m = function...` inside the closure that performs it, but the
// closure mutates the captured `obj` cell; the owning function's converged `obj`
// (the value the return projection reads) never sees the write. This pass reattaches
// those method fields onto the returned slot.
//
// Soundness is execution-order-driven, not value-kind-driven: a method field is
// added PRESENT only when its writing closure is a local function the owning body
// invokes by a synchronous statement call that dominates the function exit (so the
// write runs before every return). A closure whose invocation is not proven before
// the return (an async coroutine.spawn callback, a conditional call) contributes the
// field as OPTIONAL, so a method call still resolves the callable while a may-absent
// field keeps its nilability. A field never installed stays absent (a genuinely
// missing method still errors). Only function-valued writes participate; this never
// invents a data field nor a type the closure body did not prove.
func (d *Driver) flowBackClosureMethodWrites(prog *program) {
	if prog == nil {
		return
	}
	ft := funcTyper{d}
	cache := map[summary.FuncRef]map[int][]closureMethodWrite{}
	for _, ref := range d.refs {
		g := prog.graphs[ref]
		if g == nil {
			continue
		}
		returnedSlots := returnedLocalSlots(g)
		if len(returnedSlots) == 0 {
			continue
		}
		bySlot := map[int][]closureMethodWrite{}
		for slot, sym := range returnedSlots {
			writes := d.collectClosureMethodWrites(prog, ft, ref, g, sym)
			if len(writes) == 0 {
				continue
			}
			bySlot[slot] = writes
		}
		if len(bySlot) > 0 {
			cache[ref] = bySlot
		}
	}
	if len(cache) == 0 {
		d.returnMethodWrites = nil
		return
	}
	d.returnMethodWrites = cache
}

// applyReturnMethodWrites layers ref's cached closure-installed method fields onto
// the projected return types: slot i's record gains each method field a nested
// closure installs on the returned table, present or optional per the closure's
// proven execution order. A method already on the record (written in the owning body
// or a literal) is authoritative and not overwritten.
func (d *Driver) applyReturnMethodWrites(ref summary.FuncRef, returns []typ.Type) []typ.Type {
	bySlot, ok := d.returnMethodWrites[ref]
	if !ok || len(bySlot) == 0 {
		return returns
	}
	for slot, writes := range bySlot {
		if slot < 0 || slot >= len(returns) {
			continue
		}
		base := returns[slot]
		if base == nil || typ.IsAbsentOrUnknown(base) {
			continue
		}
		av := product.FromType(base)
		for _, w := range writes {
			if existing, ok := product.FieldOf(av, w.name); ok && !existing.IsZero() {
				continue
			}
			fieldType := typ.Type(w.fn)
			if !w.present {
				fieldType = typ.NewOptional(w.fn)
			}
			av = product.WithField(av, w.name, product.FromType(fieldType))
		}
		returns[slot] = av.ProjectValue()
	}
	return returns
}

// returnedLocalSlots maps each return-tuple slot that returns a local-identifier
// symbol to that symbol. A slot returned under different symbols on different return
// statements is dropped (ambiguous; the closure attribution would be unsound), as is
// a non-identifier slot.
func returnedLocalSlots(g *cfg.Graph) map[int]cfg.SymbolID {
	out := map[int]cfg.SymbolID{}
	conflict := map[int]bool{}
	g.EachReturn(func(_ cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		for i := range info.Symbols {
			sym := info.Symbols[i]
			if sym == 0 {
				continue
			}
			if prev, seen := out[i]; seen && prev != sym {
				conflict[i] = true
				continue
			}
			out[i] = sym
		}
	})
	for i := range conflict {
		delete(out, i)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// collectClosureMethodWrites gathers every `sym.<name> = function...` write across
// the module's graphs (the owning function and the closures that capture sym),
// typing each function value and classifying it present/optional by whether its
// writing closure is proven to run before owner's exit. The shared binding table
// gives sym the same id in every body, so a write in any graph targeting sym is the
// same heap cell the owning function returns.
func (d *Driver) collectClosureMethodWrites(prog *program, ft funcTyper, owner summary.FuncRef, ownerGraph *cfg.Graph, sym cfg.SymbolID) []closureMethodWrite {
	if sym == 0 {
		return nil
	}
	byName := map[string]*closureMethodWrite{}
	order := []string{}
	for wref, wg := range prog.graphs {
		if wg == nil {
			continue
		}
		var inv closureInvocation
		if wref == owner {
			inv = invocationInstalled
		} else {
			inv = d.closureInvocationKind(prog, ownerGraph, wg)
			if inv == invocationNever {
				// A closure never invoked nor passed anywhere never runs; its writes are
				// not attributed (the method stays genuinely absent).
				continue
			}
		}
		wg.EachAssign(func(_ cfg.Point, ai *cfg.AssignInfo) {
			if ai == nil {
				return
			}
			for i := range ai.Targets {
				target := ai.Targets[i]
				if target.Kind != cfg.TargetField || target.BaseSymbol != sym || len(target.FieldPath) != 1 {
					continue
				}
				name := target.FieldPath[0]
				if name == "" {
					continue
				}
				fnExpr, ok := ai.SourceAt(i).(*ast.FunctionExpr)
				if !ok || fnExpr == nil {
					continue
				}
				fn := ft.FuncType(fnExpr)
				if fn == nil {
					continue
				}
				present := inv == invocationInstalled
				w := byName[name]
				if w == nil {
					w = &closureMethodWrite{name: name, fn: fn, present: present}
					byName[name] = w
					order = append(order, name)
					continue
				}
				// Multiple writers of the same field name: the field is present only when
				// every contributing writer is proven to run; the type is the union.
				w.present = w.present && present
				w.fn = unionFunction(w.fn, fn)
			}
		})
	}
	if len(order) == 0 {
		return nil
	}
	out := make([]closureMethodWrite, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out
}

// closureInvocationKind classifies how closure graph wg is reachable from the owning
// function. The closure is identified by its function-binding symbol (the local
// function or assigned binding); an anonymous closure (a coroutine.spawn callback) is
// matched by its literal function expression where it is passed as a call argument.
//
//   - invocationInstalled: the closure's binding is called by a statement that
//     dominates the exit, OR the closure is passed as a call argument (registered as a
//     callback). Either way it runs as the object's initializer, so its method fields
//     are present on the returned table.
//   - invocationConditional: the closure's binding is called only on a path that does
//     not dominate the exit (it may not run before a return), so its method is optional.
//   - invocationNever: the closure is defined but never called nor passed; its writes
//     never run, so its method is not attributed.
func (d *Driver) closureInvocationKind(prog *program, ownerGraph *cfg.Graph, wg *cfg.Graph) closureInvocation {
	wfn := wg.Func()
	if wfn == nil {
		return invocationNever
	}
	var closureSym cfg.SymbolID
	for sym, fn := range prog.funcSyms {
		if fn == wfn {
			closureSym = sym
			break
		}
	}
	result := invocationNever
	raise := func(to closureInvocation) {
		if to > result {
			result = to
		}
	}
	ownerGraph.EachCall(func(p cfg.Point, info *cfg.CallInfo) {
		if result == invocationInstalled || info == nil {
			return
		}
		// A direct call to the closure's binding in the owning body: dominating the
		// exit makes it the proven initializer; a non-dominating call may not run.
		if closureSym != 0 && info.CalleeSymbol == closureSym {
			if graphDominatesExit(ownerGraph, p) {
				raise(invocationInstalled)
			} else {
				raise(invocationConditional)
			}
		}
		// The closure passed as a call argument (a coroutine.spawn callback / an event
		// registration) is wired to run as the object's initializer: installed.
		if callPassesClosure(info, wfn, closureSym, ownerGraph.Bindings()) {
			raise(invocationInstalled)
		}
	})
	// The closure may be registered as a callback from within a nested body (the
	// spawn call sits in the owning body's nested scope); scan every graph for it
	// being passed as a value.
	if result < invocationInstalled {
		for _, g := range prog.graphs {
			if g == nil {
				continue
			}
			g.EachCall(func(_ cfg.Point, info *cfg.CallInfo) {
				if result == invocationInstalled || info == nil {
					return
				}
				if callPassesClosure(info, wfn, closureSym, g.Bindings()) {
					raise(invocationInstalled)
				}
			})
			if result == invocationInstalled {
				break
			}
		}
	}
	return result
}

// callPassesClosure reports whether call info passes the closure (by its literal
// function expression, or by its binding symbol) as one of its arguments — a callback
// the call may invoke. It matches both an inline anonymous closure argument
// (spawn(function() ... end)) and a named closure passed by reference (spawn(init)).
func callPassesClosure(info *cfg.CallInfo, wfn *ast.FunctionExpr, closureSym cfg.SymbolID, bindings *bind.BindingTable) bool {
	if info == nil || info.Call == nil {
		return false
	}
	for i, arg := range info.Call.Args {
		if fnArg, ok := arg.(*ast.FunctionExpr); ok && fnArg == wfn {
			return true
		}
		if closureSym != 0 && i < len(info.ArgSymbols) && info.ArgSymbols[i] == closureSym {
			return true
		}
		if closureSym != 0 && bindings != nil {
			if s := identSymbol(arg, bindings); s == closureSym {
				return true
			}
		}
	}
	return false
}

// unionFunction joins two function field types written under the same name. A method
// table built by two closures with the same field name takes the value-domain union,
// the sound over-approximation of either being installed.
func unionFunction(a, b *typ.Function) *typ.Function {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	joined := product.Domain.Join(product.FromType(a), product.FromType(b)).ProjectValue()
	if fn, ok := joined.(*typ.Function); ok {
		return fn
	}
	return a
}

// indexFieldSourceSymbol returns the symbol the __index field of a table literal
// references ({__index = methods} -> the methods symbol), or 0 when the literal has
// no __index field whose value is a resolvable identifier.
func indexFieldSourceSymbol(tbl *ast.TableExpr, bindings *bind.BindingTable) cfg.SymbolID {
	for _, field := range tbl.Fields {
		if field == nil || field.Key == nil {
			continue
		}
		name, ok := staticFieldKeyName(field.Key)
		if !ok || name != metaIndexField {
			continue
		}
		return identSymbol(field.Value, bindings)
	}
	return 0
}

// staticFieldKeyName returns the static field name of a table-literal key (a string
// literal key or an identifier key), reporting whether the key is static.
func staticFieldKeyName(key ast.Expr) (string, bool) {
	switch k := key.(type) {
	case *ast.StringExpr:
		return k.Value, true
	case *ast.IdentExpr:
		return k.Value, true
	default:
		return "", false
	}
}

// identSymbol resolves an identifier expression to its symbol via a binding table,
// returning 0 for a non-identifier or unresolved expression.
func identSymbol(e ast.Expr, bindings *bind.BindingTable) cfg.SymbolID {
	ident, ok := e.(*ast.IdentExpr)
	if !ok || bindings == nil {
		return 0
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok {
		return 0
	}
	return sym
}

// exprValueAt resolves a setmetatable table/metatable argument's value type at a
// point. An identifier argument (the instance local, or a captured metatable) reads
// from the point's env, falling back to the module-wide captures. A table-literal
// argument (the inline instance `setmetatable({...}, mt)`) builds a record from its
// static field names, typing each from its statically-resolvable value (a record's
// own metatable __index, a captured local) or gradual `any` when the value does not
// resolve — the field is present in the literal regardless. Returns nil so a
// non-resolving argument is skipped rather than fabricated.
func exprValueAt(e ast.Expr, g *cfg.Graph, ps flow.PointState, captures map[cfg.SymbolID]typ.Type) typ.Type {
	if g == nil {
		return nil
	}
	switch ex := e.(type) {
	case *ast.IdentExpr:
		sym := identSymbol(ex, g.Bindings())
		if sym == 0 {
			return nil
		}
		if av, ok := ps.Env[symKey(sym)]; ok && !av.IsZero() {
			return projectValue(av)
		}
		if captures != nil {
			if t, ok := captures[sym]; ok {
				return t
			}
		}
		return nil
	case *ast.TableExpr:
		return tableLiteralRecord(ex, g, ps, captures)
	case *ast.AttrGetExpr:
		base := exprValueAt(ex.Object, g, ps, captures)
		if base == nil || typ.IsAbsentOrUnknown(base) {
			return nil
		}
		name, ok := staticFieldKeyName(ex.Key)
		if !ok || name == "" {
			return nil
		}
		ft, ok := core.Field(base, name)
		if !ok || ft == nil {
			return nil
		}
		return ft
	default:
		return nil
	}
}

// tableLiteralRecord builds the record an inline setmetatable table-literal argument
// denotes: every statically-named field present in the literal, typed from a
// resolvable identifier value or gradual `any`. The field set is the instance's data
// fields; their presence (not their precise types) is what the prototype-receiver
// enrichment needs.
func tableLiteralRecord(tbl *ast.TableExpr, g *cfg.Graph, ps flow.PointState, captures map[cfg.SymbolID]typ.Type) typ.Type {
	builder := typ.NewRecord()
	count := 0
	for _, field := range tbl.Fields {
		if field == nil || field.Key == nil {
			continue
		}
		name, ok := staticFieldKeyName(field.Key)
		if !ok {
			continue
		}
		ft := typ.Any
		if t := exprValueAt(field.Value, g, ps, captures); t != nil && !typ.IsUnknown(t) {
			ft = t
		}
		builder.Field(name, ft)
		count++
	}
	if count == 0 {
		return nil
	}
	return builder.Build()
}

// metaIndexField is the Lua metatable __index slot the canonical receiver
// enrichment reads to link an instance to its method prototype.
const metaIndexField = "__index"

// isFunctionField reports whether a record field's type is a callable (a method),
// distinguishing the prototype's method surface from an instance's data fields.
func isFunctionField(t typ.Type) bool {
	_, ok := t.(*typ.Function)
	return ok
}

// mergeInstanceFields returns proto with the collected instance data fields added.
// A data field observed on every instance construction (its count matches the
// number of instances of the class) is a required field; a field seen on only some
// constructions, or already optional at its source, is optional. A data field whose
// name collides with a prototype method is left as the method (the method surface
// wins). The result is the instance contract the method receiver `self` carries.
func mergeInstanceFields(proto *typ.Record, fields map[string]*fieldAcc, instanceCount int) typ.Type {
	builder := typ.NewRecord()
	existing := make(map[string]bool, len(proto.Fields))
	for _, f := range proto.Fields {
		existing[f.Name] = true
		switch {
		case f.Optional && f.Readonly:
			builder.OptReadonlyField(f.Name, f.Type)
		case f.Optional:
			builder.OptField(f.Name, f.Type)
		case f.Readonly:
			builder.ReadonlyField(f.Name, f.Type)
		default:
			builder.Field(f.Name, f.Type)
		}
	}
	for name, fa := range fields {
		if existing[name] {
			continue
		}
		ft := fa.typ
		if ft == nil {
			ft = typ.Unknown
		}
		if fa.optional || fa.count < instanceCount {
			builder.OptField(name, ft)
		} else {
			builder.Field(name, ft)
		}
	}
	if proto.Metatable != nil {
		builder.Metatable(proto.Metatable)
	}
	if proto.HasMapComponent() {
		builder.MapComponent(proto.MapKey, proto.MapValue)
	}
	return builder.SetOpen(proto.Open).Build()
}

// fieldAcc accumulates one instance data field across a class's constructions: its
// joined type, how many instances carried it, and whether any source was optional.
type fieldAcc struct {
	typ      typ.Type
	count    int
	optional bool
}

// buildFuncResult assembles one function's api.FuncResult from the converged
// canonical state, in the shape the legacy diagnostic passes consume.
//
// BRIDGED from the canonical engine (sound inputs and computed facts):
//   - Graph: the function's CFG, the same graph the canonical solve ranged over.
//   - Evidence: the raw graph-event trace (assignments, calls, returns, branches,
//     identifier uses). It is a sound INPUT the canonical input builder already
//     consumes, not a solved fact, so surfacing it to the passes fabricates
//     nothing. It backs the syntactic checks (control flow, identifier presence).
//   - GlobalTypes: the immutable value namespace of predeclared globals.
//
// DEFAULTED to nil (the recorded transfer/bridge GAPS — the worklist driving
// canonical->legacy parity):
//   - Scopes, BaseScope, FlowInputs: the per-point lexical scope and extracted
//     flow inputs. The canonical state carries per-point env value types, not
//     these legacy structures. WithIdent reads them and falls back to evidence.
//   - Facts, FlowSolution, NarrowSynth, FnRefinement: the legacy Solve/Narrow
//     phase outputs. The canonical flow produces the converged value/numeric/
//     condition state, not these structures. The passes that strictly nil-guard
//     them (WithReturn, WithExhaustiveness on NarrowSynth) no-op. The value-operand
//     pass WithField runs on Graph+Evidence and reads its observation surface from
//     these defaulted fields as the value-domain unknown, so it over-reports
//     arithmetic/comparison on unknown where legacy resolved a concrete type — a
//     canonical-only diagnostic the differential surfaces as a fidelity gap.
//   - TypeOps, QueryContext: the call-typing seam. Left nil until the observation
//     surface above carries facts; populating them now only feeds the same empty
//     surface, so they are deferred with the rest.
//
// The canonical-computed value types (per-point env, return tuple, param
// contracts) are exposed through the Driver's ReturnTypes/ParamTypes/PointType
// accessors rather than forced into a legacy field they do not match — storing
// them under FlowSolution/NarrowSynth would be fabrication. Routing them into the
// observation surface is the transfer-fidelity work this measurement scopes.
func (d *Driver) buildFuncResult(sess api.AnalysisSession, prog *program, ref summary.FuncRef) *api.FuncResult {
	g := prog.graphs[ref]

	var evidence api.FlowEvidence
	if store := sess.StoreHandle(); store != nil && g != nil {
		evidence = store.EvidenceForGraph(g)
	}

	// Observation surface: project the converged FunctionState into the per-point /
	// per-symbol facts and the declared-type inputs the diagnostic passes query
	// through observation.Projector. The flow.Solution stays nil (the canonical flow
	// has no path-sensitive narrowing solution); the Projector reads the per-point
	// facts and declared types instead, which are the canonical-computed types.
	facts := d.buildFunctionFacts(g, evidence)
	// A method/field-definition body's implicit `self` parameter types as the
	// receiver's record (function T:m() / function T.m()): self.f then reads the
	// receiver field's type rather than the gradual default. The receiver type comes
	// from the module-wide converged value of the receiver symbol (or a receiver
	// type-name), so this runs after buildModuleCaptures populated it.
	d.seedMethodSelf(&facts, prog, g)
	funcSigs := d.funcSignatures(prog, g)
	// A named function (a module-level definition or a local-function binding) is a
	// defined identifier wherever it is referenced; the ident pass reads the declared
	// types, so merge the function-binding signatures into the declared-type context
	// (without marking them annotated). This types a recursive self-reference and a
	// forward reference to a sibling function as the function rather than undefined.
	mergeFuncSignaturesIntoDeclared(facts, funcSigs, g)
	result := &api.FuncResult{
		Graph:       g,
		Evidence:    evidence,
		GlobalTypes: d.cfg.GlobalTypes,
		// The return check resolves the function's declared return annotation against
		// BaseScope; a generic function returning `T` must resolve `T` to its bounded
		// type parameter (the same scope its parameter annotations resolved in), or the
		// return type re-resolves to an unresolved typ.Ref and a sound `return x`
		// (x: T) mismatches it. A non-generic function's scope is the module base scope.
		BaseScope: d.returnScope(g),
		Scopes:    d.buildPointScopes(g),
		FlowInputs:        buildObservationInputs(g, facts),
		Facts:             d.newCanonicalFacts(g, d.states[ref], facts, funcSigs, edgeNarrower(prog.transfers[ref])),
		LiteralSignatures: d.buildLiteralSignatures(prog, g),
		TypeOps:           d.cfg.Types,
		QueryContext: func() *db.QueryContext {
			if sess == nil {
				return nil
			}
			return sess.Context()
		}(),
	}
	result.NarrowSynth = &returnSynth{
		driver: d,
		obs:    observation.FromFuncResult(result, nil).WithProofValues().TypeOf,
		ctx:    result.QueryContext,
	}
	// The converged parameter-narrowing effects (an assert/guard wrapper proving a
	// presence/type check on every normal return) and the never-returns-normally
	// fact ARE the function's behavioral refinement. Project them into the legacy
	// FunctionRefinement vocabulary so the module export can publish the same
	// assert-style summary the legacy interproc projection does, letting a
	// cross-module importer narrow the wrapped argument (and its correlated
	// (value, err) siblings) by the imported callee's proven refinement.
	result.FnRefinement = refinementFromParamNarrows(prog.paramNarrows[ref], prog.noReturn[ref])
	return result
}

// refinementFromParamNarrows projects a function's body-proven parameter-narrowing
// effects and never-returns fact into a constraint.FunctionRefinement. Each
// non-equality, non-condition effect proving parameter Param satisfies a
// presence/type check on every normal return becomes an OnReturn constraint rooted
// at the parameter placeholder ($Param) along the effect's field path — the
// relative, assert-style refinement an importer substitutes with the actual
// argument path. A parameter-equality or condition-argument effect carries no
// placeholder-rooted constraint expressible in the OnReturn vocabulary, so it is
// projected as the narrowing-only effect it is (left to the call-graph-local
// ParamNarrow machinery) rather than a fabricated constraint. Terminates carries
// the proven never-returns-normally fact. Returns nil when nothing is proven.
func refinementFromParamNarrows(narrows []transfer.ParamNarrow, terminates bool) *constraint.FunctionRefinement {
	var onReturn []constraint.Constraint
	for _, e := range narrows {
		if e.EqParam >= 0 || e.CondArg || e.CastType != nil || e.Param < 0 {
			continue
		}
		c, ok := paramNarrowConstraint(e)
		if !ok {
			continue
		}
		onReturn = append(onReturn, c)
	}
	if len(onReturn) == 0 && !terminates {
		return nil
	}
	refinement := &constraint.FunctionRefinement{Terminates: terminates}
	if len(onReturn) > 0 {
		refinement.OnReturn = constraint.FromConstraints(onReturn...)
	}
	return refinement
}

// paramNarrowConstraint maps one parameter-narrowing effect to the OnReturn
// constraint it proves: a presence check (CheckNil/CheckNotNil/CheckTruthy/
// CheckFalsy) becomes the matching nil/not-nil/falsy/truthy constraint. The
// constraint is rooted at the parameter placeholder $Param extended by the effect's
// field-path segments, the relative form the importer substitutes with the argument
// path. An effect whose check has no constraint vocabulary (a type-equality check,
// whose narrowing the importer applies through the call-graph-local path rather than
// a published constraint) yields ok=false.
func paramNarrowConstraint(e transfer.ParamNarrow) (constraint.Constraint, bool) {
	path := constraint.ParamPath(e.Param)
	for _, seg := range e.Segments {
		path = path.Append(seg)
	}
	switch e.Check {
	case cfg.CheckNil:
		return constraint.IsNil{Path: path}, true
	case cfg.CheckFalsy:
		return constraint.Falsy{Path: path}, true
	case cfg.CheckNotNil:
		return constraint.NotNil{Path: path}, true
	case cfg.CheckTruthy:
		return constraint.Truthy{Path: path}, true
	default:
		return nil, false
	}
}

// buildLiteralSignatures resolves the declared signature of every function literal
// nested directly in g, so the observation surface types a function-literal expression
// (a callback argument, a table-field function) as its annotated callable rather than
// the empty `fun()`. A method/field-definition literal resolves with its implicit
// `self` typed as the receiver (MethodFuncType); any other literal resolves from its
// own parameter and return annotations (FuncType). The map is the canonical
// counterpart of the legacy nested-analysis LiteralSignatures.
func (d *Driver) buildLiteralSignatures(prog *program, g *cfg.Graph) map[*ast.FunctionExpr]*typ.Function {
	if g == nil {
		return nil
	}
	ft := funcTyper{d}
	// A function literal nested in a generic function captures the enclosing
	// function's type parameters, so its annotations resolve over the enclosing
	// type-param scope: a table-field function `count = function(self: Collection<T>)`
	// inside `M.new<T>()` resolves `T` to M.new's bounded parameter, the same type the
	// declared `local c: Collection<T>` carries, so the literal checks clean.
	enclosing := d.typeParamScope(g.Func())
	out := make(map[*ast.FunctionExpr]*typ.Function)
	for _, nested := range g.NestedFunctions() {
		fn := nested.Func
		if fn == nil {
			continue
		}
		if info, ok := prog.methodDefs[fn]; ok && info != nil {
			if sig := ft.MethodFuncTypeOver(info, enclosing); sig != nil {
				out[fn] = sig
				continue
			}
		}
		if sig := ft.FuncTypeOver(fn, enclosing); sig != nil {
			out[fn] = sig
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// program is the canonical driver's summary.Program: the module's call graph,
// with each function's inputs and per-node transfer assembled once. It is the
// concrete seam the summary fixpoint ranges over.
type program struct {
	graphs    map[summary.FuncRef]*cfg.Graph
	transfers map[summary.FuncRef]equation.NodeTransfer
	params    map[summary.FuncRef]int

	// declaredTypes is each function's narrowing-base map: the declared/annotated
	// types its transfer reads as the base for per-edge guard narrowing. It is the
	// SAME map instance the transfer holds (transfer.New aliases it), so adding a
	// captured optional's converged type here makes the existing narrowBase refine
	// that capture on a guard edge without re-building the transfer. Populated in
	// addFunction; the capture-refine loop extends it with captured optionals once
	// the module-wide capture map is known.
	declaredTypes map[summary.FuncRef]map[cfg.SymbolID]typ.Type

	// funcExprs maps each ref to the function literal it analyzes, the key the
	// diagnostic bridge stores results under (the same *ast.FunctionExpr key the
	// passes range over).
	funcExprs map[summary.FuncRef]*ast.FunctionExpr

	// byName resolves a callee identifier (a function name, a method name, or a
	// local function binding) to the module function it names, so a call site
	// becomes a call-graph edge.
	byName map[string]summary.FuncRef

	// funcSyms maps every function-binding symbol in the module (a function
	// definition, a local-function binding) to the function literal it names. The
	// session shares one binding table across the CFG hierarchy, so a recursive or
	// captured reference to a named function inside any body carries the same symbol
	// id its defining scope assigns. A body read of such a symbol therefore resolves
	// to its callee signature regardless of which graph the read is in — the
	// module-wide closure of the per-graph function-binding map.
	funcSyms map[cfg.SymbolID]*ast.FunctionExpr

	// methodDefs maps a function literal defined as a method or field on a receiver
	// (function T:m() / function T.m()) to its defining FuncDefInfo, so the
	// observation surface types the method's implicit `self` parameter as the
	// receiver's record type rather than the gradual default.
	methodDefs map[*ast.FunctionExpr]*cfg.FuncDefInfo

	// fieldFuncs maps a container symbol (the base of a `function M.f()` definition)
	// to its field functions by field name. The shared binding table gives the
	// container the same symbol in any body that captures it, so a call to a
	// field-path callee (`M.new()`) inside a nested function resolves to the defined
	// function's signature even though the captured container has no value in the
	// nested function's Env.
	fieldFuncs map[cfg.SymbolID]map[string]*ast.FunctionExpr

	// refs is every module function in deterministic discovery order (root first,
	// then nested functions in CFG point order), so the driver summarizes them
	// reproducibly.
	refs []summary.FuncRef

	// paramNarrows is the parameter-narrowing effects each function's body proves on
	// every normal return (a wrapper around assert / `if x == nil then error()`). A
	// call site reads the callee's effects to narrow its arguments. The effects are a
	// syntactic property of the body, computed once when the function is registered
	// and then closed transitively over exit-dominating wrapper calls.
	paramNarrows map[summary.FuncRef][]transfer.ParamNarrow

	// delegatedCalls is each function's exit-dominating calls and the caller-parameter
	// each argument forwards, the input to the transitive closure that propagates a
	// callee's parameter narrowing to a wrapper that forwards a parameter to it.
	delegatedCalls map[summary.FuncRef][]transfer.DelegatedCall

	// inferredParams maps a function ref to the inferred type of each unannotated
	// parameter, joined across the module's call sites. A function with no annotation
	// on a parameter takes the static type its callers pass there (`get_page_data(page)`
	// called with a `Page?` argument types the body's `page` as `Page?` rather than
	// the gradual `any`), so the body's field reads and narrowing have a concrete base.
	inferredParams map[summary.FuncRef]map[int]typ.Type

	// noReturn marks each module function that never returns normally: its body
	// always raises, so a statement call to it terminates the caller's live flow. It
	// is the least fixed point of "every exit path ends in a no-return step": a
	// function whose CFG exit is unreachable (direct error()) seeds it, and a
	// function all of whose exit-dominating calls are no-return inherits it.
	noReturn map[summary.FuncRef]bool

	// siblingNils is each function's currently installed (value, err) correlation
	// binds, the snapshot the post-solve recompute compares against to decide whether
	// the converged per-return types proved a new correlation and a re-solve is needed.
	siblingNils map[summary.FuncRef][]transfer.SiblingNilBind

	// keysCollectors records, for each function that provably collects the keys of one
	// of its parameters (`local keys = {}; for k in pairs(param) do table.insert(keys, k)
	// end; return keys`), which parameter the returned keys table comes from and which
	// return slot carries it. It is detected STRUCTURALLY (keyscoll.DetectKeysCollector,
	// the pattern recognizer — not a name match) once per function at program build,
	// where the function's own evidence is available. A caller reads the callee's entry
	// to instantiate a key-presence fact: a value iterated out of that returned keys
	// table is provably a key of the actual argument the caller passed for that
	// parameter, so a `container[name]` read with that key is present.
	keysCollectors map[summary.FuncRef]*keyscoll.KeysCollectorInfo
}

func (p *program) Graph(ref summary.FuncRef) *cfg.Graph { return p.graphs[ref] }
func (p *program) NumParams(ref summary.FuncRef) int    { return p.params[ref] }

// refByFunc resolves a function literal to its FuncRef, the inverse of funcExprs.
// It is how the diagnostic bridge maps a binding's function node to the ref whose
// summary supplies the function's signature.
func (p *program) refByFunc(fn *ast.FunctionExpr) (summary.FuncRef, bool) {
	for ref, f := range p.funcExprs {
		if f == fn {
			return ref, true
		}
	}
	return summary.FuncRef{}, false
}

func (p *program) Transfer(ref summary.FuncRef) equation.NodeTransfer {
	return p.transfers[ref]
}

// Callees derives ref's call-graph edges by walking every call site in its graph
// and resolving the callee name (or, for a method call with no static callee
// name, the method name) to a module function. Calls to stdlib, imported
// modules, or otherwise unresolved names are not call-graph nodes and are
// skipped: their return is the value-domain default, not a body to summarize.
func (p *program) Callees(ref summary.FuncRef) []summary.FuncRef {
	g := p.graphs[ref]
	if g == nil {
		return nil
	}
	seen := make(map[summary.FuncRef]bool)
	var out []summary.FuncRef
	g.EachCallSite(func(_ cfg.Point, call *cfg.CallInfo) {
		if call == nil {
			return
		}
		name := call.CalleeName
		if name == "" {
			name = call.Method
		}
		if name == "" {
			return
		}
		// A self-edge (callee == ref) is kept: it is the recursion the summary
		// db cycle solves from the bottom seed, not an edge to elide.
		callee, ok := p.byName[name]
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
// assembles the name resolution map used to derive call-graph edges.
//
// It is two phases over one BFS-discovered set of graphs: phase 1 registers
// every function's graph/params/transfer and a function-to-ref index; phase 2
// resolves the names by which the module refers to each function (a definition
// name, a local-function binding, a method name) to that function's ref. The two
// phases keep name resolution independent of discovery order: a call to a
// function defined later in the source still resolves.
func (d *Driver) buildProgram(sess api.AnalysisSession, rootGraph *cfg.Graph) *program {
	p := &program{
		graphs:    make(map[summary.FuncRef]*cfg.Graph),
		transfers: make(map[summary.FuncRef]equation.NodeTransfer),
		params:    make(map[summary.FuncRef]int),
		funcExprs: make(map[summary.FuncRef]*ast.FunctionExpr),
		byName:     make(map[string]summary.FuncRef),
		funcSyms:   make(map[cfg.SymbolID]*ast.FunctionExpr),
		methodDefs:     make(map[*ast.FunctionExpr]*cfg.FuncDefInfo),
		fieldFuncs:     make(map[cfg.SymbolID]map[string]*ast.FunctionExpr),
		paramNarrows:   make(map[summary.FuncRef][]transfer.ParamNarrow),
		delegatedCalls: make(map[summary.FuncRef][]transfer.DelegatedCall),
		noReturn:       make(map[summary.FuncRef]bool),
		inferredParams: make(map[summary.FuncRef]map[int]typ.Type),
		declaredTypes:  make(map[summary.FuncRef]map[cfg.SymbolID]typ.Type),
		keysCollectors: make(map[summary.FuncRef]*keyscoll.KeysCollectorInfo),
	}

	// Phase 1: BFS the hierarchy. Each function's body may define nested
	// functions, whose graphs the session builds on demand. Discovery order is
	// root first, then nested functions in their parent's CFG point order, so
	// p.refs is deterministic.
	funcToRef := make(map[*ast.FunctionExpr]summary.FuncRef)
	type nameBinding struct {
		name string
		fn   *ast.FunctionExpr
	}
	var bindings []nameBinding

	queue := []*cfg.Graph{rootGraph}
	enqueued := map[uint64]bool{rootGraph.ID(): true}
	for len(queue) > 0 {
		g := queue[0]
		queue = queue[1:]

		ref := d.addFunction(sess, p, g)
		if fn := g.Func(); fn != nil {
			funcToRef[fn] = ref
		}

		// Collect the names the enclosing scope gives g's nested functions.
		// function f() / function T.m() / function T:m() come from FuncDefInfo;
		// local function f() comes from LocalFunctionAssignment.
		g.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
			if info == nil || info.Name == "" || info.FuncExpr == nil {
				return
			}
			bindings = append(bindings, nameBinding{name: info.Name, fn: info.FuncExpr})
			if info.Symbol != 0 {
				p.funcSyms[info.Symbol] = info.FuncExpr
			}
			// A method/field definition on a receiver (function T:m(), function T.m())
			// records its FuncDefInfo so the method body's implicit `self` types as the
			// receiver record.
			if (info.TargetKind == cfg.FuncDefMethod || info.TargetKind == cfg.FuncDefField) && info.Receiver != nil {
				p.methodDefs[info.FuncExpr] = info
			}
			// A single-segment field definition (function M.f()) records the field
			// function under its container symbol, so a field-path call (M.f()) inside a
			// body that captures M resolves to f's signature.
			if base := info.TargetPath.Symbol; base != 0 && len(info.TargetPath.Segments) == 1 {
				seg := info.TargetPath.Segments[0]
				if (seg.Kind == constraint.SegmentField || seg.Kind == constraint.SegmentIndexString) && seg.Name != "" {
					byField := p.fieldFuncs[base]
					if byField == nil {
						byField = make(map[string]*ast.FunctionExpr)
						p.fieldFuncs[base] = byField
					}
					if _, taken := byField[seg.Name]; !taken {
						byField[seg.Name] = info.FuncExpr
					}
				}
			}
		})
		for _, lfa := range g.LocalFunctionAssignments() {
			if lfa.Name == "" || lfa.Func == nil {
				continue
			}
			bindings = append(bindings, nameBinding{name: lfa.Name, fn: lfa.Func})
			if lfa.Symbol != 0 {
				p.funcSyms[lfa.Symbol] = lfa.Func
			}
		}

		// A table-literal field holding a function (`local m = { f = function() end }`)
		// is a field function too: a call m.f(...) resolves to the literal. Recording it
		// under the container symbol lets the call-site resolution and the parameter-
		// narrowing effect treat such a method-table wrapper the same as a `function
		// m.f()` definition.
		registerTableFieldFuncs(p, g)

		for _, nested := range g.NestedFunctions() {
			if nested.Func == nil {
				continue
			}
			ng := sess.GetOrBuildCFG(nested.Func)
			if ng == nil || enqueued[ng.ID()] {
				continue
			}
			enqueued[ng.ID()] = true
			queue = append(queue, ng)
		}
	}

	// Phase 2: resolve each collected name to its function's ref. First binding
	// for a name wins (definition order within a scope is deterministic).
	for _, b := range bindings {
		ref, ok := funcToRef[b.fn]
		if !ok {
			continue
		}
		if _, taken := p.byName[b.name]; !taken {
			p.byName[b.name] = ref
		}
	}

	// Phase 3: close the parameter-narrowing effects over wrapper delegation. A
	// function that forwards a parameter to a callee on every normal return inherits
	// the callee's narrowing of that parameter (outerAssert(val) -> innerAssert(val)).
	// The names resolved in phase 2 are required to map each delegated call to its
	// callee ref, so this runs last.
	d.closeParamNarrows(p)

	// Phase 4a: compute the no-return set as the least fixed point over the call
	// graph. A function whose CFG exit is unreachable always raises directly (the
	// builtin error() prunes its successor); a function all of whose exit-dominating
	// calls resolve to no-return functions inherits it. Resolving a callee needs the
	// phase-2 name resolution, so this runs here.
	d.computeNoReturn(p)

	// Phase 4a2: infer each unannotated parameter's type from the module's call sites,
	// then seed it on the owning transfer. A `function f(x)` whose only callers pass a
	// concrete-typed argument types the body's x as that type rather than the gradual
	// any, so x's field reads and narrowing have a real base.
	d.computeInferredParams(p)
	for ref := range p.graphs {
		tr, ok := p.transfers[ref].(*transfer.Transfer)
		if !ok || tr == nil {
			continue
		}
		tr.SetInferredParams(d.inferredParamsBySlot(p, ref))
	}

	// Phase 4b: install the (value, err) inverse-correlation binds. A
	// `local v, err = f()` whose callee f proves the Lua error-return pattern lets a
	// branch proving err nil narrow v to non-nil. Resolving f to prove the pattern
	// needs the phase-2 name resolution, so this runs after the transfers are built
	// and injects the binds.
	p.siblingNils = make(map[summary.FuncRef][]transfer.SiblingNilBind, len(p.graphs))
	for ref, g := range p.graphs {
		tr, ok := p.transfers[ref].(*transfer.Transfer)
		if !ok || tr == nil {
			continue
		}
		binds := d.siblingNilBinds(p, g)
		tr.SetSiblingNils(binds)
		p.siblingNils[ref] = binds
	}

	// Phase 4c: install the local type-predicate guards. A predicate function is
	// defined in one graph but called (and narrows) from a sibling or nested function,
	// so the facts are collected module-wide first, then every transfer shares the
	// module-wide function map and carries its own graph's assigned-result binds. A
	// branch `if P(arg)` / `if ok` (with `local ok = P(arg)`) then narrows the predicate
	// argument to its tested kind on the true edge.
	predicateByFunc := make(map[cfg.SymbolID]transfer.PredicateFact)
	for _, g := range p.graphs {
		collectPredicateFacts(predicateByFunc, sess.EvidenceForGraph(g))
	}
	if len(predicateByFunc) > 0 {
		for ref, g := range p.graphs {
			tr, ok := p.transfers[ref].(*transfer.Transfer)
			if !ok || tr == nil {
				continue
			}
			tr.SetPredicateGuards(predicateByFunc, predicateCondSymBinds(g, predicateByFunc))
		}
	}
	return p
}

// closeParamNarrows propagates parameter-narrowing effects across exit-dominating
// wrapper calls to a fixpoint: when a function forwards its parameter j to a callee
// argument that the callee narrows, j inherits that narrowing. It iterates over the
// recorded delegated calls until no new effect appears (bounded by the finite set of
// (function, parameter, check) triples), so a chain of wrappers and a non-cyclic
// delegation graph both converge.
func (d *Driver) closeParamNarrows(p *program) {
	if len(p.delegatedCalls) == 0 {
		return
	}
	has := func(ref summary.FuncRef, e transfer.ParamNarrow) bool {
		for _, x := range p.paramNarrows[ref] {
			if x.Param == e.Param && x.Check == e.Check && x.EqParam == e.EqParam && sameSegments(x.Segments, e.Segments) {
				return true
			}
		}
		return false
	}
	for changed := true; changed; {
		changed = false
		for ref, calls := range p.delegatedCalls {
			callerGraph := p.graphs[ref]
			for _, dc := range calls {
				calleeRef, ok := d.resolveDelegatedCallee(p, callerGraph, dc.Call)
				if !ok {
					continue
				}
				for _, ce := range p.paramNarrows[calleeRef] {
					// Only a bare-parameter presence/truthy narrowing forwards through a
					// bare-parameter argument; a field-path or equality effect is not
					// propagated (its argument mapping is not a single forwarded parameter).
					if len(ce.Segments) != 0 || ce.EqParam >= 0 {
						continue
					}
					if ce.Param < 0 || ce.Param >= len(dc.ArgParams) {
						continue
					}
					callerParam := dc.ArgParams[ce.Param]
					if callerParam < 0 {
						continue
					}
					inherited := transfer.ParamNarrow{Param: callerParam, Check: ce.Check, EqParam: -1}
					if has(ref, inherited) {
						continue
					}
					p.paramNarrows[ref] = append(p.paramNarrows[ref], inherited)
					changed = true
				}
			}
		}
	}
}

// computeInferredParams fills p.inferredParams with each unannotated parameter's
// type joined across the module's call sites. For every call to a module function,
// each argument's static type (resolved from the caller's defining assignments and
// the callee return signatures, no solve needed) is joined into the matching
// parameter's inferred type when that parameter carries no declared annotation.
func (d *Driver) computeInferredParams(p *program) {
	for _, g := range p.graphs {
		if g == nil {
			continue
		}
		ct := callTyper{d: d, g: g}
		g.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
			if info == nil || info.Call == nil || info.Call.Method != "" {
				return
			}
			calleeRef, ok := ct.resolveCalleeRef(info.Call, p)
			if !ok {
				return
			}
			fn := p.funcExprs[calleeRef]
			if fn == nil {
				return
			}
			for argIdx, arg := range info.Call.Args {
				paramIdx := calleeParamSlotForArg(p, calleeRef, fn, argIdx)
				if paramIdx < 0 || d.paramHasAnnotation(fn, argIdx) {
					continue
				}
				at := d.staticArgType(p, g, arg)
				if at == nil || typ.IsAbsentOrUnknown(at) {
					continue
				}
				d.joinInferredParam(p, calleeRef, paramIdx, at)
			}
		})
	}
}

// calleeParamSlotForArg maps a call argument index to the callee parameter slot it
// fills, accounting for an implicit method-receiver `self` slot the graph's
// parameter layout includes but the source argument list does not. For a plain
// function the slots coincide; the receiver case is excluded by the caller (method
// calls are skipped), so the mapping is identity here.
func calleeParamSlotForArg(p *program, calleeRef summary.FuncRef, fn *ast.FunctionExpr, argIdx int) int {
	if fn == nil || fn.ParList == nil {
		return -1
	}
	if argIdx < 0 || argIdx >= len(fn.ParList.Names) {
		return -1
	}
	return argIdx
}

// paramHasAnnotation reports whether fn's source parameter at index argIdx carries a
// type annotation. An annotated parameter keeps its declared type; only an
// unannotated one takes the inferred call-site type.
func (d *Driver) paramHasAnnotation(fn *ast.FunctionExpr, argIdx int) bool {
	if fn == nil || fn.ParList == nil {
		return false
	}
	if argIdx < 0 || argIdx >= len(fn.ParList.Types) {
		return false
	}
	return fn.ParList.Types[argIdx] != nil
}

// joinInferredParam folds at into the inferred type of calleeRef's parameter slot.
func (d *Driver) joinInferredParam(p *program, calleeRef summary.FuncRef, slot int, at typ.Type) {
	byIdx := p.inferredParams[calleeRef]
	if byIdx == nil {
		byIdx = make(map[int]typ.Type)
		p.inferredParams[calleeRef] = byIdx
	}
	if prev, ok := byIdx[slot]; ok && prev != nil {
		byIdx[slot] = typ.NewUnion(prev, at)
		return
	}
	byIdx[slot] = at
}

// staticArgType resolves a call argument expression's type without the solve. A
// literal/table yields its literal type; an identifier bound by `local x = T:annot`
// or `local x, ... = f()` resolves through the declared annotation or the source
// call's return slot. An expression whose type cannot be pinned statically yields
// nil (no inference, the parameter stays gradual).
func (d *Driver) staticArgType(p *program, g *cfg.Graph, arg ast.Expr) typ.Type {
	switch e := arg.(type) {
	case *ast.IdentExpr:
		return d.staticIdentType(p, g, e)
	default:
		return nil
	}
}

// staticIdentType resolves an identifier's type from its defining assignment in g:
// a declared annotation on the local, or the return-slot type of the source call
// that produced it (`local page, err = load_page()` -> page is load_page's return 0).
func (d *Driver) staticIdentType(p *program, g *cfg.Graph, ident *ast.IdentExpr) typ.Type {
	bindings := g.Bindings()
	if bindings == nil {
		return nil
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return nil
	}
	// A require() alias (`local m = require("mod")`) resolves to the module export
	// type, pre-resolved before the solve. The export is the static type a call site
	// forwards when it passes the alias as an argument.
	if t, ok := d.moduleAliasTypes[sym]; ok && t != nil && !typ.IsAbsentOrUnknown(t) {
		return t
	}
	var result typ.Type
	g.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if result != nil || info == nil {
			return
		}
		for i := range info.Targets {
			target := info.Targets[i]
			if target.Kind != cfg.TargetIdent || target.Symbol != sym {
				continue
			}
			// A declared annotation on the local is authoritative.
			if ann := info.TypeAnnotationAt(i); ann != nil {
				if t := d.resolveType(ann, d.baseScope()); t != nil && !typ.IsAbsentOrUnknown(t) {
					result = t
					return
				}
			}
			// Otherwise the type of the source call's return slot the target binds.
			if call, retIdx := info.CallForTarget(i); call != nil && call.Call != nil {
				rt := d.callReturnSlotType(p, g, call.Call, retIdx)
				zdbg("staticIdent sym=%d retIdx=%d rt=%v", sym, retIdx, rt)
				if rt != nil {
					result = rt
				}
			}
			return
		}
	})
	return result
}

// callReturnSlotType resolves return slot retIdx of a call statically: the callee's
// declared/inferred return signature slot. An in-module callee resolves through its
// ref signature; a cross-module callee through the captured/aliased module member.
func (d *Driver) callReturnSlotType(p *program, g *cfg.Graph, call *ast.FuncCallExpr, retIdx int) typ.Type {
	ct := callTyper{d: d, g: g}
	var sig typ.Type
	if ref, ok := ct.resolveCalleeRef(call, p); ok {
		sig = d.signatureForRef(p, ref)
	}
	if sig == nil {
		sig = d.calleeSignatureFor(ct, call)
	}
	if sig == nil {
		sig = ct.capturedFieldCalleeSignature(call.Func)
	}
	fn := unwrap.Function(sig)
	if fn == nil || retIdx < 0 || retIdx >= len(fn.Returns) {
		return nil
	}
	return fn.Returns[retIdx]
}

// inferredParamsBySlot re-keys ref's inferred parameter types from source-parameter
// index into the graph's parameter SLOT layout (the same re-key seedEntry applies to
// declared types), so an implicit method receiver does not shift the mapping.
func (d *Driver) inferredParamsBySlot(p *program, ref summary.FuncRef) map[int]typ.Type {
	src := p.inferredParams[ref]
	if len(src) == 0 {
		return nil
	}
	g := p.graphs[ref]
	if g == nil {
		return src
	}
	slots := g.ParamSlotsReadOnly()
	if len(slots) == 0 {
		return src
	}
	out := make(map[int]typ.Type, len(src))
	for i, slot := range slots {
		srcIdx, ok := slot.SourceParamIndex()
		if !ok {
			continue
		}
		if t, ok := src[srcIdx]; ok && t != nil {
			out[i] = t
		}
	}
	return out
}

// computeNoReturn fills p.noReturn with the least fixed point of "never returns
// normally" over the call graph. A function's CFG exit being unreachable from its
// entry seeds it (the builtin error() prunes its successor edge, so a body that
// always raises directly has no live exit). A function then inherits it when an
// exit-dominating statement call resolves to an already-no-return function — a
// `function w(x) inner(x) end` wrapper around a no-return inner. The iteration
// terminates because the set only grows and is bounded by the function count.
func (d *Driver) computeNoReturn(p *program) {
	for ref, g := range p.graphs {
		if g == nil {
			continue
		}
		if !graphReachesExit(g, g.Entry()) {
			p.noReturn[ref] = true
		}
		zdbg("noReturn-seed ref=%v reachesExit=%v -> noReturn=%v", ref, graphReachesExit(g, g.Entry()), p.noReturn[ref])
	}
	for changed := true; changed; {
		changed = false
		for ref, g := range p.graphs {
			if g == nil || p.noReturn[ref] {
				continue
			}
			if d.bodyAlwaysRaises(p, g) {
				p.noReturn[ref] = true
				changed = true
			}
		}
	}
}

// bodyAlwaysRaises reports whether g has an exit-dominating statement call to a
// function already known to be no-return: such a call terminates every path to the
// exit, so g itself never returns normally.
func (d *Driver) bodyAlwaysRaises(p *program, g *cfg.Graph) bool {
	ct := callTyper{d: d, g: g}
	raises := false
	g.EachCall(func(point cfg.Point, info *cfg.CallInfo) {
		if raises || info == nil || info.Call == nil {
			return
		}
		if !graphDominatesExit(g, point) {
			return
		}
		ref, ok := ct.resolveCalleeRef(info.Call, p)
		if ok && p.noReturn[ref] {
			raises = true
		}
	})
	return raises
}

// graphReachesExit reports whether g's exit is reachable from p by a forward CFG
// walk. A path that terminates with error() (a node with no successors) does not
// reach the exit, so a body all of whose paths raise has an unreachable exit.
func graphReachesExit(g *cfg.Graph, p cfg.Point) bool {
	if g == nil {
		return false
	}
	exit := g.Exit()
	seen := make(map[cfg.Point]bool)
	stack := []cfg.Point{p}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == exit {
			return true
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		stack = append(stack, g.Successors(cur)...)
	}
	return false
}

// graphDominatesExit reports whether q is on every entry-to-exit path: the exit is
// unreachable from the entry once q is removed.
func graphDominatesExit(g *cfg.Graph, q cfg.Point) bool {
	if g == nil {
		return false
	}
	entry := g.Entry()
	exit := g.Exit()
	if q == entry {
		return true
	}
	seen := make(map[cfg.Point]bool)
	stack := []cfg.Point{entry}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == q || seen[cur] {
			continue
		}
		if cur == exit {
			return false
		}
		seen[cur] = true
		stack = append(stack, g.Successors(cur)...)
	}
	return true
}

// resolveDelegatedCallee resolves a delegated call's callee to its module FuncRef,
// reusing the call-site callee resolution. A callee that is not a module function
// (a stdlib/imported call) reports false.
func (d *Driver) resolveDelegatedCallee(p *program, callerGraph *cfg.Graph, call *ast.FuncCallExpr) (summary.FuncRef, bool) {
	ct := callTyper{d: d, g: callerGraph}
	return ct.resolveCalleeRef(call, p)
}

// sameSegments reports whether two field-segment paths are identical by name.
func sameSegments(a, b []constraint.Segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

// registerTableFieldFuncs indexes the function-valued fields of a table-literal
// assignment (`local m = { f = function() end }`) under the assigned container's
// symbol, so a field-path call m.f(...) resolves to the function literal exactly as a
// `function m.f()` definition does. Only an identifier target with a table-literal
// source and statically named function fields is indexed; a dynamic key or a
// non-function field is skipped.
func registerTableFieldFuncs(p *program, g *cfg.Graph) {
	if g == nil {
		return
	}
	g.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				return
			}
			table, ok := src.(*ast.TableExpr)
			if !ok {
				return
			}
			for _, field := range table.Fields {
				if field == nil || field.Key == nil {
					continue
				}
				name := tableFieldName(field.Key)
				if name == "" {
					continue
				}
				fn, ok := field.Value.(*ast.FunctionExpr)
				if !ok || fn == nil {
					continue
				}
				byField := p.fieldFuncs[target.Symbol]
				if byField == nil {
					byField = make(map[string]*ast.FunctionExpr)
					p.fieldFuncs[target.Symbol] = byField
				}
				if _, taken := byField[name]; !taken {
					byField[name] = fn
				}
			}
		})
	})
}

// tableFieldName resolves a table-literal field key to its static name (a string key
// or an identifier key), or the empty string for a dynamic/positional key.
func tableFieldName(key ast.Expr) string {
	switch k := key.(type) {
	case *ast.StringExpr:
		return k.Value
	case *ast.IdentExpr:
		return k.Value
	default:
		return ""
	}
}

// addFunction registers one function's graph, parameter count, and per-node
// transfer, returning its FuncRef. The transfer is assembled from the function's
// canonical inputs exactly as the standalone engine does.
func (d *Driver) addFunction(sess api.AnalysisSession, p *program, g *cfg.Graph) summary.FuncRef {
	ref := summary.FuncRef{GraphID: g.ID()}
	if _, exists := p.graphs[ref]; exists {
		return ref
	}

	evidence := sess.EvidenceForGraph(g)
	in := input.Build(g, evidence, d.resolveType, d.baseScope())
	p.graphs[ref] = g
	p.params[ref] = in.Scope.NumParams()
	// The declared types of annotated parameters and annotated locals are the
	// narrowing base the transfer's edge narrowing widens to: a `local r: A|B = {...}`
	// narrows the declared union per edge, not the precise constructor value seeded in
	// the Env. The same resolution the diagnostic surface reads (buildFunctionFacts)
	// supplies them; it is annotation-only and does not depend on the solve.
	declared := d.buildFunctionFacts(g, evidence).declared
	// Retain the narrowing-base map the transfer aliases, so the capture-refine loop
	// can add captured optionals to it (the same instance the transfer reads).
	p.declaredTypes[ref] = declared
	// A method body's implicit `self` (function T:m()) is seeded with the receiver's
	// class so the flow tracks self.field reads: without it self is unknown and every
	// captured field, the locals it feeds, and the records it builds collapse to
	// unknown. The receiver class resolves from its type-name binding (the named-type
	// path that needs no converged value), so it is available at program-build time. A
	// value receiver (an anonymous table) has no named binding here and stays unseeded
	// (the sound carry-forward), recovered through the observation surface's capture.
	tr := transfer.New(in, opsResolver{d}, funcTyper{d}, callTyper{d: d, g: g}, d.typeCheckBinds(g), nil, declared, d.methodSelfSeed(p, g))
	// A `expr :: T` cast asserts the operand has the annotated type. Resolve it
	// through the same annotation resolver the parameter and declared-local types
	// use, against the module base scope, so the transfer types a cast operand
	// (e.g. `pairs(cfg :: {[string]: string})`) by its asserted type.
	baseScope := d.baseScope()
	tr.SetCastResolver(func(expr ast.TypeExpr) typ.Type {
		return d.resolveType(expr, baseScope)
	})
	// A bare identifier naming a `type` used as a value (`M.AppError = AppError`)
	// resolves to that type's reified Meta, the same MetaForName rule the synth flow
	// applies, so the field carries the type value (with the built-in `:is` guard).
	tr.SetTypeNameValueResolver(func(name string) typ.Type {
		if meta := baseScope.MetaForName(name); meta != nil {
			return meta
		}
		return nil
	})
	p.transfers[ref] = tr
	// The parameter-narrowing effects (wrapper assert / if-error guards) are a
	// syntactic property of the body the transfer's recognizers extract once here, so
	// a call site can narrow its arguments by the callee's proven param refinements.
	if eff := tr.ParamNarrowEffects(); len(eff) > 0 {
		p.paramNarrows[ref] = eff
	}
	if del := tr.ExitDominatingCalls(); len(del) > 0 {
		p.delegatedCalls[ref] = del
	}
	if fn := g.Func(); fn != nil {
		p.funcExprs[ref] = fn
	}
	// A function whose body is the structural keys-collector pattern (creates a
	// table, iterates a parameter with pairs, inserts each key, returns the table)
	// carries a keys-of-parameter provenance on its returned slot. The recognizer is
	// purely structural; the function's own evidence is available here.
	if info := keyscoll.DetectKeysCollector(g, evidence); info != nil {
		p.keysCollectors[ref] = info
	}
	p.refs = append(p.refs, ref)
	return ref
}

// methodSelfSeed resolves the implicit `self` type to seed a method body's entry
// state with. It applies only to a method/field definition (function T:m()) whose
// self the user did not annotate; an explicitly annotated self yields nil so the
// annotation wins. The receiver type is resolved through receiverType, the single
// source of truth the observation surface also reads: a receiver naming a module
// type resolves immediately (the named-type path needs no converged value), while a
// value receiver (the split-pattern OOP prototype) resolves to the receiver symbol's
// module-wide converged record. At program-build time moduleCaptures is empty, so a
// value receiver yields nil and self stays unseeded (the sound carry-forward); the
// capture-refine re-solve seeds it once the enriched prototype is known. The parent
// graph records the method's FuncDefInfo before this function's graph is dequeued, so
// methodDefs is populated by the time addFunction runs for the method body.
func (d *Driver) methodSelfSeed(p *program, g *cfg.Graph) typ.Type {
	if g == nil {
		return nil
	}
	fn := g.Func()
	if fn == nil {
		return nil
	}
	info, ok := p.methodDefs[fn]
	if !ok || info == nil || info.Receiver == nil {
		return nil
	}
	bindings := g.Bindings()
	if bindings == nil || !phasecore.HasUnannotatedSelfParam(fn, bindings) {
		return nil
	}
	return d.receiverType(info)
}

// collectGlobalNames is the predeclared global names the binder seeds, derived
// from the global value namespace in deterministic order. It mirrors the legacy
// driver's collectGlobalNames over the same source of globals.
func collectGlobalNames(globalTypes map[string]typ.Type) []string {
	if globalTypes == nil {
		return nil
	}
	all := cfg.SortedFieldNames(globalTypes)
	names := make([]string, 0, len(all))
	for _, name := range all {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
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
// return annotation against: the function's type-param scope (the module base scope
// extended with its bounded type parameters) when g is generic, else the plain
// module base scope. A generic function returning a type parameter then resolves it
// to the same bounded parameter its body reads, so a sound `return x` matches the
// declared return.
func (d *Driver) returnScope(g *cfg.Graph) *scope.State {
	if g == nil {
		return d.baseScope()
	}
	return d.typeParamScope(g.Func())
}

// buildModuleScope enriches the configured base scope with every type definition
// the module declares, so a named annotation referring to a module-local type
// resolves structurally. It walks the module's CFG hierarchy (root chunk plus
// every nested function) and applies the legacy scope.EnrichWithTypeDefs over each
// graph's TypeDef nodes, resolving each definition through the same annotation
// resolver the rest of the driver uses. Accumulating across the hierarchy makes a
// module-level alias visible to every function body, matching the legacy flow,
// which seeds each function's base scope from the enclosing type namespace.
func (d *Driver) buildModuleScope(sess api.AnalysisSession, rootGraph *cfg.Graph) *scope.State {
	base := d.cfg.Stdlib
	if d.resolver == nil || rootGraph == nil {
		return base
	}
	defResolver := d.typeDefResolver()

	queue := []*cfg.Graph{rootGraph}
	enqueued := map[uint64]bool{rootGraph.ID(): true}
	for len(queue) > 0 {
		g := queue[0]
		queue = queue[1:]
		base = scope.EnrichWithTypeDefs(g, base, defResolver)
		for _, nested := range g.NestedFunctions() {
			if nested.Func == nil {
				continue
			}
			ng := sess.GetOrBuildCFG(nested.Func)
			if ng == nil || enqueued[ng.ID()] {
				continue
			}
			enqueued[ng.ID()] = true
			queue = append(queue, ng)
		}
	}
	return base
}

// buildModuleAliasTypes resolves each require() alias symbol to its module export
// type from the manifests. It is the static capture type of a require alias,
// available before the solve, so a `time.now()` call whose base is a captured alias
// resolves its member off the module export rather than collapsing to unknown.
func (d *Driver) buildModuleAliasTypes(sess api.AnalysisSession, rootGraph *cfg.Graph) map[cfg.SymbolID]typ.Type {
	aliases := d.buildModuleAliases(sess, rootGraph)
	if len(aliases) == 0 || d.cfg.Manifests == nil {
		return nil
	}
	out := make(map[cfg.SymbolID]typ.Type, len(aliases))
	for sym, path := range aliases {
		if export := io.LookupEnrichedExport(d.cfg.Manifests, path); export != nil && !typ.IsAbsentOrUnknown(export) {
			out[sym] = export
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildModuleAliases collects every require() alias the module introduces (a
// symbol bound by `local m = require("mod")`), across the whole CFG hierarchy. A
// nested function may introduce its own alias under the shared binding table, so
// accumulating across the hierarchy makes a qualified annotation `m.T` resolve in
// any body, matching the legacy session-level alias map the annotation resolver
// reads. The aliases map a binding symbol to the module path so the resolver
// translates the alias name to its manifest path before looking up the type.
func (d *Driver) buildModuleAliases(sess api.AnalysisSession, rootGraph *cfg.Graph) map[cfg.SymbolID]string {
	if rootGraph == nil {
		return nil
	}
	out := make(map[cfg.SymbolID]string)
	queue := []*cfg.Graph{rootGraph}
	enqueued := map[uint64]bool{rootGraph.ID(): true}
	for len(queue) > 0 {
		g := queue[0]
		queue = queue[1:]
		evidence := sess.EvidenceForGraph(g)
		for sym, path := range modules.AliasesFromAssignments(evidence.Assignments, g) {
			out[sym] = path
		}
		for _, nested := range g.NestedFunctions() {
			if nested.Func == nil {
				continue
			}
			ng := sess.GetOrBuildCFG(nested.Func)
			if ng == nil || enqueued[ng.ID()] {
				continue
			}
			enqueued[ng.ID()] = true
			queue = append(queue, ng)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildPointScopes is the per-CFG-point type-name scope the diagnostic passes
// read (the ident pass's type-name-as-value guard, the field pass's named-type
// resolution). It returns g's block-aware per-point scopes (precomputed by
// buildHierarchyScopes): each point carries exactly the type definitions lexically
// visible there, so a bare reference to a module-level type name resolves to that
// type while a block-local or forward type name resolves to nothing outside its
// block / before its definition.
func (d *Driver) buildPointScopes(g *cfg.Graph) map[cfg.Point]*scope.State {
	if g == nil || d.resolver == nil {
		return nil
	}
	if d.pointScopes != nil {
		if scopes, ok := d.pointScopes[g.ID()]; ok {
			return scopes
		}
	}
	return d.computePointScopes(g, d.baseScope())
}

// buildHierarchyScopes computes the block-aware per-point type-name scope for
// every graph in the module's CFG hierarchy, keyed by graph ID. It walks the
// hierarchy from the root chunk; each graph's points are scoped by the legacy
// block-aware RPO walk (ComputeScopes), which enters a child scope at every
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

	type item struct {
		g    *cfg.Graph
		base *scope.State
	}
	// The root chunk's base is the configured stdlib scope (its predeclared globals
	// and imported type names), NOT the flat module scope: the module's own top-level
	// `type X` defs are re-introduced block-aware as ComputeScopes encounters them in
	// RPO, so a block-local or forward type is not visible at points where Lua's
	// lexical rules exclude it. The root chunk fn carries no type parameters, so
	// genericScopeOver leaves the stdlib base unchanged here.
	rootBase := d.genericScopeOver(nil, rootGraph.Func(), d.cfg.Stdlib)
	queue := []item{{g: rootGraph, base: rootBase}}
	enqueued := map[uint64]bool{rootGraph.ID(): true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		scopes := d.computePointScopes(cur.g, cur.base)
		out[cur.g.ID()] = scopes
		// A nested function defined in this graph sees the type namespace visible at
		// its definition point. ComputeScopes pops a block's locals on exit, so the
		// scope at the function-definition point carries the enclosing module-level
		// types but not a sibling block's locals.
		exitScope := cur.base
		if scopes != nil {
			if s, ok := scopes[cur.g.Exit()]; ok && s != nil {
				exitScope = s
			}
		}
		for _, nested := range cur.g.NestedFunctions() {
			if nested.Func == nil {
				continue
			}
			ng := sess.GetOrBuildCFG(nested.Func)
			if ng == nil || enqueued[ng.ID()] {
				continue
			}
			enqueued[ng.ID()] = true
			childBase := exitScope
			if defScope := d.scopeAtNestedDef(scopes, cur.g, nested.Func); defScope != nil {
				childBase = defScope
			}
			childBase = d.genericScopeOver(nil, nested.Func, childBase)
			queue = append(queue, item{g: ng, base: childBase})
		}
	}
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

// computePointScopes runs the legacy block-aware scope walk over g from base,
// resolving each `type X` through the Run's single-sourced typedef resolver so a
// per-point scope shares the same recursive family the module scope carries. The
// walk enters a child scope at every block body and pops it on exit, giving each
// point the type names lexically visible there.
func (d *Driver) computePointScopes(g *cfg.Graph, base *scope.State) map[cfg.Point]*scope.State {
	if g == nil || d.resolver == nil {
		return nil
	}
	defResolver := d.typeDefResolver()
	services := checkphase.ScopeServicesFuncs{
		TypeResolver: func(info *cfg.TypeDefInfo, _ cfg.Point, sc *scope.State) typ.Type {
			if info == nil || info.TypeExpr == nil {
				return nil
			}
			return defResolver(info.Name, info.TypeExpr, scope.ToTypeParamExprs(info.TypeParams), sc)
		},
	}
	return checkphase.ComputeScopes(g, base, services, checkphase.ScopeOptions{})
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

// funcSignatures maps every function-binding symbol visible in g (a nested
// function definition, function-definition statement, or local-function binding)
// to the canonical signature of the function it names. A callee identifier in g's
// body resolves through this map to the callee's converged summary returns, so a
// call's result type is the callee's typed return rather than unknown.
func (d *Driver) funcSignatures(prog *program, g *cfg.Graph) map[cfg.SymbolID]typ.Type {
	if g == nil {
		return nil
	}
	out := make(map[cfg.SymbolID]typ.Type)
	bind := func(sym cfg.SymbolID, fn *ast.FunctionExpr) {
		if sym == 0 || fn == nil {
			return
		}
		ref, ok := prog.refByFunc(fn)
		if !ok {
			return
		}
		if sig := d.signatureForRef(prog, ref); sig != nil {
			out[sym] = sig
		}
	}
	// Module-wide function-binding symbols: a named function is visible under the
	// same symbol id in any body that references it (the shared binding table), so a
	// recursive self-call or a capture of a sibling function resolves to its callee
	// signature. Seeding the whole module's function symbols here (not just g's own
	// definitions) types those references; graph-local definitions below still win
	// when a symbol is rebound locally.
	for sym, fn := range prog.funcSyms {
		bind(sym, fn)
	}

	g.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if info != nil {
			bind(info.Symbol, info.FuncExpr)
		}
	})
	for _, lfa := range g.LocalFunctionAssignments() {
		bind(lfa.Symbol, lfa.Func)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// typeCheckBinds derives the canonical value-narrowing binds for every
// `local val, err = T:is(x)` type-check assignment in g, the canonical counterpart
// of the legacy Type:is PredicateLink. For each such assignment it resolves the
// checked type T (the named type the `:is` receiver refers to), then records that
// the checked argument x and the value target val are proved to be T on the
// err == nil edge. The transfer's NarrowEdge applies the bind so a body read inside
// `if err == nil then ...` observes the checked value as T rather than the gradual
// default. An assignment whose checked type does not resolve produces no bind.
func (d *Driver) typeCheckBinds(g *cfg.Graph) []transfer.TypeCheckBind {
	if g == nil {
		return nil
	}
	bindings := g.Bindings()
	var out []transfer.TypeCheckBind
	g.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for i := range info.Targets {
			target := info.Targets[i]
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			call, retIdx := info.CallForTarget(i)
			if call == nil || retIdx != 1 || !call.IsTypeCheck || call.Method != "is" || call.Receiver == nil {
				continue
			}
			checked := d.typeCheckResultType(call.TypeCheckName)
			if checked == nil {
				continue
			}
			narrowSyms := d.typeCheckNarrowSyms(info, i, retIdx, call, bindings)
			if len(narrowSyms) == 0 {
				continue
			}
			out = append(out, transfer.TypeCheckBind{
				ErrSym:     target.Symbol,
				NarrowSyms: narrowSyms,
				Type:       checked,
			})
		}
	})
	return out
}

// collectPredicateFacts accumulates the module-wide local type-predicate facts from
// evidence into byFunc, keyed by the predicate function's symbol. A predicate is a
// local function whose body returns a builtin `type(param) == kind` test; symbols are
// module-unique, so a predicate defined in one graph is callable (and narrows) from a
// sibling or nested function's body. It is the canonical counterpart of the legacy
// LocalTypePredicateEvidence the ConditionExtractor consumes.
func collectPredicateFacts(byFunc map[cfg.SymbolID]transfer.PredicateFact, evidence api.FlowEvidence) {
	for _, pred := range evidence.LocalTypePredicates {
		if pred.Symbol == 0 || pred.Kind == "" || pred.ParamIndex < 0 {
			continue
		}
		// Keep the first recorded fact per function symbol: the predicate detector emits
		// a single param/kind per function (it breaks on the first matching parameter).
		if _, seen := byFunc[pred.Symbol]; seen {
			continue
		}
		byFunc[pred.Symbol] = transfer.PredicateFact{ParamIndex: pred.ParamIndex, Kind: pred.Kind}
	}
}

// predicateCondSymBinds derives g's assigned-result predicate guards: for each
// `local ok = P(arg)` whose callee P names a recorded PredicateFact, it records the ok
// symbol to the argument narrowing the predicate proves. The transfer's NarrowEdge
// applies it so a branch `if ok` narrows the argument on the true edge, the assigned
// counterpart of the direct-call form `if P(arg)`.
func predicateCondSymBinds(g *cfg.Graph, byFunc map[cfg.SymbolID]transfer.PredicateFact) map[cfg.SymbolID]transfer.PredicateGuard {
	if g == nil || len(byFunc) == 0 {
		return nil
	}
	bindings := g.Bindings()
	byCondSym := make(map[cfg.SymbolID]transfer.PredicateGuard)
	g.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for i := range info.Targets {
			target := info.Targets[i]
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			call, retIdx := info.CallForTarget(i)
			if call == nil || retIdx != 0 || call.Call == nil {
				continue
			}
			argSym, kind, ok := predicateCallNarrow(call.Call, byFunc, bindings)
			if !ok {
				continue
			}
			byCondSym[target.Symbol] = transfer.PredicateGuard{NarrowSym: argSym, Kind: kind}
		}
	})
	if len(byCondSym) == 0 {
		return nil
	}
	return byCondSym
}

// predicateCallNarrow resolves a predicate call `P(arg)` against the recorded
// PredicateFact map to the argument symbol it narrows and the kind it proves. P's
// callee must name a recorded predicate function, and the argument at the tested
// parameter index must resolve to an identifier symbol. A method-like call, an
// unrecognized callee, or a non-identifier argument yields ok=false.
func predicateCallNarrow(call *ast.FuncCallExpr, byFunc map[cfg.SymbolID]transfer.PredicateFact, bindings *bind.BindingTable) (cfg.SymbolID, string, bool) {
	if call == nil || bindings == nil || callsite.IsMethodLikeExpr(call) {
		return 0, "", false
	}
	fnIdent, ok := call.Func.(*ast.IdentExpr)
	if !ok || fnIdent == nil {
		return 0, "", false
	}
	fnSym, ok := bindings.SymbolOf(fnIdent)
	if !ok || fnSym == 0 {
		return 0, "", false
	}
	fact, ok := byFunc[fnSym]
	if !ok || fact.Kind == "" {
		return 0, "", false
	}
	if fact.ParamIndex < 0 || fact.ParamIndex >= len(call.Args) {
		return 0, "", false
	}
	argIdent, ok := call.Args[fact.ParamIndex].(*ast.IdentExpr)
	if !ok || argIdent == nil {
		return 0, "", false
	}
	argSym, ok := bindings.SymbolOf(argIdent)
	if !ok || argSym == 0 {
		return 0, "", false
	}
	return argSym, fact.Kind, true
}

// typeCheckResultType resolves the type a `T:is(x)` guard proves about its checked
// value: the named type T from the module base scope. A name that does not resolve
// to a concrete type yields nil (no narrowing).
func (d *Driver) typeCheckResultType(name string) typ.Type {
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
}

// typeCheckNarrowSyms is the set of symbols a `local val, err = T:is(x)` guard
// proves to be T on the success edge: the value target val (the slot at the same
// call's return index 0) and the checked argument x when it is an identifier. Both
// narrow to T, matching the legacy PredicateLink, which narrows the checked path and
// the inverse value path.
func (d *Driver) typeCheckNarrowSyms(info *cfg.AssignInfo, errTargetIdx, errRetIdx int, call *cfg.CallInfo, bindings *bind.BindingTable) []cfg.SymbolID {
	seen := make(map[cfg.SymbolID]bool)
	var syms []cfg.SymbolID
	add := func(sym cfg.SymbolID) {
		if sym == 0 || seen[sym] {
			return
		}
		seen[sym] = true
		syms = append(syms, sym)
	}
	// The value target shares the call: it is the target whose return index is 0
	// (errTargetIdx maps to retIdx 1, so the value target is errTargetIdx-1).
	valIdx := errTargetIdx - errRetIdx
	if valIdx >= 0 && valIdx < len(info.Targets) {
		vt := info.Targets[valIdx]
		if vt.Kind == cfg.TargetIdent && vt.Symbol != 0 {
			add(vt.Symbol)
		}
	}
	// The checked argument x narrows to T as well.
	if len(call.Args) == 1 && bindings != nil {
		if ident, ok := call.Args[0].(*ast.IdentExpr); ok && ident != nil {
			if sym, ok := bindings.SymbolOf(ident); ok {
				add(sym)
			}
		}
	}
	return syms
}

// siblingNilBinds derives the (value, err) inverse-correlation binds for every
// multi-return assignment `local v, err = f()` in g whose callee f proves the Lua
// error-return pattern. On the edge a later branch proves err nil, the value
// target(s) narrow to non-nil. The convention is the canonical Lua layout: value
// at return slot 0, error at slot 1. The proof is the callee's own body — the
// inverse pattern (success returns `(value, nil)`, failure returns `(nil, error)`),
// or a forward-only `(value, nil)` when the value return slot is optional — so the
// correlation is recorded only when the callee genuinely returns the pair that way.
func (d *Driver) siblingNilBinds(p *program, g *cfg.Graph) []transfer.SiblingNilBind {
	if g == nil {
		return nil
	}
	var out []transfer.SiblingNilBind
	g.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		call, first := info.ExpandingSourceCall()
		zdbg("assign targets=%d sources=%d expanding=%v", len(info.Targets), len(info.Sources), call != nil)
		if call == nil || call.Call == nil {
			return
		}
		// The error target is the slot bound to return index 1 (target first+1); the
		// value targets are the slots bound to return index 0 (target first). A
		// `local v, err = T:is(x)` type-check guard is handled by typeCheckBinds, so
		// skip that specific method-is form; an ordinary call (which the CFG also flags
		// IsTypeCheck syntactically) is the sibling-correlation case handled here.
		if call.IsTypeCheck && call.Method == "is" && call.Receiver != nil {
			return
		}
		valTargetIdx := first
		errTargetIdx := first + 1
		if errTargetIdx >= len(info.Targets) || valTargetIdx < 0 {
			return
		}
		errTarget := info.Targets[errTargetIdx]
		valTarget := info.Targets[valTargetIdx]
		zdbg("  first=%d errKind=%v errSym=%d valKind=%v valSym=%d typecheck=%v", first, errTarget.Kind, errTarget.Symbol, valTarget.Kind, valTarget.Symbol, call.IsTypeCheck)
		if errTarget.Kind != cfg.TargetIdent || errTarget.Symbol == 0 {
			return
		}
		if valTarget.Kind != cfg.TargetIdent || valTarget.Symbol == 0 {
			return
		}
		proves := d.calleeProvesErrorReturn(p, g, call)
		zdbg("siblingBind callee=%q errSym=%d valSym=%d proves=%v", call.CalleeName, errTarget.Symbol, valTarget.Symbol, proves)
		if !proves {
			return
		}
		out = append(out, transfer.SiblingNilBind{
			ErrSym:    errTarget.Symbol,
			ValueSyms: []cfg.SymbolID{valTarget.Symbol},
		})
	})
	return out
}

// calleeProvesErrorReturn reports whether the callee of call proves the Lua
// (value, err) inverse pattern at slots (0, 1). It first reads an ErrorReturn label
// off the resolved callee type (the label a synthesized signature may carry), then
// falls back to proving the pattern from the callee's own body when the callee is a
// module function whose graph this program holds.
func (d *Driver) calleeProvesErrorReturn(p *program, callerGraph *cfg.Graph, call *cfg.CallInfo) bool {
	ct := callTyper{d: d, g: callerGraph}
	if ref, ok := ct.resolveCalleeRef(call.Call, p); ok {
		if cg := p.graphs[ref]; cg != nil {
			return d.provesErrorReturnFromBody(p, cg, ref, d.signatureForRef(p, ref))
		}
	}
	// A cross-module or otherwise type-resolved callee: read the convention off the
	// resolved signature's ErrorReturn label when present.
	return signatureHasErrorReturn(d.calleeSignatureFor(ct, call.Call))
}

// calleeSignatureFor resolves a call's callee to a function signature for
// correlation lookup, trying the callee-identifier binding, then a field-path
// callee defined in this program (M.f), then a field-path callee whose base is an
// imported module value (`service.get_email`, base bound by require()). It is the
// annotation/summary-resolved signature, not the live Env value, so it is available
// at program-build time: the imported-module path reads the module alias type, which
// the driver resolves before the program is built and which carries the exported
// signature's ErrorReturn label.
func (d *Driver) calleeSignatureFor(ct callTyper, call *ast.FuncCallExpr) typ.Type {
	if call == nil {
		return nil
	}
	if ident, ok := call.Func.(*ast.IdentExpr); ok && ident != nil {
		if sig := ct.calleeSignature(ident); sig != nil {
			return sig
		}
	}
	if sig := ct.fieldCalleeSignature(call.Func); sig != nil {
		return sig
	}
	return ct.capturedFieldCalleeSignature(call.Func)
}

// provesErrorReturnFromBody proves the (value, err) inverse pattern at slots (0, 1)
// from a callee graph's return statements. Each return is classified by the nil
// state of its value and error slots: a success return is `(non-nil, nil)`, a
// failure return is `(nil, non-nil)`; any other shape (both non-nil, both nil, or
// indeterminate) is inconsistent and voids the proof. A body that exhibits both a
// success and a failure return proves the full bidirectional correlation; a body
// that only ever succeeds proves the forward direction (err == nil => value != nil)
// only when the declared value return slot is optional, so the present-narrowing
// does not fabricate non-nil for a slot the callee never returns nil for.
//
// A non-literal return slot (a field access `return u.email, nil`, an identifier, a
// call) classifies by the converged value type the callee's solve proved at that
// return point: a non-optional value type is nonNil, an exactly-nil type is nil, an
// optional/unknown type stays indeterminate (voids the proof). The state is read
// from d.states[ref] when it is populated; at pre-solve program-build time it is
// absent, so only the syntactically certain literal forms classify and the proof is
// recomputed after the solve once the per-return types are known.
func (d *Driver) provesErrorReturnFromBody(p *program, cg *cfg.Graph, ref summary.FuncRef, sig typ.Type) bool {
	const valueIdx, errorIdx = 0, 1
	fs, hasState := d.states[ref]
	sawSuccess, sawFailure, inconsistent, classified := false, false, false, false
	cg.EachReturn(func(pt cfg.Point, info *cfg.ReturnInfo) {
		if inconsistent || info == nil || len(info.Exprs) == 0 {
			return
		}
		var ps flow.PointState
		if hasState {
			ps = fs.Points[pt]
		}
		// A sole tail call in the value slot (`return helper(...)`) delegates its whole
		// result vector to the callee, so the (value, err) pair this return contributes
		// is the callee's. When the callee proves the inverse pattern, the delegation
		// carries both the success and failure shapes; an undelegatable expanding tail
		// (a callee that does not prove the pattern, or a vararg) cannot be pinned.
		if len(info.Exprs) == 1 && returnSlotExpands(info, 0) {
			if d.delegatedReturnProvesErrorReturn(p, cg, info) {
				classified = true
				sawSuccess = true
				sawFailure = true
				return
			}
			inconsistent = true
			return
		}
		valState, okVal := d.classifyReturnSlotAt(p, cg, info, valueIdx, ps, hasState)
		var errState returnSlotNil
		var okErr bool
		if errorIdx < len(info.Exprs) {
			errState, okErr = d.classifyReturnSlotAt(p, cg, info, errorIdx, ps, hasState)
		} else {
			// A short return (`return v`) implicitly supplies nil for the trailing
			// error slot in Lua, the success shape of the (value, err) convention --
			// unless the last present expression expands to multiple values (a vararg
			// `...`; a sole tail call is the delegation case handled above), in which
			// case the error slot is filled at runtime and the proof cannot pin it.
			if returnSlotExpands(info, len(info.Exprs)-1) {
				inconsistent = true
				return
			}
			errState, okErr = nilExpr, true
		}
		if !okVal || !okErr {
			inconsistent = true
			return
		}
		classified = true
		switch {
		case valState == nonNilExpr && errState == nilExpr:
			sawSuccess = true
		case valState == nilExpr && errState == nonNilExpr:
			sawFailure = true
		default:
			inconsistent = true
		}
	})
	if inconsistent || !classified || !sawSuccess {
		return false
	}
	if sawFailure {
		return true
	}
	return valueReturnSlotOptional(sig, valueIdx)
}

// delegatedReturnProvesErrorReturn reports whether a sole tail-call return
// (`return helper(...)`) delegates to a callee that itself proves the (value, err)
// inverse pattern: the delegated call's result vector becomes this function's, so
// the callee's proof carries through. The delegate is resolved with the same
// machinery the direct call-site correlation uses, so a delegate proven by its own
// body, by a synthesized signature, or by a cross-module label all carry through.
func (d *Driver) delegatedReturnProvesErrorReturn(p *program, cg *cfg.Graph, info *cfg.ReturnInfo) bool {
	call := info.SourceCallAt(0)
	if call == nil || call.Call == nil {
		return false
	}
	return d.calleeProvesErrorReturn(p, cg, call)
}

// classifyReturnSlotAt classifies the nil state of a return expression slot,
// extending classifyReturnSlot with a call-expression slot (`return nil,
// build_error(x)` or an interior call). A call slot is classified by the converged
// type of its first return: a non-optional result is nonNil, an exactly-nil result
// is nil, an optional/unknown result stays indeterminate (voids the proof). The
// non-call forms route to classifyReturnSlot unchanged.
func (d *Driver) classifyReturnSlotAt(p *program, cg *cfg.Graph, info *cfg.ReturnInfo, idx int, ps flow.PointState, hasState bool) (returnSlotNil, bool) {
	if call := info.SourceCallAt(idx); call != nil && call.Call != nil {
		return d.classifyCallSlotNil(p, cg, call.Call)
	}
	return d.classifyReturnSlot(info.Exprs[idx], cg, ps, hasState)
}

// classifyCallSlotNil classifies the nil state a call expression contributes to a
// single return slot, from the call's first-return type resolved against the
// callee's declared/inferred signature. An unresolved or gradual result stays
// indeterminate so the proof is voided rather than fabricated.
func (d *Driver) classifyCallSlotNil(p *program, cg *cfg.Graph, call *ast.FuncCallExpr) (returnSlotNil, bool) {
	return classifyNilType(d.callReturnSlotType(p, cg, call, 0))
}

// classifyNilType classifies a resolved value type into the nil state a return slot
// proof reads: an exactly-nil type is nil, a definitely-present type is nonNil, an
// optional/unknown/absent type stays indeterminate (voids the proof).
func classifyNilType(t typ.Type) (returnSlotNil, bool) {
	if t == nil || typ.IsAbsentOrUnknown(t) {
		return indeterminateExpr, false
	}
	if unwrap.Alias(t).Kind() == kind.Nil {
		return nilExpr, true
	}
	if _, optional := typ.SplitNilableFieldType(t); optional {
		return indeterminateExpr, false
	}
	return nonNilExpr, true
}

// classifyReturnSlot classifies a return expression's nil state, preferring the
// syntactically certain literal forms (a nil literal, a non-nil constructor/literal)
// and otherwise — when the converged state is available — the value type the solve
// proved for the expression at the return point. A non-optional proved type is
// nonNil; an exactly-nil proved type is nil; an optional/unknown type, or any
// expression with no proved type, stays indeterminate so the proof is voided rather
// than fabricated.
func (d *Driver) classifyReturnSlot(e ast.Expr, cg *cfg.Graph, ps flow.PointState, hasState bool) (returnSlotNil, bool) {
	if state, ok := classifyReturnSlotNil(e); ok {
		return state, true
	}
	if !hasState {
		return indeterminateExpr, false
	}
	return classifyNilType(exprValueAt(e, cg, ps, d.moduleCaptures))
}

// returnSlotNil is the nil classification of a single return expression slot.
type returnSlotNil uint8

const (
	indeterminateExpr returnSlotNil = iota
	nilExpr
	nonNilExpr
)

// classifyReturnSlotNil classifies a return expression as the nil literal, a
// provably non-nil literal/constructor/arithmetic form, or indeterminate. Only the
// syntactically certain forms classify; an identifier or call (whose value the
// proof cannot pin from the AST alone) is indeterminate and voids the proof.
func classifyReturnSlotNil(e ast.Expr) (returnSlotNil, bool) {
	switch e.(type) {
	case *ast.NilExpr:
		return nilExpr, true
	case *ast.StringExpr, *ast.NumberExpr, *ast.TrueExpr, *ast.FalseExpr,
		*ast.TableExpr, *ast.ArithmeticOpExpr, *ast.StringConcatOpExpr,
		*ast.FunctionExpr:
		return nonNilExpr, true
	default:
		return indeterminateExpr, false
	}
}

// returnSlotExpands reports whether the return expression at index i may expand to
// fill more than one result slot at runtime: a call in tail position (its result
// vector flows into the trailing slots) or a vararg `...`. Only the last expression
// of a return expands; an interior expression contributes exactly one value.
func returnSlotExpands(info *cfg.ReturnInfo, i int) bool {
	if info == nil || i < 0 || i >= len(info.Exprs) {
		return false
	}
	if info.SourceCallAt(i) != nil {
		return true
	}
	_, vararg := info.Exprs[i].(*ast.Comma3Expr)
	return vararg
}

// valueReturnSlotOptional reports whether sig's return slot at idx is optional, the
// admission condition for a forward-only (success-only) error-return correlation.
func valueReturnSlotOptional(sig typ.Type, idx int) bool {
	fn := unwrap.Function(sig)
	if fn == nil || idx < 0 || idx >= len(fn.Returns) {
		return false
	}
	return unwrap.IsOptionalLike(fn.Returns[idx])
}

// signatureHasErrorReturn reports whether sig's contract spec carries an
// ErrorReturn label at the canonical (value 0, error 1) slots.
func signatureHasErrorReturn(sig typ.Type) bool {
	if sig == nil {
		return false
	}
	spec := contract.ExtractSpec(sig)
	if spec == nil {
		return false
	}
	er := spec.Effects.GetErrorReturn(0)
	return er != nil && er.ErrorIndex == 1
}

// callTyper adapts the driver to the transfer's CallTyper seam: it types a
// call/method-call expression's return vector by resolving the callee/receiver type
// and running the legacy call pipeline (ops.NewCallPipeline). It reuses the exact
// machinery the observation surface's callReturnsWithExpected uses — generic type-
// argument inference from the argument types, the cast/intercept-aware finish, and
// the multi-return tuple — so the canonical flow types call results through one
// pipeline, never a parallel implementation.
//
// Callee resolution priority: the live Env value the transfer resolved (a
// function-valued local), then the module-wide function signature of the callee
// symbol (a named or local function, even before its summary converges, via
// declared annotations), then the predeclared global's value type, then the field
// path read through TypeOps. A callee that resolves to no function yields no
// returns, so the transfer leaves the slot at the value-domain Top.
type callTyper struct {
	d *Driver
	g *cfg.Graph
}

// opsResolver adapts the driver to the transfer's OperatorResolver seam: it routes
// arithmetic/relational operator typing through the shared TypeOps engine, the same
// resolver the legacy flow uses (core.QueryResolver). It holds the driver and reads
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
func (ct callTyper) CallReturns(call *ast.FuncCallExpr, argTypes []typ.Type, exprType func(ast.Expr) typ.Type) ([]typ.Type, bool) {
	d := ct.d
	if call == nil || d.cfg.Types == nil || d.activeProgram == nil {
		return nil, false
	}
	for i := range argTypes {
		if argTypes[i] == nil {
			argTypes[i] = typ.Unknown
		}
	}

	// The intercept chain resolves the call forms whose result is not the callee's
	// ordinary return: require("m") -> the module's enriched export, T(x) type casts,
	// select(...), setmetatable, and T:is(x). It is the same chain (and order) the
	// legacy synthesizer runs before the pipeline, so the canonical flow types these
	// forms identically.
	if types, ok := ct.runIntercepts(call, exprType); ok {
		return types, true
	}

	var callee typ.Type
	var receiver typ.Type
	if call.Method != "" {
		receiver = ct.resolveReceiver(call.Receiver, exprType)
		if receiver == nil || typ.IsAbsentOrUnknown(receiver) {
			return nil, false
		}
	} else {
		callee = ct.resolveCallee(call.Func, exprType)
		if callee == nil || typ.IsAbsentOrUnknown(callee) {
			return nil, false
		}
		// A gradual-top `any` callee (a call/field-path through an opaque external such
		// as a `local m = require("unresolved")` alias, or any other gradual `any`
		// value) yields a single gradual `any` result. `any` is the gradual escape
		// hatch: a call through it is permissive, not strict unknown, so the result
		// stays gradual rather than collapsing to unknown and rejecting a later use of
		// the result. This mirrors how a method call on an `any` receiver resolves to
		// `any` through the pipeline below.
		if typ.IsAny(callee) {
			return []typ.Type{typ.Any}, true
		}
		fn := unwrap.Function(callee)
		if fn == nil {
			// A callee typed as an already-applied generic alias is an Instantiated,
			// which unwrap.Function does not expand; expand it only when the ordinary
			// unwrap yields no function, matching the observation surface.
			if expanded := subst.ExpandInstantiated(callee); expanded != callee {
				if unwrap.Function(expanded) != nil {
					callee = expanded
				}
			}
		}
		fn = unwrap.Function(callee)
		if fn == nil {
			return nil, false
		}
		// A callee with no declared/inferred returns may be a genuinely void function
		// or one whose return the cross-module export under-typed. Either way the
		// pipeline yields a bare nil, which would clobber the slot to nil and reject a
		// later use of the result; leaving it unknown (the over-approximation the
		// observation surface refines) is sound and avoids that false positive.
		if len(fn.Returns) == 0 {
			return nil, false
		}
	}

	def := ops.CallDef{
		Callee: callee,
		Args:   argTypes,
		Query:  d.cfg.Types,
	}
	if call.Method != "" {
		def.IsMethod = true
		def.Receiver = receiver
		def.MethodName = call.Method
		def.Callee = nil
	}
	if len(call.TypeArgs) > 0 {
		typeArgs := make([]typ.Type, 0, len(call.TypeArgs))
		for _, ta := range call.TypeArgs {
			t := d.resolveType(ta, d.baseScope())
			if t == nil {
				t = typ.Unknown
			}
			typeArgs = append(typeArgs, t)
		}
		def.TypeArgs = typeArgs
	}

	result := ops.NewCallPipeline(d.activeCtx, def, len(call.Args)).Run()
	returns := callResultReturns(result)
	if len(returns) == 0 {
		return nil, false
	}
	// Apply the callee's runtime-effect return transforms (the same transforms the
	// observation surface applies after the pipeline), so a contract effect that
	// shapes the return from the argument values takes hold here too. The dominant
	// case is channel.select's SelectResultOfCases: it rebuilds the bare result record
	// {channel, ok, value: unknown} into a union of per-case records carrying each
	// case's channel identity and value type, which the channel-identity narrowing then
	// refines. Without it the transfer stores the bare record, value stays unknown, and
	// a per-case `result.value.f` read has nothing to narrow.
	returns = d.applyEffectReturns(callee, receiver, argTypes, returns, call.Method != "")
	return returns, true
}

// applyEffectReturns runs the callee's runtime-effect return transforms over the
// pipeline-produced return tuple, the canonical counterpart of the observation
// surface's applyEffectReturnTransforms. It resolves the effect runtime arguments
// (the argument values the effect spec consults), then for each return slot applies
// the function's effect transform, substituting a transformed slot. A callee with no
// function shape or no returns is returned unchanged.
func (d *Driver) applyEffectReturns(callee, receiver typ.Type, args, returns []typ.Type, isMethod bool) []typ.Type {
	fn := unwrap.Function(callee)
	if fn == nil || len(returns) == 0 {
		return returns
	}
	effectArgs := ops.RuntimeArgsForEffects(d.activeCtx, d.cfg.Types, callee, args, receiver, isMethod, false)
	var out []typ.Type
	for i := range returns {
		transformed := transform.ApplyEffectTransform(fn, effectArgs, i, returns[i])
		if transformed == nil || transformed == returns[i] {
			continue
		}
		if out == nil {
			out = make([]typ.Type, len(returns))
			copy(out, returns)
		}
		out[i] = transformed
	}
	if out != nil {
		return out
	}
	return returns
}

// IterVars types a generic-for loop's iteration variables from its iterator
// expression. It resolves the iterator function's iteration effect (the Iterator
// label on its contract spec: indexed for ipairs-style, keyed for pairs-style),
// resolves the iterated source argument's type, and reads the container's element /
// key / value types through the same core helpers the legacy iter inference uses
// (core.ElementType / core.KeyType / core.EntryValueType). It returns one type per
// loop variable, mirroring extract.inferIterVarsFromCallCore so both flows type the
// loop variables identically. An iterator with no iteration effect (and not the ipairs
// /pairs builtin) yields no types, so the transfer leaves the variables untyped.
func (ct callTyper) IterVars(iter *ast.FuncCallExpr, count int, exprType func(ast.Expr) typ.Type) ([]typ.Type, bool) {
	if iter == nil || count <= 0 || iter.Method != "" {
		return nil, false
	}
	kind, srcIdx, ok := ct.iteratorKind(iter)
	if !ok || srcIdx < 0 || srcIdx >= len(iter.Args) {
		return nil, false
	}
	source := exprType(iter.Args[srcIdx])
	// A gradual `any` source (an unannotated parameter, an :: any value) iterated by
	// pairs is a uniform gradual keyed container: every key and value is gradual `any`.
	// The core key/value helpers return nil for `any` and isKeyedContainer rejects it,
	// so without this the key/value loop variables collapse to strict unknown and a
	// `key`/`v` use is wrongly rejected. Indexed (ipairs) iteration over `any` is NOT
	// special-cased: it falls through to the general path below, where the integer
	// index var is set and the element var resolves through core.ElementType (nil for
	// `any`, i.e. left untyped) — the same sound projection an explicitly-`any` source
	// already produced, which keeps the element var from over-typing into the
	// equality-discriminant narrowing the loop body may run on it.
	if typ.IsAny(source) && kind == effect.IterateKeyed {
		out := make([]typ.Type, count)
		for i := range out {
			out[i] = typ.Any
		}
		return out, true
	}
	if source == nil || typ.IsAbsentOrUnknown(source) {
		return nil, false
	}
	out := make([]typ.Type, count)
	switch kind {
	case effect.IterateIndexed:
		if count > 0 {
			out[0] = typ.Integer
		}
		if count > 1 {
			out[1] = core.ElementType(source)
			// A gradual dynamic source (a bare `any`, or an `any?` parameter whose
			// optional wraps `any`) has no structural element type, so core.ElementType
			// yields nil and the element variable would collapse to unknown — a later
			// field read off it then degrades to `never`. Iterating a dynamic container
			// keeps its elements dynamic: the element is gradual `any`, the same sound
			// projection the extract iterator inference applies (iter.go IsPlaceholder).
			if out[1] == nil && unwrap.Underlying(source).Kind().IsPlaceholder() {
				out[1] = typ.Any
			}
		}
	case effect.IterateKeyed:
		// Keyed iteration types its variables only over a genuine keyed container (a
		// Map, or a record carrying a map component): such a container's key/value
		// relation is uniform, so every entry yields the declared key/value type. A
		// closed record inferred from individual field/index writes is NOT a uniform
		// map — iterating it as one would unsoundly assume a single written entry's
		// type holds for every key — so it yields no loop-variable type and the
		// transfer leaves the variables untyped (the sound default).
		if !isKeyedContainer(source) {
			return nil, false
		}
		if count > 0 {
			out[0] = core.KeyType(source)
		}
		if count > 1 {
			out[1] = core.EntryValueType(source)
		}
	default:
		return nil, false
	}
	return out, true
}

// isKeyedContainer reports whether t is a uniform keyed container whose key/value
// relation holds for every entry: a Map, or a record/structure carrying an explicit
// map component. A closed record (only named fields, no map component) is not such a
// container; iterating it as a uniform map would be unsound, so keyed iteration over
// it yields no loop-variable type.
func isKeyedContainer(t typ.Type) bool {
	switch v := unwrap.Underlying(t).(type) {
	case *typ.Map:
		return true
	case *typ.Record:
		return v.HasMapComponent() || v.Open
	}
	return false
}

// iteratorKind resolves a generic-for iterator's iteration kind and iterated source
// parameter index. It prefers the Iterator effect on the iterator function's contract
// spec (so a user-defined or stdlib iterator with a declared iteration effect types
// its loop variables), falling back to the ipairs/pairs builtin recognition on a
// predeclared global, the documented builtin iteration forms.
func (ct callTyper) iteratorKind(iter *ast.FuncCallExpr) (effect.IteratorKind, int, bool) {
	fnType := ct.resolveCallee(iter.Func, func(e ast.Expr) typ.Type {
		// Resolve only the callee through the standard callee resolution; the source
		// argument is typed by the caller's exprType, so a bare exprType here suffices.
		return typ.Unknown
	})
	if spec := contract.ExtractSpec(fnType); spec != nil {
		if it := spec.GetIterator(); it != nil {
			idx, ok := effect.ResolveParamIndex(it.Source, len(iter.Args))
			if !ok {
				return 0, 0, false
			}
			switch it.Kind {
			case effect.IterateIndexed, effect.IterateKeyed:
				return it.Kind, idx, true
			}
		}
	}
	ident, ok := iter.Func.(*ast.IdentExpr)
	if !ok || ident == nil {
		return 0, 0, false
	}
	switch ident.Value {
	case "ipairs":
		return effect.IterateIndexed, 0, true
	case "pairs":
		return effect.IterateKeyed, 0, true
	}
	return 0, 0, false
}

// KeyedIterSource reports whether iter is a keyed (pairs-style) iteration and, if
// so, returns the iterated source-argument expression. It reuses iteratorKind so
// the keyed/indexed decision is the contract-spec iteration effect (or the pairs/
// ipairs builtin), not a name match: only a keyed iteration's first loop variable
// is a key of the source, so only that case yields a source for KeyOf production.
func (ct callTyper) KeyedIterSource(iter *ast.FuncCallExpr) (ast.Expr, bool) {
	if iter == nil || iter.Method != "" {
		return nil, false
	}
	kind, srcIdx, ok := ct.iteratorKind(iter)
	if !ok || kind != effect.IterateKeyed || srcIdx < 0 || srcIdx >= len(iter.Args) {
		return nil, false
	}
	return iter.Args[srcIdx], true
}

// IndexedIterKeyProvenance reports whether iter is an indexed (ipairs-style)
// iteration over an array whose elements are PROVABLY keys of a container, and if
// so returns that container's path. It closes the interprocedural key-presence
// chain: a local `names = sorted_keys(c)` binds `names` to the keys table a
// keys-collector function returns from its parameter, so each `name` iterated out
// of `names` is a key of the actual argument `c`. The transfer emits KeyOf(c, name)
// into the loop body, so a `c[name]` read inside is present.
//
// The provenance is sound only when the iterated source is the SINGLE-assignment
// result of a call to a function the structural keys-collector recognizer accepts
// (keyscoll.DetectKeysCollector), at that function's keys return slot. A source
// assigned from anything else (a literal, an arbitrary function, a reassigned
// local) yields no provenance, so its read stays optional.
func (ct callTyper) IndexedIterKeyProvenance(iter *ast.FuncCallExpr) (constraint.Path, bool) {
	if iter == nil || iter.Method != "" || ct.g == nil {
		return constraint.Path{}, false
	}
	kind, srcIdx, ok := ct.iteratorKind(iter)
	if !ok || kind != effect.IterateIndexed || srcIdx < 0 || srcIdx >= len(iter.Args) {
		return constraint.Path{}, false
	}
	srcIdent, ok := iter.Args[srcIdx].(*ast.IdentExpr)
	if !ok {
		return constraint.Path{}, false
	}
	bindings := ct.bindings()
	if bindings == nil {
		return constraint.Path{}, false
	}
	srcSym, ok := bindings.SymbolOf(srcIdent)
	if !ok || srcSym == 0 {
		return constraint.Path{}, false
	}
	prog := ct.d.activeProgram
	if prog == nil {
		return constraint.Path{}, false
	}
	// The source array must be assigned EXACTLY ONCE, and that single assignment
	// must be a call to a keys-collector at its keys return slot. A symbol assigned at
	// more than one point (a reassignment from any source) carries no single sound
	// provenance, so it is declined: the iterated array might hold an arbitrary value
	// the later assignment introduced.
	assignCount := 0
	var collectorPath constraint.Path
	var collectorFound bool
	ct.g.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		idx, retIndex, callInfo := assignTargetCall(info, srcSym)
		if idx < 0 {
			return
		}
		assignCount++
		if callInfo == nil || callInfo.Call == nil {
			return
		}
		ref, ok := ct.resolveCalleeRef(callInfo.Call, prog)
		if !ok {
			return
		}
		kc := prog.keysCollectors[ref]
		if kc == nil || kc.ReturnIndex != retIndex {
			return
		}
		path, pok := ct.argContainerPath(callsite.RuntimeArgAt(callInfo, kc.ParamIndex))
		if !pok {
			return
		}
		collectorPath = path
		collectorFound = true
	})
	if assignCount != 1 || !collectorFound {
		return constraint.Path{}, false
	}
	return collectorPath, true
}

// assignTargetCall resolves the call source producing the assignment target bound
// to symbol sym, returning that target's index, its return slot, and the source
// call. A target not bound to sym yields index -1; a target bound to sym whose
// source is not a call yields a nil call (the assignment still counts, so a
// reassignment from a non-call source is observed by the caller's count).
func assignTargetCall(info *cfg.AssignInfo, sym cfg.SymbolID) (int, int, *cfg.CallInfo) {
	for i, target := range info.Targets {
		if target.Kind != cfg.TargetIdent || target.Symbol != sym {
			continue
		}
		call, retIndex := info.CallForTarget(i)
		return i, retIndex, call
	}
	return -1, 0, nil
}

// argContainerPath builds the container path of a keys-collector's actual argument
// (`sorted_keys(c)` -> path of `c`). It accepts a bare identifier and a static
// field path, the same shapes the consumption matches, and keys by the bare symbol
// (Version 0) so the emitted KeyOf matches the version-insensitive consumption. A
// non-static or symbol-less argument yields no path.
func (ct callTyper) argContainerPath(arg ast.Expr) (constraint.Path, bool) {
	bindings := ct.bindings()
	if bindings == nil || arg == nil {
		return constraint.Path{}, false
	}
	path := flowpath.FromExprWithBindings(arg, nil, bindings)
	if path.IsEmpty() || path.Symbol == 0 {
		return constraint.Path{}, false
	}
	return path, true
}

// resolveCallee resolves a non-method call's callee type. It tries the live Env
// value (a function-valued local), then the callee symbol's module-wide function
// signature, then a predeclared global's value type, then the expression's own
// resolved type (a field-path callee M.f the transfer tracks).
func (ct callTyper) resolveCallee(funcExpr ast.Expr, exprType func(ast.Expr) typ.Type) typ.Type {
	if funcExpr == nil {
		return nil
	}
	// A named-function callee resolves to its module-wide signature first: that
	// signature carries the function's inferred return tuple (the funcTyper-built
	// Env value only carries declared annotations, so for an unannotated-return
	// function it is a returnless callable that would type the result as nil).
	if ident, ok := funcExpr.(*ast.IdentExpr); ok {
		if sig := ct.calleeSignature(ident); sig != nil {
			return sig
		}
		if t := exprType(funcExpr); !typ.IsAbsentOrUnknown(t) {
			return t
		}
		if d := ct.d; d.cfg.GlobalTypes != nil {
			if t := d.cfg.GlobalTypes[ident.Value]; t != nil && !typ.IsAbsentOrUnknown(t) {
				return t
			}
		}
		return nil
	}
	// A field-path callee (M.f) resolves to the field function's module-wide
	// signature first: a `function M.f()` definition is visible under M's symbol in
	// any body that captures M, so a call M.f() inside a nested method resolves to f's
	// signature even though the captured container M has no value in the nested
	// function's Env. This carries f's declared/inferred return so the call result is
	// typed rather than unknown.
	if sig := ct.fieldCalleeSignature(funcExpr); sig != nil {
		return sig
	}
	// A field-path callee whose base is a captured module value (`local time =
	// require("time")` read inside a nested function as `time.now()`) resolves the
	// base from the module-capture fallback and reads the field/method off it. The
	// transfer Env does not track a captured free variable, so without this the call
	// types as unknown and a returned record field built from it collapses to unknown.
	if sig := ct.capturedFieldCalleeSignature(funcExpr); sig != nil {
		return sig
	}
	// Otherwise the callee is the expression's own resolved type (a field path the
	// transfer tracks, a call-result function value).
	if t := exprType(funcExpr); !typ.IsAbsentOrUnknown(t) {
		return t
	}
	return nil
}

// capturedFieldCalleeSignature resolves a field-path callee `base.field` whose base
// identifier is a module-level value captured into a nested function (its type is
// in moduleCaptures but it has no live Env value here). It reads base's captured
// type and resolves the field's member type — a module export interface's method
// (`time.now`) or a captured record's field function — so the call types its
// return rather than collapsing to unknown.
func (ct callTyper) capturedFieldCalleeSignature(funcExpr ast.Expr) typ.Type {
	attr, ok := funcExpr.(*ast.AttrGetExpr)
	if !ok {
		return nil
	}
	baseIdent, ok := attr.Object.(*ast.IdentExpr)
	if !ok {
		return nil
	}
	field, ok := attr.Key.(*ast.StringExpr)
	if !ok || field.Value == "" {
		return nil
	}
	baseType := ct.capturedBaseType(baseIdent)
	zdbg("capturedFieldCallee base=%q field=%q baseType=%v", baseIdent.Value, field.Value, baseType)
	if baseType == nil {
		return nil
	}
	ft, ok := fieldMemberType(baseType, field.Value)
	if !ok || ft == nil || typ.IsAbsentOrUnknown(ft) {
		return nil
	}
	return ft
}

// fieldMemberType resolves a named member (an interface method or a record/map
// field) off a container type, reusing the structural field resolver. It is the
// member a `base.field` access reads when base resolves to its captured type.
func fieldMemberType(base typ.Type, name string) (typ.Type, bool) {
	return core.Field(base, name)
}

// capturedBaseType resolves an identifier's type from the module-capture fallback:
// the module-wide type of a symbol captured from an enclosing scope. It returns nil
// when the identifier is not a captured module value.
func (ct callTyper) capturedBaseType(ident *ast.IdentExpr) typ.Type {
	if ident == nil || ct.g == nil {
		return nil
	}
	bindings := ct.g.Bindings()
	if bindings == nil {
		return nil
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return nil
	}
	if t, ok := ct.d.moduleAliasTypes[sym]; ok && t != nil && !typ.IsAbsentOrUnknown(t) {
		return t
	}
	if t, ok := ct.d.moduleCaptures[sym]; ok && t != nil && !typ.IsAbsentOrUnknown(t) {
		return t
	}
	return nil
}

// fieldCalleeSignature resolves a field-path callee (M.f) to the module function
// defined as that field (function M.f()). It reads the program's field-function
// registry keyed by the base container's symbol, so a captured container resolves its
// field functions across function boundaries.
func (ct callTyper) fieldCalleeSignature(funcExpr ast.Expr) typ.Type {
	attr, ok := funcExpr.(*ast.AttrGetExpr)
	if !ok {
		return nil
	}
	baseIdent, ok := attr.Object.(*ast.IdentExpr)
	if !ok {
		return nil
	}
	field, ok := attr.Key.(*ast.StringExpr)
	if !ok || field.Value == "" {
		return nil
	}
	prog := ct.d.activeProgram
	if prog == nil || ct.g == nil {
		return nil
	}
	bindings := ct.g.Bindings()
	if bindings == nil {
		return nil
	}
	sym, ok := bindings.SymbolOf(baseIdent)
	if !ok || sym == 0 {
		return nil
	}
	byField, ok := prog.fieldFuncs[sym]
	if !ok {
		return nil
	}
	fn, ok := byField[field.Value]
	if !ok || fn == nil {
		return nil
	}
	ref, ok := prog.refByFunc(fn)
	if !ok {
		return nil
	}
	return ct.d.signatureForRef(prog, ref)
}

// resolveReceiver resolves a method call's receiver type from the live Env, falling
// back to a captured value or a predeclared global so a method on an imported
// module table (string:..., time.now():...) resolves.
func (ct callTyper) resolveReceiver(recvExpr ast.Expr, exprType func(ast.Expr) typ.Type) typ.Type {
	if recvExpr == nil {
		return nil
	}
	if t := exprType(recvExpr); !typ.IsAbsentOrUnknown(t) {
		return t
	}
	if ident, ok := recvExpr.(*ast.IdentExpr); ok {
		if sig := ct.calleeSignature(ident); sig != nil {
			return sig
		}
		if d := ct.d; d.cfg.GlobalTypes != nil {
			if t := d.cfg.GlobalTypes[ident.Value]; t != nil && !typ.IsAbsentOrUnknown(t) {
				return t
			}
		}
	}
	return nil
}

// runIntercepts runs the call/method intercept chain (require, select,
// setmetatable, type-cast, T:is) against call, returning the intercepted return
// types when an intercept handles it. It builds the same chain the legacy
// synthesizer builds (intercept.NewChainBuilder with the module manifests), so a
// require("m") resolves to the module's enriched export, a T(x) cast resolves to T,
// and a T:is(x) guard resolves to its (T?, error?) tuple — the forms whose result
// is not the callee's ordinary return.
func (ct callTyper) runIntercepts(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) ([]typ.Type, bool) {
	d := ct.d
	env := intercept.CallEnv{
		Scope:   d.baseScope(),
		Recurse: intercept.ExprSynth(func(e ast.Expr) typ.Type { return exprType(e) }),
		TypeLookup: func(name string) typ.Type {
			if d.cfg.GlobalTypes == nil {
				return nil
			}
			if t, ok := d.cfg.GlobalTypes[name]; ok {
				return t
			}
			return nil
		},
	}
	if ct.g != nil {
		env.Bindings = ct.g.Bindings()
	}
	chain := intercept.NewChainBuilder().WithManifests(d.cfg.Manifests).WithVariadicResolver(d.baseScope()).Build()
	if call.Method != "" {
		if res := chain.InterceptMethodCall(call, env); res.Skip {
			return res.Types, true
		}
		return nil, false
	}
	if res := chain.InterceptCall(call, env); res.Skip {
		return res.Types, true
	}
	return nil, false
}

// TypeCastTarget reports whether call is a type-cast/assertion call `T(arg)` — a
// type name used as a CallableType constructor — and returns the asserted type T. It
// runs the same TypeCastIntercept the call-return chain uses (effect-based dispatch,
// not a name heuristic), so the canonical flow recognizes a cast identically here and
// in CallReturns. A call that is not a type cast, or one whose target does not
// resolve, yields false.
func (ct callTyper) TypeCastTarget(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) (typ.Type, bool) {
	d := ct.d
	if call == nil || call.Method != "" {
		return nil, false
	}
	env := intercept.CallEnv{
		Scope:   d.baseScope(),
		Recurse: intercept.ExprSynth(func(e ast.Expr) typ.Type { return exprType(e) }),
		TypeLookup: func(name string) typ.Type {
			if d.cfg.GlobalTypes == nil {
				return nil
			}
			if t, ok := d.cfg.GlobalTypes[name]; ok {
				return t
			}
			return nil
		},
	}
	if ct.g != nil {
		env.Bindings = ct.g.Bindings()
	}
	cast := &intercept.TypeCastIntercept{}
	res := cast.InterceptCall(call, env)
	if !res.Skip || len(res.Types) == 0 {
		return nil, false
	}
	return res.Types[0], true
}

// calleeSignature resolves a callee identifier to the module function it names: the
// signature of the function bound to the identifier's symbol. It reads the program's
// module-wide function-binding map (prog.funcSyms), so a recursive self-call or a
// reference to a sibling function resolves to its declared/converged signature even
// while the calling function's fixpoint is still solving.
func (ct callTyper) calleeSignature(ident *ast.IdentExpr) typ.Type {
	if ident == nil {
		return nil
	}
	prog := ct.d.activeProgram
	if prog == nil || ct.g == nil {
		return nil
	}
	bindings := ct.g.Bindings()
	if bindings == nil {
		return nil
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return nil
	}
	fn, ok := prog.funcSyms[sym]
	if !ok || fn == nil {
		return nil
	}
	ref, ok := prog.refByFunc(fn)
	if !ok {
		return nil
	}
	if sig := ct.d.signatureForRef(prog, ref); sig != nil {
		return sig
	}
	return nil
}

// ParamNarrows resolves the callee of call to a module function and returns its
// parameter-narrowing effects. It resolves the callee the same way the call typing
// does — an identifier callee through the module function-binding map, a field-path
// callee (M.f) through the field-function registry, and a method/static name through
// byName — so a wrapper called by name, field, or method narrows its arguments. A
// callee that does not resolve to a module function is an imported callee: its
// body-proven refinement rides its imported signature (the manifest function
// summary the module export published), so the effects are recovered from that
// signature's FunctionRefinement instead.
func (ct callTyper) ParamNarrows(call *ast.FuncCallExpr) []transfer.ParamNarrow {
	if call == nil {
		return nil
	}
	prog := ct.d.activeProgram
	if prog == nil {
		return nil
	}
	if ref, ok := ct.resolveCalleeRef(call, prog); ok {
		return prog.paramNarrows[ref]
	}
	return ct.importedParamNarrows(call)
}

// importedParamNarrows recovers the parameter-narrowing effects an imported callee
// proves from its resolved signature's FunctionRefinement. The exporting module
// published the body-proven assert/guard refinement as the callee's OnReturn
// condition (an isnil/notnil/falsy/truthy/hastype constraint rooted at a parameter
// placeholder); reversing the projection here gives the importer the same
// ParamNarrow effects a module-local wrapper carries, so a `test.is_nil(err)` call
// narrows its argument (and its correlated (value, err) siblings) exactly as the
// local case does. A callee whose signature carries no refinement yields none.
func (ct callTyper) importedParamNarrows(call *ast.FuncCallExpr) []transfer.ParamNarrow {
	sig := ct.capturedFieldCalleeSignature(call.Func)
	if sig == nil {
		return nil
	}
	fn := unwrap.Function(sig)
	if fn == nil || fn.Refinement == nil {
		return nil
	}
	refinement, ok := fn.Refinement.(*constraint.FunctionRefinement)
	if !ok || refinement == nil || !refinement.OnReturn.HasConstraints() {
		return nil
	}
	var out []transfer.ParamNarrow
	for _, c := range refinement.OnReturn.MustConstraints() {
		if e, ok := paramNarrowFromConstraint(c); ok {
			out = append(out, e)
		}
	}
	return out
}

// paramNarrowFromConstraint reverses paramNarrowConstraint: a placeholder-rooted
// OnReturn constraint becomes the parameter-narrowing effect it encodes. The
// placeholder index is the parameter; the constraint's remaining path segments are
// the proven field path; the constraint kind is the proven check. A constraint not
// rooted at a parameter placeholder (a constraint on a non-parameter value, or a
// vocabulary the call-site narrowing cannot apply) yields ok=false.
func paramNarrowFromConstraint(c constraint.Constraint) (transfer.ParamNarrow, bool) {
	path, check, ok := constraintPathAndCheck(c)
	if !ok || !path.IsPlaceholder() {
		return transfer.ParamNarrow{}, false
	}
	idx := path.PlaceholderIndex()
	if idx < 0 {
		return transfer.ParamNarrow{}, false
	}
	var segs []constraint.Segment
	if len(path.Segments) > 0 {
		segs = append(segs, path.Segments...)
	}
	return transfer.ParamNarrow{Param: idx, Segments: segs, Check: check, EqParam: -1}, true
}

// constraintPathAndCheck classifies one OnReturn constraint into its path and the
// CondCheckKind it proves, the inverse of paramNarrowConstraint's mapping. A
// constraint kind with no call-site narrowing vocabulary yields ok=false.
func constraintPathAndCheck(c constraint.Constraint) (constraint.Path, cfg.CondCheckKind, bool) {
	switch v := c.(type) {
	case constraint.IsNil:
		return v.Path, cfg.CheckNil, true
	case constraint.Falsy:
		return v.Path, cfg.CheckFalsy, true
	case constraint.NotNil:
		return v.Path, cfg.CheckNotNil, true
	case constraint.Truthy:
		return v.Path, cfg.CheckTruthy, true
	default:
		return constraint.Path{}, cfg.CheckNone, false
	}
}

// IsNoReturn reports whether call's callee is a module function the program proved
// never returns normally. A statement call to it terminates the caller's flow.
func (ct callTyper) IsNoReturn(call *ast.FuncCallExpr) bool {
	if call == nil {
		return false
	}
	prog := ct.d.activeProgram
	if prog == nil {
		return false
	}
	ref, ok := ct.resolveCalleeRef(call, prog)
	zdbg("IsNoReturn callee=%q resolved=%v noReturn=%v", extraction.ExtractCalleeName(call.Func), ok, ok && prog.noReturn[ref])
	if !ok {
		return false
	}
	return prog.noReturn[ref]
}

// resolveCalleeRef resolves call's callee to its module FuncRef. It tries the callee
// identifier's binding symbol (a named or local function), then a field-path callee
// (M.f) through the field-function registry, then the statically resolved callee /
// method name through byName. It returns false when no module function is named.
func (ct callTyper) resolveCalleeRef(call *ast.FuncCallExpr, prog *program) (summary.FuncRef, bool) {
	bindings := ct.bindings()
	if ident, ok := call.Func.(*ast.IdentExpr); ok && ident != nil && bindings != nil {
		if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
			if fn, ok := prog.funcSyms[sym]; ok && fn != nil {
				if ref, ok := prog.refByFunc(fn); ok {
					return ref, true
				}
			}
		}
	}
	if attr, ok := call.Func.(*ast.AttrGetExpr); ok && bindings != nil {
		if baseIdent, ok := attr.Object.(*ast.IdentExpr); ok {
			if field, ok := attr.Key.(*ast.StringExpr); ok && field.Value != "" {
				if sym, ok := bindings.SymbolOf(baseIdent); ok && sym != 0 {
					if byField, ok := prog.fieldFuncs[sym]; ok {
						if fn, ok := byField[field.Value]; ok && fn != nil {
							if ref, ok := prog.refByFunc(fn); ok {
								return ref, true
							}
						}
					}
				}
			}
		}
	}
	name := extraction.ExtractCalleeName(call.Func)
	if name == "" {
		name = call.Method
	}
	if name != "" {
		if ref, ok := prog.byName[name]; ok {
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

// callResultReturns extracts the Lua return vector from a call pipeline result: the
// expression-adjusted Returns when present, else the packed return type unpacked
// from a tuple, else the single return. It mirrors the observation surface's
// callResultReturns so both flows read the pipeline result identically.
func callResultReturns(result ops.CallResult) []typ.Type {
	if len(result.Returns) > 0 {
		out := make([]typ.Type, len(result.Returns))
		copy(out, result.Returns)
		return out
	}
	if tuple, ok := result.Type.(*typ.Tuple); ok {
		out := make([]typ.Type, len(tuple.Elements))
		copy(out, tuple.Elements)
		return out
	}
	if result.Type == nil {
		return nil
	}
	return []typ.Type{result.Type}
}

// funcTyper adapts the driver to the transfer's FuncTyper seam: it resolves a
// function literal's signature from its declared annotations, plus the inferred
// summary return when the literal declares no return. The inferred lookup reads the
// same summary query signatureForRef uses (the converged returns, or the in-flight
// query's current returns during the solve), which the call-graph fixpoint widens, so
// it is stable inside the solve rather than a not-yet-converged dependence.
type funcTyper struct{ d *Driver }

// FuncType builds fn's signature from its declared parameter and return
// annotations, splicing the inferred summary return when fn declares none. The result
// is the structural callable a function-valued table field carries.
func (ft funcTyper) FuncType(fn *ast.FunctionExpr) *typ.Function {
	return ft.FuncTypeOver(fn, ft.d.baseScope())
}

// FuncTypeOver builds fn's signature resolving its annotations over an enclosing
// base scope, so a function literal nested in a generic function resolves a
// captured type parameter to the enclosing function's bounded parameter.
func (ft funcTyper) FuncTypeOver(fn *ast.FunctionExpr, base *scope.State) *typ.Function {
	if fn == nil {
		return nil
	}
	d := ft.d
	builder := typ.Func()
	sc := d.genericScopeOver(builder, fn, base)
	d.applyParamList(builder, fn, sc)
	d.applyReturnList(builder, fn, sc)
	// A literal that declares no return annotation otherwise carries an empty return
	// tuple, so its single-value call result types as nil. Splice in the inferred
	// summary return the body produces — the same return signatureForRef builds — so
	// the literal value a local function binds (and any field it is stored under)
	// carries its inferred return. The inferred lookup runs only for an
	// unannotated-return literal that maps to a program ref; an annotated return is
	// authoritative and a literal with no ref keeps the declared (possibly empty) tuple.
	if len(fn.ReturnTypes) == 0 {
		if prog := d.activeProgram; prog != nil {
			if ref, ok := prog.refByFunc(fn); ok {
				if returns := d.inferredReturnTypes(ref); len(returns) > 0 {
					builder.Returns(returns...)
				}
			}
		}
	}
	return builder.Build()
}

// inferredReturnTypes is ref's inferred return tuple from the converged summary, or
// its current return tuple from the in-flight summary query during the solve. It is
// the inferred half of returnTypesForRef, factored so FuncTypeOver splices a literal's
// inferred return without re-resolving declared annotations.
func (d *Driver) inferredReturnTypes(ref summary.FuncRef) []typ.Type {
	if returns := d.ReturnTypes(ref); len(returns) > 0 {
		return returns
	}
	return d.liveReturnTypes(ref)
}

// MethodFuncType builds a method definition's signature with the implicit leading
// `self` parameter typed as the receiver's class. It mirrors the legacy method
// self-type resolution (resolveSelfTypeForMethod): the receiver name `T` in
// `function T:m()` binds the instance contract in the type namespace, so self is
// the named type `T`, and the declared parameter/return annotations follow. This
// is the callable the class field `T.m` holds.
func (ft funcTyper) MethodFuncType(info *cfg.FuncDefInfo) *typ.Function {
	return ft.MethodFuncTypeOver(info, ft.d.baseScope())
}

// MethodFuncTypeOver builds a method definition's signature resolving its
// annotations over an enclosing base scope, the method counterpart of
// FuncTypeOver for a literal nested in a generic function.
func (ft funcTyper) MethodFuncTypeOver(info *cfg.FuncDefInfo, base *scope.State) *typ.Function {
	if info == nil || info.FuncExpr == nil {
		return nil
	}
	d := ft.d
	fn := info.FuncExpr
	builder := typ.Func()
	sc := d.genericScopeOver(builder, fn, base)
	selfType := d.methodSelfType(info, sc)
	phasecore.ApplyParamList(builder, fn, phasecore.ParamListConfig{
		ResolveType:      d.resolveType,
		ResolveScope:     sc,
		UntypedParamType: typ.Any,
		ImplicitSelf:     true,
		ImplicitSelfType: selfType,
	})
	d.applyReturnList(builder, fn, sc)
	// A method with no return annotation otherwise carries an empty return tuple, so
	// a sibling `self:m()` call types its result as nil. Splice in the inferred
	// summary return the body produces (the same splice FuncTypeOver applies to a
	// plain literal), so the class field `T.m` holds the method's inferred return and
	// a `self:m()` call sees it. The lookup runs only for an unannotated-return method
	// mapping to a program ref; a declared return is authoritative.
	if len(fn.ReturnTypes) == 0 {
		if prog := d.activeProgram; prog != nil {
			if ref, ok := prog.refByFunc(fn); ok {
				if returns := d.inferredReturnTypes(ref); len(returns) > 0 {
					builder.Returns(returns...)
				}
			}
		}
	}
	return builder.Build()
}

// methodSelfType resolves the implicit `self` type for a method definition. It
// prefers the explicit type-namespace binding for the receiver name (the instance
// contract `T` declared by `type T = {...}`), matching the legacy
// resolveSelfTypeForMethod priority. A receiver with no named-type binding yields
// the receiver value's tracked record type, falling back to a gradual self so an
// unannotated class still types its methods.
func (d *Driver) methodSelfType(info *cfg.FuncDefInfo, sc *scope.State) typ.Type {
	if info == nil {
		return typ.Any
	}
	if info.ReceiverName != "" {
		if sc == nil {
			sc = d.baseScope()
		}
		if sc != nil {
			if named, ok := sc.LookupType(info.ReceiverName); ok && named != nil {
				return unwrapSelfAlias(named)
			}
		}
	}
	return typ.Any
}

// unwrapSelfAlias unwraps a class type-name binding to the canonical type a
// `self: T` annotation resolves to. A module type definition stores `type T` as
// an *Alias wrapping its target; a self-recursive class (its method signatures
// mention T) resolves to a *Recursive family. The expected self type a field
// check compares against is the recursive family the annotation resolver
// produces, so the implicit `self` must be that same family, not the alias
// wrapper. Returning the alias target makes the synthesized method `self: T`
// match the declared `self: T` field type by the same recursive identity.
func unwrapSelfAlias(t typ.Type) typ.Type {
	if al, ok := t.(*typ.Alias); ok && al.Target != nil {
		return al.Target
	}
	return t
}

// genericScope registers fn's generic type parameters (<T, U>) on builder and
// returns the resolution scope in which the parameter/return annotations resolve
// those names to type-parameter references. Reusing the legacy type-param scope
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
	sc := base
	if sc == nil {
		sc = d.baseScope()
	}
	if fn == nil || len(fn.TypeParams) == 0 {
		return sc
	}
	typeParams := make(map[string]typ.Type, len(fn.TypeParams))
	for _, tp := range fn.TypeParams {
		var constr typ.Type
		if tp.Constraint != nil {
			constr = d.resolveType(tp.Constraint, sc)
		}
		param := typ.NewTypeParam(tp.Name, constr)
		typeParams[tp.Name] = param
		if builder != nil {
			builder.TypeParam(tp.Name, constr)
		}
	}
	if sc == nil {
		return sc
	}
	return sc.WithTypeParams(typeParams)
}

// typeParamScope is the resolution scope a generic function's own body annotations
// (its parameter and return types) resolve against: the module base scope extended
// with fn's type parameters bound to their bounded typ.TypeParam. It is the same
// scope genericScope builds for the signature, without the signature builder, so a
// `function f<T: Printable>(x: T): T` body resolves `T` to the bounded type
// parameter rather than an unresolved typ.Ref. A non-generic function resolves
// against the base scope unchanged.
func (d *Driver) typeParamScope(fn *ast.FunctionExpr) *scope.State {
	return d.genericScope(nil, fn)
}

// applyParamList lowers fn's parameter list onto builder, reusing the legacy
// phasecore.ApplyParamList so the canonical signature obeys the same parameter
// rules: an unannotated parameter is OPTIONAL (a missing call argument becomes nil
// in Lua) and gradual `any` (dynamic, usable in every operation), not a required
// opaque unknown. Annotations resolve against sc (the module base scope, or, for a
// generic function, the base scope extended with the function's type parameters).
// The implicit `self` of a method definition is not prepended here; it is the
// receiver-typed parameter the observation surface seeds for the body.
func (d *Driver) applyParamList(builder *typ.FunctionBuilder, fn *ast.FunctionExpr, sc *scope.State) {
	phasecore.ApplyParamList(builder, fn, phasecore.ParamListConfig{
		ResolveType:      d.resolveType,
		ResolveScope:     sc,
		UntypedParamType: typ.Any,
	})
}

// applyReturnList lowers fn's declared return annotations onto builder, resolving
// them against sc (the generic type-param scope when fn is generic). An unannotated
// return resolves to unknown (its type is the converged summary the caller reads
// separately); a return list is set only when fn declares one.
func (d *Driver) applyReturnList(builder *typ.FunctionBuilder, fn *ast.FunctionExpr, sc *scope.State) {
	if fn == nil || len(fn.ReturnTypes) == 0 {
		return
	}
	returns := make([]typ.Type, 0, len(fn.ReturnTypes))
	for _, rt := range fn.ReturnTypes {
		if rt == nil {
			returns = append(returns, typ.Unknown)
			continue
		}
		t := d.resolveType(rt, sc)
		if t == nil {
			t = typ.Unknown
		}
		returns = append(returns, t)
	}
	builder.Returns(returns...)
}

// signatureForRef builds the canonical function signature of ref: its declared
// parameter types (resolved annotations; an unannotated parameter is an optional
// gradual `any`) and its return tuple. It is the type a caller observes for the
// function the ref analyzes.
func (d *Driver) signatureForRef(prog *program, ref summary.FuncRef) *typ.Function {
	fn := prog.funcExprs[ref]
	if fn == nil {
		return nil
	}
	builder := typ.Func()
	sc := d.genericScope(builder, fn)
	d.applyParamList(builder, fn, sc)
	if returns := d.returnTypesForRef(prog, ref, sc); len(returns) > 0 {
		builder.Returns(returns...)
	}
	sig := builder.Build()
	return d.constrainUnannotatedParams(prog, ref, fn, sig)
}

// constrainUnannotatedParams narrows each unannotated parameter slot of sig to the
// obligation the function body PROVES the caller must satisfy. A parameter with no
// proven obligation keeps its gradual `any` (the sound default for an unannotated
// Lua parameter), so a caller is constrained ONLY where the body proves a
// precondition: an arithmetic operand pins the parameter to number (the converged
// Contracts component), and a parameter forwarded to a typed callee's parameter pins
// it to that callee's parameter type. An annotated parameter is the function's
// declared contract and is left untouched.
func (d *Driver) constrainUnannotatedParams(prog *program, ref summary.FuncRef, fn *ast.FunctionExpr, sig *typ.Function) *typ.Function {
	if sig == nil || fn == nil || fn.ParList == nil || d.derivingContracts {
		return sig
	}
	contracts := d.bodyParamContracts(prog, ref)
	if len(contracts) == 0 {
		return sig
	}
	offset := len(sig.Params) - len(fn.ParList.Names)
	if offset < 0 {
		return sig
	}
	var params []typ.Param
	for i := range fn.ParList.Names {
		slot := i + offset
		if slot < 0 || slot >= len(sig.Params) {
			continue
		}
		if d.paramHasAnnotation(fn, i) {
			continue
		}
		obligation, ok := contracts[i]
		if !ok || obligation == nil || typ.IsAbsentOrUnknown(obligation) || typ.IsAny(obligation) {
			continue
		}
		current := sig.Params[slot].Type
		// The obligation only ever narrows a gradual slot. A slot the body already
		// pins to a concrete type (a prior obligation) keeps the more precise of the
		// two; a gradual `any`/`unknown` slot takes the obligation outright.
		next := obligation
		if !typ.IsAbsentOrUnknown(current) && !typ.IsAny(current) {
			next = paramevidence.HardContractJoin(current, obligation)
			if next == nil {
				continue
			}
		}
		if typ.TypeEquals(next, current) && !sig.Params[slot].Optional {
			continue
		}
		if params == nil {
			params = append([]typ.Param(nil), sig.Params...)
		}
		params[slot].Type = next
		// A body-proven precondition is a hard obligation: the body uses the value in a
		// position that fails for nil (an arithmetic operand, a typed callee argument),
		// so the parameter is no longer the gradual optional an unannotated parameter
		// otherwise is. Dropping optionality lets the call-site arg-check reject a nil/
		// unknown argument the obligation excludes.
		params[slot].Optional = false
	}
	if params == nil {
		return sig
	}
	return rebuildFunctionParams(sig, params)
}

// rebuildFunctionParams returns a copy of fn with its parameter list replaced,
// preserving every other component (type parameters, variadic, returns, effects,
// spec, refinement). It is the param-list analogue of typjoin.WithReturns the
// signature constraint uses to narrow an unannotated parameter slot.
func rebuildFunctionParams(fn *typ.Function, params []typ.Param) *typ.Function {
	if fn == nil {
		return fn
	}
	builder := typ.Func()
	for _, tp := range fn.TypeParams {
		if tp == nil {
			continue
		}
		builder.TypeParam(tp.Name, tp.Constraint)
	}
	for _, p := range params {
		if p.Optional {
			builder.OptParam(p.Name, p.Type)
		} else {
			builder.Param(p.Name, p.Type)
		}
	}
	if fn.Variadic != nil {
		builder.Variadic(fn.Variadic)
	}
	if len(fn.Returns) > 0 {
		builder.Returns(fn.Returns...)
	}
	builder.Effects(fn.Effects)
	builder.Spec(fn.Spec)
	builder.WithRefinement(fn.Refinement)
	return builder.Build()
}

// bodyParamContracts is the body-proven parameter obligation of ref keyed by SOURCE
// parameter index: the converged Contracts component (Summary.Params, re-keyed from
// the graph parameter SLOT layout to the source index so an implicit method receiver
// does not shift the mapping) joined with the obligation each parameter forwarded to
// a typed callee imposes. A parameter the body never constrains is absent.
func (d *Driver) bodyParamContracts(prog *program, ref summary.FuncRef) map[int]typ.Type {
	out := make(map[int]typ.Type)
	g := prog.graphs[ref]
	slotContracts := d.ParamTypes(ref)
	if len(slotContracts) > 0 && g != nil {
		slots := g.ParamSlotsReadOnly()
		for slot, t := range slotContracts {
			if t == nil || typ.IsAbsentOrUnknown(t) || typ.IsAny(t) {
				continue
			}
			if slot < 0 || slot >= len(slots) {
				continue
			}
			srcIdx, ok := slots[slot].SourceParamIndex()
			if !ok {
				continue
			}
			out[srcIdx] = t
		}
	}
	for srcIdx, t := range d.typedCalleeArgContracts(prog, ref) {
		if t == nil {
			continue
		}
		if prev, ok := out[srcIdx]; ok && prev != nil {
			joined := paramevidence.HardContractJoin(prev, t)
			if joined != nil {
				out[srcIdx] = joined
			}
			continue
		}
		out[srcIdx] = t
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// typedCalleeArgContracts derives the precondition each of ref's unannotated
// parameters proves by being forwarded, as a bare identifier, to a TYPED parameter
// of a callee invoked in ref's body. `local function helper(client, model_id)
// return client.invoke(model_id, ...) end` proves model_id: string when invoke's
// first parameter is declared string, because the body unconditionally passes
// model_id into a slot the callee requires to be a string.
//
// The obligation is recorded only when the callee's parameter type is CONCRETE: a
// gradual `any`/`unknown` callee parameter (an untyped client.invoke) imposes
// nothing, so the forwarded parameter stays gradual and an importer passing any
// value still type-checks. The result is keyed by ref's SOURCE parameter index.
func (d *Driver) typedCalleeArgContracts(prog *program, ref summary.FuncRef) map[int]typ.Type {
	g := prog.graphs[ref]
	if g == nil {
		return nil
	}
	bindings := g.Bindings()
	if bindings == nil {
		return nil
	}
	paramBySym := d.paramSourceIndexBySym(g)
	if len(paramBySym) == 0 {
		return nil
	}
	ct := callTyper{d: d, g: g}
	exprType := d.bodyParamExprType(prog, ref, g)
	// Resolving a body callee's signature re-enters signatureForRef; the guard makes
	// that resolution return the base (declared) signature instead of recursing back
	// into the contract narrowing. The declared parameter types are all the derivation
	// reads, so the guard loses no information.
	prev := d.derivingContracts
	d.derivingContracts = true
	defer func() { d.derivingContracts = prev }()
	out := make(map[int]typ.Type)
	g.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Call == nil {
			return
		}
		call := info.Call
		callee := unwrap.Function(ct.resolveCallee(call.Func, exprType))
		if callee == nil {
			return
		}
		// A method call inserts an implicit receiver into the callee parameter list; a
		// dotted field/function call does not. Mirror the call-typing parameter offset
		// so an argument maps to the parameter slot it actually fills.
		paramOffset := 0
		if call.Method != "" {
			paramOffset = 1
		}
		for argIdx, arg := range call.Args {
			ident, ok := arg.(*ast.IdentExpr)
			if !ok {
				continue
			}
			sym, ok := bindings.SymbolOf(ident)
			if !ok || sym == 0 {
				continue
			}
			srcIdx, isParam := paramBySym[sym]
			if !isParam {
				continue
			}
			expected := calleeParamType(callee, argIdx+paramOffset)
			if expected == nil || typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
				continue
			}
			if prev, ok := out[srcIdx]; ok && prev != nil {
				if joined := paramevidence.HardContractJoin(prev, expected); joined != nil {
					out[srcIdx] = joined
				}
				continue
			}
			out[srcIdx] = expected
		}
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

// calleeParamType is callee's runtime parameter type at slot idx: the declared
// parameter type (unwrapped to its non-optional runtime form), or the variadic
// element for an over-arity slot. An out-of-range slot with no variadic yields nil.
func calleeParamType(callee *typ.Function, idx int) typ.Type {
	if callee == nil || idx < 0 {
		return nil
	}
	if idx < len(callee.Params) {
		p := callee.Params[idx]
		if p.Optional {
			// An optional callee parameter admits nil, so it does not prove a hard
			// non-nilable precondition on the forwarded argument.
			return nil
		}
		return p.Type
	}
	return callee.Variadic
}

// paramSourceIndexBySym maps each of g's parameter symbols to its SOURCE parameter
// index (the position in the source parameter list), skipping an implicit method
// receiver slot that has no source position.
func (d *Driver) paramSourceIndexBySym(g *cfg.Graph) map[cfg.SymbolID]int {
	if g == nil {
		return nil
	}
	slots := g.ParamSlotsReadOnly()
	if len(slots) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]int, len(slots))
	for _, slot := range slots {
		if slot.Symbol == 0 {
			continue
		}
		srcIdx, ok := slot.SourceParamIndex()
		if !ok {
			continue
		}
		out[slot.Symbol] = srcIdx
	}
	return out
}

// bodyParamExprType resolves a body expression's type for callee resolution during
// the contract derivation: a read of one of ref's parameters resolves to its
// call-site-inferred type (so a forwarded `client` resolves to the imported record
// whose `.invoke` member carries the typed signature). Any other expression resolves
// to the value-domain unknown, so callee resolution falls back to the module-wide
// signatures and globals.
func (d *Driver) bodyParamExprType(prog *program, ref summary.FuncRef, g *cfg.Graph) func(ast.Expr) typ.Type {
	bindings := g.Bindings()
	paramBySym := d.paramSourceIndexBySym(g)
	inferred := prog.inferredParams[ref]
	var resolve func(ast.Expr) typ.Type
	resolve = func(e ast.Expr) typ.Type {
		switch ex := e.(type) {
		case *ast.IdentExpr:
			if bindings == nil {
				return typ.Unknown
			}
			sym, ok := bindings.SymbolOf(ex)
			if !ok || sym == 0 {
				return typ.Unknown
			}
			if srcIdx, isParam := paramBySym[sym]; isParam {
				if t, ok := inferred[srcIdx]; ok && t != nil && !typ.IsAbsentOrUnknown(t) {
					return t
				}
			}
			return typ.Unknown
		case *ast.AttrGetExpr:
			// A field/method access off a parameter (`client.invoke`) resolves the base's
			// inferred type, then the member, so the callee resolution sees the typed
			// member function the forwarded parameter exposes.
			key, isField := ex.Key.(*ast.StringExpr)
			if !isField || key.Value == "" {
				return typ.Unknown
			}
			name := key.Value
			base := resolve(ex.Object)
			if base == nil || typ.IsAbsentOrUnknown(base) {
				return typ.Unknown
			}
			member, ok := fieldMemberType(base, name)
			if !ok || member == nil {
				return typ.Unknown
			}
			return member
		default:
			return typ.Unknown
		}
	}
	return resolve
}

// returnTypesForRef is ref's return tuple as concrete types. A declared return
// annotation is authoritative (it is the function's contract, resolved through the
// annotation resolver against sc — the generic type-param scope when ref is
// generic, so a returned `T` resolves to the type parameter the pipeline
// instantiates); a function with no annotation falls back to the converged summary
// returns the body produced.
func (d *Driver) returnTypesForRef(prog *program, ref summary.FuncRef, sc *scope.State) []typ.Type {
	fn := prog.funcExprs[ref]
	if fn != nil && len(fn.ReturnTypes) > 0 {
		declared := make([]typ.Type, 0, len(fn.ReturnTypes))
		any := false
		for _, rt := range fn.ReturnTypes {
			if rt == nil {
				declared = append(declared, typ.Unknown)
				continue
			}
			t := d.resolveType(rt, sc)
			if t == nil {
				t = typ.Unknown
			} else {
				any = true
			}
			declared = append(declared, t)
		}
		if any {
			return declared
		}
	}
	// No declared return: the function's return tuple is its inferred summary. Prefer
	// the already-converged summary; during the in-flight solve (d.summaries not yet
	// populated) read the callee's current summary through the live query, so the
	// call-graph fixpoint resolves an intra-module callee's inferred return and
	// records the proper callee dependency.
	if returns := d.ReturnTypes(ref); len(returns) > 0 {
		return returns
	}
	return d.liveReturnTypes(ref)
}

// liveReturnTypes reads ref's CURRENT return tuple from the in-flight summary query,
// for a call typed during the solve before d.summaries is populated. It returns nil
// when no live query is active (the converged path already handled it) or the
// current summary carries no returns yet.
func (d *Driver) liveReturnTypes(ref summary.FuncRef) []typ.Type {
	if d.activeQueries == nil || d.activeCtx == nil {
		return nil
	}
	s := d.activeQueries.Summarize(d.activeCtx, ref)
	if len(s.Returns) == 0 {
		return nil
	}
	out := make([]typ.Type, len(s.Returns))
	for i, av := range s.Returns {
		out[i] = projectValue(av)
	}
	return d.applyReturnMethodWrites(ref, out)
}

// edgeNarrower extracts the per-edge path-sensitive narrowing seam from a
// function's transfer when it implements it (the canonical transfer does), so the
// observation surface refines a branch predecessor's out-state by the guard the
// observed edge carries. A transfer without the seam yields nil (no narrowing,
// path-insensitive observation).
func edgeNarrower(t equation.NodeTransfer) equation.EdgeNarrower {
	n, ok := t.(equation.EdgeNarrower)
	if !ok {
		return nil
	}
	return n
}

// projectValue recovers the full value-domain type (shape plus nilability) an
// AbstractValue carries, the canonical egress at the diagnostic boundary. A zero
// value (no interned node) projects to the value-domain default unknown — the
// sound over-approximation for a slot the transfer did not establish.
func projectValue(av product.AbstractValue) typ.Type {
	if av.IsZero() {
		return typ.Unknown
	}
	return av.ProjectValue()
}

// symKey mirrors the canonical transfer's Env key convention: a symbol's value is
// keyed by its CFG SymbolID rendered as "s"+id. It matches summary.symKey so a
// reader of the per-point env reads under the same key the transfer wrote.
func symKey(sym cfg.SymbolID) string {
	return "s" + flowSymID(uint64(sym))
}

// symFromKey inverts symKey: it parses a symbol-keyed Env entry ("s"+id) back to
// its SymbolID. A non-symbol key (the return-slot key "r"+i) is not a symbol and
// returns false, so the module-capture scan skips it.
func symFromKey(key string) (cfg.SymbolID, bool) {
	if len(key) < 2 || key[0] != 's' {
		return 0, false
	}
	var v uint64
	for i := 1; i < len(key); i++ {
		c := key[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + uint64(c-'0')
	}
	return cfg.SymbolID(v), true
}

func flowSymID(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// compile-time assertion: the program implements summary.Program.
var _ summary.Program = (*program)(nil)
