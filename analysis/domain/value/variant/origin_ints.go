package variant

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
)

func containsCase(cases caseset.View, value int) bool {
	low, high := 0, cases.Len()
	for low < high {
		middle := int(uint(low+high) >> 1)
		candidate := cases.At(middle)
		if candidate < value {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low < cases.Len() && cases.At(low) == value
}

func allCasesKnown(selection caseset.View, cases []originCase) bool {
	for i := 0; i < selection.Len(); i++ {
		known := false
		for _, candidate := range cases {
			if candidate.index == selection.At(i) {
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

func casesIntersect(selection caseset.View, values []int) bool {
	for _, value := range values {
		if containsCase(selection, value) {
			return true
		}
	}
	return false
}

func sameCases(selection caseset.View, values []int) bool {
	if selection.Len() != len(values) {
		return false
	}
	for i, value := range values {
		if selection.At(i) != value {
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
