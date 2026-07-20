package callboundary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

type numCeilFactKey pathdom.PathKey

type numCeilFactLane struct{}

// numCeilLane is a must-fact lane for upper bounds proven on normal return.
// It cannot use the generic factset.Set directly: same-key collisions keep the
// strongest local duplicate during normalization, while must-joins keep only
// shared keys and weaken the surviving ceiling to max(left, right). This
// inverts the floor lane's min-join: a numeric upper bound is more precise
// when smaller, so the weaker (larger) common ceiling is the one both
// branches guarantee.
var numCeilLane numCeilFactLane

func (numCeilFactLane) Normalize(in []NumCeilFact) []NumCeilFact {
	return normalizeNumCeilFacts(in, true)
}

func (numCeilFactLane) NormalizeOwned(in []NumCeilFact) []NumCeilFact {
	return normalizeNumCeilFacts(in, false)
}

func normalizeNumCeilFacts(in []NumCeilFact, clone bool) []NumCeilFact {
	if len(in) == 0 {
		return nil
	}
	byPath := make(map[numCeilFactKey]NumCeilFact, len(in))
	for _, fact := range in {
		if !fact.Path.IsPlaceholder() {
			continue
		}
		if clone {
			fact.Path = fact.Path.Clone()
		}
		key := numCeilKeyOf(fact)
		if kept, ok := byPath[key]; ok && kept.Ceil <= fact.Ceil {
			continue
		}
		byPath[key] = fact
	}
	if len(byPath) == 0 {
		return nil
	}
	out := make([]NumCeilFact, 0, len(byPath))
	for _, fact := range byPath {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Path.Equal(out[j].Path) {
			return out[i].Path.Less(out[j].Path)
		}
		return out[i].Ceil < out[j].Ceil
	})
	return out
}

func (numCeilFactLane) Clone(in []NumCeilFact) []NumCeilFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]NumCeilFact, len(in))
	for i, fact := range in {
		out[i] = NumCeilFact{
			Path: fact.Path.Clone(),
			Ceil: fact.Ceil,
		}
	}
	return out
}

func (lane numCeilFactLane) Equal(a, b []NumCeilFact) bool {
	if len(a) == len(b) {
		same := true
		for i := range a {
			if numCeilKeyOf(a[i]) != numCeilKeyOf(b[i]) || a[i].Ceil != b[i].Ceil {
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
		if numCeilKeyOf(a[i]) != numCeilKeyOf(b[i]) || a[i].Ceil != b[i].Ceil {
			return false
		}
	}
	return true
}

func (lane numCeilFactLane) LessOrEq(a, b []NumCeilFact) bool {
	leftCeils := numCeilMinCeilByPath(a)
	rightCeils := numCeilMinCeilByPath(b)
	if len(rightCeils) == 0 {
		return true
	}
	for key, rightCeil := range rightCeils {
		leftCeil, ok := leftCeils[key]
		if !ok || leftCeil > rightCeil {
			return false
		}
	}
	return true
}

func (lane numCeilFactLane) Join(a, b []NumCeilFact) []NumCeilFact {
	a = lane.Normalize(a)
	b = lane.Normalize(b)
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	right := make(map[numCeilFactKey]int64, len(b))
	for _, fact := range b {
		right[numCeilKeyOf(fact)] = fact.Ceil
	}
	out := make([]NumCeilFact, 0, len(a))
	for _, left := range a {
		ceil, ok := right[numCeilKeyOf(left)]
		if !ok {
			continue
		}
		if left.Ceil > ceil {
			ceil = left.Ceil
		}
		out = append(out, NumCeilFact{
			Path: left.Path.Clone(),
			Ceil: ceil,
		})
	}
	return lane.Normalize(out)
}

func (lane numCeilFactLane) Widen(prev, next []NumCeilFact) []NumCeilFact {
	return lane.Join(prev, next)
}

func numCeilKeyOf(fact NumCeilFact) numCeilFactKey {
	return numCeilFactKey(fact.Path.Key())
}

func numCeilMinCeilByPath(in []NumCeilFact) map[numCeilFactKey]int64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[numCeilFactKey]int64, len(in))
	for _, fact := range in {
		if !fact.Path.IsPlaceholder() {
			continue
		}
		key := numCeilKeyOf(fact)
		if kept, ok := out[key]; !ok || kept > fact.Ceil {
			out[key] = fact.Ceil
		}
	}
	return out
}
