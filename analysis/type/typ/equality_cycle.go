package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

func needsCycleCheck(k kind.Kind) bool {
	switch k {
	case kind.Union, kind.Intersection, kind.Record, kind.Function,
		kind.Generic, kind.Instantiated, kind.Interface, kind.Recursive,
		kind.TypeParam:
		return true
	}

	return false
}

type typePair struct {
	a uintptr
	b uintptr
}
