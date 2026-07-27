package factapply

import (
	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
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
	stateKey, ok := pathaddr.StateKeyFromPathKey(pathKey)
	if !ok {
		return s, false
	}
	localKey, ok := ks.FromPathKey(stateKey.PathKey())
	if !ok {
		return s, false
	}
	edit := s.EditPathEvidence(reg)
	writeLocalPathKeyWithStaticStringAliasEdit(ks, &edit, localKey, value)
	for _, target := range s.EquivalentStateKeys(ks, stateKey) {
		writeStateKeyWithStaticStringAliasEdit(ks, &edit, target, value)
	}
	return edit.Done(), true
}

func appendLocalPathKeyWithStaticStringAlias(keys []keyspace.Key, ks *keyspace.KeySpace, localKey keyspace.Key) []keyspace.Key {
	keys = append(keys, localKey)
	if canonical, ok := ks.FieldCanonical(localKey); ok {
		keys = append(keys, canonical)
	}
	return keys
}

func writeStateKeyWithStaticStringAliasEdit(
	ks *keyspace.KeySpace,
	edit *state.PathEvidenceEdit,
	stateKey pathaddr.StateKey,
	value product.Value,
) {
	localKey, ok := ks.FromPathKey(stateKey.PathKey())
	if !ok {
		return
	}
	writeLocalPathKeyWithStaticStringAliasEdit(ks, edit, localKey, value)
}

func writeLocalPathKeyWithStaticStringAliasEdit(
	ks *keyspace.KeySpace,
	edit *state.PathEvidenceEdit,
	localKey keyspace.Key,
	value product.Value,
) {
	edit.WriteLocalPathKey(localKey, value)
	if canonical, ok := ks.FieldCanonical(localKey); ok {
		edit.WriteLocalPathKey(canonical, value)
	}
}

