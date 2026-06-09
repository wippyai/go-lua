package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
	"github.com/wippyai/go-lua/compiler/check/domain/paramboundary"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/scope"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/core"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/query/typepath"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// observation.go is the diagnostic bridge's observation surface. The diagnostic
// passes ask the observation.Projector for "the type of expression E at point P";
// the Projector answers per-symbol/per-point reads through flow.TypeFacts and
// resolves declared annotations through a Synth. This file projects those
// questions onto the converged FunctionState.Points.
//
// The surface is built from:
//
//   - canonicalFacts: a flow.TypeFacts backed by the converged per-point env. It
//     answers EffectiveTypeAt(point, sym) from the in-state of point (the join of
//     its predecessors' converged out-states), DeclaredAt/IsAnnotated from the
//     function's declared-type map. This is exactly what the Projector's
//     symbolType/declaredSymbolType reads.
//   - the per-function flow.Inputs.{DeclaredTypes, AnnotatedVars}, derived from the
//     parameter annotations and annotated local declarations, which WithAssign and
//     WithIdent read directly.
//   - a returnSynth: a thin api.Synth whose annotation resolution is the driver's
//     resolver and whose expression typing delegates to the canonical observation
//     Projector. WithReturn reads it only to resolve declared return annotations;
//     it is the same resolver and the same projector the rest of the surface uses,
//     not a parallel type checker.
//
// functionObservationContext is the per-function immutable source context the
// canonical observer derives once from the graph: resolved annotations,
// annotated-symbol membership, binding signatures, and parameter symbol layout.
// It is not exported api.FunctionFacts; those are final Summary-derived output.
type functionObservationContext struct {
	declared  map[cfg.SymbolID]typ.Type
	annotated flow.AnnotatedSymbols
	// bindings are immutable value-binding facts, not source declarations. They
	// carry canonical signatures for named/local function bindings so effective
	// reads and identifier-definedness can see the binding without polluting
	// DeclaredAt or FlowInputs.DeclaredTypes.
	bindings map[cfg.SymbolID]typ.Type
	// paramSyms are the function's parameter symbols in declaration order. An
	// unannotated parameter (not in annotated, with no declared type) is a gradual
	// `any` when the body imposes no obligation on it: a Lua parameter with no
	// annotation is dynamic and usable in every operation.
	paramSyms []cfg.SymbolID
}

func cloneFunctionObservationContext(in functionObservationContext) functionObservationContext {
	out := functionObservationContext{}
	if len(in.declared) > 0 {
		out.declared = make(map[cfg.SymbolID]typ.Type, len(in.declared))
		for sym, t := range in.declared {
			out.declared[sym] = t
		}
	} else {
		out.declared = make(map[cfg.SymbolID]typ.Type)
	}
	out.annotated = in.annotated.Clone()
	if len(in.bindings) > 0 {
		out.bindings = make(map[cfg.SymbolID]typ.Type, len(in.bindings))
		for sym, t := range in.bindings {
			out.bindings[sym] = t
		}
	} else {
		out.bindings = make(map[cfg.SymbolID]typ.Type)
	}
	if len(in.paramSyms) > 0 {
		out.paramSyms = append([]cfg.SymbolID(nil), in.paramSyms...)
	}
	return out
}

// buildFunctionObservationContext resolves the static source context every part
// of the observation surface reads. Annotations resolve against the module base
// scope through the driver's resolver.
func (d *Driver) buildFunctionObservationContext(g *cfg.Graph, evidence api.FlowEvidence) functionObservationContext {
	obsCtx := functionObservationContext{
		declared: make(map[cfg.SymbolID]typ.Type),
	}
	if g == nil {
		return obsCtx
	}

	// Predeclared globals: a use of a predeclared name (print, pairs, require, ...)
	// resolves to its global symbol; the declared-type map carries its value type so
	// the ident pass sees it as defined and the observation surface types it as its
	// function/value type rather than the value-domain unknown. The driver admits
	// Config.GlobalTypes into a deterministic globalenv.TypeOverlay at construction;
	// this bridge consumes that carrier rather than the raw external map.
	if len(d.globalTypes) > 0 {
		bindings := g.Bindings()
		for _, binding := range d.globalTypes {
			name := binding.Name.String()
			t := binding.Type
			sym, ok := g.GlobalSymbol(name)
			if !ok {
				continue
			}
			if _, exists := obsCtx.declared[sym]; exists {
				continue
			}
			if bindings != nil {
				if k, ok := bindings.Kind(sym); ok && k != cfg.SymbolGlobal {
					continue
				}
			}
			obsCtx.declared[sym] = t
		}
	}

	// Parameters: a declared annotation pins the parameter symbol's declared type,
	// resolved from the function's parameter list. The canonical ParamSlots layout
	// maps each parameter slot to its source annotation, accounting for an implicit
	// method receiver `self` at slot 0 (SourceIndex -1): a `function T:m(x: A)` binds
	// self and x, so x's annotation `A` aligns with the second slot, not the first.
	// Reading the raw ParList.Types in slot order would shift every method parameter's
	// declared type by one. A generic function's annotations resolve in its type-param
	// scope, so a parameter typed `T` carries the bounded type parameter rather than an
	// unresolved typ.Ref; the body method/field check then reads the bound's members.
	annScope := d.typeParamScope(g.Func())
	params := g.ParamSymbols()
	obsCtx.paramSyms = params
	for _, slot := range g.ParamSlotsReadOnly() {
		if slot.Symbol == 0 || slot.TypeAnnotation == nil {
			continue
		}
		t := d.resolveType(slot.TypeAnnotation, annScope)
		if t == nil {
			continue
		}
		obsCtx.declared[slot.Symbol] = t
		obsCtx.annotated.Add(slot.Symbol)
	}

	// Annotated local declarations: local x: T = ... pins x's declared type from
	// its aligned annotation. The annotation resolves against the block-aware scope
	// LEXICALLY VISIBLE at the declaration point, not the flat module scope: a
	// reference to a block-local type used outside its block, or a forward reference
	// to a type defined later, then resolves to nothing (the declaration mismatches
	// the unresolved annotation), and a shadowed type name resolves to the binding
	// active at the declaration rather than the innermost block's definition.
	pointScopes := d.buildPointScopes(g)
	for _, assign := range evidence.Assignments {
		info := assign.Info
		if info == nil || !info.IsLocal {
			continue
		}
		declScope := annScope
		if pointScopes != nil {
			if sc, ok := pointScopes[assign.Point]; ok && sc != nil {
				declScope = d.genericScopeOver(nil, g.Func(), sc)
			}
		}
		for i := range info.TypeAnnotations {
			ann := info.TypeAnnotationAt(i)
			if ann == nil {
				continue
			}
			target, ok := info.TargetAt(i)
			if !ok || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			// A parameter symbol the param loop already resolved (in the function's
			// type-param scope) is authoritative: an implicit param-binding assignment
			// carries the same annotation, but re-resolving it here against the base
			// scope would drop a generic parameter's bound (`x: T` -> unresolved Ref
			// instead of the bounded type parameter). Leave the param's declared type
			// intact; this loop pins only genuine local declarations.
			if _, isParam := obsCtx.declared[target.Symbol]; isParam && obsCtx.annotated.Contains(target.Symbol) {
				continue
			}
			// Resolve a local declaration against the scope lexically visible at its
			// declaration point, extended with the function's type parameters (so a
			// local typed by a type parameter — `local result: {U}` inside `map<T, U>` —
			// still carries the same bounded type parameter the parameter and
			// call-result types carry, and an element write `result[i] = f(v)` compares
			// `U` against `U` consistently). A block-local, forward, or shadowed type
			// name then resolves to the binding actually visible here.
			t := d.resolveType(ann, declScope)
			if t == nil {
				continue
			}
			obsCtx.declared[target.Symbol] = t
			obsCtx.annotated.Add(target.Symbol)
		}
	}
	return obsCtx
}

