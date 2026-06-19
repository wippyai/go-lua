package access

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func fieldDepth(t typ.Type, name string, depth int) fieldResult {
	if stopDepth(t, depth) {
		return fieldResult{}
	}
	if top, ok := SpecialAccessType(t); ok {
		return fieldResult{t: top, ok: true}
	}

	return descendAccessWrappers(t, depth, func(t typ.Type, depth int) fieldResult {
		if top, ok := SpecialAccessType(t); ok {
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
		default:
			return fieldResult{}
		}
	}, func(res fieldResult) fieldResult {
		if res.ok {
			res.nilable = true
		}
		return res
	})
}

func SpecialAccessType(t typ.Type) (typ.Type, bool) {
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
