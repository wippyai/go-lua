package callsite

import "github.com/wippyai/go-lua/compiler/cfg"

// eachSymbolWithAliases visits sym followed by its direct-alias chain.
// Iteration stops on zero, self-loop, cycle, or when fn returns true.
func eachSymbolWithAliases(graph *cfg.Graph, sym cfg.SymbolID, fn func(cfg.SymbolID) bool) {
	if sym == 0 || fn == nil {
		return
	}

	if graph == nil {
		fn(sym)
		return
	}

	seen := make(map[cfg.SymbolID]struct{}, 4)
	current := sym
	for current != 0 {
		if _, ok := seen[current]; ok {
			return
		}
		seen[current] = struct{}{}

		if fn(current) {
			return
		}

		next := graph.DirectAliasSymbol(current)
		if next == 0 || next == current {
			return
		}
		current = next
	}
}
