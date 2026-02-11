package effects

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// LookupFunc resolves a function effect by symbol.
type LookupFunc func(sym cfg.SymbolID) *constraint.FunctionEffect

// Propagate computes the complete effect for a function by combining its
// local effects with effects propagated from callees.
func Propagate(result *api.FuncResult, lookup LookupFunc) *constraint.FunctionEffect {
	if result == nil {
		return nil
	}

	fnEffect := result.FnEffect
	if fnEffect == nil {
		fnEffect = &constraint.FunctionEffect{}
	}

	if result.Graph == nil {
		return fnEffect
	}

	// Start with the function's own effect row.
	var row effect.Row
	if fnEffect.Row != nil {
		if r, ok := fnEffect.Row.(effect.Row); ok {
			row = r
		}
	}

	// Start with the function's own Terminates value.
	terminates := fnEffect.Terminates

	result.Graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}

		var calleeEffect *constraint.FunctionEffect

		// Symbol-based lookup (all functions have symbols).
		if info.CalleeSymbol != 0 && lookup != nil {
			calleeEffect = lookup(info.CalleeSymbol)
		}

		// Direct alias resolution (local f = B; f()).
		if calleeEffect == nil && info.CalleeSymbol != 0 {
			if aliasSym := result.Graph.DirectAliasSymbol(info.CalleeSymbol); aliasSym != 0 && lookup != nil {
				calleeEffect = lookup(aliasSym)
			}
		}

		// Fallback to extracting effect from synthesized type.
		if calleeEffect == nil && result.NarrowSynth != nil && info.Callee != nil {
			if t := result.NarrowSynth.TypeOf(info.Callee, p); t != nil {
				calleeEffect = EffectFromType(t)
			}
		}

		if calleeEffect == nil {
			return
		}
		if calleeEffect.Row != nil {
			if calleeRow, ok := calleeEffect.Row.(effect.Row); ok {
				row = effect.Union(row, calleeRow)
			}
		}
	})

	// Compute Terminates from flow reachability.
	if !terminates && result.FlowSolution != nil && result.Graph.CFG() != nil {
		terminates = TerminatesFromReachability(result)
	}

	var effectRow typ.EffectInfo
	if !row.Pure() || row.IsOpen() {
		effectRow = row
	}

	return &constraint.FunctionEffect{
		Row:        effectRow,
		OnReturn:   fnEffect.OnReturn,
		OnTrue:     fnEffect.OnTrue,
		OnFalse:    fnEffect.OnFalse,
		Terminates: terminates,
	}
}

// LookupEffectBySym resolves effects from the store or global type information.
func LookupEffectBySym(
	store api.EffectStore,
	bindings *bind.BindingTable,
	globalTypes map[string]typ.Type,
	sym cfg.SymbolID,
) *constraint.FunctionEffect {
	if store == nil || sym == 0 {
		return nil
	}
	if eff := store.LookupEffectBySym(sym); eff != nil {
		return eff
	}
	if bindings != nil && globalTypes != nil {
		if name := bindings.Name(sym); name != "" {
			if t, ok := globalTypes[name]; ok && t != nil {
				return EffectFromType(t)
			}
		}
	}
	return nil
}

// TerminatesFromReachability determines if a function never returns normally
// by checking reachability of all return and exit points via flow analysis.
func TerminatesFromReachability(result *api.FuncResult) bool {
	if result == nil || result.FlowSolution == nil || result.Graph == nil || result.Graph.CFG() == nil {
		return false
	}

	g := result.Graph.CFG()

	// Check if any return node is reachable.
	for _, p := range g.RPO() {
		node := g.Node(p)
		if node == nil || node.Kind != cfg.NodeReturn {
			continue
		}
		cond := result.FlowSolution.ConditionAt(p)
		if !cond.IsFalse() {
			return false
		}
	}

	// Check if exit node is reachable.
	exitCond := result.FlowSolution.ConditionAt(g.Exit())
	return exitCond.IsFalse()
}

// EffectFromType extracts FunctionEffect from a function type's declared effect annotations.
func EffectFromType(t typ.Type) *constraint.FunctionEffect {
	if t == nil {
		return nil
	}
	fn, ok := unwrap.Alias(t).(*typ.Function)
	if !ok || fn == nil {
		return nil
	}
	if fn.Refinement != nil {
		if eff, ok := fn.Refinement.(*constraint.FunctionEffect); ok {
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
	return &constraint.FunctionEffect{
		Row:        row,
		Terminates: terminates,
	}
}
