// Package readexpr adapts Lua expression paths to check-body state reads.
package readexpr

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type Config struct {
	Registry   *axis.Registry
	Facts      factflow.Facts
	Visibility *visibility.Resolver
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
		return Project(Config{Registry: reg, Facts: config.Facts, Visibility: config.Visibility}, point, p, in)
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
		return projected, true
	} else if blocked && !hasExactPresent {
		return product.Value{}, false
	}

	if hasExactPresent {
		return exactPresent, true
	}

	keyType, ok := segmentKeyType(p.Segments[len(p.Segments)-1])
	if !ok {
		return product.Value{}, false
	}
	projected, ok := typeaccess.RuntimeIndex(typ.NewMap(typ.Any, typ.Unknown), keyType)
	if !ok {
		return product.Value{}, false
	}
	return typevalue.FromType(reg, projected), true
}

func projectFromStructuralEvidence(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool, bool) {
	reg := config.Registry
	root := p
	root.Segments = nil
	if rootValue, ok := readPathValue(reg, config.Visibility, point, root, in); ok {
		if projected, ok := projectFromValueEvidence(reg, rootValue, p.Segments); ok {
			return projected, true, false
		}
	}

	parent := p.Parent()
	parentValue, hasParent := Project(config, point, parent, in)
	if !runtimeMayBeTable(reg, parentValue, hasParent) {
		return product.Value{}, false, true
	}
	if projected, ok := projectFromValueEvidence(reg, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		return projected, true, false
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

func projectFromValueEvidence(reg *axis.Registry, value product.Value, suffix []segment.Segment) (product.Value, bool) {
	if len(suffix) == 0 {
		return product.Value{}, false
	}
	parentType, ok := structuralTypeFromValue(reg, value)
	if !ok {
		return product.Value{}, false
	}
	projected, ok := typeaccess.ProjectSegments(parentType, suffix)
	if !ok {
		return product.Value{}, false
	}
	return typevalue.FromType(reg, projected), true
}

func structuralTypeFromValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	origin := product.Get(reg, value, variantorigin.Key)
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			if !origin.IsBottom() && !origin.IsTop() {
				if narrowed, ok := variant.NarrowByOrigin(t, origin.Family(), origin.Cases()); ok {
					return narrowed, true
				}
			}
			return t, true
		}
	}
	if !origin.IsBottom() && !origin.IsTop() {
		return variant.TypeFromOrigin(origin.Family(), origin.Cases())
	}
	return nil, false
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
