package body

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
			rewritten := append(strings.Split(modulePath, "."), parts[1:]...)
			if t, ok := r.external.ResolveTypeRef(rewritten); ok {
				return t, true
			}
		}
	}
	return r.external.ResolveTypeRef(parts)
}