// seedMethodSelf records only source-declared receiver facts for a method body's
// implicit `self` parameter. A method defined on a named type (`function T:m()`
// where T is a type binding) has a declared receiver contract, so the diagnostic
// bridge may mark self annotated to T. A value receiver (`local methods = {};
// function methods:m()`) has no source annotation: its runtime self is produced by
// the PrototypeSelf product axis, and marking it declared from moduleCaptures would
// leak the old driver scan back into the bridge.
//
// A self parameter the user annotated explicitly is left untouched. A value
// receiver stays unannotated so EffectiveTypeAt observes the solved point-state
// value seeded through EntryValues/PrototypeSelf.
func (d *Driver) seedMethodSelf(obsCtx *functionObservationContext, prog *program, g *cfg.Graph) {
	if obsCtx == nil || prog == nil || g == nil {
		return
	}
	fn := g.Func()
	if fn == nil {
		return
	}
	ref, ok := prog.refByFunc(fn)
	if !ok {
		return
	}
	info := prog.methodDef(ref)
	if info == nil || info.Receiver == nil {
		return
	}
	bindings := g.Bindings()
	if bindings == nil {
		return
	}
	// Only an unannotated self (implicit method self, or an explicit unannotated
	// `self`) is seeded; an explicit annotation is authoritative.
	if !phasecore.HasUnannotatedSelfParam(fn, bindings) {
		return
	}
	params := g.ParamSymbols()
	if len(params) == 0 {
		return
	}
	selfSym := params[0]
	if selfSym == 0 || bindings.Name(selfSym) != "self" {
		return
	}
	recv := d.namedReceiverType(info, d.baseScope())
	if recv == nil || typ.IsAbsentOrUnknown(recv) {
		return
	}
	obsCtx.declared[selfSym] = recv
	obsCtx.annotated.Add(selfSym)
}

// namedReceiverType resolves only an explicit type-namespace receiver binding.
// It intentionally does not fall back to moduleCaptures: value-receiver self
// values are flow facts owned by the PrototypeSelf point-state axis.
func (d *Driver) namedReceiverType(info *cfg.FuncDefInfo, sc *scope.State) typ.Type {
	if info == nil {
		return nil
	}
	if ident, ok := info.Receiver.(*ast.IdentExpr); ok && ident != nil {
		if sc == nil {
			sc = d.baseScope()
		}
		if sc != nil {
			if named, ok := sc.LookupType(ident.Value); ok && named != nil && !typ.IsAbsentOrUnknown(named) {
				return named
			}
		}
	}
	return nil
}

// recordFunctionBindingTypes records each function-binding symbol's canonical
// signature as an immutable binding fact. These facts are definition/value facts:
// they make named functions observable through EffectiveTypeAt and the identifier
// pass without becoming source annotations. A source declaration remains
// authoritative when both exist.
func recordFunctionBindingTypes(obsCtx *functionObservationContext, funcSigs map[cfg.SymbolID]typ.Type, g *cfg.Graph) {
	if obsCtx == nil {
		return
	}
	if len(funcSigs) == 0 || g == nil {
		return
	}
	if obsCtx.bindings == nil {
		obsCtx.bindings = make(map[cfg.SymbolID]typ.Type, len(funcSigs))
	}
	for sym, sig := range funcSigs {
		if sym == 0 || sig == nil {
			continue
		}
		if _, exists := obsCtx.declared[sym]; exists {
			continue
		}
		obsCtx.bindings[sym] = sig
	}
}

// recordCallbackEnvBindingTypes records callback-scoped global overlay facts as
// immutable value bindings. The facts package has already lowered overlay names
// to this callback body's graph symbols, so the bridge only admits those
// normalized facts into the same non-declaration surface used for function
// bindings. Source declarations remain authoritative when both exist.
func recordCallbackEnvBindingTypes(obsCtx *functionObservationContext, entries []callbackenv.GlobalBinding) {
	if obsCtx == nil || len(entries) == 0 {
		return
	}
	if obsCtx.bindings == nil {
		obsCtx.bindings = make(map[cfg.SymbolID]typ.Type, len(entries))
	}
	for _, entry := range entries {
		if entry.Symbol == 0 || entry.Type == nil || typ.IsAbsentOrUnknown(entry.Type) {
			continue
		}
		if _, exists := obsCtx.declared[entry.Symbol]; exists {
			continue
		}
		obsCtx.bindings[entry.Symbol] = entry.Type
	}
}

// canonicalFacts is the flow.TypeFacts the diagnostic passes' observation
// Projector reads, backed by the converged FunctionState. It answers a per-point
// per-symbol type query from the converged env and a declared-type query from the
// function's resolved annotations.
type canonicalFacts struct {
	graph    *cfg.Graph
	state    state.FunctionState
	declared map[cfg.SymbolID]typ.Type
	annotate flow.AnnotatedSymbols
	bindings map[cfg.SymbolID]typ.Type
	consts   map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue

	// unannotatedParams is the set of parameter symbols with no declared annotation.
	// A read of one whose converged value is the value-domain unknown resolves to
	// gradual `any` (a Lua parameter without an annotation is dynamic, usable in
	// every operation). A parameter the body constrains carries its inferred value
	// and is not defaulted.
	unannotatedParams functionsymbols.Set

	paths   pathProjector
	driver  *Driver
	program *program
	reader  summary.Reader
}

// newCanonicalFacts builds the per-function diagnostic facts over the solved
// FunctionState. The per-point in-state it reads is the solver-derived
// state.FunctionState.InPoints, so the graph the solve ran over is not
// re-consulted here (the in-state is read, never re-derived).
func (d *Driver) newCanonicalFacts(g *cfg.Graph, fs state.FunctionState, obsCtx functionObservationContext, prog *program, reader summary.Reader, _ api.FlowEvidence) *canonicalFacts {
	unannotated := paramboundary.UnannotatedRootsFromFacts(obsCtx.paramSyms, obsCtx.declared, obsCtx.annotated)
	var consts map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue
	if prog != nil && g != nil {
		if ref, ok := prog.refByGraph(g); ok {
			consts = prog.inputs[ref].ConstValues
		}
	}
	callables := newCallableProjector(d, prog, reader)
	return &canonicalFacts{
		graph:             g,
		state:             fs,
		declared:          obsCtx.declared,
		annotate:          obsCtx.annotated,
		bindings:          obsCtx.bindings,
		consts:            consts,
		unannotatedParams: unannotated,
		paths:             newPathProjector(fs, unannotated, callables),
		driver:            d,
		program:           prog,
		reader:            reader,
	}
}

