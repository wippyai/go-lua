package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func resolveSynthedCalleeType(
	info *cfg.CallInfo,
	p cfg.Point,
	synth func(expr ast.Expr, p cfg.Point) typ.Type,
) typ.Type {
	if info == nil || synth == nil {
		return nil
	}
	if info.Callee != nil {
		if t := synth(info.Callee, p); t != nil {
			return t
		}
	}
	// Method calls (x:foo(...)) often do not have a direct callee expression.
	// Resolve receiver method/field type when available.
	if info.Method != "" && info.Receiver != nil {
		if recv := synth(info.Receiver, p); recv != nil {
			if mt, ok := core.Method(recv, info.Method); ok {
				return mt
			}
			if ft, ok := core.Field(recv, info.Method); ok {
				return ft
			}
		}
	}
	return nil
}

func resolveCalleeTypeBySymbolCandidates(
	candidates []cfg.SymbolID,
	p cfg.Point,
	resolveBySym func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool),
) typ.Type {
	if resolveBySym == nil {
		return nil
	}
	for _, sym := range candidates {
		if t, ok := resolveBySym(p, sym); ok && t != nil {
			return t
		}
	}
	return nil
}

// EffectFromType extracts FunctionRefinement from a function type's declared effect annotations.
func EffectFromType(t typ.Type) *constraint.FunctionRefinement {
	if t == nil {
		return nil
	}
	fn, ok := unwrap.Alias(t).(*typ.Function)
	if !ok || fn == nil {
		return nil
	}
	if fn.Refinement != nil {
		if eff, ok := fn.Refinement.(*constraint.FunctionRefinement); ok {
			return eff
		}
	}

	var row effect.Row
	if fn.Effects != nil {
		if r, ok := fn.Effects.(effect.Row); ok {
			row = r
		}
	}
	terminates := row.HasDiverge()
	if !terminates {
		for _, r := range fn.Returns {
			if r == nil {
				continue
			}
			if typ.IsNever(unwrap.Alias(r)) {
				terminates = true
				break
			}
		}
	}
	if row.Pure() && !row.IsOpen() && !terminates {
		return nil
	}
	return &constraint.FunctionRefinement{
		Row:        row,
		Terminates: terminates,
	}
}

// ResolveCalleeEffect resolves the best effect for a callsite using one canonical order:
//  1. symbol candidate lookup (store/snapshot facts)
//  2. synthesized callee type
//  3. symbol-resolved callee type
func ResolveCalleeEffect(
	info *cfg.CallInfo,
	p cfg.Point,
	graph *cfg.Graph,
	primary, secondary *bind.BindingTable,
	lookup func(sym cfg.SymbolID) *constraint.FunctionRefinement,
	synth func(expr ast.Expr, p cfg.Point) typ.Type,
	resolveBySym func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool),
	effectFromType func(t typ.Type) *constraint.FunctionRefinement,
) *constraint.FunctionRefinement {
	if info == nil {
		return nil
	}
	candidates := ResolverCalleeSymbolCandidates(info, graph, primary, secondary)
	if lookup != nil {
		for _, sym := range candidates {
			if eff := lookup(sym); eff != nil {
				return eff
			}
		}
	}
	if effectFromType == nil {
		return nil
	}
	if t := resolveSynthedCalleeType(info, p, synth); t != nil {
		if eff := effectFromType(t); eff != nil {
			return eff
		}
	}
	if t := resolveCalleeTypeBySymbolCandidates(candidates, p, resolveBySym); t != nil {
		if eff := effectFromType(t); eff != nil {
			return eff
		}
	}
	return nil
}

// ResolveCalleeType resolves a callee type using one canonical order:
//  1. synthesized callee expression type
//  2. symbol-resolved type from canonical candidates
func ResolveCalleeType(
	info *cfg.CallInfo,
	p cfg.Point,
	graph *cfg.Graph,
	primary, secondary *bind.BindingTable,
	synth func(expr ast.Expr, p cfg.Point) typ.Type,
	resolveBySym func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool),
) typ.Type {
	if info == nil {
		return nil
	}
	if t := resolveSynthedCalleeType(info, p, synth); t != nil {
		return t
	}
	return resolveCalleeTypeBySymbolCandidates(
		ResolverCalleeSymbolCandidates(info, graph, primary, secondary),
		p,
		resolveBySym,
	)
}

// PreferSpecCarryingCallee recovers a callee signature that carries late-attached
// contract spec evidence when the synth surface resolved the same callable to its
// source signature, which lacks it. ResolveCalleeType resolves a non-method local
// callee through the synth surface (the assignment overlay's declared source
// type), which can short-circuit before the spec-carrying symbol resolution; the
// symbol resolver holds the converged FunctionFact projection for the same symbol.
// When the synth resolution does not satisfy carries but a candidate of the same
// callable shape does, swap in that candidate. Method calls, a nil resolver, or a
// nil predicate return fnType unchanged.
//
// The carries predicate selects the spec feature the caller needs (correlation
// labels, return-length postconditions); the shape guard ensures the candidate is
// the same callable the synth surface resolved rather than an unrelated symbol.
func PreferSpecCarryingCallee(
	fnType typ.Type,
	info *cfg.CallInfo,
	p cfg.Point,
	graph *cfg.Graph,
	primary, secondary *bind.BindingTable,
	resolveBySym func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool),
	carries func(typ.Type) bool,
) typ.Type {
	if resolveBySym == nil || carries == nil || IsMethodCallInfo(info) {
		return fnType
	}
	if carries(fnType) {
		return fnType
	}
	for _, sym := range ResolverCalleeSymbolCandidates(info, graph, primary, secondary) {
		if sym == 0 {
			continue
		}
		candidate, ok := resolveBySym(p, sym)
		if !ok || candidate == nil {
			continue
		}
		if SameCallableShape(fnType, candidate) && carries(candidate) {
			return candidate
		}
	}
	return fnType
}

// SameCallableShape reports whether two resolved callee types describe the same
// callable arity, so a spec-carrying candidate is the same function the synth
// surface resolved rather than an unrelated symbol.
func SameCallableShape(a, b typ.Type) bool {
	fa := unwrap.Function(a)
	fb := unwrap.Function(b)
	if fa == nil || fb == nil {
		return false
	}
	return len(fa.Params) == len(fb.Params) && len(fa.Returns) == len(fb.Returns)
}
