package factapply

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ResolvedPathAddress is an immutable, point-resolved path authority. It owns
// the syntax suffix and the exact local key that the visibility resolver chose
// during Freeze; application never consults a resolver or a fact provider.
type ResolvedPathAddress struct {
	path               pathdom.Path
	owner              *keyspace.KeySpace
	local              keyspace.Key
	stateKey           pathaddr.StateKey
	rootOrVisible      pathaddr.StateKey
	rootOrVisibleLocal keyspace.Key
	structural         pathaddr.StateKey
	prefixes           []ResolvedPathAddress
	valid              bool
}

// LocalKey returns the exact resolver-versioned State address frozen for this
// path. The key remains owned by the resolver's KeySpace.
func (a ResolvedPathAddress) LocalKey() (keyspace.Key, bool) {
	if !a.valid || a.local.Kind != keyspace.KindResolverSym || a.local.Sym == 0 || a.local.Ver == 0 {
		return keyspace.Key{}, false
	}
	return a.local, true
}

// FreezeResolvedPathAddress resolves one path exactly once. The returned value
// is closed over structural keyspace identity and owns all variable storage.
func FreezeResolvedPathAddress(resolver *visibility.Resolver, point cfg.Point, path pathdom.Path) (ResolvedPathAddress, error) {
	if resolver == nil || resolver.KeySpace() == nil || path.IsEmpty() || path.Symbol == 0 {
		return ResolvedPathAddress{}, fmt.Errorf("factapply: resolved path address requires resolver and symbol path")
	}
	prefixes := make([]ResolvedPathAddress, len(path.Segments)+1)
	for count := range prefixes {
		prefixPath := path.RootOnly().AppendSegments(path.Segments[:count])
		prefixPath.Version = path.Version
		frozen, err := freezeResolvedPathAddressOne(resolver, point, prefixPath)
		if err != nil {
			return ResolvedPathAddress{}, err
		}
		prefixes[count] = frozen
	}
	for i := range prefixes {
		prefixes[i].prefixes = prefixes
	}
	return prefixes[len(prefixes)-1], nil
}

// FreezeBoundaryPathAddress closes a path over one certified structural root
// carried by a linked relation frame. Unlike a local address it has no SSA
// version: captures and globals are boundary storage, and manufacturing a
// resolver version would conflate the two namespaces.
func FreezeBoundaryPathAddress(ks *keyspace.KeySpace, root keyspace.Key, path pathdom.Path) (ResolvedPathAddress, error) {
	if ks == nil || !ks.Valid() || root.Kind != keyspace.KindUnversionedSym || root.Sym == 0 || root.Ver != 0 ||
		root.Segs != 0 || path.IsEmpty() || path.Symbol != root.Sym || path.Version != 0 {
		return ResolvedPathAddress{}, fmt.Errorf("factapply: structural boundary path requires its certified root")
	}
	prefixes := make([]ResolvedPathAddress, len(path.Segments)+1)
	for count := range prefixes {
		prefixPath := pathdom.Path{Symbol: path.Symbol, Segments: append([]segment.Segment(nil), path.Segments[:count]...)}
		local := ks.FromPath(prefixPath)
		raw, ok := pathaddr.StateKeyFromPathKey(ks.FormatReadOnly(local))
		if !ok {
			return ResolvedPathAddress{}, fmt.Errorf("factapply: structural boundary path has no State spelling")
		}
		prefixes[count] = ResolvedPathAddress{
			path: prefixPath, owner: ks, local: local, stateKey: raw,
			rootOrVisible: raw, rootOrVisibleLocal: local, structural: raw, valid: true,
		}
	}
	for index := range prefixes {
		prefixes[index].prefixes = prefixes
	}
	return prefixes[len(prefixes)-1], nil
}