// CallReturnTypesAt is the diagnostic observation bridge for call expressions.
// It projects the call through the same selected-target CallOutcome carrier the
// transfer path uses, instead of reinterpreting the callee from a possibly
// polluted expression type. This keeps declared-return checks on the canonical
// summary/entry-context path.
func (f *canonicalFacts) CallReturnTypesAt(point cfg.Point, call *ast.FuncCallExpr, expected typ.Type) ([]typ.Type, bool) {
	if f == nil || f.driver == nil || f.program == nil || call == nil {
		return nil, false
	}
	callPoint := f.callObservationPoint(point, call)
	if values, ok := f.intrinsicCallReturnTypesAt(callPoint, call); ok {
		return values, true
	}
	callCtx, ok := f.productCallContextAt(callPoint, call)
	if !ok {
		return nil, false
	}
	ct := callTyper{d: f.driver, g: f.graph}
	site, ok := ct.productCallSiteFrame(call, callCtx)
	if !ok {
		return nil, false
	}
	outcome := ct.productCallOutcomeProjection(
		site,
		callCtx,
		productCallOutcomeOptions{},
		func(ctx canonicalcall.EntryContext) summary.Summary {
			return ct.summaryForCallEntryContext(ctx)
		},
	).outcome()
	values := outcome.InferredReturnValues()
	if len(values) == 0 {
		return nil, false
	}
	return product.ProjectValuesOrUnknown(values), true
}

func (f *canonicalFacts) callObservationPoint(fallback cfg.Point, call *ast.FuncCallExpr) cfg.Point {
	if f == nil || f.graph == nil || call == nil {
		return fallback
	}
	out := fallback
	f.graph.EachCallSite(func(point cfg.Point, info *cfg.CallInfo) {
		if info != nil && info.Call == call {
			out = point
		}
	})
	return out
}

func (f *canonicalFacts) intrinsicCallReturnTypesAt(point cfg.Point, call *ast.FuncCallExpr) ([]typ.Type, bool) {
	tr, ok := f.transfer()
	if !ok {
		return nil, false
	}
	in := f.inState(point)
	values, ok := tr.IntrinsicCallReturnValues(&in, call, nil)
	if !ok || len(values) == 0 {
		return nil, false
	}
	return product.ProjectValuesOrUnknown(values), true
}

func (f *canonicalFacts) productCallContextAt(point cfg.Point, call *ast.FuncCallExpr) (transfer.ProductCallContext, bool) {
	if f == nil || f.program == nil || f.graph == nil || call == nil {
		return transfer.ProductCallContext{}, false
	}
	tr, ok := f.transfer()
	if !ok {
		return transfer.ProductCallContext{}, false
	}
	in := f.inState(point)
	return tr.ProductCallContext(&in, call), true
}

func (f *canonicalFacts) transfer() (*transfer.Transfer, bool) {
	if f == nil || f.program == nil || f.graph == nil {
		return nil, false
	}
	ref, ok := f.program.refByGraph(f.graph)
	if !ok {
		return nil, false
	}
	tr, ok := f.program.transfers[ref].(*transfer.Transfer)
	if !ok || tr == nil {
		return nil, false
	}
	return tr, true
}

func (f *canonicalFacts) observedExprTypeAt(point cfg.Point, expr ast.Expr, expected typ.Type) typ.Type {
	if expr == nil {
		return nil
	}
	path := flowpath.FromExprWithBindings(expr, nil, f.graph.Bindings())
	if !path.IsEmpty() && path.Symbol != 0 {
		if expected != nil && expr == nil {
			return expected
		}
		if tv := f.RefinedPathAt(point, path); tv.Type != nil {
			return tv.Type
		}
		if tv := f.DeclaredAt(point, path.Symbol); tv.Type != nil && !typ.IsAbsentOrUnknown(tv.Type) {
			return tv.Type
		}
	}
	return typ.Unknown
}

// compile-time assertion: canonicalFacts implements the observation surface.
var _ flow.TypeFacts = (*canonicalFacts)(nil)
var _ flow.BindingValueFacts = (*canonicalFacts)(nil)
var _ flow.PathFacts = (*canonicalFacts)(nil)
var _ flow.ProductFacts = (*canonicalFacts)(nil)
var _ flow.ProductPathFacts = (*canonicalFacts)(nil)
var _ flow.ProductPathObservationFacts = (*canonicalFacts)(nil)
var _ flow.PathChildFacts = (*canonicalFacts)(nil)
var _ flow.AssignmentSourceFacts = (*canonicalFacts)(nil)
var _ flow.TransferValueFacts = (*canonicalFacts)(nil)
var _ flow.ConditionProofFacts = (*canonicalFacts)(nil)
var _ flow.PathObservationFacts = (*canonicalFacts)(nil)
var _ flow.ConstFacts = (*canonicalFacts)(nil)

// compile-time assertion: canonicalFacts also exposes the length proof the
// observation surface consults to refine an in-bounds index read.
var _ flow.LengthFacts = (*canonicalFacts)(nil)
var _ flow.PathLengthFacts = (*canonicalFacts)(nil)

// compile-time assertion: canonicalFacts exposes the numeric proofs the
// observation surface consults to refine an in-range dynamic-index read.
var _ flow.NumericFacts = (*canonicalFacts)(nil)

// compile-time assertion: canonicalFacts is also the canonical producer's
// normalized solved-flow projection. It supplies FlowOps directly from the
// product state, without constructing a solver-shaped carrier it did not compute.
var _ api.FlowOps = (*canonicalFacts)(nil)

// DeclaredAt returns the declared (annotated) type of sym. Declared types are
// flow-insensitive, so the point is unused.
func (f *canonicalFacts) DeclaredAt(_ cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if t, ok := f.declared[sym]; ok && t != nil {
		return flow.TypedValue{Type: t, State: flow.StateResolved}
	}
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

// ConstValueAtSym exposes canonical passive const facts to the diagnostic
// observation boundary. These facts normalize exact paths such as obj[key] when
// key is a proven literal; they do not interpret values or add a second fixpoint.
func (f *canonicalFacts) ConstValueAtSym(p cfg.Point, sym cfg.SymbolID) *flow.ConstValue {
	if f == nil || sym == 0 || f.consts == nil {
		return nil
	}
	at := f.consts[sym]
	if at == nil {
		return nil
	}
	val := at[p]
	if val == nil || val.Kind == flow.ConstUnknown {
		return nil
	}
	return val
}

func (f *canonicalFacts) bindingTypeAt(sym cfg.SymbolID) flow.TypedValue {
	if t, ok := f.bindings[sym]; ok && t != nil {
		return flow.TypedValue{Type: t, State: flow.StateResolved}
	}
	return flow.TypedValue{Type: nil, State: flow.StateUnknown}
}

// BindingValueAt returns immutable function-binding value facts without treating
// them as source declarations.
func (f *canonicalFacts) BindingValueAt(_ cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	return f.bindingTypeAt(sym)
}

func (f *canonicalFacts) staticEffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	declared := f.DeclaredAt(p, sym)
	if f.IsAnnotated(sym) && declared.State == flow.StateResolved && declared.Type != nil {
		return declared
	}
	if binding := f.bindingTypeAt(sym); binding.Type != nil {
		return binding
	}
	return declared
}

