package access

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (q *query) indexInUnion(u *typ.Union, key typ.Type, depth int, mode indexMode) fieldResult {
	if u == nil || len(u.Members) == 0 {
		return fieldResult{}
	}
	return q.distributeUnion(u.Members, depth, func(member typ.Type, depth int) fieldResult {
		return q.index(member, key, depth, mode, fieldResult{ok: true})
	})
}

func (q *query) indexInIntersection(in *typ.Intersection, key typ.Type, depth int, mode indexMode) fieldResult {
	if in == nil {
		return fieldResult{}
	}
	return distributeIntersection(in.Members, depth, func(member typ.Type, depth int) (typ.Type, bool) {
		return q.indexAtDepth(member, key, depth, mode)
	})
}

func (q *query) indexAtDepth(container typ.Type, key typ.Type, depth int, mode indexMode) (typ.Type, bool) {
	return q.index(container, key, depth, mode, fieldResult{}).materialize()
}
