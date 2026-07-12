package summary

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

func normalizeFreshHeapAllocations(in []identity.ID) []identity.ID {
	if len(in) == 0 {
		return nil
	}
	out := append([]identity.ID(nil), in...)
	sort.Slice(out, func(i, j int) bool { return identityIDLess(out[i], out[j]) })
	n := 0
	for _, id := range out {
		if id == (identity.ID{}) || n != 0 && out[n-1] == id {
			continue
		}
		out[n] = id
		n++
	}
	if n == 0 {
		return nil
	}
	return out[:n]
}

func identityIDLess(a, b identity.ID) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Site != b.Site {
		return a.Site < b.Site
	}
	return a.Index < b.Index
}

func freshHeapAllocationsEqual(a, b []identity.ID) bool {
	a = normalizeFreshHeapAllocations(a)
	b = normalizeFreshHeapAllocations(b)
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

func freshHeapAllocationsLessOrEq(a, b []identity.ID) bool {
	a = normalizeFreshHeapAllocations(a)
	b = normalizeFreshHeapAllocations(b)
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			i++
			j++
			continue
		}
		if identityIDLess(a[i], b[j]) {
			return false
		}
		j++
	}
	return i == len(a)
}

func joinFreshHeapAllocations(a, b []identity.ID) []identity.ID {
	if len(a) == 0 {
		return normalizeFreshHeapAllocations(b)
	}
	if len(b) == 0 {
		return normalizeFreshHeapAllocations(a)
	}
	return normalizeFreshHeapAllocations(append(append(make([]identity.ID, 0, len(a)+len(b)), a...), b...))
}
