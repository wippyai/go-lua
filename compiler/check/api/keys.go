// Package api defines the checker contract types used across phases and layers.
package api

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// GraphKey uniquely identifies a graph within a parent scope for query lookups.
// The key is stable and comparable, enabling memoization across iterations.
type GraphKey struct {
	GraphID    uint64 // Unique CFG ID from SessionStore.Graphs()
	ParentHash uint64 // Parent scope hash from SessionStore.Parents()
}

// FunctionFactKey identifies one FunctionFact projection inside one graph
// product. Query inputs use this key for fine-grained dependencies on a single
// function symbol instead of the whole legacy product.
type FunctionFactKey struct {
	GraphKey GraphKey
	Symbol   cfg.SymbolID
}

// LiteralSigKey identifies one function literal signature inside one graph
// product.
type LiteralSigKey struct {
	GraphKey GraphKey
	Func     *ast.FunctionExpr
}

// CapturedTypeKey identifies one captured symbol type inside one graph product.
type CapturedTypeKey struct {
	GraphKey GraphKey
	Symbol   cfg.SymbolID
}

// ConstructorFieldKey identifies one module-wide constructor field map by
// class symbol. The GraphKey is always ModuleFactsKey.
type ConstructorFieldKey struct {
	GraphKey GraphKey
	Symbol   cfg.SymbolID
}

// ModuleFactsKey identifies module-wide legacy facts that are not tied
// to one function graph, such as constructor field summaries keyed by class
// symbol.
func ModuleFactsKey() GraphKey {
	return GraphKey{}
}

// SymbolKey uniquely identifies a symbol within a parent scope.
type SymbolKey struct {
	Symbol     cfg.SymbolID
	ParentHash uint64
}

// FuncKey uniquely identifies a function analysis request for memoization.
// Fact dependencies are tracked by the query database as legacy function-result
// compatibility paths read fact products.
//
//   - GraphID: Unique identifier for the function's control flow graph. Each CFG
//     receives a monotonically increasing ID during construction, ensuring distinct
//     functions have distinct GraphIDs even if they have identical source code.
//
//   - ParentHash: Hash of the parent scope state. Functions with identical code but
//     different lexical environments (e.g., different captured variables or type
//     definitions in scope) must be analyzed separately.
type FuncKey struct {
	GraphID    uint64
	ParentHash uint64
}

// KeyForGraph creates a GraphKey from a graph and parent scope.
func KeyForGraph(graph *cfg.Graph, parentHash uint64) GraphKey {
	var graphID uint64
	if graph != nil {
		graphID = graph.ID()
	}
	return GraphKey{
		GraphID:    graphID,
		ParentHash: parentHash,
	}
}

// CompareGraphKeys provides canonical ordering for GraphKey.
func CompareGraphKeys(a, b GraphKey) int {
	if a.GraphID < b.GraphID {
		return -1
	}
	if a.GraphID > b.GraphID {
		return 1
	}
	if a.ParentHash < b.ParentHash {
		return -1
	}
	if a.ParentHash > b.ParentHash {
		return 1
	}
	return 0
}

// SortedGraphKeys returns GraphKeys from m in canonical order.
func SortedGraphKeys[T any](m map[GraphKey]T) []GraphKey {
	if len(m) == 0 {
		return nil
	}
	keys := make([]GraphKey, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return CompareGraphKeys(keys[i], keys[j]) < 0 })
	return keys
}
