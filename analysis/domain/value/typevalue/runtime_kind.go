package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

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
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return runtimekind.Value{}, false
		}
		return RuntimeKindFromType(expanded)
	case *typ.Alias:
		target := tt.UnaliasedTarget()
		if target == nil || target == t {
			return runtimekind.Value{}, false
		}
		return RuntimeKindFromType(target)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return runtimekind.Value{}, false
		}
		return RuntimeKindFromType(tt.Body)
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
