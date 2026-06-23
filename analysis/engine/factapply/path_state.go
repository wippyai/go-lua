package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
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
	pathKey := factPathKeyAt(resolver, point, path)
	if pathKey == "" {
		return s, false
	}
	ks := resolver.KeySpace()
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return s, false
	}
	out := s
	out = writeLocalPathKeyWithStaticStringAlias(reg, ks, out, localKey, value)
	for _, target := range s.EquivalentPathKeys(ks, pathKey) {
		out = writePathKeyWithStaticStringAlias(reg, ks, out, target, value)
	}
	return out, true
}

func writePathKeyWithStaticStringAlias(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	s state.State,
	pathKey pathdom.PathKey,
	value product.Value,
) state.State {
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return s
	}
	return writeLocalPathKeyWithStaticStringAlias(reg, ks, s, localKey, value)
}

func writeLocalPathKeyWithStaticStringAlias(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	s state.State,
	localKey keyspace.Key,
	value product.Value,
) state.State {
	out := s.WriteLocalPathKey(reg, localKey, value)
	if canonical, ok := ks.FieldCanonical(localKey); ok {
		out = out.WriteLocalPathKey(reg, canonical, value)
	}
	return out
}

func invalidatePathSubtreeAt(
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (state.State, bool) {
	return invalidatePathAt(s, resolver, point, path, state.State.InvalidatePathKeySubtree)
}

func invalidatePathDescendantsAt(
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (state.State, bool) {
	return invalidatePathAt(s, resolver, point, path, state.State.InvalidatePathKeyDescendants)
}

// invalidatePathAt resolves path to a key at point and applies invalidate to
// the key and each equivalent alias, failing if the key is unresolved or any
// invalidation reports no change.
func invalidatePathAt(
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
	invalidate func(state.State, *keyspace.KeySpace, pathdom.PathKey) (state.State, bool),
) (state.State, bool) {
	pathKey := factPathKeyAt(resolver, point, path)
	if pathKey == "" {
		return s, false
	}
	ks := resolver.KeySpace()
	out := s
	for _, target := range pathKeyWithEquivalentAliases(ks, s, pathKey) {
		var ok bool
		out, ok = invalidate(out, ks, target)
		if !ok {
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
	includeDescendantAliases bool,
) (state.State, bool) {
	pathKey := factPathKeyAt(resolver, point, path)
	if pathKey == "" {
		return s, false
	}
	ks := resolver.KeySpace()
	keys := pathMutationRootKeys(ks, s, pathKey, includeDescendantAliases)
	out := s
	for _, key := range keys {
		k, ok := ks.FromStateKey(key)
		if !ok || k.Sym == 0 {
			continue
		}
		out = out.UpdateValue(reg, statekey.SymbolValue(k.Sym), func(value product.Value) product.Value {
			if product.Equal(reg, value, product.Bottom(reg)) {
				return value
			}
			return product.Set(reg, value, variantorigin.Key, variantorigin.Top())
		})
	}
	return out, true
}

func invalidateRootStructuralWitnessForPathMutationAt(
	reg *axis.Registry,
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (state.State, bool) {
	pathKey := factPathKeyAt(resolver, point, path)
	if pathKey == "" {
		return s, false
	}
	ks := resolver.KeySpace()
	keys := pathMutationRootKeys(ks, s, pathKey, true)
	out := s
	for _, key := range keys {
		k, ok := ks.FromStateKey(key)
		if !ok || k.Sym == 0 {
			continue
		}
		out = out.UpdateValue(reg, statekey.SymbolValue(k.Sym), func(value product.Value) product.Value {
			if product.Equal(reg, value, product.Bottom(reg)) {
				return value
			}
			return product.Set(reg, value, typewitness.Key, typewitness.Top())
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
	pathKey := factPathKeyAt(resolver, point, path)
	if pathKey == "" {
		return s, false
	}
	ks := resolver.KeySpace()
	targets := pathStaticMemberInvalidationTargets(ks, s, pathKey, descendantsOnly)
	out := s
	for _, target := range targets {
		k, ok := ks.FromStateKey(target.key)
		if !ok || k.Sym == 0 {
			continue
		}
		root := out.ReadValue(reg, statekey.SymbolValue(k.Sym))
		id, ok := product.Get(reg, root, identity.Key).ID()
		if !ok {
			continue
		}
		segs := ks.Segments(k)
		object := out.ReadHeapTableObject(reg, id)
		var changed bool
		if target.descendantsOnly {
			object, changed = object.WithoutStaticMemberDescendants(ks, segs)
		} else {
			object, changed = object.WithoutStaticMemberSubtree(ks, segs)
		}
		if changed {
			out = out.WriteHeapTableObject(reg, id, object)
		}
	}
	return out, true
}

func pathMutationRootKeys(ks *keyspace.KeySpace, s state.State, pathKey pathdom.PathKey, includeDescendantAliases bool) []pathdom.PathKey {
	keys := pathKeyWithEquivalentAliases(ks, s, pathKey)
	if !includeDescendantAliases {
		return dedupePathKeys(keys)
	}
	prefixes, ok := s.PathKeyDescendantInvalidationPrefixes(ks, pathKey)
	if !ok {
		return dedupePathKeys(keys)
	}
	keys = append(keys, prefixes.Descendants...)
	keys = append(keys, prefixes.Subtrees...)
	return dedupePathKeys(keys)
}

type pathStaticMemberInvalidationTarget struct {
	key             pathdom.PathKey
	descendantsOnly bool
}

func pathStaticMemberInvalidationTargets(ks *keyspace.KeySpace, s state.State, pathKey pathdom.PathKey, descendantsOnly bool) []pathStaticMemberInvalidationTarget {
	targetKeys := pathKeyWithEquivalentAliases(ks, s, pathKey)
	targets := make([]pathStaticMemberInvalidationTarget, 0, len(targetKeys))
	for _, equivalent := range targetKeys {
		targets = append(targets, pathStaticMemberInvalidationTarget{
			key:             equivalent,
			descendantsOnly: descendantsOnly,
		})
	}
	if !descendantsOnly {
		return dedupePathStaticMemberInvalidationTargets(targets)
	}
	prefixes, ok := s.PathKeyDescendantInvalidationPrefixes(ks, pathKey)
	if !ok {
		return dedupePathStaticMemberInvalidationTargets(targets)
	}
	for _, prefix := range prefixes.Descendants {
		targets = append(targets, pathStaticMemberInvalidationTarget{
			key:             prefix,
			descendantsOnly: true,
		})
	}
	for _, prefix := range prefixes.Subtrees {
		targets = append(targets, pathStaticMemberInvalidationTarget{
			key:             prefix,
			descendantsOnly: false,
		})
	}
	return dedupePathStaticMemberInvalidationTargets(targets)
}

func pathKeyWithEquivalentAliases(ks *keyspace.KeySpace, s state.State, pathKey pathdom.PathKey) []pathdom.PathKey {
	if pathKey == "" {
		return nil
	}
	keys := append([]pathdom.PathKey{pathKey}, s.EquivalentPathKeys(ks, pathKey)...)
	return dedupePathKeys(keys)
}

func dedupePathKeys(in []pathdom.PathKey) []pathdom.PathKey {
	if len(in) < 2 {
		return in
	}
	seen := make(map[pathdom.PathKey]struct{}, len(in))
	out := in[:0]
	for _, key := range in {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func dedupePathStaticMemberInvalidationTargets(in []pathStaticMemberInvalidationTarget) []pathStaticMemberInvalidationTarget {
	if len(in) < 2 {
		return in
	}
	seen := make(map[pathStaticMemberInvalidationTarget]struct{}, len(in))
	out := in[:0]
	for _, target := range in {
		if target.key == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		out = append(out, target)
	}
	return out
}
