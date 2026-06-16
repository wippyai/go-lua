package access

import (
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func fieldDepth(t typ.Type, name string, depth int) fieldResult {
	if stopDepth(t, depth) {
		return fieldResult{}
	}
	if top, ok := specialAccessType(t); ok {
		return fieldResult{t: top, ok: true}
	}

	switch v := unwrap.Annotated(t).(type) {
	case *typ.Record:
		return fieldInRecord(v, name)
	case *typ.Interface:
		return fieldInInterface(v, t, name)
	case *typ.Map:
		return fieldInMap(v.Key, v.Value, name)
	case *typ.ReadonlyMap:
		return fieldInMap(v.Key, v.Value, name)
	case *typ.Union:
		return fieldInUnion(v, name, depth+1)
	case *typ.Intersection:
		return fieldInIntersection(v, name, depth+1)
	case *typ.Optional:
		res := fieldDepth(v.Inner, name, depth+1)
		if res.ok {
			res.nilable = true
		}
		return res
	case *typ.Alias:
		return fieldDepth(v.UnaliasedTarget(), name, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return fieldResult{}
		}
		return fieldDepth(v.Body, name, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return fieldResult{}
		}
		return fieldDepth(expanded, name, depth+1)
	default:
		return fieldResult{}
	}
}

func fieldAtDepth(t typ.Type, name string, depth int) (typ.Type, bool) {
	return fieldDepth(t, name, depth).materialize()
}

func specialAccessType(t typ.Type) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	if typ.IsAny(t) {
		return typ.Any, true
	}
	if typ.IsUnknown(t) {
		return typ.Unknown, true
	}
	if typ.IsNever(t) {
		return typ.Never, true
	}
	if typetable.IsBuiltinTopMarker(t) {
		return typ.Any, true
	}
	return nil, false
}

func stopDepth(t typ.Type, depth int) bool {
	return t == nil || depth > typ.DefaultRecursionDepth
}
