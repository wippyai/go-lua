package typecall

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// CallableReturn projects a callable witness to its first return type.
func CallableReturn(t typ.Type) (typ.Type, bool) {
	return callableReturnDepth(t, 0)
}

func callableReturnDepth(t typ.Type, depth int) (typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}
	t = unwrap.Alias(t)
	switch v := t.(type) {
	case *typ.Function:
		return firstReturn(v)
	case *typ.Optional:
		return callableReturnDepth(v.Inner, depth+1)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if rt, ok := callableReturnDepth(member, depth+1); ok {
				out = append(out, rt)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return normalize.UnionForEvidence(out...), true
	case *typ.Intersection:
		for _, member := range v.Members {
			if rt, ok := callableReturnDepth(member, depth+1); ok {
				return rt, true
			}
		}
		return nil, false
	case *typ.Record:
		return recordCallReturn(v, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return nil, false
		}
		return callableReturnDepth(expanded, depth+1)
	default:
		if typ.IsAny(t) {
			return typ.Any, true
		}
		if typ.IsUnknown(t) {
			return typ.Unknown, true
		}
		return nil, false
	}
}

func firstReturn(fn *typ.Function) (typ.Type, bool) {
	if fn == nil || len(fn.Returns) == 0 || fn.Returns[0] == nil {
		return nil, false
	}
	return fn.Returns[0], true
}

func recordCallReturn(r *typ.Record, depth int) (typ.Type, bool) {
	if r == nil || r.Metatable == nil || typetable.IsMetatableUnconstrained(r.Metatable) {
		return nil, false
	}
	call, ok := fieldAtDepth(r.Metatable, "__call", depth+1)
	if !ok {
		return nil, false
	}
	return callableReturnDepth(call, depth+1)
}
