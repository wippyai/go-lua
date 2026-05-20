package api

import "github.com/wippyai/go-lua/compiler/check/scope"

// ParentScopeForGraph resolves the canonical parent scope for a graph.
// It prefers the canonical parent-scope hash recorded in store and uses
// defaultScope only when no stable parent is available.
func ParentScopeForGraph(store ParentScopes, graphID uint64, defaultScope *scope.State) *scope.State {
	if store == nil || graphID == 0 {
		return defaultScope
	}
	parentHash := store.GraphParentHashOf(graphID)
	if parentHash == 0 {
		return defaultScope
	}
	if parent := store.Parents()[parentHash]; parent != nil {
		return parent
	}
	return defaultScope
}

// ParentHashForGraph resolves the canonical parent hash for a graph.
// It prefers the stable graph-parent hash recorded in store and uses
// defaultScope.Hash() when no stable hash exists.
func ParentHashForGraph(store ParentScopes, graphID uint64, defaultScope *scope.State) uint64 {
	if store != nil && graphID != 0 {
		if parentHash := store.GraphParentHashOf(graphID); parentHash != 0 {
			return parentHash
		}
	}
	if defaultScope != nil {
		return defaultScope.Hash()
	}
	return 0
}