// RefinedAt returns the flow-narrowed type of sym at point p: the converged value
// of sym in the in-state of p (the join of p's reachable predecessors' out-states,
// or p's own seeded state at the entry). A symbol with no converged value has no
// refinement.
func (f *canonicalFacts) RefinedAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	return f.refinedAt(p, sym, false)
}

func (f *canonicalFacts) PostEffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	refined := f.refinedAt(p, sym, true)
	if refined.Type != nil {
		return refined
	}
	return f.staticEffectiveTypeAt(p, sym)
}

func (f *canonicalFacts) PostRefinedPathAt(p cfg.Point, path constraint.Path) flow.TypedValue {
	if len(path.Segments) == 0 {
		return f.refinedAt(p, path.Symbol, true)
	}
	return f.paths.WithPostState().RefinedPathAt(p, path)
}

func (f *canonicalFacts) refinedAt(p cfg.Point, sym cfg.SymbolID, post bool) flow.TypedValue {
	if sym == 0 {
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	in := f.pointState(p, post)
	av, ok := flow.PointFactsOf(in).SymbolValue(sym)
	// A function-binding symbol's summary-sensitive identity is point-state data:
	// FunctionRefs names the body, while Env/Cells carries the callable shape. Read
	// that product before projecting the structural function value; do not recover
	// precision from the driver's module-wide function-signature map.
	envIsFunc := false
	if ok && !av.IsZero() {
		if t := product.ProjectValueOrUnknown(av); t != nil && t.Kind() == kind.Function {
			envIsFunc = true
		}
	}
	if !ok || av.IsZero() || envIsFunc {
		if sig := f.paths.callables.TypeAt(in, constraint.NewPath(sym, "")); !typ.IsAbsentOrUnknown(sig) {
			return flow.TypedValue{Type: sig, State: flow.StateResolved}
		}
	}
	if !ok || av.IsZero() {
		// Captured free variables are owned by PointState.Cells. If the solved
		// in-state has no cell/env value, observation must not recover precision from
		// a module-wide side map; that would mask a missing capture seed.
		// An unannotated parameter the body imposes no obligation on is gradual
		// `any` (dynamic, usable in every operation), not opaque unknown.
		if f.unannotatedParams.Contains(sym) {
			return flow.TypedValue{Type: typ.Any, State: flow.StateResolved}
		}
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	t := product.ProjectValueOrUnknown(av)
	if t == nil || typ.IsUnknown(t) {
		// A converged-but-unknown value for an unannotated parameter is the gradual
		// default: the body did not pin it to a concrete type, so it stays `any`.
		if f.unannotatedParams.Contains(sym) {
			return flow.TypedValue{Type: typ.Any, State: flow.StateResolved}
		}
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	// A generic parameter typed `T` seeds the bottom-up transfer with an unresolved
	// reference: the transfer resolves the annotation against the module base scope,
	// where the function's type parameter is not bound, so the env carries `Ref{T}`.
	// The authoritative type is the bounded type parameter the declared-type context
	// resolved in the function's type-param scope. Defer to it when the env reference
	// names that same parameter, so a body read of `x` (and the return check) observe
	// `x: T : Printable` rather than the unbound reference; any other refinement (a
	// genuine narrowed value) is returned unchanged.
	if t.Kind() == kind.Ref {
		if declared, ok := f.declared[sym]; ok {
			if tp, isParam := declared.(*typ.TypeParam); isParam && tp.Name == refName(t) {
				return flow.TypedValue{Type: declared, State: flow.StateResolved}
			}
		}
	}
	return flow.TypedValue{Type: t, State: flow.StateResolved}
}

// RefinedValueAt returns the product carrier for sym at p without projecting
// through typ.Type. It is the semantic observation boundary for consumers that
// need carrier evidence such as gradual-top provenance.
func (f *canonicalFacts) RefinedValueAt(p cfg.Point, sym cfg.SymbolID) flow.ProductValue {
	return f.paths.RefinedValueAt(p, sym)
}

// refName is the referenced type name of an unresolved reference, or the empty
// string when t is not a reference.
func refName(t typ.Type) string {
	if ref, ok := t.(*typ.Ref); ok {
		return ref.Name
	}
	return ""
}

// EffectiveTypeAt returns the best available type: the converged refinement if
// present, else the declared type, else unknown.
func (f *canonicalFacts) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	refined := f.RefinedAt(p, sym)
	if refined.Type != nil {
		return refined
	}
	return f.staticEffectiveTypeAt(p, sym)
}

// RefinedPathAt projects a solved product path from the canonical point state.
// Function identities are enriched through the same summary-sensitive ref axis
// RefinedAt uses for root function values; callers still reconcile the result
// against declared source types at the observation boundary.
func (f *canonicalFacts) RefinedPathAt(p cfg.Point, path constraint.Path) flow.TypedValue {
	if len(path.Segments) == 0 {
		return f.RefinedAt(p, path.Symbol)
	}
	return f.paths.RefinedPathAt(p, path)
}

// RefinedPathValueAt projects a solved product path from canonical point state
// without dropping carrier evidence at the typ.Type boundary.
func (f *canonicalFacts) RefinedPathValueAt(p cfg.Point, path constraint.Path) flow.ProductValue {
	return f.paths.RefinedPathValueAt(p, path)
}

// ObserveProductPathValue exposes product-carrier evidence through one
// normalized path-read surface. It preserves gradual-top provenance and other
// product-domain facts that typ.Type projection intentionally erases.
func (f *canonicalFacts) ObserveProductPathValue(q flow.ProductPathObservationQuery) flow.ProductValue {
	if q.Path.IsEmpty() || q.Path.Symbol == 0 {
		return flow.ProductValue{State: flow.StateUnknown}
	}
	paths := f.paths
	if q.View == flow.PathReadPost {
		paths = paths.WithPostState()
	}
	return paths.RefinedPathValueAt(q.Point, q.Path)
}

// ObserveChildPaths exposes finite child path facts already materialized in the
// canonical point state. It does not derive descendants from recursive products.
func (f *canonicalFacts) ObserveChildPaths(q flow.PathChildQuery) []flow.PathFact {
	if f == nil {
		return nil
	}
	return f.paths.ObserveChildPaths(q)
}

// AssignmentSourceValueAt evaluates source-owned assignment evidence against the
// canonical solved product projection. The target annotation/static slot is not
// consulted here; observation reconciles boundary types separately.
func (f *canonicalFacts) AssignmentSourceValueAt(p cfg.Point, target constraint.Path, source flow.AssignmentSource) typ.Type {
	return flow.AssignmentSourceValue(flow.AssignmentSourceQuery{
		Point:  p,
		Target: target,
		Source: source,
		Flow:   f,
	})
}

// AssignedValueTypeAt evaluates assignment-source evidence and reconciles it
// with the statically extracted slot type using the flow transfer law.
func (f *canonicalFacts) AssignedValueTypeAt(p cfg.Point, target constraint.Path, static typ.Type, source flow.AssignmentSource) typ.Type {
	if f == nil {
		return static
	}
	return flow.AssignmentEvidenceType(static, f.AssignmentSourceValueAt(p, target, source))
}

// MutatorValueTypeAt evaluates the lowered mutator value path and nested value
// template against canonical facts.
func (f *canonicalFacts) MutatorValueTypeAt(p cfg.Point, valuePath constraint.Path, static typ.Type, template flow.ValueTemplate) typ.Type {
	if f == nil {
		return static
	}
	valueType := static
	if valuePath.HasSymbol() {
		if resolved := f.NarrowedTypeAt(p, valuePath); !typ.IsAbsentOrUnknown(resolved) {
			valueType = resolved
		}
	}
	if len(template.Slots) == 0 {
		return valueType
	}
	return flow.ApplyValueTemplate(valueType, template, func(source flow.AssignmentSource) typ.Type {
		return f.AssignmentSourceValueAt(p, constraint.Path{}, source)
	})
}

// MutatorKeyTypeAt evaluates the lowered dynamic mutator key path against
// canonical facts and applies the same dynamic-key widening law as transfer.
func (f *canonicalFacts) MutatorKeyTypeAt(p cfg.Point, keyPath constraint.Path, static typ.Type) typ.Type {
	if static == nil && !keyPath.HasSymbol() {
		return nil
	}
	keyType := static
	if f != nil && keyPath.HasSymbol() {
		if resolved := f.NarrowedTypeAt(p, keyPath); !typ.IsAbsentOrUnknown(resolved) {
			keyType = resolved
		}
	}
	return flow.NormalizeDynamicKeyType(keyType)
}

// IndexReadPointFacts selects the solved point-state slice for indexed-read
// reducers. Flow owns the readback algebra after this boundary.
func (f *canonicalFacts) IndexReadPointFacts(p cfg.Point, view flow.PathReadView) flow.PointFacts {
	return flow.PointFactsOf(f.indexReadState(p, view))
}

func (f *canonicalFacts) indexReadState(p cfg.Point, view flow.PathReadView) flow.PointState {
	switch view {
	case flow.PathReadPre:
		return f.inState(p)
	case flow.PathReadPost:
		return f.pointState(p, true)
	default:
		return f.pointState(p, true)
	}
}

func (f *canonicalFacts) ProvenanceRoutesAt(p cfg.Point, path constraint.Path) []flow.ProvenanceRoute {
	return flow.PointFactsOf(f.pointState(p, true)).ProvenanceRoutes(path)
}

func (f *canonicalFacts) AppendElementFieldSourceRoutesAt(p cfg.Point, q flow.AppendElementFieldRouteQuery) []flow.ProvenanceRoute {
	return flow.PointFactsOf(f.pointState(p, true)).AppendElementFieldSourceRoutes(q)
}

func (f *canonicalFacts) BodyContracts() paramevidence.Contracts {
	return f.state.Contracts
}

// IsAnnotated reports whether sym carries an explicit type annotation.
func (f *canonicalFacts) IsAnnotated(sym cfg.SymbolID) bool {
	return f.annotate.Contains(sym)
}

// inState is the converged state entering point p: a pure read of the solver's
// derived per-point IN-state (state.FunctionState.InPoints), the join over p's
// reachable predecessors' edge-narrowed out-states the equation builder computed
// from the same reachable point set and narrower it solved over. A point absent
// from the map is unreachable and yields the empty (Bottom) state.
//
// The in-state is read here, never re-derived: the builder is the single source
// of truth, so the merge-LUB it computed (one guarded branch joined with the
// opposite branch) is observed exactly, and a narrowing never survives past its
// guard.
func (f *canonicalFacts) inState(p cfg.Point) flow.PointState {
	return f.pointState(p, false)
}

func (f *canonicalFacts) pointState(p cfg.Point, post bool) flow.PointState {
	if post {
		if ps, ok := f.state.Points[p]; ok {
			return ps
		}
	}
	if ps, ok := f.state.InPoints[p]; ok {
		return ps
	}
	return flow.PointStateDomain.Bottom()
}

// LengthLowerBoundAt returns the proven lower bound on the length of the container
// symbol sym entering point p. The path-shaped projector owns the flow container
// identity; this symbol compatibility method delegates to that canonical route.
func (f *canonicalFacts) LengthLowerBoundAt(p cfg.Point, sym cfg.SymbolID) (int64, bool) {
	if sym == 0 {
		return 0, false
	}
	return f.LengthLowerBoundForPathAt(p, constraint.Path{Symbol: sym})
}

// LengthLowerBoundForPathAt returns the proven lower bound on a container path,
// including field/index segments below a root symbol (for example #result.items).
func (f *canonicalFacts) LengthLowerBoundForPathAt(p cfg.Point, path constraint.Path) (int64, bool) {
	return f.paths.LengthLowerBoundForPathAt(p, path)
}

// ConditionAt exposes the point condition to the observation surface for
// condition-proof projection.
func (f *canonicalFacts) ConditionAt(p cfg.Point) constraint.Condition {
	return f.inState(p).Cond
}

// ProvesTypeAt answers condition-only type proofs from the point condition.
func (f *canonicalFacts) ProvesTypeAt(p cfg.Point, path constraint.Path, t typ.Type) bool {
	return f.conditionProofProjector().ProvesTypeAt(p, path, t)
}

// ConditionTypeAt returns the path type proven by the point condition.
func (f *canonicalFacts) ConditionTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	return f.conditionProofProjector().ConditionTypeAt(p, path)
}

// ConditionedTypeAt returns the path type proven by the point condition plus an
// expression-local condition.
func (f *canonicalFacts) ConditionedTypeAt(p cfg.Point, path constraint.Path, extra constraint.Condition) typ.Type {
	return f.conditionProofProjector().ConditionedTypeAt(p, path, extra)
}

// ConditionedSeedTypeAt projects a caller-supplied seed type under the point
// condition plus an expression-local condition.
func (f *canonicalFacts) ConditionedSeedTypeAt(p cfg.Point, seedPath constraint.Path, seedType typ.Type, queryPath constraint.Path, extra constraint.Condition) typ.Type {
	return f.conditionProofProjector().ConditionedSeedTypeAt(p, seedPath, seedType, queryPath, extra)
}

// ObservePath implements the high-level path observation policy directly over
// FunctionState. View selection, declared reconciliation, condition proofs,
// authoritative Never, and normalized index-read proofs are all handled here.
func (f *canonicalFacts) ObservePath(q flow.PathObservationQuery) flow.PathObservation {
	if f == nil || q.Path.IsEmpty() {
		return flow.PathObservation{}
	}
	declared := f.pathObservationDeclaredType(q)

	var direct flow.PathObservationCandidate
	if q.LocalCondition == nil && len(q.Path.Segments) > 0 {
		if t, ok := f.pathObservationSolvedType(q); ok {
			direct = flow.PathObservationCandidate{
				Type:   t,
				Source: flow.PathObservationDirectPath,
				OK:     true,
			}
		}
	}

	solved, solvedOK := f.pathObservationSolvedType(q)
	var proof typ.Type
	if q.AllowConditionProof {
		switch {
		case q.LocalCondition != nil:
			proof = f.pathObservationConditionedType(q, solved, declared)
		case f.hasConditionProofAt(q.Point):
			proof = f.ConditionTypeAt(q.Point, q.Path)
		}
	}

	var solvedCandidate flow.PathObservationCandidate
	if solvedOK {
		solvedCandidate = flow.PathObservationCandidate{
			Type:   solved,
			Source: flow.PathObservationFactProjection,
			OK:     true,
		}
	}

	return f.withPathObservationIndexRead(q, flow.SelectPathObservationResult(flow.PathObservationSelection{
		Query:         q,
		Declared:      declared,
		Direct:        direct,
		Solved:        solvedCandidate,
		Proof:         proof,
		AdmitSelected: true,
	}))
}

func (f *canonicalFacts) pathObservationSolvedType(q flow.PathObservationQuery) (typ.Type, bool) {
	var refined flow.TypedValue
	if q.View == flow.PathReadPost {
		refined = f.PostRefinedPathAt(q.Point, q.Path)
	} else {
		refined = f.RefinedPathAt(q.Point, q.Path)
	}
	if len(q.Path.Segments) == 0 {
		if refined.State != flow.StateResolved || typ.IsAbsentOrUnknown(refined.Type) {
			return nil, false
		}
		if root := f.soundPathObservationRoot(q.Point, q.Path.Symbol, refined.Type); root != nil {
			return root, true
		}
		return nil, false
	}
	var direct typ.Type
	if refined.State == flow.StateResolved && !typ.IsAbsentOrUnknown(refined.Type) {
		direct = f.preserveDeclaredPathNilability(q, refined.Type)
	}
	if derived, ok := f.pathObservationRootDerivedType(q); ok {
		if typ.IsAbsentOrUnknown(direct) || pathObservationDerivedPreferred(derived, direct) {
			return derived, true
		}
	}
	if typ.IsAbsentOrUnknown(direct) {
		return nil, false
	}
	return direct, true
}

func (f *canonicalFacts) pathObservationRootDerivedType(q flow.PathObservationQuery) (typ.Type, bool) {
	if q.Path.Symbol == 0 || len(q.Path.Segments) == 0 {
		return nil, false
	}
	rootPath := constraint.Path{Root: q.Path.Root, Symbol: q.Path.Symbol, Version: q.Path.Version}
	root, ok := f.pathObservationSolvedType(flow.PathObservationQuery{
		Point: q.Point,
		Path:  rootPath,
		View:  q.View,
	})
	if !ok || typ.IsAbsentOrUnknown(root) {
		return nil, false
	}
	derived := typepath.Strict(root, q.Path.Segments)
	if typ.IsAbsentOrUnknown(derived) {
		return nil, false
	}
	return derived, true
}

func pathObservationDerivedPreferred(derived, direct typ.Type) bool {
	if typ.TypeEquals(derived, direct) {
		return false
	}
	if typ.MorePrecise(derived, direct) {
		return true
	}
	derivedSubDirect := subtype.IsSubtype(derived, direct)
	directSubDerived := subtype.IsSubtype(direct, derived)
	return derivedSubDirect && !directSubDerived
}

func (f *canonicalFacts) preserveDeclaredPathNilability(q flow.PathObservationQuery, solved typ.Type) typ.Type {
	if typ.IsAbsentOrUnknown(solved) || f.pathObservationHasProductValue(q) {
		return solved
	}
	declared := f.pathObservationDeclaredType(q)
	if typ.IsAbsentOrUnknown(declared) {
		return solved
	}
	_, declaredNilable := typ.SplitNilableFieldType(declared)
	_, solvedNilable := typ.SplitNilableFieldType(solved)
	if !declaredNilable || solvedNilable {
		return solved
	}
	return typ.NewOptional(solved)
}

func (f *canonicalFacts) pathObservationHasProductValue(q flow.PathObservationQuery) bool {
	if len(q.Path.Segments) == 0 {
		return true
	}
	paths := f.paths
	if q.View == flow.PathReadPost {
		paths = paths.WithPostState()
	}
	av, ok := paths.pathValueAt(q.Point, q.Path)
	return ok && !av.IsZero()
}

func (f *canonicalFacts) soundPathObservationRoot(point cfg.Point, sym cfg.SymbolID, refined typ.Type) typ.Type {
	declared := f.annotatedDeclaredType(point, sym)
	if declared == nil {
		return refined
	}
	if declared.Kind().IsPlaceholder() {
		if refined.Kind().IsPlaceholder() {
			return nil
		}
		return refined
	}
	if !subtype.IsSubtype(refined, declared) {
		return nil
	}
	return refined
}

func (f *canonicalFacts) pathObservationConditionedType(q flow.PathObservationQuery, solved typ.Type, declared typ.Type) typ.Type {
	if q.LocalCondition == nil || q.Path.IsEmpty() {
		return nil
	}
	seedPath := constraint.Path{Root: q.Path.Root, Symbol: q.Path.Symbol, Version: q.Path.Version}
	seedType := solved
	if len(q.Path.Segments) > 0 || typ.IsAbsentOrUnknown(seedType) {
		seedType, _ = f.pathObservationSolvedType(flow.PathObservationQuery{
			Point: q.Point,
			Path:  seedPath,
			View:  q.View,
		})
	}
	if typ.IsAbsentOrUnknown(seedType) && len(q.Path.Segments) == 0 {
		seedType = declared
	}
	if typ.IsAbsentOrUnknown(seedType) {
		seedType = f.pathObservationDeclaredType(flow.PathObservationQuery{
			Point: q.Point,
			Path:  seedPath,
			View:  q.View,
		})
	}
	if typ.IsAbsentOrUnknown(seedType) {
		return f.ConditionedTypeAt(q.Point, q.Path, *q.LocalCondition)
	}
	return f.ConditionedSeedTypeAt(q.Point, seedPath, seedType, q.Path, *q.LocalCondition)
}

func (f *canonicalFacts) pathObservationDeclaredType(q flow.PathObservationQuery) typ.Type {
	path := q.Path
	if path.Symbol == 0 {
		return nil
	}
	base := f.annotatedDeclaredType(q.Point, path.Symbol)
	if base == nil {
		base = f.effectivePathObservationRoot(q.Point, path.Symbol, q.View)
	}
	if base == nil || len(path.Segments) == 0 {
		return base
	}
	return typepath.Strict(base, path.Segments)
}

func (f *canonicalFacts) annotatedDeclaredType(point cfg.Point, sym cfg.SymbolID) typ.Type {
	if sym == 0 || !f.IsAnnotated(sym) {
		return nil
	}
	tv := f.DeclaredAt(point, sym)
	if tv.State != flow.StateResolved || tv.Type == nil || typ.IsUnknown(tv.Type) {
		return nil
	}
	return tv.Type
}

func (f *canonicalFacts) effectivePathObservationRoot(point cfg.Point, sym cfg.SymbolID, view flow.PathReadView) typ.Type {
	var tv flow.TypedValue
	if view == flow.PathReadPost {
		tv = f.PostEffectiveTypeAt(point, sym)
	} else {
		tv = f.EffectiveTypeAt(point, sym)
	}
	if tv.Type != nil && (tv.State == flow.StateResolved || !typ.IsUnknown(tv.Type)) {
		return tv.Type
	}
	return nil
}

func (f *canonicalFacts) hasConditionProofAt(point cfg.Point) bool {
	cond := f.ConditionAt(point)
	return cond.IsFalse() || cond.HasConstraints()
}

func (f *canonicalFacts) withPathObservationIndexRead(q flow.PathObservationQuery, obs flow.PathObservation) flow.PathObservation {
	if !obs.Resolved() || typ.IsNever(obs.Type) || q.IndexRead == nil {
		return obs
	}
	if refined, ok := flow.RefineIndexReadObservation(flow.IndexReadObservationQuery{
		Point:  q.Point,
		Result: obs.Type,
		Index:  *q.IndexRead,
		Proofs: f,
	}); ok {
		obs.Type = refined
	}
	return obs
}

func (f *canonicalFacts) conditionProofProjector() flow.ConditionProofProjector {
	return flow.ConditionProofProjector{
		Resolver:    canonicalConditionResolver{},
		ResolveType: f.resolveConditionTypeKey,
		ConditionAt: f.ConditionAt,
		RootTypeAt:  f.conditionProofRootTypeAt,
	}
}

func (f *canonicalFacts) conditionProofRootTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if path.Symbol == 0 {
		return nil
	}
	tv := f.staticEffectiveTypeAt(p, path.Symbol)
	if tv.State != flow.StateResolved || typ.IsAbsentOrUnknown(tv.Type) {
		return nil
	}
	return tv.Type
}

