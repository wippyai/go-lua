package sourcevalue

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ReadPathValue reads the path-visible state value for p.
func ReadPathValue(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	p pathdom.Path,
	in state.State,
) (product.Value, bool) {
	if p.IsEmpty() {
		return product.Value{}, false
	}
	if len(p.Segments) == 0 {
		if p.Symbol == 0 {
			return product.Value{}, false
		}
		value := in.ReadValue(reg, key.SymbolValue(p.Symbol))
		if product.Equal(reg, value, product.Bottom(reg)) {
			return product.Value{}, false
		}
		return refineRootValueThroughEquivalentPaths(reg, resolver, point, p, in, value), true
	}
	return ExactPathValue(reg, resolver, point, p, in)
}

func refineRootValueThroughEquivalentPaths(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	p pathdom.Path,
	in state.State,
	value product.Value,
) product.Value {
	if resolver == nil || p.Symbol == 0 {
		return value
	}
	stateKey, ok := resolver.StateKeyAt(point, p)
	if !ok {
		return value
	}
	ks := resolver.KeySpace()
	domain := product.Domain(reg)
	for _, equivalent := range in.EquivalentPathKeys(ks, stateKey.PathKey()) {
		equivalentKey, ok := ks.FromStateKey(equivalent)
		if !ok || equivalentKey.Segs != 0 {
			continue
		}
		if equivalentValue, ok := equivalentRootSymbolValue(reg, resolver, point, equivalentKey, in); ok {
			if meet := domain.Meet(value, equivalentValue); !domain.Equal(meet, domain.Bottom()) {
				value = meet
			}
		}
	}
	return value
}

func equivalentRootSymbolValue(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	root keyspace.Key,
	in state.State,
) (product.Value, bool) {
	if root.Kind != keyspace.KindResolverSym || root.Sym == 0 || root.Segs != 0 {
		return product.Value{}, false
	}
	currentKey, ok := resolver.StateKeyAt(point, pathdom.Path{Symbol: root.Sym})
	if !ok || currentKey.PathKey() != resolver.KeySpace().Format(root) {
		return product.Value{}, false
	}
	value := in.ReadValue(reg, key.SymbolValue(root.Sym))
	if product.Equal(reg, value, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return value, true
}

// ExactPathValue reads the exact visibility-scoped path value for p.
func ExactPathValue(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	p pathdom.Path,
	in state.State,
) (product.Value, bool) {
	if resolver == nil {
		return product.Value{}, false
	}
	pathKey := resolver.StructKeyAt(point, p)
	if pathKey.Kind == keyspace.KindInvalid {
		return product.Value{}, false
	}
	ks := resolver.KeySpace()
	value, ok := readLocalPathKeyWithFieldCanonicalAlias(reg, ks, in, pathKey)
	if !ok {
		return product.Value{}, false
	}
	return value, true
}

// HeapMemberFromValue reads a static heap-table member from a table identity
// value.
func HeapMemberFromValue(reg *axis.Registry, ks *keyspace.KeySpace, in state.State, value product.Value, suffix []segment.Segment) (product.Value, bool) {
	ownerPresence := product.PresenceOf(value)
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return product.Value{}, false
	}
	key, ok := heapidentity.StaticMemberSuffixKey(ks, suffix)
	if !ok {
		return product.Value{}, false
	}
	object := in.ReadHeapTableObject(reg, id)
	root := object.Root()
	rootID, ok := product.Get(reg, root, identity.Key).ID()
	if !ok || rootID != id {
		return product.Value{}, false
	}
	if product.Equal(reg, product.Meet(reg, root, value), product.Bottom(reg)) {
		return product.Value{}, false
	}
	member, ok := readStaticMemberWithFieldCanonicalAlias(ks, object, key, suffix)
	if !ok || product.Equal(reg, member, product.Bottom(reg)) {
		return product.Value{}, false
	}
	if !presence.Equal(ownerPresence, presence.Present()) {
		member = product.WithPresence(reg, member, presence.Join(product.PresenceOf(member), ownerPresence))
	}
	return member, true
}

func readLocalPathKeyWithFieldCanonicalAlias(reg *axis.Registry, ks *keyspace.KeySpace, in state.State, pathKey keyspace.Key) (product.Value, bool) {
	value := in.ReadLocalPathKey(reg, pathKey)
	if !product.Equal(reg, value, product.Bottom(reg)) {
		return value, true
	}
	canonical, ok := ks.FieldCanonical(pathKey)
	if !ok {
		return product.Value{}, false
	}
	value = in.ReadLocalPathKey(reg, canonical)
	if product.Equal(reg, value, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return value, true
}

func readStaticMemberWithFieldCanonicalAlias(ks *keyspace.KeySpace, object heapidentity.TableObject, key keyspace.Key, suffix []segment.Segment) (product.Value, bool) {
	if member, ok := object.StaticMember(key); ok {
		return member, true
	}
	canonical, ok := heapidentity.FieldCanonicalStaticMemberSuffixKey(ks, suffix)
	if !ok {
		return product.Value{}, false
	}
	return object.StaticMember(canonical)
}

// RuntimeMayBeTable reports whether value could still be a table-like value.
func RuntimeMayBeTable(reg *axis.Registry, value product.Value, hasValue bool) bool {
	if !hasValue {
		return true
	}
	if presence.Equal(product.PresenceOf(value), presence.Absent()) {
		return false
	}
	kinds := product.Get(reg, value, runtimekind.Key)
	if kinds.IsBottom() || kinds.IsTop() {
		return true
	}
	return kinds.Contains(runtimekind.Table)
}

// WithoutNilRuntimeKind removes nil from a runtime-kind lane when present.
func WithoutNilRuntimeKind(reg *axis.Registry, value product.Value) product.Value {
	kinds := product.Get(reg, value, runtimekind.Key)
	if !kinds.Contains(runtimekind.Nil) {
		return value
	}
	return product.Set(reg, value, runtimekind.Key, kinds.Without(runtimekind.Nil))
}

// HasExactIdentity reports whether value carries an exact identity lane.
func HasExactIdentity(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return false
	}
	_, ok := product.Get(reg, value, identity.Key).ID()
	return ok
}

// InheritTopOriginEvidence copies explicit or gradual top evidence from parent.
func InheritTopOriginEvidence(reg *axis.Registry, value, parent product.Value) product.Value {
	parentEvidence := product.Get(reg, parent, evidence.Key)
	if parentEvidence.IsGradualTop() || parentEvidence.IsExplicitTop() {
		return product.Set(reg, value, evidence.Key, parentEvidence)
	}
	return value
}
