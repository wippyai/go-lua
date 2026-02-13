package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// ResolveCalleeEffect resolves the best effect for a callsite using one canonical order:
//  1. symbol candidate lookup (store/snapshot facts)
//  2. synthesized callee type
//  3. symbol-resolved callee type
func ResolveCalleeEffect(
	info *cfg.CallInfo,
	p cfg.Point,
	graph *cfg.Graph,
	primary, fallback *bind.BindingTable,
	lookup func(sym cfg.SymbolID) *constraint.FunctionEffect,
	synth func(expr ast.Expr, p cfg.Point) typ.Type,
	resolveBySym func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool),
	effectFromType func(t typ.Type) *constraint.FunctionEffect,
) *constraint.FunctionEffect {
	if info == nil {
		return nil
	}
	candidates := CalleeSymbolCandidatesWithAliases(info, graph, primary, fallback)
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
	if synth != nil && info.Callee != nil {
		if t := synth(info.Callee, p); t != nil {
			if eff := effectFromType(t); eff != nil {
				return eff
			}
		}
	}
	// Method calls (x:foo(...)) often do not have a direct callee expression.
	// Resolve effect from receiver method/field type when available.
	if synth != nil && info.Method != "" && info.Receiver != nil {
		if recv := synth(info.Receiver, p); recv != nil {
			if mt, ok := core.Method(recv, info.Method); ok {
				if eff := effectFromType(mt); eff != nil {
					return eff
				}
			}
			if ft, ok := core.Field(recv, info.Method); ok {
				if eff := effectFromType(ft); eff != nil {
					return eff
				}
			}
		}
	}
	if resolveBySym != nil {
		for _, sym := range candidates {
			if t, ok := resolveBySym(p, sym); ok && t != nil {
				if eff := effectFromType(t); eff != nil {
					return eff
				}
			}
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
	primary, fallback *bind.BindingTable,
	synth func(expr ast.Expr, p cfg.Point) typ.Type,
	resolveBySym func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool),
) typ.Type {
	if info == nil {
		return nil
	}
	if synth != nil && info.Callee != nil {
		if t := synth(info.Callee, p); t != nil {
			return t
		}
	}
	if synth != nil && info.Method != "" && info.Receiver != nil {
		if recv := synth(info.Receiver, p); recv != nil {
			if mt, ok := core.Method(recv, info.Method); ok {
				return mt
			}
			if ft, ok := core.Field(recv, info.Method); ok {
				return ft
			}
		}
	}
	if resolveBySym == nil {
		return nil
	}
	for _, sym := range CalleeSymbolCandidatesWithAliases(info, graph, primary, fallback) {
		if t, ok := resolveBySym(p, sym); ok && t != nil {
			return t
		}
	}
	return nil
}
