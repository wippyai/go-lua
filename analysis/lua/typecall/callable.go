package typecall

import (
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Callable returns the concrete function witness for a callable type.
func Callable(t typ.Type) (*typ.Function, bool) {
	return callableDepth(t, 0)
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
		if !typ.TypeEquals(witness, fn) {
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
		if !typ.TypeEquals(witness, fn) {
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
