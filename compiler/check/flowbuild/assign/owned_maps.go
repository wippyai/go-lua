package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
)

// ClosedMapVars proves that a fresh local map is populated only by direct
// literal writes before its sole indexed read. A value learned from those
// writes is safe only while no other reference can add or mutate entries.
// Deliberately reject loops, captures and every unclassified use.
func ClosedMapVars(graph *cfg.Graph, inputs *flow.Inputs) map[cfg.SymbolID]bool {
	if graph == nil || inputs == nil || len(inputs.RefinableAnnotatedVars) == 0 ||
		graph.Bindings() == nil || graphHasCycle(graph) {
		return nil
	}
	bindings := graph.Bindings()
	var closed map[cfg.SymbolID]bool
	for sym := range inputs.RefinableAnnotatedVars {
		if kind, ok := bindings.Kind(sym); !ok || kind != basecfg.SymbolLocal {
			continue
		}
		if !closedMapUses(graph, bindings, inputs, sym) {
			continue
		}
		if closed == nil {
			closed = make(map[cfg.SymbolID]bool)
		}
		closed[sym] = true
	}
	return closed
}

func closedMapUses(graph *cfg.Graph, bindings *bind.BindingTable, inputs *flow.Inputs, sym cfg.SymbolID) bool {
	if capturedByNested(graph, bindings, sym) {
		return false
	}
	initialized, reads, writes := 0, 0, 0
	safe := true
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if !safe || info == nil {
			return
		}
		if info.NumericFor != nil || len(info.IterExprs) > 0 {
			safe = false
			return
		}
		if len(info.Targets) != 1 || len(info.Sources) != 1 {
			for _, source := range info.Sources {
				if exprUsesSymbol(source, bindings, sym) {
					safe = false
				}
			}
			for _, target := range info.Targets {
				if target.Symbol == sym || target.BaseSymbol == sym ||
					exprUsesSymbol(target.Expr, bindings, sym) ||
					exprUsesSymbol(target.Base, bindings, sym) ||
					exprUsesSymbol(target.Key, bindings, sym) {
					safe = false
				}
			}
			return
		}
		target, source := info.Targets[0], info.Sources[0]
		switch {
		case target.Kind == cfg.TargetIdent && target.Symbol == sym:
			table, ok := source.(*ast.TableExpr)
			if !info.IsLocal || !ok || len(table.Fields) != 0 {
				safe = false
				return
			}
			initialized++
		case target.Kind == cfg.TargetIndex && target.BaseSymbol == sym:
			base, direct := target.Base.(*ast.IdentExpr)
			value, literal := source.(*ast.TableExpr)
			if !direct || !literal || base == nil || value == nil ||
				exprUsesSymbol(target.Key, bindings, sym) || exprUsesSymbol(source, bindings, sym) ||
				!hasDynamicMapWrite(inputs, p, sym) {
				safe = false
				return
			}
			writes++
		case target.BaseSymbol == sym || exprUsesSymbol(target.Expr, bindings, sym) ||
			exprUsesSymbol(target.Base, bindings, sym) || exprUsesSymbol(target.Key, bindings, sym):
			safe = false
		case target.Kind == cfg.TargetIdent && info.IsLocal && indexedMapRead(source, bindings, sym):
			reads++
		default:
			if exprUsesSymbol(source, bindings, sym) {
				safe = false
			}
		}
	})
	graph.EachStmtCall(func(_ cfg.Point, info *cfg.CallInfo) {
		if info != nil && exprUsesSymbol(info.Call, bindings, sym) {
			safe = false
		}
	})
	graph.EachReturn(func(_ cfg.Point, info *cfg.ReturnInfo) {
		for _, expr := range info.Exprs {
			if exprUsesSymbol(expr, bindings, sym) {
				safe = false
			}
		}
	})
	graph.EachBranch(func(_ cfg.Point, info *cfg.BranchInfo) {
		if info != nil && exprUsesSymbol(info.Condition, bindings, sym) {
			safe = false
		}
	})
	graph.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if info != nil && (info.TargetPath.Symbol == sym || info.ReceiverSymbol == sym ||
			exprUsesSymbol(info.Receiver, bindings, sym)) {
			safe = false
		}
	})
	return safe && initialized == 1 && writes > 0 && reads == 1
}

func exprUsesSymbol(expr ast.Expr, bindings *bind.BindingTable, sym cfg.SymbolID) bool {
	var refs []cfg.SymbolID
	collectExprSymbols(expr, bindings, &refs)
	for _, ref := range refs {
		if ref == sym {
			return true
		}
	}
	return false
}

func indexedMapRead(expr ast.Expr, bindings *bind.BindingTable, sym cfg.SymbolID) bool {
	index, ok := expr.(*ast.AttrGetExpr)
	if !ok || index.Key == nil || exprUsesSymbol(index.Key, bindings, sym) {
		return false
	}
	base, ok := index.Object.(*ast.IdentExpr)
	if !ok {
		return false
	}
	bound, ok := bindings.SymbolOf(base)
	return ok && bound == sym
}

func hasDynamicMapWrite(inputs *flow.Inputs, p cfg.Point, sym cfg.SymbolID) bool {
	for _, write := range inputs.IndexerAssignments {
		if write.Point == p && write.Symbol == sym && len(write.Segments) == 0 {
			return true
		}
	}
	return false
}

func capturedByNested(graph *cfg.Graph, bindings *bind.BindingTable, sym cfg.SymbolID) bool {
	for _, nested := range graph.NestedFunctions() {
		if nested.Func == nil {
			continue
		}
		for _, captured := range bindings.CapturedSymbols(nested.Func) {
			if captured == sym {
				return true
			}
		}
		if capturedByNested(cfg.BuildWithBindings(nested.Func, bindings), bindings, sym) {
			return true
		}
	}
	return false
}

func graphHasCycle(graph *cfg.Graph) bool {
	state := make(map[cfg.Point]uint8)
	var visit func(cfg.Point) bool
	visit = func(p cfg.Point) bool {
		if state[p] == 1 {
			return true
		}
		if state[p] == 2 {
			return false
		}
		state[p] = 1
		for _, next := range graph.Successors(p) {
			if visit(next) {
				return true
			}
		}
		state[p] = 2
		return false
	}
	for _, p := range graph.RPO() {
		if visit(p) {
			return true
		}
	}
	return false
}
