package typecall

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// memberCallDistributeUnion applies query to each union member (skipping nil and
// never arms) and unions the results: any arm that is not callable fails the
// whole call. It is the canonical union member-call distribution.
func memberCallDistributeUnion(members []typ.Type, depth int, query func(member typ.Type, depth int) memberCallResult) memberCallResult {
	out := make([]typ.Type, 0, len(members))
	checked := false
	for _, member := range members {
		if isNilType(member) || typ.IsNever(member) {
			continue
		}
		res := query(member, depth+1)
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

// witnessUnion applies query to each non-nil union member and requires every arm
// to yield the same callable witness; any mismatch or non-callable arm fails.
func witnessUnion(u *typ.Union, depth int, query func(member typ.Type, depth int) (*typ.Function, bool)) (*typ.Function, bool) {
	if u == nil || len(u.Members) == 0 {
		return nil, false
	}
	var witness *typ.Function
	for _, member := range u.Members {
		if isNilType(member) {
			continue
		}
		fn, ok := query(member, depth+1)
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

// memberCallDistributeIntersection applies query to each intersection member and
// meets the callable results. It is the canonical intersection member-call
// distribution.
func memberCallDistributeIntersection(members []typ.Type, depth int, query func(member typ.Type, depth int) memberCallResult) memberCallResult {
	out := make([]typ.Type, 0, len(members))
	for _, member := range members {
		res := query(member, depth+1)
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
