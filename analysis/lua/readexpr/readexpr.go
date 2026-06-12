// Package readexpr projects static Lua table/index reads from semantic facts.
package readexpr

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factflow/source"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type Config struct {
	Registry   *axis.Registry
	Facts      factflow.Facts
	Visibility *visibility.Resolver
}

func Provider(config Config) source.ExpressionValueProvider {
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
	if p.IsEmpty() || len(p.Segments) == 0 {
		return product.Value{}, false
	}

	if exact, ok := exactPathValue(reg, config.Visibility, point, p, in); ok {
		switch gotPresence := product.PresenceOf(exact); {
		case presence.Equal(gotPresence, presence.Present()):
			return withoutNilRuntimeKind(reg, product.WithPresence(reg, exact, presence.Present())), true
		case presence.Equal(gotPresence, presence.Absent()):
			return product.Absent(reg), true
		}
	}

	parent := p.Parent()
	parentValue, hasParent := readPathValue(reg, config.Visibility, point, parent, in)
	if !runtimeMayBeTable(reg, parentValue, hasParent) {
		return product.Value{}, false
	}

	keyType, ok := segmentKeyType(p.Segments[len(p.Segments)-1])
	if !ok {
		return product.Value{}, false
	}
	projected, ok := typeaccess.RuntimeIndex(typ.NewMap(typ.Any, typ.Unknown), keyType)
	if !ok {
		return product.Value{}, false
	}
	return valueFromType(reg, projected), true
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

func valueFromType(reg *axis.Registry, t typ.Type) product.Value {
	value := product.Top()
	if kindValue, ok := runtimeKindFromType(t); ok {
		value = product.Set(reg, value, runtimekind.Key, kindValue)
	}
	if normalized := typ.NormalizeNilType(t); normalized != nil && normalized.Kind() == kind.Nil {
		value = product.WithPresence(reg, value, presence.Absent())
	}
	return value
}

func runtimeKindFromType(t typ.Type) (runtimekind.Value, bool) {
	if t == nil {
		return runtimekind.Value{}, false
	}
	switch tt := typ.NormalizeNilType(t).(type) {
	case *typ.Literal:
		switch tt.Base {
		case kind.Boolean:
			return runtimekind.Singleton(runtimekind.Boolean), true
		case kind.Integer, kind.Number:
			return runtimekind.Singleton(runtimekind.Number), true
		case kind.String:
			return runtimekind.Singleton(runtimekind.String), true
		default:
			return runtimekind.Value{}, false
		}
	case *typ.Optional:
		return runtimeKindFromType(tt.Inner)
	case *typ.Union:
		var out runtimekind.Value
		seen := false
		for _, member := range tt.Members {
			memberKind, ok := runtimeKindFromType(member)
			if !ok {
				return runtimekind.Value{}, false
			}
			if seen {
				out = runtimekind.Join(out, memberKind)
			} else {
				out = memberKind
				seen = true
			}
		}
		return out, seen
	default:
		switch tt.Kind() {
		case kind.Nil:
			return runtimekind.Singleton(runtimekind.Nil), true
		case kind.Boolean:
			return runtimekind.Singleton(runtimekind.Boolean), true
		case kind.Number, kind.Integer:
			return runtimekind.Singleton(runtimekind.Number), true
		case kind.String:
			return runtimekind.Singleton(runtimekind.String), true
		case kind.Function:
			return runtimekind.Singleton(runtimekind.Function), true
		case kind.Record, kind.Array, kind.Tuple, kind.Map, kind.ReadonlyMap:
			return runtimekind.Singleton(runtimekind.Table), true
		default:
			return runtimekind.Value{}, false
		}
	}
}

func withoutNilRuntimeKind(reg *axis.Registry, value product.Value) product.Value {
	kinds := product.Get(reg, value, runtimekind.Key)
	if !kinds.Contains(runtimekind.Nil) {
		return value
	}
	return product.Set(reg, value, runtimekind.Key, kinds.Without(runtimekind.Nil))
}
