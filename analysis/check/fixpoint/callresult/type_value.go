package callresult

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func valueFromType(reg *axis.Registry, t typ.Type) product.Value {
	value := product.Top()
	if kindValue, ok := runtimeKindFromType(t); ok {
		value = product.Set(reg, value, runtimekind.Key, kindValue)
	}
	if t != nil && typ.NormalizeNilType(t).Kind() == kind.Nil {
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
