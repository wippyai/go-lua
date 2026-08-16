package readexpr

import (
	valuerefine "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/check/internal/staticmemberwitness"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

func staticMemberLocalKey(config Config, point cfg.Point, owner pathdom.Path, member segment.Segment) (keyspace.Key, bool) {
	if config.Visibility == nil || owner.IsEmpty() {
		return keyspace.Key{}, false
	}
	ownerKey, ok := visibility.AddressAt(config.Visibility, point, owner).VisibleLocalKeyspaceKey()
	if !ok {
		return keyspace.Key{}, false
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return keyspace.Key{}, false
	}
	return ks.AppendSegment(ownerKey, member)
}

func readStaticMemberValue(config Config, pathKey pathdom.PathKey, in state.State) (product.Value, bool) {
	ks := config.Visibility.KeySpace()
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return product.Value{}, false
	}
	return readStaticMemberLocalValue(config, localKey, in)
}

func readStaticMemberLocalValue(config Config, localKey keyspace.Key, in state.State) (product.Value, bool) {
	if config.Visibility == nil {
		return product.Value{}, false
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return product.Value{}, false
	}
	value, ok := in.ReadLocalPathStaticMember(localKey)
	if ok {
		return value, true
	}
	canonical, ok := ks.FieldCanonical(localKey)
	if ok {
		if value, ok := in.ReadLocalPathStaticMember(canonical); ok {
			return value, true
		}
	}
	if unversioned, ok := unversionedStaticMemberKey(ks, localKey); ok {
		if value, ok := in.ReadLocalPathStaticMember(unversioned); ok {
			return value, true
		}
		if canonical, ok := ks.FieldCanonical(unversioned); ok {
			if value, ok := in.ReadLocalPathStaticMember(canonical); ok {
				return value, true
			}
		}
	}
	if stable, ok := stableStaticMemberKey(ks, localKey); ok {
		if value, ok := in.ReadLocalPathStaticMember(stable); ok {
			return value, true
		}
		if canonical, ok := ks.FieldCanonical(stable); ok {
			return in.ReadLocalPathStaticMember(canonical)
		}
	}
	return product.Value{}, false
}

func unversionedStaticMemberKey(ks *keyspace.KeySpace, localKey keyspace.Key) (keyspace.Key, bool) {
	if ks == nil || localKey.Kind != keyspace.KindResolverSym || localKey.Sym == 0 {
		return keyspace.Key{}, false
	}
	segments, ok := ks.SegmentsView(localKey)
	if !ok {
		return keyspace.Key{}, false
	}
	return ks.LookupResolverKey(localKey.Sym, 0, segments)
}

func stableStaticMemberKey(ks *keyspace.KeySpace, localKey keyspace.Key) (keyspace.Key, bool) {
	if ks == nil || localKey.Kind != keyspace.KindResolverSym || localKey.Sym == 0 {
		return keyspace.Key{}, false
	}
	segments, ok := ks.SegmentsView(localKey)
	if !ok {
		return keyspace.Key{}, false
	}
	return ks.FromStableSymbol(localKey.Sym, segments)
}

func mergeCurrentStaticMemberValue(reg *axis.Registry, typeValues *typevalue.Cache, fallback, current product.Value, fromPathKey bool) product.Value {
	if currentStaticMemberValueReplacesFallback(reg, typeValues, fallback, current) {
		return current
	}
	if fromPathKey && currentValueHasType(reg, typeValues, current) {
		return current
	}
	if merged := valuerefine.MeetConstraint(reg, fallback, current); !product.Equal(reg, merged, product.Bottom(reg)) {
		return merged
	}
	if fromPathKey {
		return current
	}
	return fallback
}

func overlayStaticMemberWitness(config Config, point cfg.Point, root pathdom.Path, in state.State, value product.Value) product.Value {
	reg := config.Registry
	if config.Visibility == nil || !sourcevalue.RuntimeMayBeTable(reg, value, true) {
		return value
	}
	rootKey, ok := visibility.AddressAt(config.Visibility, point, root).VisibleKeyspaceKey()
	if !ok {
		return value
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return value
	}
	selfIndexMember := false
	builder := staticmemberwitness.NewBuilder()
	poll := cancellation.NewPoller(config.Cancel, cancellation.EveryExpensive)
	in.ForEachPathStaticMember(func(memberKey keyspace.Key, memberValue product.Value) bool {
		if poll.Poll() {
			return false
		}
		memberSuffix, ok := ks.ExactRemainderAfterPrefix(memberKey, rootKey)
		if !ok || len(memberSuffix) == 0 {
			return true
		}
		if selfIndexStaticMemberSuffix(memberSuffix) && sameExactIdentity(reg, value, memberValue) {
			selfIndexMember = true
			return false
		}
		if product.Equal(reg, memberValue, product.Bottom(reg)) {
			return true
		}
		memberValue = currentStaticMemberValue(config, point, root, memberSuffix, memberKey, in, memberValue)
		memberType, ok := config.TypeValues.TypeOf(reg, memberValue)
		if !ok || memberType == nil {
			return true
		}
		builder.Add(memberSuffix, memberType)
		return true
	})
	if config.Cancel != nil && config.Cancel.Canceled() {
		return value
	}
	if selfIndexMember {
		return value
	}
	staticType, ok := builder.Build()
	if !ok {
		return value
	}
	if existing, ok := config.TypeValues.TypeOf(reg, value); ok && existing != nil && !typ.IsAny(existing) && !typ.IsUnknown(existing) && !typ.IsNever(existing) {
		if _, isMap := unwrap.Alias(existing).(*typ.Map); isMap {
			// A map's declared type is invariant under a conforming key write (a
			// non-conforming write is a separate assignment error), so the root
			// witness stays the declared map rather than intersecting with the
			// per-key static-member record. Individual key reads remain precise
			// through the static-member facts; preserving the map witness keeps a
			// covariant mutable-map alias from being admitted unsoundly.
			return value
		}
		if merged, ok := mergeStaticMemberWitness(existing, staticType); ok {
			staticType = merged
		} else {
			return value
		}
		if typ.SameNodeOrAcyclicEqual(existing, staticType) {
			return value
		}
	}
	return typevalue.WithWitness(reg, value, staticType)
}

