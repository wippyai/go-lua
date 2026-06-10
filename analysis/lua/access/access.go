// Package access provides Lua table and callable access projections.
package access

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Field resolves a dot-field projection against a type.
func Field(t typ.Type, name string) (typ.Type, bool) {
	return fieldDepth(t, name, 0).materialize()
}

// MissingFieldReadsNil reports whether a missing field read on t has defined
// Lua table semantics and produces nil instead of an indexing error.
func MissingFieldReadsNil(t typ.Type) bool {
	return missingFieldReadsNilDepth(t, 0)
}

// CallableReturn projects a callable witness to its first return type.
func CallableReturn(t typ.Type) (typ.Type, bool) {
	return callableReturnDepth(t, 0)
}

type fieldResult struct {
	t       typ.Type
	ok      bool
	nilable bool
}

func (r fieldResult) materialize() (typ.Type, bool) {
	if !r.ok {
		return nil, false
	}
	if r.t == nil {
		r.t = typ.Unknown
	}
	if r.nilable {
		return typ.NewOptional(r.t), true
	}
	return r.t, true
}

func fieldDepth(t typ.Type, name string, depth int) fieldResult {
	if stopDepth(t, depth) {
		return fieldResult{}
	}
	if top, ok := specialAccessType(t); ok {
		return fieldResult{t: top, ok: true}
	}

	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Record:
		return fieldInRecord(v, name)
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

func fieldInRecord(r *typ.Record, name string) fieldResult {
	if r == nil {
		return fieldResult{}
	}
	if f := r.GetField(name); f != nil {
		return fieldResult{t: f.Type, ok: true, nilable: f.Optional}
	}
	if member := r.GetStaticStringIndex(name); member != nil {
		return fieldResult{t: member.Type, ok: true, nilable: member.Optional}
	}
	if r.HasMapComponent() {
		if mapKeyAcceptsStringField(r.MapKey, name) {
			return fieldResult{t: r.MapValue, ok: true, nilable: true}
		}
	}
	if r.Open {
		return fieldResult{t: typ.Unknown, ok: true}
	}
	return fieldResult{}
}

func fieldInMap(key typ.Type, value typ.Type, name string) fieldResult {
	if !mapKeyAcceptsStringField(key, name) {
		return fieldResult{}
	}
	if value == nil {
		return fieldResult{t: typ.Nil, ok: true}
	}
	return fieldResult{t: value, ok: true, nilable: true}
}

func mapKeyAcceptsStringField(key typ.Type, name string) bool {
	return key != nil && subtype.IsSubtype(typ.LiteralString(name), key)
}

func fieldInUnion(u *typ.Union, name string, depth int) fieldResult {
	if u == nil || len(u.Members) == 0 {
		return fieldResult{}
	}
	out := make([]typ.Type, 0, len(u.Members))
	nilable := false

	for _, member := range u.Members {
		res := fieldDepth(member, name, depth+1)
		if !res.ok {
			if missingFieldReadsNilDepth(member, depth+1) {
				nilable = true
				continue
			}
			return fieldResult{}
		}
		if res.nilable {
			nilable = true
		}
		if res.t != nil {
			out = append(out, res.t)
		}
	}

	if len(out) == 0 {
		if nilable {
			return fieldResult{t: typ.Nil, ok: true}
		}
		return fieldResult{}
	}
	return fieldResult{t: normalize.UnionForProjection(out...), ok: true, nilable: nilable}
}

func fieldInIntersection(in *typ.Intersection, name string, depth int) fieldResult {
	if in == nil {
		return fieldResult{}
	}
	out := make([]typ.Type, 0, len(in.Members))
	for _, member := range in.Members {
		if ft, ok := fieldAtDepth(member, name, depth+1); ok {
			out = append(out, ft)
		}
	}
	if len(out) == 0 {
		return fieldResult{}
	}
	if len(out) == 1 {
		return fieldResult{t: out[0], ok: true}
	}
	return fieldResult{t: normalize.IntersectionForMeet(out...), ok: true}
}

func fieldAtDepth(t typ.Type, name string, depth int) (typ.Type, bool) {
	return fieldDepth(t, name, depth).materialize()
}

func missingFieldReadsNilDepth(t typ.Type, depth int) bool {
	if stopDepth(t, depth) {
		return false
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple, *typ.Interface:
		return true
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !missingFieldReadsNilDepth(member, depth+1) {
				return false
			}
		}
		return true
	case *typ.Intersection:
		for _, member := range v.Members {
			if missingFieldReadsNilDepth(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return missingFieldReadsNilDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && missingFieldReadsNilDepth(expanded, depth+1)
	default:
		return false
	}
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
		return normalize.UnionForProjection(out...), true
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
	if unwrap.IsBuiltinTableTop(t) {
		return typ.Any, true
	}
	return nil, false
}

func stopDepth(t typ.Type, depth int) bool {
	return t == nil || depth > typ.DefaultRecursionDepth
}
