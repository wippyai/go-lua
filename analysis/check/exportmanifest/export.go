// Package exportmanifest publishes solved checker results into module manifests.
package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// FromProgramResult publishes the manifest evidence currently represented by
// the solved program result. It intentionally publishes only stable manifest
// sections with public read models: module export type and module-local type
// definitions. Direct function signatures are left untouched until body/program
// results expose signature.Function facts explicitly.
func FromProgramResult(path string, result program.Result) *manifest.Manifest {
	m := manifest.New(path)
	if export, ok := exportType(result); ok {
		m.SetExport(export)
	} else {
		m.SetExport(typ.Unknown)
	}
	publishTypeDefinitions(m, result.RootResult())
	return m
}

func exportType(result program.Result) (typ.Type, bool) {
	root := result.RootResult()
	if root == nil {
		return nil, false
	}
	if summary, ok := result.Snapshot().Read(result.RootKey()); ok {
		if t, ok := typeList(root.Registry(), summary.Returns); ok && !typ.IsUnknown(t) {
			return t, true
		}
	}
	return rootReturnType(root)
}

func rootReturnType(result *body.Result) (typ.Type, bool) {
	if result == nil || result.Graph() == nil {
		return nil, false
	}
	var candidates []typ.Type
	for _, point := range result.ReturnPoints() {
		fact, ok := result.ReturnFact(point)
		if !ok {
			continue
		}
		types, ok := sourceTypes(result, point, fact.Sources)
		if !ok {
			continue
		}
		if len(types) == 1 {
			candidates = append(candidates, types[0])
			continue
		}
		candidates = append(candidates, typ.NewTuple(types...))
	}
	return unionType(candidates)
}
