package readexpr

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func projectFromHeapIdentity(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	reg := config.Registry
	last := p.Segments[len(p.Segments)-1]
	parent := p.ParentView()
	parentProjected := product.Value{}
	hasParentProjected := false
	parentValue, _ := project(config, point, parent, in, false)
	if projected, ok := sourcevalue.HeapMemberFromValue(reg, config.Visibility.KeySpace(), in, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		parentProjected = projected
		hasParentProjected = true
	} else if projected, ok := projectHeapMemberFromRootWitness(config, in, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		parentProjected = projected
		hasParentProjected = true
	} else if projected, ok := projectHeapDynamicMember(config, parentValue, last, in); ok {
		parentProjected = projected
		hasParentProjected = true
	}

	root := p.RootOnly()
	rootProjected := product.Value{}
	hasRootProjected := false
	if rootValue, ok := sourcevalue.ReadPathValue(reg, config.Visibility, point, root, in); ok {
		if projected, ok := sourcevalue.HeapMemberFromValue(reg, config.Visibility.KeySpace(), in, rootValue, p.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		} else if projected, ok := projectHeapMemberFromRootWitness(config, in, rootValue, p.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		} else if projected, ok := projectHeapDynamicDescendant(config, rootValue, p.Segments, in); ok {
			rootProjected = projected
			hasRootProjected = true
		}
	}
	if hasParentProjected {
		return parentProjected, true
	}
	if hasRootProjected {
		return rootProjected, true
	}
	return product.Value{}, false
}

func projectHeapMemberFromRootWitness(config Config, in state.State, value product.Value, suffix []segment.Segment) (product.Value, bool) {
	reg := config.Registry
	if reg == nil || len(suffix) == 0 {
		return product.Value{}, false
	}
	id, ok := identityvalue.ExactID(reg, value)
	if !ok {
		return product.Value{}, false
	}
	object := in.ReadHeapTableObject(reg, id)
	root := object.Root()
	rootID, ok := identityvalue.ExactID(reg, root)
	if !ok || rootID != id {
		return product.Value{}, false
	}
	if product.Equal(reg, product.Meet(reg, root, value), product.Bottom(reg)) {
		return product.Value{}, false
	}
	projected, ok := projectFromValueEvidence(config, root, suffix)
	if !ok {
		return product.Value{}, false
	}
	if ownerPresence := product.PresenceOf(value); !presence.Equal(ownerPresence, presence.Present()) {
		projected = product.WithPresence(reg, projected, presence.Join(product.PresenceOf(projected), ownerPresence))
	}
	return projected, true
}

func projectHeapDynamicDescendant(config Config, root product.Value, suffix []segment.Segment, in state.State) (product.Value, bool) {
	if len(suffix) == 0 {
		return product.Value{}, false
	}
	parent, ok := sourcevalue.HeapMemberFromValue(config.Registry, config.Visibility.KeySpace(), in, root, suffix[:len(suffix)-1])
	if !ok {
		return product.Value{}, false
	}
	return projectHeapDynamicMember(config, parent, suffix[len(suffix)-1], in)
}

func projectHeapDynamicMember(config Config, parent product.Value, last segment.Segment, in state.State) (product.Value, bool) {
	reg := config.Registry
	if reg == nil || config.Visibility == nil || config.TypeValues == nil {
		return product.Value{}, false
	}
	id, ok := identityvalue.ExactID(reg, parent)
	if !ok {
		return product.Value{}, false
	}
	object := in.ReadHeapTableObject(reg, id)
	domain := product.Domain(reg)
	joined := domain.Bottom()
	found := false
	maybeMissing := false
	for _, fact := range object.DynamicIndexFacts() {
		if fact.Admission == dynamicindex.AdmissionRejected || domain.Equal(fact.Value, domain.Bottom()) {
			continue
		}
		if dynamicIndexFactDefinitelyMatchesSegment(reg, config.TypeValues, fact, last) {
			if !found {
				joined = fact.Value
				found = true
			} else {
				joined = domain.Join(joined, fact.Value)
			}
			continue
		}
		if dynamicIndexFactMayMatchSegment(reg, config.TypeValues, fact, last) {
			maybeMissing = true
			if !found {
				joined = fact.Value
				found = true
			} else {
				joined = domain.Join(joined, fact.Value)
			}
		}
	}
	if !found {
		return product.Value{}, false
	}
	if maybeMissing {
		joined = product.Join(reg, joined, product.Absent(reg))
	}
	return joined, true
}

