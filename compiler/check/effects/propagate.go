package effects

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
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

	callerParamIndexes := returnFlowParamIndexes(result.Graph)

	// Start with the function's own Terminates value.
	terminates := fnEffect.Terminates

	for _, call := range result.Evidence.Calls {
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}
		var synthFn func(ast.Expr, cfg.Point) typ.Type
		if synth := result.SolvedSynth(); synth != nil {
			synthFn = synth.TypeOf
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
				row = effect.Union(row, remapCalleeFlowInto(calleeRow, info, result.Graph, callerParamIndexes))
			}
		}
	}

	// Compute Terminates from flow reachability.
	if !terminates && conditionProofFacts(result) != nil && result.Graph.CFG() != nil {
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

// remapCalleeFlowInto rewrites a callee effect row into the caller frame. A
// callee FlowInto names a callee parameter index, which is meaningless in the
// caller: it is retained only when the caller forwards one of its own
// parameters whole into that callee slot, with the index remapped to the caller
// parameter. A callee FlowInto sourced from a literal or local argument is
// resolved at this call and contributes no caller-parameter-to-return flow, so
// it is dropped rather than aliasing an unrelated caller parameter by index.
func remapCalleeFlowInto(calleeRow effect.Row, info *cfg.CallInfo, graph *cfg.Graph, callerParams map[cfg.SymbolID]int) effect.Row {
	if len(calleeRow.Labels) == 0 {
		return calleeRow
	}
	var bindings *bind.BindingTable
	if graph != nil {
		bindings = graph.Bindings()
	}
	out := effect.Row{Tail: calleeRow.Tail}
	out.Labels = make([]effect.Label, 0, len(calleeRow.Labels))
	for _, label := range calleeRow.Labels {
		flow, ok := label.(effect.FlowInto)
		if !ok {
			out.Labels = append(out.Labels, label)
			continue
		}
		callerIdx, ok := callerParamForCalleeArg(info, flow.ParamIndex, graph, bindings, callerParams)
		if !ok {
			continue
		}
		flow.ParamIndex = callerIdx
		out.Labels = append(out.Labels, flow)
	}
	if len(out.Labels) == 0 {
		return effect.Row{Tail: calleeRow.Tail}
	}
	return out
}

// callerParamForCalleeArg maps a callee parameter index to the caller parameter
// the call forwards into it, if the argument is a whole caller-parameter read.
func callerParamForCalleeArg(
	info *cfg.CallInfo,
	calleeParamIdx int,
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	callerParams map[cfg.SymbolID]int,
) (int, bool) {
	if info == nil || bindings == nil || len(callerParams) == 0 || calleeParamIdx < 0 {
		return 0, false
	}
	arg := callsite.RuntimeArgAt(info, calleeParamIdx)
	ident, ok := arg.(*ast.IdentExpr)
	if !ok || ident == nil {
		return 0, false
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return 0, false
	}
	idx, ok := callerParams[sym]
	return idx, ok
}

// ResolveRefinementBySym resolves effects from FunctionFacts projection or global
// type information.
func ResolveRefinementBySym(
	facts api.RefinementFacts,
	bindings *bind.BindingTable,
	globalTypes globalenv.TypeOverlay,
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
	if bindings != nil && len(globalTypes) > 0 {
		if name := bindings.Name(sym); name != "" {
			if t, ok := globalTypes.Type(name); ok && t != nil {
				return EffectFromType(t)
			}
		}
	}
	return nil
}

// TerminatesFromReachability determines if a function never returns normally
// by checking reachability of all return and exit points via flow analysis.
func TerminatesFromReachability(result *api.FuncResult) bool {
	proofs := conditionProofFacts(result)
	if result == nil || proofs == nil || !result.Evidence.NormalExit.Valid {
		return false
	}

	// Check if any return node is reachable.
	for _, ret := range result.Evidence.Returns {
		cond := proofs.ConditionAt(ret.Point)
		if !cond.IsFalse() {
			return false
		}
	}

	// Check if exit node is reachable.
	exitCond := proofs.ConditionAt(result.Evidence.NormalExit.Point)
	return exitCond.IsFalse()
}

func conditionProofFacts(result *api.FuncResult) flow.ConditionProofFacts {
	if result == nil {
		return nil
	}
	return result.ConditionProofFacts()
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
