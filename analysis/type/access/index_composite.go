package access

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func indexInUnion(u *typ.Union, key typ.Type, depth int, mode indexMode) fieldResult {
	if u == nil || len(u.Members) == 0 {
		return fieldResult{}
	}
	out := make([]typ.Type, 0, len(u.Members))
	nilable := false

	for _, member := range u.Members {
		res := indexDepth(member, key, depth+1, mode)
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

func indexInIntersection(in *typ.Intersection, key typ.Type, depth int, mode indexMode) fieldResult {
	if in == nil {
		return fieldResult{}
	}
	out := make([]typ.Type, 0, len(in.Members))
	for _, member := range in.Members {
		if it, ok := indexAtDepth(member, key, depth+1, mode); ok {
			out = append(out, it)
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

func indexAtDepth(container typ.Type, key typ.Type, depth int, mode indexMode) (typ.Type, bool) {
	return indexDepth(container, key, depth, mode).materialize()
}
