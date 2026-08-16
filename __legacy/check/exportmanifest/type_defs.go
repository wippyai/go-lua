package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/manifest"
)

func publishTypeDefinitions(m *manifest.Manifest, result *body.Result) {
	if m == nil || result == nil || result.Graph() == nil {
		return
	}
	resolver := result.TypeResolver()
	for _, point := range result.Graph().RPO() {
		fact, ok := result.TypeDefinition(point)
		if !ok {
			continue
		}
		switch fact.Kind {
		case body.TypeDefinitionAlias:
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
		case body.TypeDefinitionInterface:
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
	for _, exported := range returnedExportSourcePaths(result) {
		for name, alias := range result.QualifiedTypeAliases(exported.path.Symbol) {
			var t typ.Type
			var ok bool
			if alias.Decl.ID != 0 {
				t, ok = resolver.Decl(alias.Decl)
			} else if len(alias.Path) != 0 {
				t, ok = resolver.ResolveTypeRef(alias.Path)
			}
			if ok && t != nil {
				m.DefineType(name, t)
			}
		}
	}
}
