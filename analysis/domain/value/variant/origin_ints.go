package variant

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
)

// caseSelection is a concrete, non-retained adapter shared by the legacy slice
// API and the immutable hot-path view API. It deliberately uses neither an
// interface nor a callback iterator, so passing a caseset.View cannot box or
// allocate.
type caseSelection struct {
	values []int
	view   caseset.View
	viewed bool
}

func sliceCases(values []int) caseSelection { return caseSelection{values: values} }
func viewedCases(view caseset.View) caseSelection {
	return caseSelection{view: view, viewed: true}
}

func (s caseSelection) len() int {
	if s.viewed {
		return s.view.Len()
	}
	return len(s.values)
}

func (s caseSelection) at(i int) int {
	if s.viewed {
		return s.view.At(i)
	}
	return s.values[i]
}

func (s caseSelection) contains(value int) bool {
	if s.viewed {
		low, high := 0, s.view.Len()
		for low < high {
			middle := int(uint(low+high) >> 1)
			candidate := s.view.At(middle)
			if candidate < value {
				low = middle + 1
			} else {
				high = middle
			}
		}
		return low < s.view.Len() && s.view.At(low) == value
	}
	for _, candidate := range s.values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (s caseSelection) allKnown(cases []originCase) bool {
	for i := 0; i < s.len(); i++ {
		known := false
		for _, candidate := range cases {
			if candidate.index == s.at(i) {
				known = true
				break
			}
		}
		if !known {
			return false
		}
	}
	return true
}

func (s caseSelection) intersects(values []int) bool {
	for _, value := range values {
		if s.contains(value) {
			return true
		}
	}
	return false
}

func (s caseSelection) sameSet(values []int) bool {
	if !s.viewed {
		return sameIntSet(s.values, values)
	}
	if s.view.Len() != len(values) {
		return false
	}
	for i, value := range values {
		if s.view.At(i) != value {
			return false
		}
	}
	return true
}

func compactInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	sort.Ints(values)
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func sameIntSet(a, b []int) bool {
	a = compactInts(append([]int(nil), a...))
	b = compactInts(append([]int(nil), b...))
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