func projectFromVariantOrigin(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	reg := config.Registry
	if reg == nil || len(p.Segments) == 0 {
		return product.Value{}, false
	}
	rootValue, ok := project(config, point, p.RootOnly(), in, false)
	if !ok {
		return product.Value{}, false
	}
	origin := product.Get(reg, rootValue, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return product.Value{}, false
	}
	family, cases, ok := variant.ProjectOrigin(origin.Family(), origin.CasesRef(), p.Segments)
	projectedOrigin := variantorigin.Value{}
	hasProjectedOrigin := ok
	if hasProjectedOrigin {
		projectedOrigin = variantorigin.Of(family, cases)
	}
	if rootType, ok := config.TypeValues.TypeFromVariantOrigin(origin.Family(), origin.CasesRef()); ok {
		if value, ok := projectTypeThroughPath(config, p, rootValue, rootType, projectedOrigin, hasProjectedOrigin); ok {
			return refineProjectionWithRootType(config, p, rootValue, value), true
		}
	}
	if rootType, ok := typevalue.StructuralTypeOf(reg, config.TypeValues, rootValue, typevalue.StructuralTypeOptions{
		ApplyPresence:     true,
		OptionalWhenMaybe: true,
	}); ok {
		if value, ok := projectTypeThroughPath(config, p, rootValue, rootType, projectedOrigin, hasProjectedOrigin); ok {
			return refineProjectionWithRootType(config, p, rootValue, value), true
		}
	}
	if !hasProjectedOrigin {
		return product.Value{}, false
	}
	if t, ok := config.TypeValues.TypeFromVariantOrigin(family, cases); ok {
		value := projectedPathValue(reg, config.TypeValues, t)
		value = product.Set(reg, value, variantorigin.Key, projectedOrigin)
		value = inheritProjectedParentPresence(reg, value, rootValue)
		if projectedValueCarriesContent(reg, value) {
			return refineProjectionWithRootType(config, p, rootValue, value), true
		}
	}
	value := product.Set(reg, product.Top(), variantorigin.Key, projectedOrigin)
	return refineProjectionWithRootType(config, p, rootValue, inheritProjectedParentPresence(reg, value, rootValue)), true
}

func projectTypeThroughPath(
	config Config,
	p pathdom.Path,
	rootValue product.Value,
	rootType typ.Type,
	projectedOrigin variantorigin.Value,
	hasProjectedOrigin bool,
) (product.Value, bool) {
	projectedType, ok := luatypeprojection.ApplySegments(rootType, p.Segments)
	if !ok {
		return product.Value{}, false
	}
	value := projectedPathValue(config.Registry, config.TypeValues, projectedType)
	if hasProjectedOrigin {
		value = product.Set(config.Registry, value, variantorigin.Key, projectedOrigin)
	}
	value = inheritProjectedParentPresence(config.Registry, value, rootValue)
	if !projectedValueCarriesContent(config.Registry, value) {
		return product.Value{}, false
	}
	return value, true
}

func refineProjectionWithCurrentRootType(
	config Config,
	point cfg.Point,
	p pathdom.Path,
	in state.State,
	projected product.Value,
) product.Value {
	if len(p.Segments) == 0 {
		return projected
	}
	rootValue, ok := project(config, point, p.RootOnly(), in, false)
	if !ok {
		return projected
	}
	return refineProjectionWithRootType(config, p, rootValue, projected)
}

func refineProjectionWithRootType(config Config, p pathdom.Path, rootValue, projected product.Value) product.Value {
	rootType, ok := config.TypeValues.TypeOf(config.Registry, rootValue)
	if !ok || rootType == nil {
		return projected
	}
	rootProjected, ok := projectTypeThroughPath(config, p, rootValue, rootType, variantorigin.Value{}, false)
	if !ok {
		return projected
	}
	if concreteProjectionShouldIgnoreUntrustedRootProjection(config.Registry, projected, rootProjected) {
		return projected
	}
	refined := valuerefine.MeetConstraint(config.Registry, projected, rootProjected)
	if !product.Equal(config.Registry, refined, product.Bottom(config.Registry)) {
		return refined
	}
	return projected
}