func freezeResolvedPathAddressOne(resolver *visibility.Resolver, point cfg.Point, path pathdom.Path) (ResolvedPathAddress, error) {
	view := visibility.AddressAt(resolver, point, path)
	stateKey, ok := view.VisibleStateKey()
	if !ok {
		return ResolvedPathAddress{}, fmt.Errorf("factapply: path is not visible at freeze point")
	}
	local, ok := view.VisibleLocalKeyspaceKey()
	if !ok {
		return ResolvedPathAddress{}, fmt.Errorf("factapply: visible path is not a local structural key")
	}
	rootOrVisible, ok := view.RootOrVisibleStateKey()
	if !ok {
		return ResolvedPathAddress{}, fmt.Errorf("factapply: path has no root-or-visible state key")
	}
	rootOrVisibleLocal, ok := view.RootOrVisibleKeyspaceKey()
	if !ok {
		return ResolvedPathAddress{}, fmt.Errorf("factapply: path has no root-or-visible structural key")
	}
	structural, ok := view.StructuralStateKey()
	if !ok {
		return ResolvedPathAddress{}, fmt.Errorf("factapply: path has no structural state key")
	}
	owned := path
	owned.Segments = append([]segment.Segment(nil), path.Segments...)
	return ResolvedPathAddress{
		path: owned, owner: resolver.KeySpace(), local: local, stateKey: stateKey,
		rootOrVisible: rootOrVisible, rootOrVisibleLocal: rootOrVisibleLocal,
		structural: structural, valid: true,
	}, nil
}

func resolvedPathAddressFromStateKey(ks *keyspace.KeySpace, raw pathaddr.StateKey) (ResolvedPathAddress, bool) {
	if ks == nil || !ks.Valid() || raw == "" {
		return ResolvedPathAddress{}, false
	}
	sym, version, suffix, ok := pathaddr.ParseResolverPath(raw.PathKey())
	if !ok || sym == 0 || version <= 0 {
		return ResolvedPathAddress{}, false
	}
	segments, ok := segment.InternFormattedSegments(suffix)
	if !ok {
		return ResolvedPathAddress{}, false
	}
	prefixes := make([]ResolvedPathAddress, len(segments)+1)
	for count := range prefixes {
		local, ok := ks.LookupResolverKey(sym, version, segments[:count])
		if !ok {
			return ResolvedPathAddress{}, false
		}
		stateKey, ok := pathaddr.StateKeyFromPathKey(ks.FormatReadOnly(local))
		if !ok {
			return ResolvedPathAddress{}, false
		}
		rootOrVisible, rootOrVisibleLocal := stateKey, local
		if count == 0 {
			rootOrVisibleLocal, ok = ks.LookupResolverKey(sym, 0, nil)
			if !ok {
				return ResolvedPathAddress{}, false
			}
			rootOrVisible, ok = pathaddr.StateKeyFromPathKey(ks.FormatReadOnly(rootOrVisibleLocal))
			if !ok {
				return ResolvedPathAddress{}, false
			}
		}
		prefixes[count] = ResolvedPathAddress{
			path:  pathdom.Path{Symbol: sym, Version: version, Segments: append([]segment.Segment(nil), segments[:count]...)},
			owner: ks, local: local, stateKey: stateKey, rootOrVisible: rootOrVisible,
			rootOrVisibleLocal: rootOrVisibleLocal, structural: stateKey, valid: true,
		}
	}
	for i := range prefixes {
		prefixes[i].prefixes = prefixes
	}
	return prefixes[len(prefixes)-1], true
}

func (a ResolvedPathAddress) belongsTo(ks *keyspace.KeySpace) bool {
	if !a.valid || ks == nil || !ks.Valid() || a.owner != ks ||
		a.local.Sym != a.path.Symbol ||
		a.stateKey.PathKey() != ks.FormatReadOnly(a.local) {
		return false
	}
	if a.rootOrVisible == "" || a.structural == "" ||
		a.rootOrVisible.PathKey() != ks.FormatReadOnly(a.rootOrVisibleLocal) {
		return false
	}
	segments, ok := ks.SegmentsView(a.local)
	if !ok || len(segments) != len(a.path.Segments) {
		return false
	}
	for i := range segments {
		if segments[i] != a.path.Segments[i] {
			return false
		}
	}
	return true
}

// ResolvePathAddressValue applies the canonical nil-projector path lookup from
// a frozen address: local refinement, dynamic projection, heap projection, then
// variant-origin projection. Its recursion strictly shortens the finite suffix.
func ResolvePathAddressValue(reg *axis.Registry, ks *keyspace.KeySpace, st state.State, address ResolvedPathAddress) (product.Value, bool) {
	if reg == nil || !address.belongsTo(ks) {
		return product.Value{}, false
	}
	reader, ok := newConcreteResolvedPathValueReader(reg, ks, st)
	if !ok {
		return product.Value{}, false
	}
	return ResolvePathAddressFactorValue(reg, ks, reader, address)
}