func invalidatePathSubtreeAt(
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (state.State, bool) {
	return invalidatePathAt(s, resolver, point, path, invalidateStateKeySubtree)
}

func invalidatePathDescendantsAt(
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (state.State, bool) {
	return invalidatePathAt(s, resolver, point, path, invalidateStateKeyDescendants)
}

func invalidatePathDescendantsPreservingDynamicValueKeyMembershipsAt(
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (state.State, bool) {
	return invalidatePathAt(s, resolver, point, path, invalidateStateKeyDescendantsPreservingDynamicValueKeyMemberships)
}

func invalidateStateKeySubtree(s state.State, ks *keyspace.KeySpace, stateKey pathaddr.StateKey) (state.State, bool) {
	return s.InvalidatePathKeySubtree(ks, stateKey.PathKey())
}

func invalidateStateKeyDescendants(s state.State, ks *keyspace.KeySpace, stateKey pathaddr.StateKey) (state.State, bool) {
	return s.InvalidatePathKeyDescendants(ks, stateKey.PathKey())
}

func invalidateStateKeyDescendantsPreservingDynamicValueKeyMemberships(s state.State, ks *keyspace.KeySpace, stateKey pathaddr.StateKey) (state.State, bool) {
	return s.InvalidatePathKeyDescendantsPreservingDynamicValueKeyMemberships(ks, stateKey.PathKey())
}

// invalidatePathAt resolves path to a key at point and applies invalidate to
// the key and each equivalent alias, failing if the key is unresolved or any
// invalidation reports no change.
func invalidatePathAt(
	s state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	path pathdom.Path,
	invalidate func(state.State, *keyspace.KeySpace, pathaddr.StateKey) (state.State, bool),
) (state.State, bool) {
	pathKey := factPathKeyAt(resolver, point, path)
	if pathKey == "" {
		return s, false
	}
	ks := resolver.KeySpace()
	stateKey, ok := pathaddr.StateKeyFromPathKey(pathKey)
	if !ok {
		return s, false
	}
	out := s
	for _, target := range stateKeysWithEquivalentAliases(ks, s, stateKey) {
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
	stateKey, ok := pathaddr.StateKeyFromPathKey(pathKey)
	if !ok {
		return s, false
	}
	keys := pathMutationStateKeys(ks, s, stateKey, includeDescendantAliases)
	out := s
	for _, key := range keys {
		k, ok := ks.InternStateKey(key)
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
	stateKey, ok := pathaddr.StateKeyFromPathKey(pathKey)
	if !ok {
		return s, false
	}
	keys := pathMutationStateKeys(ks, s, stateKey, true)
	out := s
	for _, key := range keys {
		k, ok := ks.InternStateKey(key)
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
	stateKey, ok := pathaddr.StateKeyFromPathKey(pathKey)
	if !ok {
		return s, false
	}
	targets := pathStaticMemberInvalidationTargets(ks, s, stateKey, descendantsOnly)
	out := s
	for _, target := range targets {
		out = invalidateHeapTableFactsForStateKey(reg, s, out, resolver, point, ks, target)
	}
	return out, true
}

func invalidateHeapTableFactsForStateKey(
	reg *axis.Registry,
	base state.State,
	out state.State,
	resolver *visibility.Resolver,
	point cfg.Point,
	ks *keyspace.KeySpace,
	target pathStaticMemberInvalidationTarget,
) state.State {
	k, ok := ks.InternStateKey(target.key)
	if !ok || k.Sym == 0 {
		return out
	}
	segs, ok := ks.SegmentsView(k)
	if !ok {
		return out
	}
	out = invalidateHeapTableFactsForOwner(reg, out, ks, base.ReadValue(reg, statekey.SymbolValue(k.Sym)), segs, target.descendantsOnly)
	root := pathdom.NewPath(k.Sym, "").AppendSegments(segs)
	for split := 1; split <= len(segs); split++ {
		ownerPath := root.RootOnly().AppendSegments(segs[:split])
		owner, ok := resolvePathValueAt(reg, resolver, point, base, ownerPath, nil)
		if !ok {
			continue
		}
		out = invalidateHeapTableFactsForOwner(reg, out, ks, owner.value, segs[split:], target.descendantsOnly)
	}
	return out
}

func invalidateHeapTableFactsForOwner(
	reg *axis.Registry,
	out state.State,
	ks *keyspace.KeySpace,
	owner product.Value,
	segs []segment.Segment,
	descendantsOnly bool,
) state.State {
	id, ok := product.Get(reg, owner, identity.Key).ID()
	if !ok {
		return out
	}
	object := out.ReadHeapTableObject(reg, id)
	var changed bool
	if next, ok := invalidateHeapObjectRootStructuralWitness(reg, object); ok {
		object = next
		changed = true
	}
	if descendantsOnly {
		if next, ok := object.WithoutStaticMemberDescendants(ks, segs); ok {
			object = next
			changed = true
		}
		if next, ok := object.WithoutDynamicIndexFactDescendants(ks, segs); ok {
			object = next
			changed = true
		}
	} else {
		if next, ok := object.WithoutStaticMemberSubtree(ks, segs); ok {
			object = next
			changed = true
		}
		if next, ok := object.WithoutDynamicIndexFactSubtree(ks, segs); ok {
			object = next
			changed = true
		}
	}
	if changed {
		return out.WriteHeapTableObject(reg, id, object)
	}
	return out
}

func invalidateHeapObjectRootStructuralWitness(reg *axis.Registry, object heapidentity.TableObject) (heapidentity.TableObject, bool) {
	root := object.Root()
	next := product.Set(reg, root, typewitness.Key, typewitness.Top())
	next = product.Set(reg, next, variantorigin.Key, variantorigin.Top())
	if product.Equal(reg, root, next) {
		return object, false
	}
	return object.WithRoot(next), true
}

func pathMutationStateKeys(ks *keyspace.KeySpace, s state.State, stateKey pathaddr.StateKey, includeDescendantAliases bool) []pathaddr.StateKey {
	keys := stateKeysWithEquivalentAliases(ks, s, stateKey)
	// Subtree invalidation computes the finite pre-mutation alias roots. Keep
	// those roots in the mutation transaction even when their concrete path
	// entries are absent: root-origin and heap ownership facts live in separate
	// lanes and must be invalidated with the same alias set.
	if prefixes, ok := s.PathKeySubtreeInvalidationPrefixes(ks, stateKey.PathKey()); ok {
		keys = append(keys, stateKeysFromPathKeys(prefixes)...)
	}
	if !includeDescendantAliases {
		return dedupeStateKeys(keys)
	}
	prefixes, ok := s.PathKeyDescendantInvalidationPrefixes(ks, stateKey.PathKey())
	if !ok {
		return dedupeStateKeys(keys)
	}
	keys = append(keys, stateKeysFromPathKeys(prefixes.Descendants)...)
	keys = append(keys, stateKeysFromPathKeys(prefixes.Subtrees)...)
	return dedupeStateKeys(keys)
}

type pathStaticMemberInvalidationTarget struct {
	key             pathaddr.StateKey
	descendantsOnly bool
}

func pathStaticMemberInvalidationTargets(ks *keyspace.KeySpace, s state.State, stateKey pathaddr.StateKey, descendantsOnly bool) []pathStaticMemberInvalidationTarget {
	targetKeys := stateKeysWithEquivalentAliases(ks, s, stateKey)
	if prefixes, ok := s.PathKeySubtreeInvalidationPrefixes(ks, stateKey.PathKey()); ok {
		targetKeys = append(targetKeys, stateKeysFromPathKeys(prefixes)...)
		targetKeys = dedupeStateKeys(targetKeys)
	}
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
	prefixes, ok := s.PathKeyDescendantInvalidationPrefixes(ks, stateKey.PathKey())
	if !ok {
		return dedupePathStaticMemberInvalidationTargets(targets)
	}
	for _, prefix := range stateKeysFromPathKeys(prefixes.Descendants) {
		targets = append(targets, pathStaticMemberInvalidationTarget{
			key:             prefix,
			descendantsOnly: true,
		})
	}
	for _, prefix := range stateKeysFromPathKeys(prefixes.Subtrees) {
		targets = append(targets, pathStaticMemberInvalidationTarget{
			key:             prefix,
			descendantsOnly: false,
		})
	}
	return dedupePathStaticMemberInvalidationTargets(targets)
}

func stateKeysWithEquivalentAliases(ks *keyspace.KeySpace, s state.State, stateKey pathaddr.StateKey) []pathaddr.StateKey {
	if stateKey == "" {
		return nil
	}
	keys := append([]pathaddr.StateKey{stateKey}, s.EquivalentStateKeys(ks, stateKey)...)
	return dedupeStateKeys(keys)
}

func stateKeysFromPathKeys(in []pathdom.PathKey) []pathaddr.StateKey {
	if len(in) == 0 {
		return nil
	}
	out := make([]pathaddr.StateKey, 0, len(in))
	for _, key := range in {
		stateKey, ok := pathaddr.StateKeyFromPathKey(key)
		if !ok {
			continue
		}
		out = append(out, stateKey)
	}
	return out
}

func dedupeStateKeys(in []pathaddr.StateKey) []pathaddr.StateKey {
	if len(in) < 2 {
		return in
	}
	seen := make(map[pathaddr.StateKey]struct{}, len(in))
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
