package callboundary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

type numFloorFactKey pathdom.PathKey

type numFloorFactLane struct{}

// numFloorLane is a must-fact lane for lower bounds proven on normal return.
// It cannot use the generic factset.Set directly: same-key collisions keep the
// strongest local duplicate during normalization, while must-joins keep only
// shared keys and weaken the surviving floor to min(left, right).
var numFloorLane numFloorFactLane

func (numFloorFactLane) Normalize(in []NumFloorFact) []NumFloorFact {
	return normalizeNumFloorFacts(in, true)
}

func (numFloorFactLane) NormalizeOwned(in []NumFloorFact) []NumFloorFact {
	return normalizeNumFloorFacts(in, false)
}

func normalizeNumFloorFacts(in []NumFloorFact, clone bool) []NumFloorFact {
	if len(in) == 0 {
		return nil
	}
	byPath := make(map[numFloorFactKey]NumFloorFact, len(in))
	for _, fact := range in {
		if !fact.Path.IsPlaceholder() {
			continue
		}
		if clone {
			fact.Path = fact.Path.Clone()
		}
		key := numFloorKeyOf(fact)
		if kept, ok := byPath[key]; ok && kept.Floor >= fact.Floor {
			continue
		}
		byPath[key] = fact
	}
	if len(byPath) == 0 {
		return nil
	}
	out := make([]NumFloorFact, 0, len(byPath))
	for _, fact := range byPath {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Path.Equal(out[j].Path) {
			return out[i].Path.Less(out[j].Path)
		}
		return out[i].Floor < out[j].Floor
	})
	return out
}

func (numFloorFactLane) Clone(in []NumFloorFact) []NumFloorFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]NumFloorFact, len(in))
	for i, fact := range in {
		out[i] = NumFloorFact{
			Path:  fact.Path.Clone(),
			Floor: fact.Floor,
		}
	}
	return out
}

func (lane numFloorFactLane) Equal(a, b []NumFloorFact) bool {
	if len(a) == len(b) {
		same := true
		for i := range a {
			if numFloorKeyOf(a[i]) != numFloorKeyOf(b[i]) || a[i].Floor != b[i].Floor {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	a = lane.Normalize(a)
	b = lane.Normalize(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if numFloorKeyOf(a[i]) != numFloorKeyOf(b[i]) || a[i].Floor != b[i].Floor {
			return false
		}
	}
	return true
}

func (lane numFloorFactLane) LessOrEq(a, b []NumFloorFact) bool {
	leftFloors := numFloorMaxFloorByPath(a)
	rightFloors := numFloorMaxFloorByPath(b)
	if len(rightFloors) == 0 {
		return true
	}
	for key, rightFloor := range rightFloors {
		leftFloor, ok := leftFloors[key]
		if !ok || leftFloor < rightFloor {
			return false
		}
	}
	return true
}

func (lane numFloorFactLane) Join(a, b []NumFloorFact) []NumFloorFact {
	a = lane.Normalize(a)
	b = lane.Normalize(b)
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	right := make(map[numFloorFactKey]int64, len(b))
	for _, fact := range b {
		right[numFloorKeyOf(fact)] = fact.Floor
	}
	out := make([]NumFloorFact, 0, len(a))
	for _, left := range a {
		floor, ok := right[numFloorKeyOf(left)]
		if !ok {
			continue
		}
		if left.Floor < floor {
			floor = left.Floor
		}
		out = append(out, NumFloorFact{
			Path:  left.Path.Clone(),
			Floor: floor,
		})
	}
	return lane.Normalize(out)
}

func (lane numFloorFactLane) Widen(prev, next []NumFloorFact) []NumFloorFact {
	return lane.Join(prev, next)
}

func numFloorKeyOf(fact NumFloorFact) numFloorFactKey {
	return numFloorFactKey(fact.Path.Key())
}

func numFloorMaxFloorByPath(in []NumFloorFact) map[numFloorFactKey]int64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[numFloorFactKey]int64, len(in))
	for _, fact := range in {
		if !fact.Path.IsPlaceholder() {
			continue
		}
		key := numFloorKeyOf(fact)
		if kept, ok := out[key]; !ok || kept < fact.Floor {
			out[key] = fact.Floor
		}
	}
	return out
}
