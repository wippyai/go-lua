package flow

import "sort"

// orderedRowIdentity is the private algebra for finite fact rows whose
// canonical order is their identity order. Domains still own payload validity
// and merging; this type owns only ordered lookup/subset/intersection mechanics.
type orderedRowIdentity[T any] struct {
	less func(T, T) bool
	same func(T, T) bool
}

func (id orderedRowIdentity[T]) EqualBy(a, b []T, equal func(T, T) bool) bool {
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

func (id orderedRowIdentity[T]) Equal(a, b []T) bool {
	return id.EqualBy(a, b, id.same)
}

func (id orderedRowIdentity[T]) Find(entries []T, row T) (int, bool) {
	i := sort.Search(len(entries), func(i int) bool {
		return !id.less(entries[i], row)
	})
	return i, i < len(entries) && id.same(entries[i], row)
}

func (id orderedRowIdentity[T]) ContainsAllBy(have, want []T, contains func(T, T) bool) bool {
	for _, w := range want {
		idx, ok := id.Find(have, w)
		if !ok || !contains(have[idx], w) {
			return false
		}
	}
	return true
}

func (id orderedRowIdentity[T]) ContainsAll(have, want []T) bool {
	return id.ContainsAllBy(have, want, id.same)
}

func (id orderedRowIdentity[T]) Intersect(a, b []T) []T {
	return id.MergeIntersect(a, b, func(left, _ T) (T, bool) {
		return left, true
	})
}

func (id orderedRowIdentity[T]) MergeIntersect(a, b []T, merge func(T, T) (T, bool)) []T {
	var out []T
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case id.less(a[i], b[j]):
			i++
		case id.less(b[j], a[i]):
			j++
		default:
			if row, ok := merge(a[i], b[j]); ok {
				out = append(out, row)
			}
			i++
			j++
		}
	}
	return out
}