func (f *canonicalFacts) resolveConditionTypeKey(key narrow.TypeKey) typ.Type {
	switch key.Kind {
	case narrow.TypeKeyBuiltin:
		if builtinKind, ok := key.BuiltinKind(); ok {
			return narrow.TypeForKind(builtinKind)
		}
	case narrow.TypeKeyHash:
		return f.typeByHash(key.Hash)
	}
	return nil
}

func (f *canonicalFacts) typeByHash(hash uint64) typ.Type {
	if hash == 0 {
		return nil
	}
	var out typ.Type
	ambiguous := false
	accept := func(t typ.Type) {
		if ambiguous || t == nil || t.Hash() != hash {
			return
		}
		if out == nil {
			out = t
			return
		}
		if !typ.TypeEquals(out, t) {
			out = nil
			ambiguous = true
		}
	}
	for _, t := range f.declared {
		accept(t)
	}
	for _, t := range f.bindings {
		accept(t)
	}
	return out
}

type canonicalConditionResolver struct{}

func (canonicalConditionResolver) Field(t typ.Type, name string) (typ.Type, bool) {
	return querycore.Field(t, name)
}

func (canonicalConditionResolver) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	return querycore.Index(t, key)
}

// NumericBoundsAt returns the proven integer bounds on sym entering point p from the
// in-state's numeric component, using the theory solver so a bound established
// transitively through a relation (an induction variable bounded by another value)
// is recovered.
func (f *canonicalFacts) NumericBoundsAt(p cfg.Point, sym cfg.SymbolID) (int64, int64, bool) {
	num := f.inState(p).Num
	if num == nil {
		return 0, 0, false
	}
	key, ok := flow.NumericVarKeyOfSymbol(sym)
	if !ok {
		return 0, 0, false
	}
	return numeric.BoundsForWithTheory(num, key)
}

