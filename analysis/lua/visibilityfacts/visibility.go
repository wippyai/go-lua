// Package visibilityfacts adapts Lua factflow facts into generic visibility tables.
package visibilityfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Resolver builds the default Lua path visibility resolver from lowered facts.
func Resolver(bindings *bind.Result, graph cfg.Graph, facts factflow.Facts) *visibility.Resolver {
	if graph == nil {
		return visibility.NewResolver(visibility.NewTable(nil))
	}
	defs := Definitions(bindings, graph, facts)
	table := visibility.BuildForward(visibility.BuildConfig{
		Graph:       graph,
		Definitions: defs,
	})
	return visibility.NewResolver(table)
}

// Definitions extracts symbol definitions needed by point-local path resolution.
func Definitions(bindings *bind.Result, graph cfg.Graph, facts factflow.Facts) []visibility.Definition {
	if graph == nil {
		return nil
	}
	assigned := make(map[symbol.ID]struct{})
	seen := make(map[definitionKey]struct{})
	var defs []visibility.Definition

	add := func(point cfg.Point, sym symbol.ID) {
		if sym == 0 {
			return
		}
		key := definitionKey{point: point, sym: sym}
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
	for _, point := range graph.RPO() {
		for _, event := range facts.ChannelSelects(point) {
			resultPath, ok := event.ResultPath()
			if ok && resultPath.Symbol != 0 {
				add(point, resultPath.Symbol)
			}
			casePath, ok := event.CasePath()
			if ok && casePath.Symbol != 0 {
				add(point, casePath.Symbol)
			}
		}
	}

	needed := pathSymbols(graph, facts)
	for sym := range needed {
		if _, ok := assigned[sym]; !ok || shouldSeedAtEntry(bindings, sym) {
			add(graph.Entry(), sym)
		}
	}
	return defs
}

type definitionKey struct {
	point cfg.Point
	sym   symbol.ID
}

func shouldSeedAtEntry(bindings *bind.Result, sym symbol.ID) bool {
	kind, ok := bindings.Kind(sym)
	if !ok {
		return true
	}
	return kind == symbol.Global || kind == symbol.Upvalue
}

func pathSymbols(graph cfg.Graph, facts factflow.Facts) map[symbol.ID]struct{} {
	needed := make(map[symbol.ID]struct{})
	addPath := func(p pathdom.Path) {
		if p.Symbol != 0 && len(p.Segments) != 0 {
			needed[p.Symbol] = struct{}{}
		}
	}
	addProofPath := func(p pathdom.Path) {
		if p.Symbol != 0 {
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
		if site, ok := facts.CallSite(point); ok {
			addPath(site.CalleePath())
			if receiver, ok := site.ReceiverPath(); ok {
				addProofPath(receiver)
			}
			if method, ok := site.MethodPath(); ok {
				addPath(method)
			}
			for _, source := range site.ArgumentSources() {
				if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
					continue
				}
				if p, ok := facts.ExpressionPath(source.ExprRef); ok {
					addProofPath(p)
				}
			}
		}
		for _, fact := range facts.BranchRefinements(point) {
			addPath(fact.TargetPath())
		}
		for _, fact := range facts.BranchLenRefinements(point) {
			addProofPath(fact.ArrayPath())
		}
		for _, fact := range facts.BranchNumFloorRefinements(point) {
			addProofPath(fact.TargetPath())
		}
		for _, fact := range facts.BranchPresenceRelations(point) {
			addPath(fact.TriggerPath())
			addPath(fact.TargetPath())
		}
		for _, fact := range facts.BranchPathRelations(point) {
			addPath(fact.LeftPath())
			addPath(fact.RightPath())
		}
		for _, fact := range facts.BranchPathEvidence(point) {
			addProofPath(fact.Path())
			if other, ok := fact.OtherPath(); ok {
				addProofPath(other)
			}
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
