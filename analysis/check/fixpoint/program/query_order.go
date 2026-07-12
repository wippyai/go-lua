package program

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// dependencyFirstFunctions orders query equations so an acyclic lexical
// callee reaches its initial summary before any caller observes it. Recursive
// components remain ordinary query equations; only their stable initial order
// changes. Runtime/dynamic calls for which the prepass cannot prove a lexical
// target deliberately contribute no edge.
func dependencyFirstFunctions(functions []query.Function, keys *programKeys, reg *axis.Registry) []query.Function {
	if len(functions) < 2 || keys == nil || reg == nil || len(keys.queryDependencies) == 0 {
		if keys != nil {
			keys.queryDependencies = nil
		}
		return functions
	}
	known := make(map[summary.SummaryKey]struct{}, len(functions))
	byKey := make(map[summary.SummaryKey]query.Function, len(functions))
	original := make([]summary.SummaryKey, 0, len(functions))
	for _, fn := range functions {
		known[fn.Key] = struct{}{}
		byKey[fn.Key] = fn
		original = append(original, fn.Key)
	}
	edges := make(map[summary.SummaryKey][]summary.SummaryKey, len(keys.queryDependencies))
	for owner, dependencies := range keys.queryDependencies {
		if _, ok := known[owner]; !ok {
			continue
		}
		for dependency := range dependencies {
			if _, ok := known[dependency]; ok {
				edges[owner] = append(edges[owner], dependency)
			}
		}
	}
	keys.queryDependencies = nil
	orderedKeys := dependencyFirstSummaryKeys(original, edges)
	if len(orderedKeys) != len(functions) {
		return functions
	}
	out := make([]query.Function, 0, len(functions))
	for _, key := range orderedKeys {
		out = append(out, byKey[key])
	}
	return out
}

func recordQueryDependencies(reg *axis.Registry, keys *programKeys, owner summary.SummaryKey, result prepassCallResult) {
	if keys == nil || reg == nil || result == nil || result.Graph() == nil {
		return
	}
	deps := queryDependenciesForPrepass(reg, *keys, owner, result)
	if len(deps) == 0 {
		return
	}
	if keys.queryDependencies == nil {
		keys.queryDependencies = make(map[summary.SummaryKey]map[summary.SummaryKey]struct{})
	}
	set := keys.queryDependencies[owner]
	if set == nil {
		set = make(map[summary.SummaryKey]struct{}, len(deps))
		keys.queryDependencies[owner] = set
	}
	for _, dependency := range deps {
		set[dependency] = struct{}{}
	}
}

func queryDependenciesForPrepass(reg *axis.Registry, keys programKeys, owner summary.SummaryKey, result prepassCallResult) []summary.SummaryKey {
	deps := make(map[summary.SummaryKey]struct{})
	add := func(key summary.SummaryKey) {
		deps[key] = struct{}{}
	}
	graph := result.Graph()
	for _, point := range graph.RPO() {
		site, ok := result.CallSiteView(point)
		if !ok {
			continue
		}
		ambiguousPath := site.CalleePathKey().Valid() && len(keys.pathMultiKeys[site.CalleePathKey()]) > 1
		current, hasCurrent := prepassCurrentIdentityKey(reg, result, point, site, keys)
		matched := false
		if !ambiguousPath {
			if expr, ok := site.Expr(); ok && expr != 0 {
				if contextKey, ok := keys.contexts.CallContextKey(owner, expr); ok && (!hasCurrent || contextKey.Ref == current.Ref) {
					add(contextKey)
					if hasCurrent && current != contextKey && current.Ref == contextKey.Ref {
						add(current)
					}
					matched = true
				}
			}
		}
		if !matched && hasCurrent {
			add(current)
			matched = true
		}
		if !matched && !ambiguousPath && site.CalleeSymbol() != 0 {
			if key, ok := keys.targetKeys[site.CalleeSymbol()]; ok {
				add(key)
				matched = true
			}
		}
		if !matched && !prepassCalleeBlocksDefinitionPath(reg, result, point, site) {
			pathKey := site.CalleePathKey()
			if key, ok := keys.pathKeys[pathKey]; ok {
				add(key)
			} else {
				for _, key := range keys.pathMultiKeys[pathKey] {
					add(key)
				}
			}
		}
		// Generic signature specialization and signature argument lowering can
		// consult a local callback's summary even when the callback is not the
		// callee. Restrict these edges to actual argument expressions.
		site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
			if !source.HasExpr || source.ExprRef == 0 {
				return true
			}
			if key, ok := keys.contexts.FunctionExpressionKey(owner, source.ExprRef); ok {
				add(key)
				return true
			}
			if functionSymbol, ok := result.ExpressionFunction(source.ExprRef); ok {
				if key, ok := keys.functionKeys[functionSymbol]; ok {
					add(key)
				}
			}
			return true
		})
	}
	out := make([]summary.SummaryKey, 0, len(deps))
	for key := range deps {
		out = append(out, key)
	}
	sortSummaryKeys(out)
	return out
}

