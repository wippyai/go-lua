package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
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
	out := writePathKeyWithStaticStringAlias(reg, s, pathKey, value)
	for _, alias := range equivalent {
		out = writePathKeyWithStaticStringAlias(reg, out, alias, value)
	}
	return out, true
}

func writePathKeyWithStaticStringAlias(
	reg *axis.Registry,
	s state.State,
	pathKey pathdom.PathKey,
	value product.Value,
) state.State {
	out := s.WritePathKey(reg, pathKey, value)
	if canonical, ok := pathaddr.FieldCanonicalPathKey(pathKey); ok {
		out = out.WritePathKey(reg, canonical, value)
	}
	return out
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

func invalidateRootOriginsForPathMutationAt(
	reg *axis.Registry,
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (state.State, bool) {
	pathKey := resolver.KeyAt(point, path)
	if pathKey == "" {
		return s, false
	}
	keys := append([]pathdom.PathKey{pathKey}, s.EquivalentPathKeys(pathKey)...)
	out := s
	for _, key := range keys {
		localPath, ok := pathaddr.LocalPathFromKey(key)
		if !ok || localPath.Symbol == 0 {
			continue
		}
		out = out.UpdateValue(reg, statekey.SymbolValue(localPath.Symbol), func(value product.Value) product.Value {
			if product.Equal(reg, value, product.Bottom(reg)) {
				return value
			}
			return product.Set(reg, value, variantorigin.Key, variantorigin.Top())
		})
	}
	return out, true
}

func invalidateHeapStaticMemberSubtreeAt(
	reg *axis.Registry,
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (state.State, bool) {
	return invalidateHeapStaticMembersAt(reg, s, resolver, point, path, false)
}

func invalidateHeapStaticMemberDescendantsAt(
	reg *axis.Registry,
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (state.State, bool) {
	return invalidateHeapStaticMembersAt(reg, s, resolver, point, path, true)
}

func invalidateHeapStaticMembersAt(
	reg *axis.Registry,
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
	descendantsOnly bool,
) (state.State, bool) {
	pathKey := resolver.KeyAt(point, path)
	if pathKey == "" {
		return s, false
	}
	keys := append([]pathdom.PathKey{pathKey}, s.EquivalentPathKeys(pathKey)...)
	out := s
	for _, key := range keys {
		localPath, ok := pathaddr.LocalPathFromKey(key)
		if !ok || localPath.Symbol == 0 {
			continue
		}
		root := out.ReadValue(reg, statekey.SymbolValue(localPath.Symbol))
		id, ok := product.Get(reg, root, identity.Key).ID()
		if !ok {
			continue
		}
		object := out.ReadHeapTableObject(reg, id)
		var changed bool
		if descendantsOnly {
			object, changed = object.WithoutStaticMemberDescendants(localPath.Segments)
		} else {
			object, changed = object.WithoutStaticMemberSubtree(localPath.Segments)
		}
		if changed {
			out = out.WriteHeapTableObject(reg, id, object)
		}
	}
	return out, true
}
