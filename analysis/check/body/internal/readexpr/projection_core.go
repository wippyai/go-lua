package readexpr

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/access"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func project(config Config, point cfg.Point, p pathdom.Path, in state.State, overlayRoot bool) (product.Value, bool) {
	if config.Cancel != nil && config.Cancel.Canceled() {
		return product.Value{}, false
	}
	reg := config.Registry
	if reg == nil {
		panic("readexpr: Config.Registry is required")
	}
	if p.IsEmpty() {
		return product.Value{}, false
	}
	pathID, ok := projectionPathKey(config, p)
	if !ok {
		return product.Value{}, false
	}
	memoKey := projectionMemoKey{point: point, path: pathID, overlayRoot: overlayRoot}
	if result, cached := config.memo.lookup(memoKey); cached {
		return result.value, result.ok
	}
	frame := projectionFrame{point: point, path: pathID}
	if config.active.contains(frame) {
		return product.Value{}, false
	}
	config.active.push(frame)
	defer config.active.pop(frame)

	if len(p.Segments) == 0 {
		value, ok := sourcevalue.ReadPathValue(reg, config.Visibility, point, p, in)
		if !ok {
			return rememberProjection(config, memoKey, product.Value{}, false)
		}
		if !overlayRoot {
			return rememberProjection(config, memoKey, value, true)
		}
		return rememberProjection(config, memoKey, overlayStaticMemberWitness(config, point, p, in, value), true)
	}
	if p.Segments[len(p.Segments)-1].Kind == segment.SegmentIndexInt {
		value, ok := staticIntegerIndexValue(config, point, p, in)
		return rememberProjection(config, memoKey, value, ok)
	}

	exactPresent := product.Value{}
	hasExactPresent := false
	if exact, ok := exactPathValue(config, point, p, in); ok {
		switch gotPresence := product.PresenceOf(exact); {
		case presence.Equal(gotPresence, presence.Present()):
			exactPresent = sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, exact, presence.Present()))
			hasExactPresent = true
			originProjected, hasOriginProjected := projectCurrentVariantOrigin(config, point, p, in)
			if identityvalue.HasExact(reg, exactPresent) {
				if hasOriginProjected {
					return rememberProjection(config, memoKey, mergeOriginProjectedWithExact(reg, originProjected, exactPresent, true), true)
				}
				if projected, ok, _ := projectFromStructuralEvidence(config, point, p, in); ok {
					if merged := product.Meet(reg, projected, exactPresent); !product.Equal(reg, merged, product.Bottom(reg)) {
						return rememberProjection(config, memoKey, merged, true)
					}
				}
				if parentValue, hasParent := project(config, point, p.ParentView(), in, false); hasParent {
					exactPresent = inheritParentTopOriginForExact(reg, exactPresent, parentValue)
				}
				return rememberProjection(config, memoKey, exactPresent, true)
			}
		case presence.Equal(gotPresence, presence.Absent()):
			if projected, ok := projectDynamicOrHeapMember(config, point, p, in, product.Value{}, false); ok {
				return rememberProjection(config, memoKey, projected, true)
			}
			return rememberProjection(config, memoKey, product.Absent(reg), true)
		}
	}
	exactPresentOnlyPresence := hasExactPresent && exactValueOnlyProvesPresence(reg, exactPresent)
	if !hasExactPresent || exactPresentOnlyPresence {
		if projected, ok := projectDynamicOrHeapMember(config, point, p, in, exactPresent, hasExactPresent); ok {
			return projected, true
		}
	}

	originProjected := product.Value{}
	hasOriginProjected := false
	if projected, ok := projectCurrentVariantOrigin(config, point, p, in); ok {
		originProjected = projected
		hasOriginProjected = true
	}

	if hasExactPresent {
		if parentValue, hasParent := project(config, point, p.ParentView(), in, false); hasParent {
			exactPresent = inheritParentTopOriginForExact(reg, exactPresent, parentValue)
		}
	}

	if projected, ok := projectFinalStaticMember(config, point, p, in); ok {
		if hasOriginProjected {
			projected = mergeStructuralAndOriginProjection(reg, projected, originProjected)
		}
		if hasExactPresent {
			if hasOriginProjected {
				return rememberProjection(config, memoKey, mergeOriginProjectedWithExact(reg, projected, exactPresent, true), true)
			}
			return rememberProjection(config, memoKey, mergeProjectedWithExact(reg, projected, exactPresent, true), true)
		}
		return rememberProjection(config, memoKey, projected, true)
	}

	if projected, ok, blocked := projectFromStructuralEvidence(config, point, p, in); ok {
		projected = overlayStaticMemberWitness(config, point, p, in, projected)
		if hasOriginProjected {
			projected = mergeStructuralAndOriginProjection(reg, projected, originProjected)
		}
		if hasExactPresent {
			if hasOriginProjected {
				return rememberProjection(config, memoKey, mergeOriginProjectedWithExact(reg, projected, exactPresent, true), true)
			}
			return rememberProjection(config, memoKey, mergeProjectedWithExact(reg, projected, exactPresent, true), true)
		}
		return rememberProjection(config, memoKey, projected, true)
	} else if blocked && !hasExactPresent {
		return rememberProjection(config, memoKey, product.Value{}, false)
	}

	if hasExactPresent {
		return rememberProjection(config, memoKey, exactPresent, true)
	}
	if hasOriginProjected {
		return rememberProjection(config, memoKey, mergeOriginProjectedWithExact(reg, originProjected, exactPresent, hasExactPresent), true)
	}

	value, hasUnknownIndexValue := unknownIndexReadValue(config, p.Segments[len(p.Segments)-1])
	if !hasUnknownIndexValue {
		return rememberProjection(config, memoKey, product.Value{}, false)
	}
	if parentValue, hasParent := project(config, point, p.ParentView(), in, false); hasParent {
		value = sourcevalue.InheritTopOriginEvidence(reg, value, parentValue)
	}
	return rememberProjection(config, memoKey, value, true)
}