// ArrayLenRefAt returns the container symbol and constant offset of a proven
// `sym <= #arr + offset` relation on sym entering point p, the symbolic length
// reference the transfer seeds for a `for i = 1, #arr` / `while i <= #arr`
// induction variable. A value with no length reference, or a length reference
// keyed on a non-symbol path, reports ok=false.
func (f *canonicalFacts) ArrayLenRefAt(p cfg.Point, sym cfg.SymbolID) (cfg.SymbolID, int64, bool) {
	num := f.inState(p).Num
	if num == nil {
		return 0, 0, false
	}
	key, ok := flow.NumericVarKeyOfSymbol(sym)
	if !ok {
		return 0, 0, false
	}
	arrKey, offset, ok := num.LenRefWithOffsetFor(key)
	if !ok {
		return 0, 0, false
	}
	arrSym, ok := flow.SymbolOfNumericVarKey(arrKey)
	if !ok {
		return 0, 0, false
	}
	return arrSym, offset, true
}

func (f *canonicalFacts) ArrayLenRefPathAt(p cfg.Point, sym cfg.SymbolID) (constraint.Path, int64, bool) {
	arrSym, offset, ok := f.ArrayLenRefAt(p, sym)
	if !ok {
		return constraint.Path{}, 0, false
	}
	return flowpath.WithVersion(constraint.Path{Symbol: arrSym}, f.graph, p), offset, true
}

