package body

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/visibilityfacts"
)

func defaultVisibilityResolver(bindings *bind.Result, built *cfgbuild.Result, facts factflow.Facts, genericFors map[cfg.Point]GenericForFact) *visibility.Resolver {
	var graph cfg.Graph
	if built != nil {
		graph = built.Graph
	}
	defs := visibilityfacts.Definitions(bindings, graph, facts)
	defs = append(defs, genericForVariableDefinitions(bindings, graph, genericFors)...)
	return visibility.NewResolver(visibility.BuildForward(visibility.BuildConfig{
		Graph:       graph,
		Definitions: defs,
	}))
}

func genericForVariableDefinitions(bindings *bind.Result, graph cfg.Graph, genericFors map[cfg.Point]GenericForFact) []visibility.Definition {
	if bindings == nil || graph == nil {
		return nil
	}
	var defs []visibility.Definition
	for _, point := range graph.RPO() {
		fact, ok := genericFors[point]
		if !ok || fact.Role != GenericForRoleVariable || !fact.HasSymbols ||
			fact.VariableIndex < 0 || fact.VariableIndex >= len(fact.Symbols) {
			continue
		}
		sym := fact.Symbols[fact.VariableIndex]
		if sym == 0 {
			continue
		}
		defs = append(defs, visibility.Definition{
			Point:  point,
			Symbol: sym,
			Root:   bindings.Name(sym),
		})
	}
	return defs
}