func projectCurrentVariantOrigin(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	projected, ok := projectFromVariantOrigin(config, point, p, in)
	if !ok {
		return product.Value{}, false
	}
	return refineProjectionWithCurrentRootType(config, point, p, in, projected), true
}

func exactPathValue(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	if exact, ok := sourcevalue.ExactPathValue(config.Registry, config.Visibility, point, p, in); ok {
		return exact, true
	}
	if config.ProofVisibility == nil || config.ProofVisibility == config.Visibility {
		return product.Value{}, false
	}
	proofState := in
	if config.ProofState != nil {
		if st, ok := config.ProofState(point); ok {
			proofState = st
		}
	}
	return sourcevalue.ExactPathValue(config.Registry, config.ProofVisibility, point, p, proofState)
}

func inheritParentTopOriginForExact(reg *axis.Registry, exact, parent product.Value) product.Value {
	if exactHasConcreteNonTopProof(reg, exact) {
		return exact
	}
	return sourcevalue.InheritTopOriginEvidence(reg, exact, parent)
}

func exactHasConcreteNonTopProof(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t)
}

func currentValueHasType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) bool {
	var t typ.Type
	var ok bool
	if typeValues == nil {
		t, ok = typevalue.TypeOf(reg, value)
	} else {
		t, ok = typeValues.TypeOf(reg, value)
	}
	return ok && !weakCallablePlaceholderType(t)
}

func weakCallablePlaceholderType(t typ.Type) bool {
	fn, ok := unwrap.Alias(t).(*typ.Function)
	if !ok || fn == nil {
		return false
	}
	if len(fn.TypeParams) != 0 || len(fn.Params) != 0 || len(fn.Returns) != 1 {
		return false
	}
	return typ.IsAny(fn.Variadic) && typ.IsAny(fn.Returns[0])
}

func rememberProjection(config Config, key projectionMemoKey, value product.Value, ok bool) (product.Value, bool) {
	if config.memo != nil {
		config.memo.remember(key, projectionResult{value: value, ok: ok})
	}
	return value, ok
}

