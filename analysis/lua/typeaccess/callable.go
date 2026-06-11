package typeaccess

import (
	"github.com/wippyai/go-lua/analysis/type/identity"
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

// Callable returns the concrete function witness for a callable type.
func Callable(t typ.Type) (*typ.Function, bool) {
	return callableDepth(t, 0)
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
	return normalize.UnionForProjection(out...), true
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

func callableDepth(t typ.Type, depth int) (*typ.Function, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}

	switch v := unwrap.Annotated(t).(type) {
	case *typ.Function:
		return v, true
	case *typ.Record:
		return callableRecord(v, depth+1)
	case *typ.Union:
		return callableUnion(v, depth+1)
	case *typ.Intersection:
		return callableIntersection(v, depth+1)
	case *typ.Optional:
		return callableDepth(v.Inner, depth+1)
	case *typ.Alias:
		return callableDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return nil, false
		}
		return callableDepth(expanded, depth+1)
	default:
		return nil, false
	}
}

func callableRecord(r *typ.Record, depth int) (*typ.Function, bool) {
	call, ok := metamethodInRecord(r, "__call", depth+1)
	if !ok {
		return nil, false
	}
	return functionWitnessDepth(call, depth+1)
}

func callableUnion(u *typ.Union, depth int) (*typ.Function, bool) {
	if u == nil || len(u.Members) == 0 {
		return nil, false
	}
	var witness *typ.Function
	for _, member := range u.Members {
		if isNilType(member) {
			continue
		}
		fn, ok := callableDepth(member, depth+1)
		if !ok {
			return nil, false
		}
		if witness == nil {
			witness = fn
			continue
		}
		if !identity.TypeEquals(witness, fn) {
			return nil, false
		}
	}
	return witness, witness != nil
}

func callableIntersection(in *typ.Intersection, depth int) (*typ.Function, bool) {
	if in == nil {
		return nil, false
	}
	for _, member := range in.Members {
		if fn, ok := callableDepth(member, depth+1); ok {
			return fn, true
		}
	}
	return nil, false
}

func functionWitnessDepth(t typ.Type, depth int) (*typ.Function, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}

	switch v := unwrap.Annotated(t).(type) {
	case *typ.Function:
		return v, true
	case *typ.Union:
		return functionWitnessUnion(v, depth+1)
	case *typ.Intersection:
		return functionWitnessIntersection(v, depth+1)
	case *typ.Optional:
		return functionWitnessDepth(v.Inner, depth+1)
	case *typ.Alias:
		return functionWitnessDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return nil, false
		}
		return functionWitnessDepth(expanded, depth+1)
	default:
		return nil, false
	}
}

func functionWitnessUnion(u *typ.Union, depth int) (*typ.Function, bool) {
	if u == nil || len(u.Members) == 0 {
		return nil, false
	}
	var witness *typ.Function
	for _, member := range u.Members {
		if isNilType(member) {
			continue
		}
		fn, ok := functionWitnessDepth(member, depth+1)
		if !ok {
			return nil, false
		}
		if witness == nil {
			witness = fn
			continue
		}
		if !identity.TypeEquals(witness, fn) {
			return nil, false
		}
	}
	return witness, witness != nil
}

func functionWitnessIntersection(in *typ.Intersection, depth int) (*typ.Function, bool) {
	if in == nil {
		return nil, false
	}
	for _, member := range in.Members {
		if fn, ok := functionWitnessDepth(member, depth+1); ok {
			return fn, true
		}
	}
	return nil, false
}

func isNilType(t typ.Type) bool {
	t = unwrap.Annotated(t)
	return t != nil && t.Kind() == typ.Nil.Kind()
}
