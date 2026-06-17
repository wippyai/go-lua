package access

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// distributeUnion applies query to each union member and recombines the results
// as evidence: any member that fails non-nilably fails the whole access, a
// member that is merely missing-reads-nil contributes nil, and the surviving
// field types union together. It is the canonical union access-distribution.
func distributeUnion(members []typ.Type, depth int, query func(member typ.Type, depth int) fieldResult) fieldResult {
	out := make([]typ.Type, 0, len(members))
	nilable := false
	for _, member := range members {
		res := query(member, depth+1)
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
	return fieldResult{t: normalize.UnionForEvidence(out...), ok: true, nilable: nilable}
}

// distributeIntersection applies query to each intersection member and meets the
// successful results. It is the canonical intersection access-distribution.
func distributeIntersection(members []typ.Type, depth int, query func(member typ.Type, depth int) (typ.Type, bool)) fieldResult {
	out := make([]typ.Type, 0, len(members))
	for _, member := range members {
		if t, ok := query(member, depth+1); ok {
			out = append(out, t)
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
