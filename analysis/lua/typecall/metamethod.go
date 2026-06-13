package typecall

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// GetMetamethod resolves a direct metatable field from t.
func GetMetamethod(t typ.Type, name string) (typ.Type, bool) {
	return metamethodDepth(t, name, 0)
}

// HasMetamethod reports whether GetMetamethod can resolve name on t.
func HasMetamethod(t typ.Type, name string) bool {
	_, ok := GetMetamethod(t, name)
	return ok
}

func metamethodDepth(t typ.Type, name string, depth int) (typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}
	if top, ok := specialAccessType(t); ok {
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

func metamethodInRecord(r *typ.Record, name string, depth int) (typ.Type, bool) {
	if r == nil || r.Metatable == nil || typetable.IsMetatableUnconstrained(r.Metatable) {
		return nil, false
	}
	return fieldAtDepth(r.Metatable, name, depth+1)
}

func metamethodInUnion(u *typ.Union, name string, depth int) (typ.Type, bool) {
	if u == nil || len(u.Members) == 0 {
		return nil, false
	}
	out := make([]typ.Type, 0, len(u.Members))
	for _, member := range u.Members {
		if isNilType(member) {
			continue
		}
		mt, ok := metamethodDepth(member, name, depth+1)
		if !ok {
			return nil, false
		}
		out = append(out, mt)
	}
	if len(out) == 0 {
		return nil, false
	}
	return normalize.UnionForEvidence(out...), true
}

func metamethodInIntersection(in *typ.Intersection, name string, depth int) (typ.Type, bool) {
	if in == nil {
		return nil, false
	}
	out := make([]typ.Type, 0, len(in.Members))
	for _, member := range in.Members {
		if mt, ok := metamethodDepth(member, name, depth+1); ok {
			out = append(out, mt)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	if len(out) == 1 {
		return out[0], true
	}
	return normalize.IntersectionForMeet(out...), true
}
