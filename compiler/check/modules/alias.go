package modules

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// CollectAliases extracts module alias mappings from require() assignments in a graph.
//
// This scans the graph for patterns like:
//
//	local json = require("json")
//
// And builds a map from the symbol ID (for `json`) to the module path ("json").
// This enables the type checker to resolve module types when the alias is used.
//
// The function handles direct require() calls with a string literal argument,
// including multi-target assignments (e.g., local m = require("mod")).
// It does not require the assignment to be local, since module aliases can be
// introduced via re-assignments or outer scope bindings.
//
// This is called once per graph; nested functions merge their local aliases
// with the session-level alias map.
func CollectAliases(graph *cfg.Graph) map[cfg.SymbolID]string {
	if graph == nil {
		return nil
	}

	aliases := make(map[cfg.SymbolID]string)

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || len(info.Targets) == 0 || len(info.Sources) == 0 {
			return
		}
		for i, target := range info.Targets {
			if target.Kind != cfg.TargetIdent {
				continue
			}
			if i >= len(info.Sources) {
				continue
			}
			source := info.Sources[i]
			if source == nil {
				continue
			}
			call, ok := source.(*ast.FuncCallExpr)
			if !ok || call.Method != "" || call.Receiver != nil {
				continue
			}
			ident, ok := call.Func.(*ast.IdentExpr)
			if !ok || ident.Value != "require" {
				continue
			}
			if len(call.Args) != 1 {
				continue
			}
			strLit, ok := call.Args[0].(*ast.StringExpr)
			if !ok {
				continue
			}

			sym := target.Symbol
			if sym == 0 && target.Name != "" {
				var symOk bool
				sym, symOk = graph.SymbolAt(p, target.Name)
				if !symOk {
					continue
				}
			}
			if sym == 0 {
				continue
			}
			aliases[sym] = strLit.Value
		}
	})

	if len(aliases) == 0 {
		return nil
	}
	return aliases
}

// MergeAliases returns a merged alias map with overlay taking precedence.
// Always returns a fresh map when any input is non-nil to avoid mutation side effects.
func MergeAliases(base, overlay map[cfg.SymbolID]string) map[cfg.SymbolID]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]string, len(base)+len(overlay))
	for sym, path := range base {
		out[sym] = path
	}
	for sym, path := range overlay {
		out[sym] = path
	}
	return out
}
