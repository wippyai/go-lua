package summary

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

func normalizeFreshHeapAllocations(in []FreshHeapAllocation) []FreshHeapAllocation {
	if len(in) == 0 {
		return nil
	}
	out := append([]FreshHeapAllocation(nil), in...)
	sort.Slice(out, func(i, j int) bool { return identityIDLess(out[i].ID, out[j].ID) })
	n := 0
	for _, allocation := range out {
		if allocation.ID == (identity.ID{}) || allocation.Placement == placement.Bottom {
			continue
		}
		if n != 0 && out[n-1].ID == allocation.ID {
			out[n-1].Placement = placement.Join(out[n-1].Placement, allocation.Placement)
			continue
		}
		out[n] = allocation
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

func freshHeapAllocationsEqual(a, b []FreshHeapAllocation) bool {
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

func freshHeapAllocationsLessOrEq(a, b []FreshHeapAllocation) bool {
	a = normalizeFreshHeapAllocations(a)
	b = normalizeFreshHeapAllocations(b)
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i].ID == b[j].ID {
			if !placement.LessOrEq(a[i].Placement, b[j].Placement) {
				return false
			}
			i++
			j++
			continue
		}
		if identityIDLess(a[i].ID, b[j].ID) {
			return false
		}
		j++
	}
	return i == len(a)
}

func joinFreshHeapAllocations(a, b []FreshHeapAllocation) []FreshHeapAllocation {
	if len(a) == 0 {
		return normalizeFreshHeapAllocations(b)
	}
	if len(b) == 0 {
		return normalizeFreshHeapAllocations(a)
	}
	return normalizeFreshHeapAllocations(append(append(make([]FreshHeapAllocation, 0, len(a)+len(b)), a...), b...))
}
