package typevalue

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// DefinitelyNonEmptyIndexContainer reports whether a type proves that at least
// one positional/index value exists. Every union arm must carry the proof.
func DefinitelyNonEmptyIndexContainer(t typ.Type) bool {
	return definitelyNonEmptyIndexContainer(t, 0)
}

func definitelyNonEmptyIndexContainer(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Tuple:
		return len(tt.Elements) > 0
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !definitelyNonEmptyIndexContainer(member, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