func currentStaticMemberValue(
	config Config,
	point cfg.Point,
	root pathdom.Path,
	suffix []segment.Segment,
	memberKey keyspace.Key,
	in state.State,
	fallback product.Value,
) product.Value {
	if len(suffix) == 0 {
		return fallback
	}
	current, ok := currentLocalPathKeyValue(config, memberKey, in)
	fromPathKey := ok
	if !ok {
		currentPath := root.AppendSegments(suffix)
		current, ok = project(config, point, currentPath, in, false)
		if !ok || !projectedValueCarriesContent(config.Registry, current) {
			return fallback
		}
	}
	if currentStaticMemberValueReplacesFallback(config.Registry, config.TypeValues, fallback, current) {
		return current
	}
	return mergeCurrentStaticMemberValue(config.Registry, config.TypeValues, fallback, current, fromPathKey)
}

func currentStaticMemberValueReplacesFallback(reg *axis.Registry, typeValues *typevalue.Cache, fallback, current product.Value) bool {
	fallbackType, fallbackOK := typeValues.TypeOf(reg, fallback)
	currentType, currentOK := typeValues.TypeOf(reg, current)
	if !fallbackOK || !currentOK {
		return false
	}
	return emptyRecordType(fallbackType) && declaredContainerType(currentType)
}

func emptyRecordType(t typ.Type) bool {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || rec == nil {
		return false
	}
	return len(rec.Fields) == 0 &&
		len(rec.StaticMembers) == 0 &&
		rec.Metatable == nil &&
		rec.MapKey == nil &&
		rec.MapValue == nil &&
		!rec.Open
}

func declaredContainerType(t typ.Type) bool {
	switch unwrap.Alias(t).(type) {
	case *typ.Array, *typ.Tuple, *typ.Map, *typ.ReadonlyMap:
		return true
	default:
		return false
	}
}

func currentPathKeyValue(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	if config.Visibility == nil {
		return product.Value{}, false
	}
	pathKey, ok := visibility.AddressAt(config.Visibility, point, p).VisiblePathKey()
	if !ok {
		return product.Value{}, false
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return product.Value{}, false
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		return product.Value{}, false
	}
	return currentLocalPathKeyValue(config, localKey, in)
}

func currentLocalPathKeyValue(config Config, localKey keyspace.Key, in state.State) (product.Value, bool) {
	if config.Visibility == nil {
		return product.Value{}, false
	}
	ks := config.Visibility.KeySpace()
	if ks == nil {
		return product.Value{}, false
	}
	value := in.ReadLocalPathKey(config.Registry, localKey)
	if !projectedValueCarriesContent(config.Registry, value) {
		canonical, ok := ks.FieldCanonical(localKey)
		if !ok {
			return product.Value{}, false
		}
		value = in.ReadLocalPathKey(config.Registry, canonical)
		if !projectedValueCarriesContent(config.Registry, value) {
			return product.Value{}, false
		}
	}
	return value, true
}

func selfIndexStaticMemberSuffix(suffix []segment.Segment) bool {
	if len(suffix) == 0 {
		return false
	}
	last := suffix[len(suffix)-1]
	return (last.Kind == segment.SegmentField || last.Kind == segment.SegmentIndexString) && last.Name == "__index"
}

func sameExactIdentity(reg *axis.Registry, left product.Value, right product.Value) bool {
	leftID, leftOK := product.Get(reg, left, identity.Key).ID()
	rightID, rightOK := product.Get(reg, right, identity.Key).ID()
	return leftOK && rightOK && leftID == rightID
}

func mergeStaticMemberWitness(existing typ.Type, static typ.Type) (typ.Type, bool) {
	return typetable.OverlayRecordMembers(existing, static)
}

func projectFinalStaticMember(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	if len(p.Segments) == 0 {
		return product.Value{}, false
	}
	parent := p.ParentView()
	parentValue, hasParent := project(config, point, parent, in, false)
	if !sourcevalue.RuntimeMayBeTable(config.Registry, parentValue, hasParent) {
		return product.Value{}, false
	}
	member := p.Segments[len(p.Segments)-1]
	value, ok := ProjectStaticMember(config, point, parent, member, in)
	if !ok {
		return product.Value{}, false
	}
	if hasParent {
		value = inheritProjectedParentPresence(config.Registry, value, parentValue)
		value = mergeFinalStaticMemberWithCurrentParent(config, p, parentValue, value)
	}
	return value, true
}

func mergeFinalStaticMemberWithCurrentParent(config Config, p pathdom.Path, parentValue product.Value, value product.Value) product.Value {
	if len(p.Segments) == 0 {
		return value
	}
	current, ok := projectFromValueEvidence(config, parentValue, p.Segments[len(p.Segments)-1:])
	if !ok {
		return value
	}
	if product.LessOrEq(config.Registry, current, value) {
		return current
	}
	if product.LessOrEq(config.Registry, value, current) {
		return value
	}
	if merged := valuerefine.MeetConstraint(config.Registry, value, current); !product.Equal(config.Registry, merged, product.Bottom(config.Registry)) {
		return merged
	}
	return current
}