// prepassCallResult is the minimal immutable result surface used for ordering.
// Keeping it as an interface makes the graph algorithm independently testable.
type prepassCallResult interface {
	Graph() cfg.Graph
	CallSiteView(cfg.Point) (factflow.CallSiteView, bool)
	CallCalleeValueAtBoundary(cfg.Point, factflow.CallSiteView) (product.Value, bool)
	ExpressionFunction(factflow.ExprRef) (symbol.ID, bool)
}

func prepassCurrentIdentityKey(reg *axis.Registry, result prepassCallResult, point cfg.Point, site factflow.CallSiteView, keys programKeys) (summary.SummaryKey, bool) {
	value, ok := result.CallCalleeValueAtBoundary(point, site)
	if !ok {
		return summary.SummaryKey{}, false
	}
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return summary.SummaryKey{}, false
	}
	key, ok := keys.functionIDs[id]
	return key, ok
}

func prepassCalleeBlocksDefinitionPath(reg *axis.Registry, result prepassCallResult, point cfg.Point, site factflow.CallSiteView) bool {
	value, ok := result.CallCalleeValueAtBoundary(point, site)
	if !ok {
		return false
	}
	if _, hasID := product.Get(reg, value, identity.Key).ID(); hasID {
		return true
	}
	return !product.Get(reg, value, runtimekind.Key).Contains(runtimekind.Function)
}

// dependencyFirstSummaryKeys condenses caller-to-callee edges with Tarjan's
// algorithm. Tarjan emits sink components first for this edge orientation,
// which is exactly dependency-first order. Vertices and adjacency use stable
// SummaryKey tie-breaks, while members of a recursive component retain their
// original query.Function rank so widening spelling cannot change.
func dependencyFirstSummaryKeys(original []summary.SummaryKey, edges map[summary.SummaryKey][]summary.SummaryKey) []summary.SummaryKey {
	known := make(map[summary.SummaryKey]struct{}, len(original))
	rank := make(map[summary.SummaryKey]int, len(original))
	vertices := make([]summary.SummaryKey, 0, len(original))
	for i, key := range original {
		if _, duplicate := known[key]; duplicate {
			continue
		}
		known[key] = struct{}{}
		rank[key] = i
		vertices = append(vertices, key)
	}
	sortSummaryKeys(vertices)
	for key := range edges {
		sortSummaryKeys(edges[key])
		edges[key] = slices.Compact(edges[key])
	}
	index := 0
	indices := make(map[summary.SummaryKey]int, len(vertices))
	low := make(map[summary.SummaryKey]int, len(vertices))
	onStack := make(map[summary.SummaryKey]bool, len(vertices))
	stack := make([]summary.SummaryKey, 0, len(vertices))
	out := make([]summary.SummaryKey, 0, len(vertices))
	var visit func(summary.SummaryKey)
	visit = func(v summary.SummaryKey) {
		index++
		indices[v] = index
		low[v] = index
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range edges[v] {
			if _, ok := known[w]; !ok {
				continue
			}
			if indices[w] == 0 {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && indices[w] < low[v] {
				low[v] = indices[w]
			}
		}
		if low[v] != indices[v] {
			return
		}
		component := make([]summary.SummaryKey, 0, 1)
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			component = append(component, w)
			if w == v {
				break
			}
		}
		slices.SortFunc(component, func(a, b summary.SummaryKey) int { return rank[a] - rank[b] })
		out = append(out, component...)
	}
	for _, vertex := range vertices {
		if indices[vertex] == 0 {
			visit(vertex)
		}
	}
	return out
}

func sortSummaryKeys(keys []summary.SummaryKey) {
	slices.SortFunc(keys, func(a, b summary.SummaryKey) int {
		if a.Less(b) {
			return -1
		}
		if b.Less(a) {
			return 1
		}
		return 0
	})
}
