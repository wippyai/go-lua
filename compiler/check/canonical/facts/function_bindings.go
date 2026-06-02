package facts

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
)

// collectFunctionBindings extracts source symbols that name module-local
// functions. It covers named function definitions and local-function assignments;
// field/table function values are separate topology facts because their identity
// is a path, not a root symbol.
func collectFunctionBindings(p Program) []topology.FunctionBinding {
	if p.RefForFuncSymbol == nil {
		return nil
	}
	var out []topology.FunctionBinding
	order := 0
	for _, owner := range p.Refs {
		g := graphOf(p, owner)
		if g == nil {
			continue
		}
		g.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
			if info == nil || info.Name == "" || info.Symbol == 0 {
				return
			}
			r, ok := p.RefForFuncSymbol(info.Symbol)
			if !ok {
				return
			}
			out = append(out, topology.FunctionBinding{Symbol: info.Symbol, FuncRef: r, Order: order})
			order++
		})
		for _, lfa := range g.LocalFunctionAssignments() {
			if lfa.Name == "" || lfa.Symbol == 0 {
				continue
			}
			r, ok := p.RefForFuncSymbol(lfa.Symbol)
			if !ok {
				continue
			}
			out = append(out, topology.FunctionBinding{Symbol: lfa.Symbol, FuncRef: r, Order: order})
			order++
		}
	}
	if len(out) == 0 {
		return nil
	}
	return compactFunctionBindingEntries(sortedFunctionBindings(out))
}

func sortedFunctionBindings(in []topology.FunctionBinding) []topology.FunctionBinding {
	slices.SortFunc(in, compareFunctionBindingEntry)
	return in
}
