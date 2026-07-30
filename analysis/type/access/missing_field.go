package access

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (q *query) missingFieldReadsNil(t typ.Type, depth int, cycle bool) bool {
	if stopDepth(t, depth) {
		return false
	}
	visit := queryKey{op: 2, t: t}
	if !q.enter(visit) {
		return cycle
	}
	defer q.leave(visit)
	return descendAccessWrappers(t, depth, nil, falseThunk, func(t typ.Type, depth int) bool {
		switch v := unwrap.Annotated(t).(type) {
		case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple, *typ.Interface:
			return true
		case *typ.Union:
			if len(v.Members) == 0 {
				return false
			}
			for _, member := range v.Members {
				if !q.missingFieldReadsNil(member, depth+1, true) {
					return false
				}
			}
			return true
		case *typ.Intersection:
			for _, member := range v.Members {
				if q.missingFieldReadsNil(member, depth+1, false) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}, func(v bool) bool { return v })
}
