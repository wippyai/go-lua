// Package typecall provides Lua callable and metamethod type projections.
package typecall

import (
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// CallableReturn projects a callable witness to its first return type.
func CallableReturn(t typ.Type) (typ.Type, bool) {
	return callableReturnDepth(t, 0)
}

// MemberCallStatus describes whether a receiver member can be called.
type MemberCallStatus uint8

const (
	MemberCallOK MemberCallStatus = iota
	MemberCallMissing
	MemberCallNotCallable
)

// MemberCall resolves a receiver member for call syntax. Unlike Field, this is
// strict for calls: every possible non-nil receiver alternative must provide a
// callable member, and optional or missing member values are not callable.
func MemberCall(t typ.Type, name string) (typ.Type, MemberCallStatus) {
	if name == "" {
		return nil, MemberCallMissing
	}
	res := memberCallDepth(t, name, 0)
	if res.status == MemberCallOK && res.t == nil {
		res.t = typ.Unknown
	}
	return res.t, res.status
}

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

type memberCallResult struct {
	t      typ.Type
	status MemberCallStatus
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

func memberCallDepth(t typ.Type, name string, depth int) memberCallResult {
	if stopDepth(t, depth) {
		return memberCallResult{status: MemberCallMissing}
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return memberCallUnion(v, name, depth+1)
	case *typ.Intersection:
		return memberCallIntersection(v, name, depth+1)
	case *typ.Optional:
		return memberCallDepth(v.Inner, name, depth+1)
	case *typ.Alias:
		return memberCallDepth(v.UnaliasedTarget(), name, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return memberCallResult{status: MemberCallMissing}
		}
		return memberCallDepth(expanded, name, depth+1)
	default:
		return memberCallSingle(t, name, depth+1)
	}
}

func memberCallUnion(u *typ.Union, name string, depth int) memberCallResult {
	if u == nil || len(u.Members) == 0 {
		return memberCallResult{status: MemberCallMissing}
	}
	out := make([]typ.Type, 0, len(u.Members))
	checked := false
	for _, member := range u.Members {
		if isNilType(member) || typ.IsNever(member) {
			continue
		}
		res := memberCallDepth(member, name, depth+1)
		if res.status != MemberCallOK {
			return res
		}
		checked = true
		if res.t != nil {
			out = append(out, res.t)
		}
	}
	if !checked {
		return memberCallResult{status: MemberCallMissing}
	}
	return memberCallResult{t: normalize.UnionForEvidence(out...), status: MemberCallOK}
}

func memberCallIntersection(in *typ.Intersection, name string, depth int) memberCallResult {
	if in == nil {
		return memberCallResult{status: MemberCallMissing}
	}
	out := make([]typ.Type, 0, len(in.Members))
	for _, member := range in.Members {
		res := memberCallDepth(member, name, depth+1)
		if res.status == MemberCallOK && res.t != nil {
			out = append(out, res.t)
		}
	}
	if len(out) == 0 {
		return memberCallResult{status: MemberCallMissing}
	}
	if len(out) == 1 {
		return memberCallResult{t: out[0], status: MemberCallOK}
	}
	return memberCallResult{t: normalize.IntersectionForMeet(out...), status: MemberCallOK}
}

func memberCallSingle(t typ.Type, name string, depth int) memberCallResult {
	if method, ok := stringMethod(name, t); ok {
		return memberCallResult{t: method, status: MemberCallOK}
	}
	memberType, ok := fieldAtDepth(t, name, depth+1)
	if !ok {
		return memberCallResult{status: MemberCallMissing}
	}
	if containsNil(memberType, depth+1) {
		return memberCallResult{t: memberType, status: MemberCallNotCallable}
	}
	if !callableValue(memberType, depth+1) {
		return memberCallResult{t: memberType, status: MemberCallNotCallable}
	}
	return memberCallResult{t: memberType, status: MemberCallOK}
}

func stringMethod(name string, receiver typ.Type) (typ.Type, bool) {
	if receiver == nil || !subtype.IsSubtype(receiver, typ.String) {
		return nil, false
	}
	switch name {
	case "byte", "char", "dump", "find", "format", "gmatch", "gsub", "len",
		"lower", "match", "pack", "packsize", "rep", "reverse", "sub", "unpack",
		"upper":
		return typ.Func().
			Param("self", typ.String).
			Variadic(typ.Any).
			Returns(typ.Any).
			Build(), true
	default:
		return nil, false
	}
}

func containsNil(t typ.Type, depth int) bool {
	if stopDepth(t, depth) {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Optional:
		return true
	case *typ.Union:
		for _, member := range v.Members {
			if containsNil(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return containsNil(v.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && containsNil(expanded, depth+1)
	default:
		return isNilType(t)
	}
}

func callableValue(t typ.Type, depth int) bool {
	if stopDepth(t, depth) {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Function:
		return true
	case *typ.Record:
		call, ok := metamethodInRecord(v, "__call", depth+1)
		return ok && !containsNil(call, depth+1) && callableValue(call, depth+1)
	case *typ.Union:
		checked := false
		for _, member := range v.Members {
			if typ.IsNever(member) {
				continue
			}
			if isNilType(member) || !callableValue(member, depth+1) {
				return false
			}
			checked = true
		}
		return checked
	case *typ.Intersection:
		for _, member := range v.Members {
			if callableValue(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return callableValue(v.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && callableValue(expanded, depth+1)
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

func fieldAtDepth(t typ.Type, name string, depth int) (typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}
	return typeaccess.Field(t, name)
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

func isNilType(t typ.Type) bool {
	t = unwrap.Annotated(t)
	return t != nil && t.Kind() == typ.Nil.Kind()
}
