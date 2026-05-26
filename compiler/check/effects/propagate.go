package effects

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// LookupFunc resolves a function refinement by symbol.
type LookupFunc func(sym cfg.SymbolID) *constraint.FunctionRefinement

// Propagate computes the complete effect for a function by combining its
// local effects with effects propagated from callees.
func Propagate(result *api.FuncResult, lookup LookupFunc) *constraint.FunctionRefinement {
	if result == nil {
		return nil
	}

	fnEffect := result.FnRefinement
	if fnEffect == nil {
		fnEffect = &constraint.FunctionRefinement{}
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
	row = effect.Union(row, inferLocalReturnFlowRow(result))

	// Start with the function's own Terminates value.
	terminates := fnEffect.Terminates

	for _, call := range result.Evidence.Calls {
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}
		var synthFn func(ast.Expr, cfg.Point) typ.Type
		if result.NarrowSynth != nil {
			synthFn = result.NarrowSynth.TypeOf
		}
		calleeEffect := callsite.ResolveCalleeEffect(
			info,
			p,
			result.Graph,
			result.Graph.Bindings(),
			result.ModuleBindings,
			lookup,
			synthFn,
			nil,
			EffectFromType,
		)

		if calleeEffect == nil {
			continue
		}
		if calleeEffect.Row != nil {
			if calleeRow, ok := calleeEffect.Row.(effect.Row); ok {
				row = effect.Union(row, calleeRow)
			}
		}
	}

	// Compute Terminates from flow reachability.
	if !terminates && result.FlowSolution != nil && result.Graph.CFG() != nil {
		terminates = TerminatesFromReachability(result)
	}

	var effectRow typ.EffectInfo
	if !row.Pure() || row.IsOpen() {
		effectRow = row
	}

	return &constraint.FunctionRefinement{
		Row:        effectRow,
		OnReturn:   fnEffect.OnReturn,
		OnTrue:     fnEffect.OnTrue,
		OnFalse:    fnEffect.OnFalse,
		Terminates: terminates,
	}
}

// ResolveRefinementBySym resolves effects from canonical function facts or global
// type information.
func ResolveRefinementBySym(
	facts api.RefinementFacts,
	bindings *bind.BindingTable,
	globalTypes map[string]typ.Type,
	sym cfg.SymbolID,
) *constraint.FunctionRefinement {
	if sym == 0 {
		return nil
	}
	if facts != nil {
		if eff := facts.LookupBySym(sym); eff != nil {
			return eff
		}
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
	if result == nil || result.FlowSolution == nil || !result.Evidence.NormalExit.Valid {
		return false
	}

	// Check if any return node is reachable.
	for _, ret := range result.Evidence.Returns {
		cond := result.FlowSolution.ConditionAt(ret.Point)
		if !cond.IsFalse() {
			return false
		}
	}

	// Check if exit node is reachable.
	exitCond := result.FlowSolution.ConditionAt(result.Evidence.NormalExit.Point)
	return exitCond.IsFalse()
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
