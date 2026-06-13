package typecall

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

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

type memberCallResult struct {
	t      typ.Type
	status MemberCallStatus
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
