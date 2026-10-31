package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/scope"
	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// inferProtectedCallbackReturn recognizes a wrapper whose every normal return
// forwards the successful result of a protected callback invocation. The
// wrapper may do setup and cleanup between the protected call and the guard.
func inferProtectedCallbackReturn(graph *cfg.Graph, parentScope *scope.State) (int, bool) {
	if graph == nil || len(graph.NestedFunctions()) != 0 || graph.Bindings() == nil {
		return 0, false
	}
	// The CFG treats a call named error as terminating. A lexical replacement
	// of either builtin invalidates that assumption and its return effect.
	for sc := parentScope; sc != nil; sc = sc.Parent() {
		if sc.IsLocal("pcall") || sc.IsLocal("cpcall") || sc.IsLocal("xpcall") || sc.IsLocal("error") {
			return 0, false
		}
	}
	params := make(map[cfg.SymbolID]int)
	paramDecls := make(map[cfg.SymbolID]cfg.Point)
	for _, slot := range graph.ParamSlotsReadOnly() {
		if idx, ok := slot.SourceParamIndex(); ok && slot.Symbol != 0 {
			params[slot.Symbol] = idx
			paramDecls[slot.Symbol] = slot.DeclPoint
		}
	}
	if len(params) == 0 {
		return 0, false
	}

	type protectedCall struct {
		point           cfg.Point
		callbackParam   int
		callbackSym     cfg.SymbolID
		guardSym        cfg.SymbolID
		resultSym       cfg.SymbolID
		protectedGlobal cfg.SymbolID
	}
	var candidates []protectedCall
	assignments := make(map[cfg.SymbolID]int)
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				if p == paramDecls[target.Symbol] && paramDecls[target.Symbol] != 0 {
					continue
				}
				assignments[target.Symbol]++
			}
		}
		if len(info.Targets) < 2 || len(info.Sources) != 1 {
			return
		}
		call, resultIndex := info.CallForTarget(1)
		if call == nil || resultIndex != 1 || !isProtectedCallGlobal(graph, p, call) || len(call.Args) == 0 {
			return
		}
		callback, ok := call.Args[0].(*ast.IdentExpr)
		if !ok {
			return
		}
		callbackSym, ok := graph.Bindings().SymbolOf(callback)
		callbackParam, isParam := params[callbackSym]
		if !ok || !isParam || info.Targets[0].Kind != cfg.TargetIdent || info.Targets[1].Kind != cfg.TargetIdent {
			return
		}
		protectedGlobal := call.CalleeSymbol
		if protectedGlobal == 0 {
			protectedGlobal, _ = graph.SymbolAt(p, call.CalleeName)
		}
		candidates = append(candidates, protectedCall{
			point: p, callbackParam: callbackParam, callbackSym: callbackSym,
			guardSym: info.Targets[0].Symbol, resultSym: info.Targets[1].Symbol,
			protectedGlobal: protectedGlobal,
		})
	})
	if len(candidates) != 1 {
		return 0, false
	}
	c := candidates[0]
	if c.guardSym == 0 || c.resultSym == 0 || assignments[c.callbackSym] != 0 ||
		assignments[c.guardSym] != 1 || assignments[c.resultSym] != 1 ||
		assignments[c.protectedGlobal] != 0 {
		return 0, false
	}

	var returns []cfg.Point
	validReturns := true
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Exprs) != 1 || len(info.Symbols) != 1 || info.Symbols[0] != c.resultSym {
			validReturns = false
			return
		}
		returns = append(returns, p)
	})
	if !validReturns || len(returns) == 0 {
		return 0, false
	}

	idom, _ := analysis.ComputeDominators(graph.CFG())
	for _, ret := range returns {
		if !analysis.Dominates(idom, c.point, ret) ||
			!successGuardDominatesReturn(graph, idom, c.guardSym, ret, assignments) {
			return 0, false
		}
	}
	return c.callbackParam, true
}

func isProtectedCallGlobal(graph *cfg.Graph, p cfg.Point, call *cfg.CallInfo) bool {
	if call == nil || call.Method != "" {
		return false
	}
	switch call.CalleeName {
	case "pcall", "cpcall", "xpcall":
	default:
		return false
	}
	sym := call.CalleeSymbol
	if sym == 0 {
		sym, _ = graph.SymbolAt(p, call.CalleeName)
	}
	kind, ok := graph.SymbolKind(sym)
	return sym != 0 && ok && kind == basecfg.SymbolGlobal
}

func successGuardDominatesReturn(graph *cfg.Graph, idom map[cfg.Point]cfg.Point, guardSym cfg.SymbolID, ret cfg.Point, assignments map[cfg.SymbolID]int) bool {
	proved := false
	graph.EachNode(func(p cfg.Point, node cfg.NodeInfo) {
		branch, ok := node.(*cfg.BranchInfo)
		if !ok || proved || !analysis.Dominates(idom, p, ret) {
			return
		}
		var failureWhenTrue bool
		var ident *ast.IdentExpr
		switch cond := branch.Condition.(type) {
		case *ast.UnaryNotOpExpr:
			ident, _ = cond.Expr.(*ast.IdentExpr)
			failureWhenTrue = true
		case *ast.IdentExpr:
			ident = cond
		default:
			return
		}
		if ident == nil {
			return
		}
		sym, ok := graph.Bindings().SymbolOf(ident)
		if !ok || sym != guardSym {
			return
		}
		for _, succ := range graph.Successors(p) {
			truthy, known := graph.EdgeCond(p, succ)
			if known && truthy == failureWhenTrue && !normalExitReachable(graph, succ, ret, assignments) {
				proved = true
			}
		}
	})
	return proved
}

// normalExitReachable rejects a failure arm that can reach the forwarded
// result, fall through the function, or stop at a call that is not the builtin
// terminating error function. Cycles with no exit are safe: they do not return.
func normalExitReachable(graph *cfg.Graph, start, ret cfg.Point, assignments map[cfg.SymbolID]int) bool {
	seen := make(map[cfg.Point]bool)
	queue := []cfg.Point{start}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if seen[p] {
			continue
		}
		seen[p] = true
		if p == ret || p == graph.Exit() {
			return true
		}
		succs := graph.Successors(p)
		if len(succs) == 0 {
			call, ok := graph.Info(p).(*cfg.CallInfo)
			if !ok || call.CalleeName != "error" || call.Method != "" {
				return true
			}
			sym := call.CalleeSymbol
			if sym == 0 {
				sym, _ = graph.SymbolAt(p, "error")
			}
			kind, known := graph.SymbolKind(sym)
			if sym == 0 || !known || kind != basecfg.SymbolGlobal || assignments[sym] != 0 {
				return true
			}
		}
		queue = append(queue, succs...)
	}
	return false
}
