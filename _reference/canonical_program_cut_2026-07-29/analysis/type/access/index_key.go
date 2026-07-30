package access

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (q *query) indexByKeyVariants(key typ.Type, depth int, mode indexMode, missingNil bool, cycle fieldResult, project func(typ.Type) fieldResult) fieldResult {
	visit := queryKey{op: 5, t: key, mode: mode}
	if !q.enter(visit) {
		return cycle
	}
	defer q.leave(visit)
	projectLeaf := func(leaf typ.Type) fieldResult {
		res := project(leaf)
		if !res.ok && mode == indexRuntime && missingNil {
			return fieldResult{t: typ.Nil, ok: true}
		}
		return res
	}
	return descendAccessWrappers(key, depth, nil, func() fieldResult {
		// Depth exhaustion must not silently deny what project would grant
		// (invariants.md Rule 1): hand the still-wrapped key to the leaf
		// projection so its own, independently depth-bounded logic (e.g. an
		// array runtime read's may-be-integer check) decides the outcome,
		// instead of collapsing an inconclusive key into "unresolved".
		return projectLeaf(key)
	}, func(key typ.Type, depth int) fieldResult {
		switch v := unwrap.Annotated(key).(type) {
		case *typ.Union:
			return q.indexKeyUnion(v, depth+1, mode, missingNil, project)
		default:
			return projectLeaf(key)
		}
	}, func(res fieldResult) fieldResult {
		if res.ok {
			res.nilable = true
		}
		return res
	})
}

func (q *query) indexKeyUnion(u *typ.Union, depth int, mode indexMode, missingNil bool, project func(typ.Type) fieldResult) fieldResult {
	if u == nil || len(u.Members) == 0 {
		return fieldResult{}
	}
	return distributeUnionResults(u.Members, depth, func(member typ.Type, depth int) fieldResult {
		return q.indexByKeyVariants(member, depth, mode, missingNil, fieldResult{ok: true}, project)
	}, func(typ.Type, int) bool {
		return mode == indexRuntime && missingNil
	})
}

func literalStringKey(t typ.Type) (string, bool) {
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return "", false
	}
	name, ok := lit.Value.(string)
	return name, ok
}

func literalIntKey(t typ.Type) (int64, bool) {
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		return 0, false
	}
	index, ok := lit.Value.(int64)
	return index, ok
}
