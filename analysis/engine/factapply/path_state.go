package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func writePathAt(
	reg *axis.Registry,
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
	value product.Value,
) (state.State, bool) {
	pathKey := resolver.KeyAt(point, path)
	if pathKey == "" {
		return s, false
	}
	equivalent := s.EquivalentPathKeys(pathKey)
	out := s.WritePathKey(reg, pathKey, value)
	for _, alias := range equivalent {
		out = out.WritePathKey(reg, alias, value)
	}
	return out, true
}

func updatePathAt(
	reg *axis.Registry,
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
	fn func(product.Value) product.Value,
) (state.State, bool) {
	pathKey := resolver.KeyAt(point, path)
	if pathKey == "" {
		return s, false
	}
	return s.UpdatePathKey(reg, pathKey, fn), true
}

func invalidatePathSubtreeAt(
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (state.State, bool) {
	pathKey := resolver.KeyAt(point, path)
	if pathKey == "" {
		return s, false
	}
	equivalent := s.EquivalentPathKeys(pathKey)
	out, ok := s.InvalidatePathKeySubtree(pathKey)
	if !ok {
		return s, false
	}
	for _, alias := range equivalent {
		var aliasOK bool
		out, aliasOK = out.InvalidatePathKeySubtree(alias)
		if !aliasOK {
			return s, false
		}
	}
	return out, true
}

func invalidatePathDescendantsAt(
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (state.State, bool) {
	pathKey := resolver.KeyAt(point, path)
	if pathKey == "" {
		return s, false
	}
	equivalent := s.EquivalentPathKeys(pathKey)
	out, ok := s.InvalidatePathKeyDescendants(pathKey)
	if !ok {
		return s, false
	}
	for _, alias := range equivalent {
		var aliasOK bool
		out, aliasOK = out.InvalidatePathKeyDescendants(alias)
		if !aliasOK {
			return s, false
		}
	}
	return out, true
}
