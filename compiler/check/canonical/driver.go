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
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/intercept"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
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
	return out
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
	d.summaries = make(map[summary.FuncRef]summary.Summary, len(prog.refs))
	d.states = make(map[summary.FuncRef]state.FunctionState, len(prog.refs))
	d.funcExprs = prog.funcExprs
	// The per-node transfer's call typing resolves callees against the fully built
	// program and runs the call pipeline against this run's query context. Expose
	// them for the solve below, then clear them when the run completes.
	d.activeProgram = prog
	d.activeCtx = sess.Context()
	d.activeQueries = queries
	defer func() { d.activeProgram = nil; d.activeCtx = nil; d.activeQueries = nil }()
	for _, ref := range prog.refs {
		d.summaries[ref] = queries.Summarize(sess.Context(), ref)
		// The converged per-point state shares SummaryQ's cache entry (IntraQ
		// reuses the same compute), so this reads the already-solved fixed point
		// rather than re-solving it.
		d.states[ref] = queries.Intra(sess.Context(), ref)
	}

	d.bridgeResults(sess, prog)
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
	d.moduleCaptures = d.buildModuleCaptures(sess, prog)
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
	return out
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
	return result
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
	for ref, g := range p.graphs {
		tr, ok := p.transfers[ref].(*transfer.Transfer)
		if !ok || tr == nil {
			continue
		}
		tr.SetSiblingNils(d.siblingNilBinds(p, g))
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
	// A method body's implicit `self` (function T:m()) is seeded with the receiver's
	// class so the flow tracks self.field reads: without it self is unknown and every
	// captured field, the locals it feeds, and the records it builds collapse to
	// unknown. The receiver class resolves from its type-name binding (the named-type
	// path that needs no converged value), so it is available at program-build time. A
	// value receiver (an anonymous table) has no named binding here and stays unseeded
	// (the sound carry-forward), recovered through the observation surface's capture.
	tr := transfer.New(in, nil, funcTyper{d}, callTyper{d: d, g: g}, d.typeCheckBinds(g), nil, declared, d.methodSelfSeed(p, g))
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
	p.refs = append(p.refs, ref)
	return ref
}

// methodSelfSeed resolves the implicit `self` type to seed a method body's entry
// state with. It applies only to a method/field definition (function T:m()) whose
// self the user did not annotate, and only when the receiver names a module type
// resolvable now (the named-type path, which needs no converged value). A value
// receiver (an anonymous table) or an explicitly annotated self yields nil, so the
// transfer leaves self unseeded (the sound carry-forward). The parent graph records
// the method's FuncDefInfo before this function's graph is dequeued, so methodDefs is
// populated by the time addFunction runs for the method body.
func (d *Driver) methodSelfSeed(p *program, g *cfg.Graph) typ.Type {
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
	ident, ok := info.Receiver.(*ast.IdentExpr)
	if !ok || ident == nil {
		return nil
	}
	sc := d.baseScope()
	if sc == nil {
		return nil
	}
	named, ok := sc.LookupType(ident.Value)
	if !ok || named == nil || typ.IsAbsentOrUnknown(named) {
		return nil
	}
	return named
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
// resolution). It seeds every point from the module base scope (which already
// carries the module's type definitions) and layers each graph-local TypeDef
// visible at the point, reusing the legacy scope.BuildTypeDefScopes. A bare
// reference to a type name (Point:is(...), M.Snapshot = Snapshot) then resolves to
// a type rather than an undefined variable.
func (d *Driver) buildPointScopes(g *cfg.Graph) map[cfg.Point]*scope.State {
	if g == nil || d.resolver == nil {
		return nil
	}
	return scope.BuildTypeDefScopes(g, d.baseScope(), d.typeDefResolver())
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
			return provesErrorReturnFromBody(cg, d.signatureForRef(p, ref))
		}
	}
	// A cross-module or otherwise type-resolved callee: read the convention off the
	// resolved signature's ErrorReturn label when present.
	return signatureHasErrorReturn(d.calleeSignatureFor(ct, call.Call))
}

// calleeSignatureFor resolves a call's callee to a function signature for
// correlation lookup, trying the callee-identifier binding then a field-path
// callee (M.f). It is the annotation/summary-resolved signature, not the live Env
// value, so it is available at program-build time.
func (d *Driver) calleeSignatureFor(ct callTyper, call *ast.FuncCallExpr) typ.Type {
	if call == nil {
		return nil
	}
	if ident, ok := call.Func.(*ast.IdentExpr); ok && ident != nil {
		if sig := ct.calleeSignature(ident); sig != nil {
			return sig
		}
	}
	return ct.fieldCalleeSignature(call.Func)
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
func provesErrorReturnFromBody(cg *cfg.Graph, sig typ.Type) bool {
	const valueIdx, errorIdx = 0, 1
	sawSuccess, sawFailure, inconsistent, classified := false, false, false, false
	cg.EachReturn(func(_ cfg.Point, info *cfg.ReturnInfo) {
		if inconsistent || info == nil || len(info.Exprs) == 0 {
			return
		}
		if errorIdx >= len(info.Exprs) {
			inconsistent = true
			return
		}
		valState, okVal := classifyReturnSlotNil(info.Exprs[valueIdx])
		errState, okErr := classifyReturnSlotNil(info.Exprs[errorIdx])
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
	return returns, true
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
// callee that does not resolve to a module function (a stdlib or imported call)
// yields none.
func (ct callTyper) ParamNarrows(call *ast.FuncCallExpr) []transfer.ParamNarrow {
	if call == nil {
		return nil
	}
	prog := ct.d.activeProgram
	if prog == nil {
		return nil
	}
	ref, ok := ct.resolveCalleeRef(call, prog)
	if !ok {
		return nil
	}
	return prog.paramNarrows[ref]
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
// function literal's declared signature from its annotations alone, with no
// dependence on the interprocedural summary fixpoint. The transfer runs inside the
// fixpoint solve, so it can read declared annotations (stable inputs) but not the
// not-yet-converged summary returns.
type funcTyper struct{ d *Driver }

// FuncType builds fn's signature from its declared parameter and return
// annotations. The result is the structural callable a function-valued table field
// carries.
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
	return builder.Build()
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
	return builder.Build()
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
	return out
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
