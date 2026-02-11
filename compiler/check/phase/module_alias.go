package phase

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func applyModuleAliasExports(
	declared flow.DeclaredTypes,
	moduleAliases map[cfg.SymbolID]string,
	manifests io.ManifestQuerier,
) flow.DeclaredTypes {
	if manifests == nil || len(moduleAliases) == 0 {
		return declared
	}
	if declared == nil {
		declared = make(flow.DeclaredTypes, len(moduleAliases))
	}
	for _, sym := range cfg.SortedSymbolIDs(moduleAliases) {
		path := moduleAliases[sym]
		if sym == 0 || path == "" {
			continue
		}
		if export := moduleAliasExport(manifests, path); export != nil {
			declared[sym] = export
		}
	}
	return declared
}

func moduleAliasExport(manifests io.ManifestQuerier, path string) typ.Type {
	if manifests == nil || path == "" {
		return nil
	}
	manifest := manifests.Manifest(path)
	if manifest == nil {
		if imports := manifests.Imports(); imports != nil {
			manifest = imports[path]
		}
	}
	if manifest == nil {
		return nil
	}
	return manifest.EnrichedExport()
}
