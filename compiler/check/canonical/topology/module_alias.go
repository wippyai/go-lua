package topology

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// ModuleAliasDiscoveryInput is the canonical graph-hierarchy input for
// require() alias discovery.
type ModuleAliasDiscoveryInput struct {
	Root *cfg.Graph

	GraphForFunc    func(*ast.FunctionExpr) *cfg.Graph
	AliasesForGraph func(*cfg.Graph) map[cfg.SymbolID]string
}

// DiscoverModuleAliases walks the CFG hierarchy and returns every require()
// alias path keyed by the binding symbol that owns the alias.
func DiscoverModuleAliases(in ModuleAliasDiscoveryInput) map[cfg.SymbolID]string {
	if in.Root == nil || in.AliasesForGraph == nil {
		return nil
	}
	out := make(map[cfg.SymbolID]string)
	WalkHierarchy(HierarchyInput{Root: in.Root, GraphForFunc: in.GraphForFunc}, func(node HierarchyNode) {
		g := node.Graph
		for sym, path := range in.AliasesForGraph(g) {
			if sym != 0 && path != "" {
				out[sym] = path
			}
		}
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

// ResolveModuleAliases resolves discovered require() alias paths to enriched
// module exports for the immutable topology fact set.
func ResolveModuleAliases(paths map[cfg.SymbolID]string, manifests io.ManifestQuerier) []ModuleAlias {
	if len(paths) == 0 || manifests == nil {
		return nil
	}
	out := make([]ModuleAlias, 0, len(paths))
	for sym, path := range paths {
		if export := io.LookupEnrichedExport(manifests, path); export != nil && !typ.IsAbsentOrUnknown(export) {
			out = append(out, ModuleAlias{Symbol: sym, Type: export})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
