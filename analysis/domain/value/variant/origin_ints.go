package variant

import "sort"

func intSet(values []int) map[int]bool {
	out := make(map[int]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
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

func casesIntersect(cases []int, constraint map[int]bool) bool {
	for _, c := range cases {
		if constraint[c] {
			return true
		}
	}
	return false
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
