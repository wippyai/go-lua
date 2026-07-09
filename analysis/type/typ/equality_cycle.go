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

// typePairSet tracks pairs already visited by recursive structural equality.
// Most type comparisons are shallow and acyclic, so retain their first pairs
// inline and allocate the overflow map only for unusually deep products.
type typePairSet struct {
	inline   [32]typePair
	inlineN  uint8
	overflow map[typePair]struct{}
}

// seenOrAdd reports whether pair was already present and otherwise records it.
func (s *typePairSet) seenOrAdd(pair typePair) bool {
	for i := range s.inlineN {
		if s.inline[i] == pair {
			return true
		}
	}
	if s.inlineN < uint8(len(s.inline)) {
		s.inline[s.inlineN] = pair
		s.inlineN++
		return false
	}
	if s.overflow == nil {
		s.overflow = make(map[typePair]struct{})
	}
	if _, ok := s.overflow[pair]; ok {
		return true
	}
	s.overflow[pair] = struct{}{}
	return false
}
