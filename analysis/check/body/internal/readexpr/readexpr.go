// Package readexpr adapts Lua expression paths to check-body state reads.
package readexpr

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type Config struct {
	Registry   *axis.Registry
	Facts      factflow.Facts
	Visibility *visibility.Resolver
	TypeValues *typevalue.Cache
}

func Provider(config Config) sourcevalue.ExpressionValueProvider {
	reg := config.Registry
	if reg == nil {
		panic("readexpr: Config.Registry is required")
	}
	return func(point cfg.Point, expr factflow.ExprRef, _ factflow.ValueSource, in state.State) (product.Value, bool) {
		p, ok := config.Facts.ExpressionPath(expr)
		if !ok {
			return product.Value{}, false
		}
		return Project(config, point, p, in)
	}
}

func Project(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	reg := config.Registry
	if reg == nil {
		panic("readexpr: Config.Registry is required")
	}
	if p.IsEmpty() {
		return product.Value{}, false
	}
	if len(p.Segments) == 0 {
		return readPathValue(reg, config.Visibility, point, p, in)
	}

	exactPresent := product.Value{}
	hasExactPresent := false
	if exact, ok := exactPathValue(reg, config.Visibility, point, p, in); ok {
		switch gotPresence := product.PresenceOf(exact); {
		case presence.Equal(gotPresence, presence.Present()):
			exactPresent = withoutNilRuntimeKind(reg, product.WithPresence(reg, exact, presence.Present()))
			hasExactPresent = true
			if hasExactIdentity(reg, exactPresent) {
				if projected, ok, _ := projectFromStructuralEvidence(config, point, p, in); ok {
					if merged := product.Meet(reg, projected, exactPresent); !product.Equal(reg, merged, product.Bottom(reg)) {
						return merged, true
					}
				}
				if parentValue, hasParent := Project(config, point, p.Parent(), in); hasParent {
					exactPresent = inheritTopOriginEvidence(reg, exactPresent, parentValue)
				}
				return exactPresent, true
			}
		case presence.Equal(gotPresence, presence.Absent()):
			return product.Absent(reg), true
		}
	}

	if projected, ok, blocked := projectFromStructuralEvidence(config, point, p, in); ok {
		if hasExactPresent {
			if merged := product.Meet(reg, projected, exactPresent); !product.Equal(reg, merged, product.Bottom(reg)) {
				return merged, true
			}
			return exactPresent, true
		}
		return dropInBoundsIndexNil(config, point, p, in, projected), true
	} else if blocked && !hasExactPresent {
		return product.Value{}, false
	}

	if hasExactPresent {
		if parentValue, hasParent := Project(config, point, p.Parent(), in); hasParent {
			exactPresent = inheritTopOriginEvidence(reg, exactPresent, parentValue)
		}
		return exactPresent, true
	}

	value, ok := unknownIndexReadValue(config, p.Segments[len(p.Segments)-1])
	if !ok {
		return product.Value{}, false
	}
	if parentValue, hasParent := Project(config, point, p.Parent(), in); hasParent {
		value = inheritTopOriginEvidence(reg, value, parentValue)
	}
	return dropInBoundsIndexNil(config, point, p, in, value), true
}

func unknownIndexReadValue(config Config, seg segment.Segment) (product.Value, bool) {
	reg := config.Registry
	keyType, ok := segmentKeyType(seg)
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

// dropInBoundsIndexNil removes the soundly-optional nil from an array element
// read when a proven length floor establishes the literal integer index is
// in range: index k >= 1 with len(array) >= k. The decision consults the
// point-local length-floor lane keyed by the array path's visible state key.
// Out-of-floor indices keep their optional nil.
func dropInBoundsIndexNil(config Config, point cfg.Point, p pathdom.Path, in state.State, value product.Value) product.Value {
	reg := config.Registry
	if config.Visibility == nil || len(p.Segments) == 0 {
		return value
	}
	last := p.Segments[len(p.Segments)-1]
	if last.Kind != segment.SegmentIndexInt || last.Index < 1 {
		return value
	}
	arrayKey := config.Visibility.KeyAt(point, p.Parent())
	if arrayKey == "" {
		return value
	}
	floor, ok := in.ReadLenFloor(arrayKey)
	if !ok || floor < int64(last.Index) {
		return value
	}
	return withoutNilRuntimeKind(reg, product.WithPresence(reg, value, presence.Present()))
}

func hasExactIdentity(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return false
	}
	_, ok := product.Get(reg, value, identity.Key).ID()
	return ok
}

