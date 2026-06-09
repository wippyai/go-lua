package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/compiler/check/scope"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/core"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// functionObservationContext is the per-function immutable source context the
// canonical observer derives once from the graph: resolved annotations,
// annotated-symbol membership, binding signatures, and parameter symbol layout.
// It is not the public function-fact projection; that is final Summary-derived output.
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
	// this projection consumes that carrier rather than the raw external map.
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
// where T is a type binding) has a declared receiver contract, so observation may
// mark self annotated to T. A value receiver (`local methods = {}; function
// methods:m()`) has no source annotation: its runtime self is produced by the
// PrototypeSelf product axis, and marking it declared from moduleCaptures would
// leak the old driver scan back into observation.
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
// to this callback body's graph symbols, so observation only admits those
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