func resolvedPrefix(ks *keyspace.KeySpace, address ResolvedPathAddress, count int) (ResolvedPathAddress, bool) {
	if !address.belongsTo(ks) || count < 0 || count > len(address.path.Segments) {
		return ResolvedPathAddress{}, false
	}
	if len(address.prefixes) != len(address.path.Segments)+1 {
		return ResolvedPathAddress{}, false
	}
	prefix := address.prefixes[count]
	return prefix, prefix.belongsTo(ks)
}

type resolvedPathInvalidator func(state.State, *keyspace.KeySpace, pathaddr.StateKey) (state.State, bool)

func invalidateResolvedPath(st state.State, ks *keyspace.KeySpace, address ResolvedPathAddress, invalidate resolvedPathInvalidator) (state.State, bool) {
	if !address.belongsTo(ks) || invalidate == nil {
		return st, false
	}
	out := st
	for _, target := range stateKeysWithEquivalentAliases(ks, st, address.stateKey) {
		var ok bool
		out, ok = invalidate(out, ks, target)
		if !ok {
			return st, false
		}
	}
	return out, true
}

func InvalidateResolvedPathSubtree(st state.State, ks *keyspace.KeySpace, address ResolvedPathAddress) (state.State, bool) {
	return invalidateResolvedPath(st, ks, address, invalidateStateKeySubtree)
}

func InvalidateResolvedPathDescendants(st state.State, ks *keyspace.KeySpace, address ResolvedPathAddress) (state.State, bool) {
	return invalidateResolvedPath(st, ks, address, invalidateStateKeyDescendants)
}

func InvalidateResolvedPathDescendantsPreservingDynamicMemberships(st state.State, ks *keyspace.KeySpace, address ResolvedPathAddress) (state.State, bool) {
	return invalidateResolvedPath(st, ks, address, invalidateStateKeyDescendantsPreservingDynamicValueKeyMemberships)
}

func InvalidateResolvedRootOrigins(reg *axis.Registry, ks *keyspace.KeySpace, st state.State, address ResolvedPathAddress, includeDescendantAliases bool) (state.State, bool) {
	if reg == nil || !address.belongsTo(ks) {
		return st, false
	}
	out := st
	for _, raw := range pathMutationStateKeys(ks, st, address.stateKey, includeDescendantAliases) {
		key, ok := ks.InternStateKey(raw)
		if !ok || key.Sym == 0 {
			continue
		}
		out = out.UpdateValue(reg, statekey.SymbolValue(key.Sym), func(value product.Value) product.Value {
			if product.Equal(reg, value, product.Bottom(reg)) {
				return value
			}
			return product.Set(reg, value, variantorigin.Key, variantorigin.Top())
		})
	}
	return out, true
}

func InvalidateResolvedRootStructuralWitness(reg *axis.Registry, ks *keyspace.KeySpace, st state.State, address ResolvedPathAddress) (state.State, bool) {
	if reg == nil || !address.belongsTo(ks) {
		return st, false
	}
	out := st
	for _, raw := range pathMutationStateKeys(ks, st, address.stateKey, true) {
		key, ok := ks.InternStateKey(raw)
		if !ok || key.Sym == 0 {
			continue
		}
		out = out.UpdateValue(reg, statekey.SymbolValue(key.Sym), func(value product.Value) product.Value {
			if product.Equal(reg, value, product.Bottom(reg)) {
				return value
			}
			return product.Set(reg, value, typewitness.Key, typewitness.Top())
		})
	}
	return out, true
}

func InvalidateResolvedHeapStaticMemberSubtree(reg *axis.Registry, ks *keyspace.KeySpace, st state.State, address ResolvedPathAddress) (state.State, bool) {
	return invalidateResolvedHeapStaticMembers(reg, ks, st, address, false)
}

func InvalidateResolvedHeapStaticMemberDescendants(reg *axis.Registry, ks *keyspace.KeySpace, st state.State, address ResolvedPathAddress) (state.State, bool) {
	return invalidateResolvedHeapStaticMembers(reg, ks, st, address, true)
}

