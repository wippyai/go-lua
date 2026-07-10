package access

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func missingFieldReadsNilDepth(t typ.Type, depth int) bool {
	if stopDepth(t, depth) {
		return false
	}
	return descendAccessWrappers(t, depth, func(t typ.Type, depth int) bool {
		switch v := unwrap.Annotated(t).(type) {
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
		default:
			return false
		}
	}, func(v bool) bool { return v })
}