func inheritTopOriginEvidence(reg *axis.Registry, value, parent product.Value) product.Value {
	parentEvidence := product.Get(reg, parent, evidence.Key)
	if parentEvidence.IsGradualTop() || parentEvidence.IsExplicitTop() {
		return product.Set(reg, value, evidence.Key, parentEvidence)
	}
	return value
}

func projectFromStructuralEvidence(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool, bool) {
	reg := config.Registry
	root := p
	root.Segments = nil
	rootProjected := product.Value{}
	hasRootProjected := false
	if rootValue, ok := readPathValue(reg, config.Visibility, point, root, in); ok {
		if projected, ok := projectFromValueEvidence(config, rootValue, p.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		}
	}

	parent := p.Parent()
	parentValue, hasParent := Project(config, point, parent, in)
	if !runtimeMayBeTable(reg, parentValue, hasParent) {
		return product.Value{}, false, true
	}
	if projected, ok := projectFromValueEvidence(config, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		// The parent-relative projection observes per-segment narrowing recorded
		// on the intermediate path (e.g. a truthy guard that removed nil from an
		// optional field), so it is at least as precise as a single root-relative
		// projection across the full suffix. Meeting them keeps a narrowed
		// non-optional result instead of re-widening it with the root's optional.
		if hasRootProjected {
			if merged := product.Meet(reg, rootProjected, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
				return merged, true, false
			}
		}
		return projected, true, false
	}

	if hasRootProjected {
		return rootProjected, true, false
	}

	return product.Value{}, false, false
}

func exactPathValue(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	p pathdom.Path,
	in state.State,
) (product.Value, bool) {
	if resolver == nil {
		return product.Value{}, false
	}
	pathKey := resolver.KeyAt(point, p)
	if pathKey == "" {
		return product.Value{}, false
	}
	value := in.ReadPathKey(reg, pathKey)
	if product.Equal(reg, value, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return value, true
}

func readPathValue(
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
		return value, true
	}
	if resolver == nil {
		return product.Value{}, false
	}
	pathKey := resolver.KeyAt(point, p)
	if pathKey == "" {
		return product.Value{}, false
	}
	value := in.ReadPathKey(reg, pathKey)
	if product.Equal(reg, value, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return value, true
}

func projectFromValueEvidence(config Config, value product.Value, suffix []segment.Segment) (product.Value, bool) {
	reg := config.Registry
	if len(suffix) == 0 {
		return product.Value{}, false
	}
	parentType, ok := structuralTypeFromValue(config, value)
	if !ok {
		return product.Value{}, false
	}
	projected, ok := luatypeprojection.ApplySegments(parentType, suffix)
	if !ok {
		return product.Value{}, false
	}
	return config.TypeValues.FromTypeWithWitness(reg, projected), true
}

func structuralTypeFromValue(config Config, value product.Value) (typ.Type, bool) {
	reg := config.Registry
	origin := product.Get(reg, value, variantorigin.Key)
	valuePresence := product.PresenceOf(value)
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			t = typeForValuePresence(t, valuePresence)
			if !origin.IsBottom() && !origin.IsTop() {
				if narrowed, ok := config.TypeValues.NarrowVariantByOrigin(t, origin.Family(), origin.Cases()); ok {
					return narrowed, true
				}
				if narrowed, ok := config.TypeValues.TypeFromVariantOrigin(origin.Family(), origin.Cases()); ok {
					return typeForValuePresence(narrowed, valuePresence), true
				}
			}
			return t, true
		}
	}
	if !origin.IsBottom() && !origin.IsTop() {
		if t, ok := config.TypeValues.TypeFromVariantOrigin(origin.Family(), origin.Cases()); ok {
			return typeForValuePresence(t, valuePresence), true
		}
	}
	return nil, false
}

func typeForValuePresence(t typ.Type, p presence.Value) typ.Type {
	switch {
	case presence.Equal(p, presence.Absent()):
		return typ.Nil
	case presence.Equal(p, presence.Present()):
		if present := typetable.PresentReadonlyEntryValue(t); present != nil {
			return present
		}
	case presence.Equal(p, presence.Maybe()):
		return normalize.Optional(t)
	}
	return t
}

func runtimeMayBeTable(reg *axis.Registry, value product.Value, hasValue bool) bool {
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

func segmentKeyType(seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return typ.LiteralString(seg.Name), true
	case segment.SegmentIndexInt:
		return typ.LiteralInt(int64(seg.Index)), true
	default:
		return nil, false
	}
}

func withoutNilRuntimeKind(reg *axis.Registry, value product.Value) product.Value {
	kinds := product.Get(reg, value, runtimekind.Key)
	if !kinds.Contains(runtimekind.Nil) {
		return value
	}
	return product.Set(reg, value, runtimekind.Key, kinds.Without(runtimekind.Nil))
}
