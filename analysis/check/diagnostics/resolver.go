package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type resultResolver struct {
	types    map[string]ast.TypeExpr
	cache    map[string]typ.Type
	active   map[string]bool
	explicit typeannotation.Resolver
	parent   typeannotation.Resolver
}

func newResultResolver(result *check.Result, explicit, parent typeannotation.Resolver) *resultResolver {
	r := &resultResolver{
		types:    make(map[string]ast.TypeExpr),
		cache:    make(map[string]typ.Type),
		active:   make(map[string]bool),
		explicit: explicit,
		parent:   parent,
	}
	if result == nil || result.Graph() == nil {
		return r
	}
	for _, point := range result.Graph().RPO() {
		fact, ok := result.TypeDefinition(point)
		if !ok || fact.Kind != cfgfacts.TypeDefinitionAlias || fact.Type == nil || fact.Type.Name == "" || fact.Type.Type == nil {
			continue
		}
		r.types[fact.Type.Name] = fact.Type.Type
	}
	return r
}

func (r *resultResolver) ResolveTypeRef(path []string) (typ.Type, bool) {
	if len(path) != 1 {
		return resolveFallback(path, r.explicit, r.parent)
	}
	name := path[0]
	if t, ok := r.cache[name]; ok {
		return t, true
	}
	expr, ok := r.types[name]
	if !ok {
		return resolveFallback(path, r.explicit, r.parent)
	}
	if r.active[name] {
		return typ.NewRef("", name), true
	}
	r.active[name] = true
	t, ok := typeannotation.Type(expr, r)
	delete(r.active, name)
	if !ok {
		return resolveFallback(path, r.explicit, r.parent)
	}
	r.cache[name] = t
	return t, true
}

func resolveFallback(path []string, resolvers ...typeannotation.Resolver) (typ.Type, bool) {
	for _, resolver := range resolvers {
		if resolver == nil {
			continue
		}
		if t, ok := resolver.ResolveTypeRef(path); ok {
			return t, true
		}
	}
	return nil, false
}
