package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func defaultVisibilityResolver(bindings *bind.Result, built *cfgbuild.Result, facts factflow.Facts) *visibility.Resolver {
	if built == nil || built.Graph == nil {
		return visibility.NewResolver(visibility.NewTable(nil))
	}
	defs := visibilityDefinitions(bindings, built, facts)
	table := visibility.BuildForward(visibility.BuildConfig{
		Graph:       built.Graph,
		Definitions: defs,
	})
	return visibility.NewResolver(table)
}

func visibilityDefinitions(bindings *bind.Result, built *cfgbuild.Result, facts factflow.Facts) []visibility.Definition {
	graph := built.Graph
	assigned := make(map[symbol.ID]struct{})
	seen := make(map[visibilityDefinitionKey]struct{})
	var defs []visibility.Definition

	add := func(point cfg.Point, sym symbol.ID) {
		if sym == 0 {
			return
		}
		key := visibilityDefinitionKey{point: point, sym: sym}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		defs = append(defs, visibility.Definition{
			Point:  point,
			Symbol: sym,
			Root:   bindings.Name(sym),
		})
	}

	for _, point := range graph.RPO() {
		if fact, ok := facts.RootAssignment(point); ok {
			sym := fact.TargetSymbol()
			assigned[sym] = struct{}{}
			add(point, sym)
		}
	}

	needed := visibilityPathSymbols(graph, facts)
	for sym := range needed {
		if _, ok := assigned[sym]; !ok || shouldSeedVisibilityAtEntry(bindings, sym) {
			add(graph.Entry(), sym)
		}
	}
	return defs
}

type visibilityDefinitionKey struct {
	point cfg.Point
	sym   symbol.ID
}

func shouldSeedVisibilityAtEntry(bindings *bind.Result, sym symbol.ID) bool {
	kind, ok := bindings.Kind(sym)
	if !ok {
		return true
	}
	return kind == symbol.Global || kind == symbol.Upvalue
}

func visibilityPathSymbols(graph cfg.Graph, facts factflow.Facts) map[symbol.ID]struct{} {
	needed := make(map[symbol.ID]struct{})
	addPath := func(p pathdom.Path) {
		if p.Symbol != 0 && len(p.Segments) != 0 {
			needed[p.Symbol] = struct{}{}
		}
	}

	for _, p := range facts.ExpressionPaths() {
		addPath(p)
	}
	for _, point := range graph.RPO() {
		if fact, ok := facts.PathAssignment(point); ok {
			addPath(fact.TargetPath())
		}
		if fact, ok := facts.PathDescendantInvalidation(point); ok {
			addPath(fact.ContainerPath())
		}
		for _, fact := range facts.BranchRefinements(point) {
			addPath(fact.TargetPath())
		}
		for _, fact := range facts.BranchPresenceRelations(point) {
			addPath(fact.TriggerPath())
			addPath(fact.TargetPath())
		}
		for _, fact := range facts.BranchPathRelations(point) {
			addPath(fact.LeftPath())
			addPath(fact.RightPath())
		}
		for _, fact := range facts.PostconditionRefinements(point) {
			addPath(fact.TargetPath())
		}
		for _, fact := range facts.PostconditionPathRelations(point) {
			addPath(fact.LeftPath())
			addPath(fact.RightPath())
		}
	}
	return needed
}