// HasKeyOf reports whether the product-state KeyPresence axis proves that
// keyPath was drawn from tablePath. This is the diagnostic counterpart of
// transfer's index-read refinement and deliberately does not scan Cond: key
// presence is runtime must-state, not a disjunct-level logical query.
func (f *canonicalFacts) HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool {
	return flow.PointFactsOf(f.inState(p)).HasKeyPresence(tablePath, keyPath)
}

// NarrowedTypeAt implements api.FlowOps over the canonical in-state projection.
func (f *canonicalFacts) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if f == nil || path.IsEmpty() {
		return nil
	}
	if tv := f.RefinedPathAt(p, path); !typ.IsAbsentOrUnknown(tv.Type) {
		return tv.Type
	}
	return nil
}

// NarrowedTypeAtWithCondition is a sound projection under an expression-local
// condition. A false condition is unreachable; non-false local conditions are
// currently represented in observation.Projector's condition-proof path, so this
// FlowOps view returns the unconditioned over-approximation instead of rebuilding
// a parallel condition interpreter.
func (f *canonicalFacts) NarrowedTypeAtWithCondition(p cfg.Point, path constraint.Path, condition constraint.Condition) typ.Type {
	if condition.IsFalse() {
		return typ.Never
	}
	return f.NarrowedTypeAt(p, path)
}

// PreStateTypeAt reads the solver-derived IN-state for p. CanonicalFacts already
// stores IN and OUT separately, so this is a direct projection, not a re-derived
// predecessor join.
func (f *canonicalFacts) PreStateTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	return f.NarrowedTypeAt(p, path)
}

