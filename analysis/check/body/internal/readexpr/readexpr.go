// Package readexpr adapts Lua expression paths to check-body state reads.
package readexpr

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/access"
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
		return sourcevalue.ReadPathValue(reg, config.Visibility, point, p, in)
	}

	exactPresent := product.Value{}
	hasExactPresent := false
	if exact, ok := sourcevalue.ExactPathValue(reg, config.Visibility, point, p, in); ok {
		switch gotPresence := product.PresenceOf(exact); {
		case presence.Equal(gotPresence, presence.Present()):
			exactPresent = sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, exact, presence.Present()))
			hasExactPresent = true
			if sourcevalue.HasExactIdentity(reg, exactPresent) {
				if projected, ok, _ := projectFromStructuralEvidence(config, point, p, in); ok {
					if merged := product.Meet(reg, projected, exactPresent); !product.Equal(reg, merged, product.Bottom(reg)) {
						return merged, true
					}
				}
				if parentValue, hasParent := Project(config, point, p.Parent(), in); hasParent {
					exactPresent = sourcevalue.InheritTopOriginEvidence(reg, exactPresent, parentValue)
				}
				return exactPresent, true
			}
		case presence.Equal(gotPresence, presence.Absent()):
			return product.Absent(reg), true
		}
	}

	if !hasExactPresent {
		if heapProjected, ok := projectFromHeapIdentity(config, point, p, in); ok {
			return heapProjected, true
		}
	}

	if hasExactPresent {
		if parentValue, hasParent := Project(config, point, p.Parent(), in); hasParent {
			exactPresent = sourcevalue.InheritTopOriginEvidence(reg, exactPresent, parentValue)
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
		return exactPresent, true
	}

	value, ok := unknownIndexReadValue(config, p.Segments[len(p.Segments)-1])
	if !ok {
		return product.Value{}, false
	}
	if parentValue, hasParent := Project(config, point, p.Parent(), in); hasParent {
		value = sourcevalue.InheritTopOriginEvidence(reg, value, parentValue)
	}
	return dropInBoundsIndexNil(config, point, p, in, value), true
}

// dropInBoundsIndexNil removes the soundly-optional nil from an array element
// read when a proven length floor establishes the literal integer index is in
// range: index k >= 1 with len(array) >= k. The decision consults the
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
	return sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, value, presence.Present()))
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

func projectFromHeapIdentity(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	reg := config.Registry
	root := p
	root.Segments = nil
	rootProjected := product.Value{}
	hasRootProjected := false
	if rootValue, ok := sourcevalue.ReadPathValue(reg, config.Visibility, point, root, in); ok {
		if projected, ok := sourcevalue.HeapMemberFromValue(reg, in, rootValue, p.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		}
	}

	parent := p.Parent()
	parentValue, _ := Project(config, point, parent, in)
	if projected, ok := sourcevalue.HeapMemberFromValue(reg, in, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		if hasRootProjected {
			if merged := product.Meet(reg, rootProjected, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
				return merged, true
			}
		}
		return projected, true
	}
	if hasRootProjected {
		return rootProjected, true
	}
	return product.Value{}, false
}

func projectFromStructuralEvidence(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool, bool) {
	reg := config.Registry
	root := p
	root.Segments = nil
	rootProjected := product.Value{}
	hasRootProjected := false
	if rootValue, ok := sourcevalue.ReadPathValue(reg, config.Visibility, point, root, in); ok {
		if projected, ok := projectFromValueEvidence(config, rootValue, p.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		}
	}

	parent := p.Parent()
	parentValue, hasParent := Project(config, point, parent, in)
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
			if merged := product.Meet(reg, rootProjected, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
				return merged, true, false
			}
		}
		return projected, true, false
	}
	if parentProjectionRejectsFinalSegment(config, parentValue, p.Segments[len(p.Segments)-1:]) {
		return product.Value{}, false, true
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
	return config.TypeValues.FromTypeWithWitness(reg, projected), true
}

func parentProjectionRejectsFinalSegment(config Config, value product.Value, suffix []segment.Segment) bool {
	if len(suffix) != 1 {
		return false
	}
	parentType, ok := typevalue.StructuralTypeOf(config.Registry, config.TypeValues, value, typevalue.StructuralTypeOptions{
		ApplyPresence:     true,
		OptionalWhenMaybe: true,
	})
	if !ok || parentType == nil || typ.IsAny(parentType) || typ.IsUnknown(parentType) || typ.IsNever(parentType) {
		return false
	}
	_, ok = luatypeprojection.ApplySegments(parentType, suffix)
	return !ok
}
