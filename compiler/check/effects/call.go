package effects

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// ResolveCallEffect resolves the effect for a call site using symbol lookup,
// alias resolution, and type synthesis fallbacks.
func ResolveCallEffect(
	info *cfg.CallInfo,
	p cfg.Point,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	effectLookupSym constraint.EffectLookupBySym,
	graph *cfg.Graph,
) *constraint.FunctionEffect {
	if info == nil {
		return nil
	}

	// Primary lookup: by symbol (all functions have symbols).
	if effectLookupSym != nil && info.CalleeSymbol != 0 {
		if eff := effectLookupSym(info.CalleeSymbol); eff != nil {
			return eff
		}
	}

	// Direct alias resolution (local f = B; f()).
	if effectLookupSym != nil && info.CalleeSymbol != 0 && graph != nil {
		if aliasSym := graph.DirectAliasSymbol(info.CalleeSymbol); aliasSym != 0 {
			if eff := effectLookupSym(aliasSym); eff != nil {
				return eff
			}
		}
	}

	// Fallback: extract effect from synthesized type.
	if synthFn != nil && info.Callee != nil {
		if fnType := synthFn(info.Callee, p); fnType != nil {
			if eff := EffectFromType(fnType); eff != nil {
				return eff
			}
		}
	}

	// Fallback: extract effect from resolved type.
	if info.CalleeSymbol != 0 && symResolver != nil {
		if looked, ok := symResolver(p, info.CalleeSymbol); ok {
			if eff := EffectFromType(looked); eff != nil {
				return eff
			}
		}
	}

	return nil
}

// CallTerminates reports whether the call is to a function that never returns.
func CallTerminates(
	info *cfg.CallInfo,
	p cfg.Point,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	effectLookupSym constraint.EffectLookupBySym,
	graph *cfg.Graph,
) bool {
	if eff := ResolveCallEffect(info, p, synthFn, symResolver, effectLookupSym, graph); eff != nil {
		return eff.Terminates
	}
	return false
}
