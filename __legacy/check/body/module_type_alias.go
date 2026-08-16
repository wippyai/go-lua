package body

import (
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
)

// requireAliasTypeResolver resolves annotations such as store_mod.Store when
// store_mod is the local binding introduced by `local store_mod = require("store")`.
// The alias is lexical type-namespace evidence, not a runtime value fallback.
type requireAliasTypeResolver struct {
	external typelookup.Source
	aliases  map[string]string
}

func newRequireAliasTypeResolver(projection moduleidentity.Projection, external typelookup.Source) requireAliasTypeResolver {
	return requireAliasTypeResolver{
		external: external,
		aliases:  projection.ModuleAliases(),
	}
}

func (r requireAliasTypeResolver) ResolveTypeRef(parts []string) (typ.Type, bool) {
	if len(parts) > 1 {
		if modulePath := r.aliases[parts[0]]; modulePath != "" {
			if t, ok := r.external.ResolveTypeRefWithModulePrefix(modulePath, parts[1:]); ok {
				return t, true
			}
		}
	}
	return r.external.ResolveTypeRef(parts)
}
