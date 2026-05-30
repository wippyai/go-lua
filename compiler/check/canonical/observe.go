package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/scope"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// observation.go is the diagnostic bridge's observation surface: the canonical
// equivalent of the legacy solved-phase facts the diagnostic passes query. The
// passes ask the observation.Projector for "the type of expression E at point P";
// the Projector answers per-symbol/per-point reads through a flow.TypeFacts and
// resolves declared annotations through a Synth. The legacy flow populates those
// from its Solve/Narrow phases; the canonical flow has no such phases, so this
// file projects the SAME questions onto the converged FunctionState.Points.
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
// The flow.Solution stays nil: the canonical flow produces no path-sensitive
// narrowing solution, and the Projector treats a nil Solution as "no narrowing
// proof available", falling back to the per-point facts and declared types this
// surface provides. That fallback is the canonical-computed type, not a masked
// miss.

// functionFacts is the per-function declared-type context the bridge derives once
// from the graph: the resolved declared type of each annotated symbol (parameters
// and annotated local declarations) and the annotated-symbol set.
type functionFacts struct {
	declared  map[cfg.SymbolID]typ.Type
	annotated map[cfg.SymbolID]bool
	// paramSyms are the function's parameter symbols in declaration order. An
	// unannotated parameter (not in annotated, with no declared type) is a gradual
	// `any` when the body imposes no obligation on it: a Lua parameter with no
	// annotation is dynamic, usable in every operation, exactly as the legacy
	// localInferenceSolver.defaultUnconstrainedParams defaults it.
	paramSyms []cfg.SymbolID
}