func invalidateResolvedHeapStaticMembers(reg *axis.Registry, ks *keyspace.KeySpace, st state.State, address ResolvedPathAddress, descendantsOnly bool) (state.State, bool) {
	if reg == nil || !address.belongsTo(ks) {
		return st, false
	}
	out := st
	for _, target := range pathStaticMemberInvalidationTargets(ks, st, address.stateKey, descendantsOnly) {
		targetAddress, ok := resolvedPathAddressFromStateKey(ks, target.key)
		if !ok {
			continue
		}
		segs := targetAddress.path.Segments
		out = invalidateHeapTableFactsForOwner(reg, out, ks, st.ReadValue(reg, statekey.SymbolValue(targetAddress.path.Symbol)), segs, target.descendantsOnly)
		for split := 1; split <= len(segs); split++ {
			prefix, ok := resolvedPrefix(ks, targetAddress, split)
			if !ok {
				continue
			}
			owner, ok := ResolvePathAddressValue(reg, ks, st, prefix)
			if !ok {
				continue
			}
			out = invalidateHeapTableFactsForOwner(reg, out, ks, owner, segs[split:], target.descendantsOnly)
		}
	}
	return out, true
}

// ResolvedPathDescendantInvalidation is the closed structural input to the
// canonical descendant-invalidation kernel. Precise is optional; when present
// it is the exact static member derived from a dynamic key before application.
type ResolvedPathDescendantInvalidation struct {
	data *resolvedPathDescendantInvalidationData
}

type resolvedPathDescendantInvalidationData struct {
	Container              ResolvedPathAddress
	Precise                ResolvedPathAddress
	HasPrecise             bool
	ClearStructuralWitness bool
}

// ApplyResolvedPathDescendantInvalidation performs the authoritative path and
// heap invalidation transaction without consulting visibility, Facts, or a
// source provider. Invalid input publishes nothing.
func ApplyResolvedPathDescendantInvalidation(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	out state.State,
	request ResolvedPathDescendantInvalidation,
) (state.State, bool) {
	resolved := request.data
	if reg == nil || resolved == nil || !resolved.Container.belongsTo(ks) ||
		(resolved.HasPrecise && !resolved.Precise.belongsTo(ks)) {
		return out, false
	}
	invalidated := out
	if resolved.HasPrecise {
		if next, ok := InvalidateResolvedRootOrigins(reg, ks, invalidated, resolved.Precise, true); ok {
			invalidated = next
		}
		if next, ok := InvalidateResolvedRootStructuralWitness(reg, ks, invalidated, resolved.Precise); ok {
			invalidated = next
		}
		if next, ok := InvalidateResolvedHeapStaticMemberSubtree(reg, ks, invalidated, resolved.Precise); ok {
			invalidated = next
		}
		if next, ok := InvalidateResolvedPathSubtree(invalidated, ks, resolved.Precise); ok {
			invalidated = next
		}
	}

	var preserved product.Value
	hasPreserved := false
	if !resolved.ClearStructuralWitness {
		preserved, hasPreserved = invalidated.ReadLocalPathStaticMember(resolved.Container.local)
	}
	if next, ok := InvalidateResolvedRootOrigins(reg, ks, invalidated, resolved.Container, true); ok {
		invalidated = next
	}
	if resolved.ClearStructuralWitness {
		if next, ok := InvalidateResolvedRootStructuralWitness(reg, ks, invalidated, resolved.Container); ok {
			invalidated = next
		}
	}
	if next, ok := InvalidateResolvedHeapStaticMemberDescendants(reg, ks, invalidated, resolved.Container); ok {
		invalidated = next
	}
	if resolved.ClearStructuralWitness {
		if next, ok := InvalidateResolvedPathDescendants(invalidated, ks, resolved.Container); ok {
			invalidated = next
		}
	} else if next, ok := InvalidateResolvedPathDescendantsPreservingDynamicMemberships(invalidated, ks, resolved.Container); ok {
		invalidated = next
	}

	invalidated = invalidated.ClearDiffConstraintsFor(resolved.Container.rootOrVisible)
	if resolved.ClearStructuralWitness {
		invalidated = invalidated.ClearDynamicIndexValueKeyMembershipsForContainer(resolved.Container.rootOrVisibleLocal)
		invalidated = invalidated.ClearKeyMembershipsForPath(resolved.Container.rootOrVisible)
	}
	if hasPreserved {
		invalidated = invalidated.WriteLocalPathStaticMember(resolved.Container.local, preserved)
	}
	return invalidated, true
}