func projectDynamicOrHeapMember(
	config Config,
	point cfg.Point,
	p pathdom.Path,
	in state.State,
	exact product.Value,
	hasExact bool,
) (product.Value, bool) {
	reg := config.Registry
	if dynamicProjected, ok := projectFromDynamicIndexFacts(config, point, p, in); ok {
		dynamicProjected = refineProjectionWithCurrentRootType(config, point, p, in, dynamicProjected)
		if value, ok := strongProjectedValueOrFallback(reg, dynamicProjected, exact, hasExact); ok {
			return value, true
		}
	}
	if heapProjected, ok := projectFromHeapIdentity(config, point, p, in); ok {
		heapProjected = refineProjectionWithCurrentRootType(config, point, p, in, heapProjected)
		if value, ok := strongProjectedValueOrFallback(reg, heapProjected, exact, hasExact); ok {
			return value, true
		}
	}
	return product.Value{}, false
}

func mergeStructuralAndOriginProjection(reg *axis.Registry, structural, origin product.Value) product.Value {
	switch {
	case product.LessOrEq(reg, origin, structural):
		return origin
	case product.LessOrEq(reg, structural, origin):
		return structural
	}
	if merged := valuerefine.MeetConstraint(reg, structural, origin); !product.Equal(reg, merged, product.Bottom(reg)) {
		return merged
	}
	return structural
}

func strongProjectedValueOrFallback(reg *axis.Registry, projected, exact product.Value, hasExact bool) (product.Value, bool) {
	value := mergeProjectedWithExact(reg, projected, exact, hasExact)
	if !projectedValueCarriesContent(reg, value) {
		return product.Value{}, false
	}
	return value, true
}

func mergeProjectedWithExact(reg *axis.Registry, projected, exact product.Value, hasExact bool) product.Value {
	if !hasExact {
		return projected
	}
	if merged := valuerefine.MeetConstraint(reg, exact, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
		return merged
	}
	return exact
}

func mergeOriginProjectedWithExact(reg *axis.Registry, projected, exact product.Value, hasExact bool) product.Value {
	if !hasExact {
		return projected
	}
	if merged := valuerefine.MeetConstraint(reg, exact, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
		return merged
	}
	return projected
}

func exactValueOnlyProvesPresence(reg *axis.Registry, value product.Value) bool {
	if reg == nil || !presence.Equal(product.PresenceOf(value), presence.Present()) {
		return false
	}
	if _, ok := reg.LookupErased(evidence.Key.ID()); ok {
		ev := product.Get(reg, value, evidence.Key)
		if ev.IsExplicitTop() || ev.IsGradualTop() {
			return false
		}
	}
	if _, ok := reg.LookupErased(identity.Key.ID()); ok {
		if identityvalue.HasExact(reg, value) {
			return false
		}
	}
	if _, ok := reg.LookupErased(runtimekind.Key.ID()); ok {
		if kindValue := product.Get(reg, value, runtimekind.Key); !kindValue.IsTop() && !runtimekind.Equal(kindValue, runtimekind.Singleton(runtimekind.Table)) {
			return false
		}
	}
	if _, ok := reg.LookupErased(typewitness.Key.ID()); ok {
		if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
			return false
		}
	}
	if _, ok := reg.LookupErased(variantorigin.Key.ID()); ok {
		if origin := product.Get(reg, value, variantorigin.Key); !origin.IsTop() {
			return false
		}
	}
	return true
}

func projectedValueCarriesContent(reg *axis.Registry, value product.Value) bool {
	if reg == nil ||
		product.Equal(reg, value, product.Top()) ||
		product.Equal(reg, value, product.Bottom(reg)) ||
		exactValueOnlyProvesPresence(reg, value) {
		return false
	}
	return true
}

func unknownIndexReadValue(config Config, seg segment.Segment) (product.Value, bool) {
	reg := config.Registry
	keyType, ok := luatypeprojection.SegmentKeyType(seg)
	if !ok {
		return product.Value{}, false
	}
	projected, ok := access.RuntimeIndex(typetable.NewMap(typ.Any, typ.Unknown), keyType)
	if !ok {
		return product.Value{}, false
	}
	if typ.IsUnknown(projected) {
		return product.Top(), true
	}
	return config.TypeValues.FromType(reg, projected), true
}