func (f *canonicalFacts) ExcludesTypeAt(p cfg.Point, path constraint.Path, declared typ.Type) bool {
	if f == nil || path.IsEmpty() || declared == nil {
		return false
	}
	cond := f.inState(p).Cond
	if cond.IsFalse() || !cond.HasConstraints() {
		return false
	}
	for i := 0; i < cond.NumDisjuncts(); i++ {
		found := false
		for _, c := range cond.DisjunctConstraints(i) {
			nht, ok := c.(constraint.NotHasType)
			if !ok {
				continue
			}
			if canonicalPathMatches(nht.Path, path) && canonicalTypeMatches(typeFromConstraintKey(nht.Type), declared) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (f *canonicalFacts) LengthBoundsAt(p cfg.Point, path constraint.Path) (int64, int64, bool) {
	if f == nil || path.IsEmpty() {
		return 0, 0, false
	}
	if lower, ok := f.LengthLowerBoundForPathAt(p, path); ok {
		return lower, 0, true
	}
	return 0, 0, false
}

// IsPointDead reports reachability from the solver output. InPoints is derived
// by the equation builder from the solved cell map; an absent IN/OUT state means
// the point is unreachable under the same fixed point diagnostics observe.
func (f *canonicalFacts) IsPointDead(p cfg.Point) bool {
	if f == nil {
		return false
	}
	if _, ok := f.state.InPoints[p]; ok {
		return false
	}
	if _, ok := f.state.Points[p]; ok {
		return false
	}
	return true
}

func (f *canonicalFacts) symbolAt(p cfg.Point, name string) (cfg.SymbolID, bool) {
	if f == nil || f.graph == nil || name == "" {
		return 0, false
	}
	sym, ok := f.graph.SymbolAt(p, name)
	if !ok || sym == 0 {
		return 0, false
	}
	return sym, true
}

func canonicalPathMatches(cpath constraint.Path, qpath constraint.Path) bool {
	if cpath.Symbol != 0 && qpath.Symbol != 0 {
		return cpath.Symbol == qpath.Symbol
	}
	if cpath.Symbol != 0 || qpath.Symbol != 0 {
		return false
	}
	if cpath.IsPlaceholder() {
		return cpath.Root == qpath.Root
	}
	return false
}

func typeFromConstraintKey(key narrow.TypeKey) typ.Type {
	if key.Kind != narrow.TypeKeyBuiltin {
		return nil
	}
	builtinKind, ok := key.BuiltinKind()
	if !ok {
		return nil
	}
	return narrow.TypeForKind(builtinKind)
}

func canonicalTypeMatches(a, b typ.Type) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Hash() == b.Hash() {
		return true
	}
	switch a.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String:
		return a.Kind() == b.Kind()
	default:
		return false
	}
}

// returnSynth is the api.Synth the WithReturn / WithExhaustiveness passes read. It
// is a facade over the two real components of the observation surface: the
// driver's annotation resolver (declared type/return resolution) and the canonical
// observation Projector (expression typing). It introduces no independent type
// logic; every method delegates to one of those two.
type returnSynth struct {
	driver *Driver
	obs    api.ExprSynth
	ctx    *db.QueryContext
}

// compile-time assertion: returnSynth satisfies api.Synth.
var _ api.Synth = (*returnSynth)(nil)

func (s *returnSynth) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	if s.obs == nil {
		return typ.Unknown
	}
	return s.obs(expr, p)
}

func (s *returnSynth) TypeOfWithExpected(expr ast.Expr, p cfg.Point, _ typ.Type) typ.Type {
	return s.TypeOf(expr, p)
}

func (s *returnSynth) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type {
	return []typ.Type{s.TypeOf(expr, p)}
}

func (s *returnSynth) FunctionType(*ast.FunctionExpr, *scope.State) *typ.Function { return nil }

func (s *returnSynth) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	out := make([]typ.Type, 0, needed)
	for i := 0; i < needed; i++ {
		if i < len(exprs) {
			out = append(out, s.TypeOf(exprs[i], p))
		} else {
			out = append(out, typ.Nil)
		}
	}
	return out
}

func (s *returnSynth) InferIterVars(_ []ast.Expr, count int, _ cfg.Point) []typ.Type {
	out := make([]typ.Type, count)
	for i := range out {
		out[i] = typ.Unknown
	}
	return out
}

func (s *returnSynth) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	if s.driver == nil || s.driver.resolver == nil {
		return typ.Unknown
	}
	if sc == nil {
		sc = s.driver.baseScope()
	}
	return s.driver.resolver.ResolveType(expr, sc)
}

func (s *returnSynth) ResolveReturnTypes(types []ast.TypeExpr, sc *scope.State) []typ.Type {
	out := make([]typ.Type, 0, len(types))
	for _, t := range types {
		if t == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, s.ResolveType(t, sc))
	}
	return out
}

func (s *returnSynth) ResolveFunctionSignature(*ast.FunctionExpr, *scope.State) *typ.Function {
	return nil
}

func (s *returnSynth) ResolveTypeDef(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *scope.State) typ.Type {
	if s.driver == nil || s.driver.resolver == nil {
		return typ.Unknown
	}
	if sc == nil {
		sc = s.driver.baseScope()
	}
	return s.driver.resolver.ResolveTypeDef(name, typeExpr, typeParams, sc)
}

// Narrow returns the same facade: the canonical observation Projector is already
// the flow-refined view (it reads the converged flow-refined per-point types).
func (s *returnSynth) Narrow() api.BaseSynth { return s }

func (s *returnSynth) WithFlow(api.FlowOps) api.BaseSynth { return s }

func (s *returnSynth) Method(t typ.Type, name string) (typ.Type, bool) {
	if s != nil && s.driver != nil && s.driver.cfg.Types != nil && s.ctx != nil {
		return s.driver.cfg.Types.Method(s.ctx, t, name)
	}
	return querycore.Method(t, name)
}

func (s *returnSynth) Field(t typ.Type, name string) (typ.Type, bool) {
	if s != nil && s.driver != nil && s.driver.cfg.Types != nil && s.ctx != nil {
		return s.driver.cfg.Types.Field(s.ctx, t, name)
	}
	return querycore.Field(t, name)
}

func (s *returnSynth) SynthWithExpected(expr ast.Expr, p cfg.Point, _ typ.Type) typ.Type {
	return s.TypeOf(expr, p)
}

func (s *returnSynth) CallQuery() querycore.TypeOps { return s.driver.cfg.Types }

func (s *returnSynth) AllowReturnTransforms() bool { return false }

func (s *returnSynth) Context() *db.QueryContext { return s.ctx }

// buildObservationInputs assembles the per-function flow.Inputs the diagnostic
// passes read directly (DeclaredTypes / AnnotatedVars), backed by the resolved
// declared-type context.
func buildObservationInputs(g *cfg.Graph, obsCtx functionObservationContext) *flow.Inputs {
	in := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type, len(obsCtx.declared)),
		BindingTypes:  make(map[cfg.SymbolID]typ.Type, len(obsCtx.bindings)),
	}
	if g != nil {
		in.Graph = g
	}
	for sym, t := range obsCtx.declared {
		in.DeclaredTypes[sym] = t
	}
	for _, sym := range obsCtx.annotated.Symbols() {
		in.AnnotatedVars.Add(sym)
	}
	for sym, t := range obsCtx.bindings {
		in.BindingTypes[sym] = t
	}
	return in
}
