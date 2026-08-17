package typecall

import (
	"github.com/wippyai/go-lua/domain/type/access"
	"github.com/wippyai/go-lua/domain/type/subst"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

// GetMetamethod resolves a direct metatable field from t.
func GetMetamethod(t typ.Type, name string) (typ.Type, bool) {
	return metamethodDepth(t, name, 0)
}

func metamethodDepth(t typ.Type, name string, depth int) (typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}
	if top, ok := metamethodSpecialAccessType(t); ok {
		return top, true
	}

	switch v := unwrap.Annotated(t).(type) {
	case *typ.Record:
		return metamethodInRecord(v, name, depth+1)
	case *typ.Union:
		return metamethodInUnion(v, name, depth+1)
	case *typ.Intersection:
		return metamethodInIntersection(v, name, depth+1)
	case *typ.Optional:
		return metamethodDepth(v.Inner, name, depth+1)
	case *typ.Alias:
		return metamethodDepth(v.UnaliasedTarget(), name, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return nil, false
		}
		return metamethodDepth(expanded, name, depth+1)
	default:
		return nil, false
	}
}

func metamethodSpecialAccessType(t typ.Type) (typ.Type, bool) {
	top, ok := access.SpecialAccessType(t)
	if !ok || typ.IsBuiltinTableTopMarker(t) {
		return nil, false
	}
	return top, true
}

func metamethodInRecord(r *typ.Record, name string, depth int) (typ.Type, bool) {
	if r == nil || r.Metatable == nil || typetable.IsMetatableUnconstrained(r.Metatable) {
		return nil, false
	}
	return access.Field(r.Metatable, name)
}

func metamethodInUnion(u *typ.Union, name string, depth int) (typ.Type, bool) {
	return typeUnion(u, depth, func(member typ.Type, depth int) (typ.Type, bool) {
		return metamethodDepth(member, name, depth)
	})
}

func metamethodInIntersection(in *typ.Intersection, name string, depth int) (typ.Type, bool) {
	return typeIntersection(in, depth, func(member typ.Type, depth int) (typ.Type, bool) {
		return metamethodDepth(member, name, depth)
	})
}