func concreteProjectionShouldIgnoreUntrustedRootProjection(reg *axis.Registry, projected, rootProjected product.Value) bool {
	if !exactHasConcreteNonTopProof(reg, projected) || exactHasConcreteNonTopProof(reg, rootProjected) {
		return false
	}
	ev := product.Get(reg, rootProjected, evidence.Key)
	return ev.IsExplicitTop() || ev.IsGradualTop()
}

func projectFromStructuralEvidence(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool, bool) {
	reg := config.Registry
	root := p.RootOnly()
	rootProjected := product.Value{}
	hasRootProjected := false
	if rootValue, ok := sourcevalue.ReadPathValue(reg, config.Visibility, point, root, in); ok {
		if projected, ok := projectFromValueEvidence(config, rootValue, p.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		}
	}

	parent := p.ParentView()
	parentValue, hasParent := project(config, point, parent, in, false)
	if !sourcevalue.RuntimeMayBeTable(reg, parentValue, hasParent) {
		return product.Value{}, false, true
	}
	if projected, ok := projectFromValueEvidence(config, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		// The parent-relative projection observes per-segment narrowing recorded
		// on the intermediate path (e.g. a truthy guard that removed nil from an
		// optional field), so it is at least as precise as a single root-relative
		// projection across the full suffix. Meeting them keeps a narrowed
		// non-optional result instead of re-widening it with the root's optional.
		if hasRootProjected {
			if merged := valuerefine.MeetConstraint(reg, rootProjected, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
				return merged, true, false
			}
		}
		return projected, true, false
	}
	if nilValue, ok := projectMissingFinalSegmentAsNil(config, in, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		return nilValue, true, false
	}

	if hasRootProjected {
		return rootProjected, true, false
	}

	return product.Value{}, false, false
}

func projectFromValueEvidence(config Config, value product.Value, suffix []segment.Segment) (product.Value, bool) {
	reg := config.Registry
	if len(suffix) == 0 {
		return product.Value{}, false
	}
	parentType, ok := typevalue.StructuralTypeOf(reg, config.TypeValues, value, typevalue.StructuralTypeOptions{
		ApplyPresence:     true,
		OptionalWhenMaybe: true,
	})
	if !ok {
		return product.Value{}, false
	}
	projected, ok := luatypeprojection.ApplySegments(parentType, suffix)
	if !ok {
		return product.Value{}, false
	}
	projectedValue := projectedPathValue(reg, config.TypeValues, projected)
	return inheritProjectedParentPresence(reg, projectedValue, value), true
}

func inheritProjectedParentPresence(reg *axis.Registry, projected, parent product.Value) product.Value {
	parentPresence := product.PresenceOf(parent)
	if presence.Equal(parentPresence, presence.Present()) {
		return projected
	}
	return product.WithPresence(reg, projected, presence.Join(product.PresenceOf(projected), parentPresence))
}

func projectedPathValue(reg *axis.Registry, typeValues *typevalue.Cache, t typ.Type) product.Value {
	value := typeValues.FromTypeWithWitness(reg, t)
	if t != nil && !typevalue.ProjectionHasNil(t) {
		value = product.WithPresence(reg, value, presence.Present())
	}
	return value
}

func projectMissingFinalSegmentAsNil(config Config, in state.State, value product.Value, suffix []segment.Segment) (product.Value, bool) {
	if len(suffix) != 1 {
		return product.Value{}, false
	}
	id, ok := identityvalue.ExactID(config.Registry, value)
	if !ok || !localExclusivePlacementProvesMissingSlotNil(in.ReadPlacement(id)) {
		return product.Value{}, false
	}
	parentType, ok := typevalue.StructuralTypeOf(config.Registry, config.TypeValues, value, typevalue.StructuralTypeOptions{
		ApplyPresence:     true,
		OptionalWhenMaybe: true,
	})
	if !ok || parentType == nil || typ.IsAny(parentType) || typ.IsUnknown(parentType) || typ.IsNever(parentType) {
		return product.Value{}, false
	}
	_, ok = luatypeprojection.ApplySegments(parentType, suffix)
	if ok || !access.MissingFieldReadsNil(parentType) {
		return product.Value{}, false
	}
	return inheritProjectedParentPresence(config.Registry, typevalue.Nil(config.Registry), value), true
}

func localExclusivePlacementProvesMissingSlotNil(place placement.Value) bool {
	return place == placement.Stack || place == placement.OwnedHeap
}
