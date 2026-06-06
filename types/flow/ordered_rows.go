package flow

import "sort"

// orderedRows* helpers are the small algebra for canonical finite-row carriers:
// sorted rows, identity lookup, subset, and must-set intersection. Domain files
// still own row validity and payload merging; this only removes repeated ordered
// set mechanics from each lattice literal.
func orderedRowsEqual[T any](a, b []T, equal func(T, T) bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func orderedRowsFind[T any](entries []T, fact T, less func(T, T) bool, same func(T, T) bool) (int, bool) {
	i := sort.Search(len(entries), func(i int) bool {
		return !less(entries[i], fact)
	})
	return i, i < len(entries) && same(entries[i], fact)
}

func orderedRowsContainAll[T any](have, want []T, less func(T, T) bool, same func(T, T) bool) bool {
	for _, w := range want {
		if _, ok := orderedRowsFind(have, w, less, same); !ok {
			return false
		}
	}
	return true
}

func orderedRowsIntersect[T any](a, b []T, less func(T, T) bool, same func(T, T) bool) []T {
	var out []T
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case less(a[i], b[j]):
			i++
		case less(b[j], a[i]):
			j++
		default:
			if same(a[i], b[j]) {
				out = append(out, a[i])
			}
			i++
			j++
		}
	}
	return out
}