// buildFunctionFacts resolves the declared types of a function's annotated
// parameters and annotated local declarations, the declared-type context every
// part of the observation surface reads. Annotations resolve against the module
// base scope through the driver's resolver.
func (d *Driver) buildFunctionFacts(g *cfg.Graph, evidence api.FlowEvidence) functionFacts {
	facts := functionFacts{
		declared:  make(map[cfg.SymbolID]typ.Type),
		annotated: make(map[cfg.SymbolID]bool),
	}
	if g == nil {
		return facts
	}

	// Predeclared globals: a use of a predeclared name (print, pairs, require, ...)
	// resolves to its global symbol; the declared-type map carries its value type so
	// the ident pass sees it as defined and the observation surface types it as its
	// function/value type rather than the value-domain unknown. This mirrors the
	// legacy buildDeclaredTypes global pass; globals are declared, not annotated.
	if len(d.cfg.GlobalTypes) > 0 {
		bindings := g.Bindings()
		for _, name := range cfg.SortedFieldNames(d.cfg.GlobalTypes) {
			t := d.cfg.GlobalTypes[name]
			if t == nil {
				continue
			}
			sym, ok := g.GlobalSymbol(name)
			if !ok {
				continue
			}
			if _, exists := facts.declared[sym]; exists {
				continue
			}
			if bindings != nil {
				if k, ok := bindings.Kind(sym); ok && k != cfg.SymbolGlobal {
					continue
				}
			}
			facts.declared[sym] = t
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
	facts.paramSyms = params
	for _, slot := range g.ParamSlotsReadOnly() {
		if slot.Symbol == 0 || slot.TypeAnnotation == nil {
			continue
		}
		t := d.resolveType(slot.TypeAnnotation, annScope)
		if t == nil {
			continue
		}
		facts.declared[slot.Symbol] = t
		facts.annotated[slot.Symbol] = true
	}

	// Annotated local declarations: local x: T = ... pins x's declared type from
	// its aligned annotation, resolved in the base scope.
	for _, assign := range evidence.Assignments {
		info := assign.Info
		if info == nil || !info.IsLocal {
			continue
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
			if _, isParam := facts.declared[target.Symbol]; isParam && facts.annotated[target.Symbol] {
				continue
			}
			// Resolve a local declaration in the same scope as the parameters (the
			// type-param scope when the function is generic): a local typed by a type
			// parameter (`local result: {U}` inside `map<T, U>`) then carries the same
			// bounded type parameter the parameter and call-result types carry, so an
			// element write `result[i] = f(v)` compares `U` against `U` consistently.
			t := d.resolveType(ann, annScope)
			if t == nil {
				continue
			}
			facts.declared[target.Symbol] = t
			facts.annotated[target.Symbol] = true
			zzDumpType("declared-local", t)
		}
	}
	return facts
}

// seedMethodSelf types a method/field-definition body's implicit `self` parameter
// as the receiver's record. A method defined as `function T:m()` (or a field
// definition `function T.m()` whose body declares a leading `self`) binds an
// implicit `self` first parameter; the legacy FunctionLiteralSignatures pass types
// it from the receiver (receiverSelfType -> the named type or the receiver value's
// converged type). Here the receiver type is the receiver symbol's module-wide
// converged value (moduleCaptures), or, for a type-name receiver, the named type
// from the base scope. With the self type known, self.f reads the receiver field's
// type rather than the gradual `any` an unannotated parameter otherwise carries.
//
// A self parameter the user annotated explicitly is left untouched (the annotation
// wins). A receiver whose type cannot be resolved leaves self at the gradual
// default, the sound carry-forward.
func (d *Driver) seedMethodSelf(facts *functionFacts, prog *program, g *cfg.Graph) {
	if facts == nil || prog == nil || g == nil {
		return
	}
	fn := g.Func()
	if fn == nil {
		return
	}
	info, ok := prog.methodDefs[fn]
	if !ok || info == nil || info.Receiver == nil {
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
	recv := d.receiverType(info)
	if recv == nil || typ.IsAbsentOrUnknown(recv) {
		return
	}
	facts.declared[selfSym] = recv
	facts.annotated[selfSym] = true
}

// receiverType resolves the record type of a method/field definition's receiver
// (the T in function T:m() / function T.m()). It mirrors the legacy
// receiverSelfType: a receiver naming a module type resolves to that named type;
// otherwise it is the receiver symbol's module-wide converged value (the table the
// methods are defined on). Returns nil when neither resolves.
func (d *Driver) receiverType(info *cfg.FuncDefInfo) typ.Type {
	if info == nil {
		return nil
	}
	// A receiver naming a module-local type (function Point:is() where `type Point`)
	// resolves to that named type.
	if ident, ok := info.Receiver.(*ast.IdentExpr); ok && ident != nil {
		if sc := d.baseScope(); sc != nil {
			if named, ok := sc.LookupType(ident.Value); ok && named != nil && !typ.IsAbsentOrUnknown(named) {
				return named
			}
		}
	}
	// Otherwise the receiver is a value (a local table the methods are defined on);
	// its type is the converged module-wide value of the receiver symbol.
	if info.ReceiverSymbol != 0 && d.moduleCaptures != nil {
		if t, ok := d.moduleCaptures[info.ReceiverSymbol]; ok && t != nil && !typ.IsAbsentOrUnknown(t) {
			zzDumpType("receiver-value", t)
			return t
		}
	}
	return nil
}

// mergeFuncSignaturesIntoDeclared adds each function-binding symbol's signature to
// the declared-type context, so the ident pass treats a named function reference as
// a defined identifier. A symbol already declared (a parameter or annotated local)
// is not overwritten, and none are marked annotated: a function binding is defined,
// not user-annotated. A signature whose symbol is not bound in g is skipped (it is
// not a reference this function resolves).
func mergeFuncSignaturesIntoDeclared(facts functionFacts, funcSigs map[cfg.SymbolID]typ.Type, g *cfg.Graph) {
	if len(funcSigs) == 0 || g == nil {
		return
	}
	for sym, sig := range funcSigs {
		if sym == 0 || sig == nil {
			continue
		}
		if _, exists := facts.declared[sym]; exists {
			continue
		}
		facts.declared[sym] = sig
	}
}

// canonicalFacts is the flow.TypeFacts the diagnostic passes' observation
// Projector reads, backed by the converged FunctionState. It answers a per-point
// per-symbol type query from the converged env and a declared-type query from the
// function's resolved annotations.
type canonicalFacts struct {
	state    state.FunctionState
	declared map[cfg.SymbolID]typ.Type
	annotate map[cfg.SymbolID]bool

	// funcSignatures maps a function-binding symbol visible in this function (a
	// nested function definition or local-function binding) to its canonical
	// signature. A callee identifier resolves to it so a call's return type is the
	// callee's converged summary returns rather than unknown.
	funcSignatures map[cfg.SymbolID]typ.Type

	// moduleCaptures is the module-wide type of every capturable symbol, read for a
	// free variable captured from an enclosing scope (a symbol with no value in this
	// function's converged env and no local declaration).
	moduleCaptures map[cfg.SymbolID]typ.Type

	// unannotatedParams is the set of parameter symbols with no declared annotation.
	// A read of one whose converged value is the value-domain unknown resolves to
	// gradual `any` (a Lua parameter without an annotation is dynamic, usable in
	// every operation), mirroring the legacy defaultUnconstrainedParams fallback. A
	// parameter the body constrains carries its inferred value and is not defaulted.
	unannotatedParams map[cfg.SymbolID]bool
}

// newCanonicalFacts builds the per-function diagnostic facts over the solved
// FunctionState. The per-point in-state it reads is the solver-derived
// state.FunctionState.InPoints, so the graph and narrower the solve ran over are
// not re-consulted here; they are accepted to keep the driver's construction
// seam stable but are not read (the in-state is read, never re-derived).
func (d *Driver) newCanonicalFacts(_ *cfg.Graph, fs state.FunctionState, facts functionFacts, funcSignatures map[cfg.SymbolID]typ.Type, _ equation.EdgeNarrower) *canonicalFacts {
	var unannotated map[cfg.SymbolID]bool
	for _, sym := range facts.paramSyms {
		if sym == 0 || facts.annotated[sym] {
			continue
		}
		if t, ok := facts.declared[sym]; ok && t != nil && !typ.IsAbsentOrUnknown(t) {
			continue
		}
		if unannotated == nil {
			unannotated = make(map[cfg.SymbolID]bool, len(facts.paramSyms))
		}
		unannotated[sym] = true
	}
	return &canonicalFacts{
		state:             fs,
		declared:          facts.declared,
		annotate:          facts.annotated,
		funcSignatures:    funcSignatures,
		moduleCaptures:    d.moduleCaptures,
		unannotatedParams: unannotated,
	}
}

// compile-time assertion: canonicalFacts implements the observation surface.
var _ flow.TypeFacts = (*canonicalFacts)(nil)

// compile-time assertion: canonicalFacts also exposes the length proof the
// observation surface consults to refine an in-bounds index read.
var _ flow.LengthFacts = (*canonicalFacts)(nil)

// DeclaredAt returns the declared (annotated) type of sym. Declared types are
// flow-insensitive, so the point is unused.
func (f *canonicalFacts) DeclaredAt(_ cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if t, ok := f.declared[sym]; ok && t != nil {
		return flow.TypedValue{Type: t, State: flow.StateResolved}
	}
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

// RefinedAt returns the flow-narrowed type of sym at point p: the converged value
// of sym in the in-state of p (the join of p's reachable predecessors' out-states,
// or p's own seeded state at the entry). A symbol with no converged value has no
// refinement.
func (f *canonicalFacts) RefinedAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if sym == 0 {
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	in := f.inState(p)
	av, ok := in.Env[symKey(sym)]
	if !ok || av.IsZero() {
		// A function-binding symbol carries no env value (it is not a flow variable
		// the transfer writes); its type is its converged callee signature.
		if sig, ok := f.funcSignatures[sym]; ok && sig != nil {
			return flow.TypedValue{Type: sig, State: flow.StateResolved}
		}
		// A free variable captured from an enclosing scope has no value in this
		// function's env; its type is the captured variable's module-wide type. A
		// symbol this function declares locally (an annotated local) is NOT a
		// capture: its declared type is authoritative, so the module-wide capture
		// (which the exit-state scan also records for every function's locals) must
		// not shadow it. Leaving such a symbol unrefined here routes EffectiveTypeAt
		// to its declared type.
		if _, declaredHere := f.declared[sym]; !declaredHere {
			if t, ok := f.moduleCaptures[sym]; ok && t != nil && !typ.IsUnknown(t) {
				return flow.TypedValue{Type: t, State: flow.StateResolved}
			}
		}
		// An unannotated parameter the body imposes no obligation on is gradual
		// `any` (dynamic, usable in every operation), not opaque unknown.
		if f.unannotatedParams[sym] {
			return flow.TypedValue{Type: typ.Any, State: flow.StateResolved}
		}
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	t := projectValue(av)
	if t == nil || typ.IsUnknown(t) {
		// A converged-but-unknown value for an unannotated parameter is the gradual
		// default: the body did not pin it to a concrete type, so it stays `any`.
		if f.unannotatedParams[sym] {
			return flow.TypedValue{Type: typ.Any, State: flow.StateResolved}
		}
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	zzDumpType("refined-value", t)
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
	return f.DeclaredAt(p, sym)
}

// IsAnnotated reports whether sym carries an explicit type annotation.
func (f *canonicalFacts) IsAnnotated(sym cfg.SymbolID) bool {
	return f.annotate[sym]
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
	if ps, ok := f.state.InPoints[p]; ok {
		return ps
	}
	return flow.PointStateDomain.Bottom()
}

// LengthLowerBoundAt returns the proven lower bound on the length of the container
// symbol sym entering point p, from the in-state's numeric component (the length
// floor the transfer seeded from array literals and table.insert appends). The
// numeric state keys a container by the same symbol Env key the transfer writes, so
// the observation surface reads the same length floor `refineIndexRead` consults.
func (f *canonicalFacts) LengthLowerBoundAt(p cfg.Point, sym cfg.SymbolID) (int64, bool) {
	if sym == 0 {
		return 0, false
	}
	num := f.inState(p).Num
	if num == nil {
		return 0, false
	}
	lower, _, ok := num.LenBoundsFor(constraint.PathKey(symKey(sym)))
	return lower, ok
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
// the narrowed-phase view (it reads the converged flow-refined per-point types).
func (s *returnSynth) Narrow() api.BaseSynth { return s }

func (s *returnSynth) WithFlow(api.FlowOps) api.BaseSynth { return s }

func (s *returnSynth) Method(t typ.Type, name string) (typ.Type, bool) {
	return querycore.Method(t, name)
}

func (s *returnSynth) Field(t typ.Type, name string) (typ.Type, bool) {
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
func buildObservationInputs(g *cfg.Graph, facts functionFacts) *flow.Inputs {
	in := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type, len(facts.declared)),
		AnnotatedVars: make(map[cfg.SymbolID]bool, len(facts.annotated)),
	}
	if g != nil {
		in.Graph = g
	}
	for sym, t := range facts.declared {
		in.DeclaredTypes[sym] = t
	}
	for sym := range facts.annotated {
		in.AnnotatedVars[sym] = true
	}
	return in
}
