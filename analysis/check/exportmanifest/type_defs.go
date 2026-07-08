package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/manifest"
)

func publishTypeDefinitions(m *manifest.Manifest, result *body.Result) {
	if m == nil || result == nil || result.Graph() == nil {
		return
	}
	resolver := typeresolve.NewWithExternal(result, result.ModuleTypes())
	for _, point := range result.Graph().RPO() {
		fact, ok := result.TypeDefinition(point)
		if !ok {
			continue
		}
		switch fact.Kind {
		case cfgbuild.TypeDefinitionAlias:
			if fact.Type == nil || fact.Type.Name == "" {
				continue
			}
			decl, ok := result.TypeDef(fact.Type)
			if !ok {
				continue
			}
			t, ok := resolver.Decl(decl)
			if !ok {
				continue
			}
			m.DefineType(fact.Type.Name, t)
		case cfgbuild.TypeDefinitionInterface:
			if fact.Interface == nil || fact.Interface.Name == "" {
				continue
			}
			decl, ok := result.InterfaceDef(fact.Interface)
			if !ok {
				continue
			}
			t, ok := resolver.Decl(decl)
			if !ok {
				continue
			}
			m.DefineType(fact.Interface.Name, t)
		}
	}
}
