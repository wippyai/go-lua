package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// FromType materializes sound point-local value evidence from a type.
func FromType(reg *axis.Registry, t typ.Type) product.Value {
	value := product.Top()
	if p, ok := presenceFromType(t); ok {
		value = product.WithPresence(reg, value, p)
	}
	if kindValue, ok := RuntimeKindFromType(t); ok {
		value = product.Set(reg, value, runtimekind.Key, kindValue)
	}
	if family, cases, ok := discriminant.OriginOfType(t); ok {
		value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(family, cases))
	}
	return value
}

// RuntimeKindFromType returns concrete Lua runtime-kind evidence for t.
func RuntimeKindFromType(t typ.Type) (runtimekind.Value, bool) {
	t = normalize(t)
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return runtimekind.Value{}, false
	}
	switch tt := t.(type) {
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
		return RuntimeKindFromType(tt.Inner)
	case *typ.Union:
		var out runtimekind.Value
		seen := false
		seenNil := false
		for _, member := range tt.Members {
			member = normalize(member)
			if member != nil && member.Kind() == kind.Nil {
				seenNil = true
				continue
			}
			memberKind, ok := RuntimeKindFromType(member)
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
		if seen {
			return out, true
		}
		if seenNil {
			return runtimekind.Singleton(runtimekind.Nil), true
		}
		return runtimekind.Value{}, false
	default:
		switch t.Kind() {
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

func presenceFromType(t typ.Type) (presence.Value, bool) {
	t = normalize(t)
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return presence.Bottom(), false
	}
	switch tt := t.(type) {
	case *typ.Optional:
		return presence.Maybe(), true
	case *typ.Union:
		seenNil := false
		for _, member := range tt.Members {
			member = normalize(member)
			if member == nil || typ.IsAny(member) || typ.IsUnknown(member) {
				return presence.Bottom(), false
			}
			if member.Kind() == kind.Nil {
				seenNil = true
			}
		}
		if seenNil {
			return presence.Maybe(), true
		}
		return presence.Present(), true
	default:
		if t.Kind() == kind.Nil {
			return presence.Absent(), true
		}
		if _, ok := RuntimeKindFromType(t); ok {
			return presence.Present(), true
		}
		return presence.Bottom(), false
	}
}

func normalize(t typ.Type) typ.Type {
	return unwrap.Annotated(typ.NormalizeNilType(t))
}
