package access

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func indexInUnion(u *typ.Union, key typ.Type, depth int, mode indexMode) fieldResult {
	if u == nil || len(u.Members) == 0 {
		return fieldResult{}
	}
	return distributeUnion(u.Members, depth, func(member typ.Type, depth int) fieldResult {
		return indexDepth(member, key, depth, mode)
	})
}

func indexInIntersection(in *typ.Intersection, key typ.Type, depth int, mode indexMode) fieldResult {
	if in == nil {
		return fieldResult{}
	}
	return distributeIntersection(in.Members, depth, func(member typ.Type, depth int) (typ.Type, bool) {
		return indexAtDepth(member, key, depth, mode)
	})
}

func indexAtDepth(container typ.Type, key typ.Type, depth int, mode indexMode) (typ.Type, bool) {
	return indexDepth(container, key, depth, mode).materialize()
}
