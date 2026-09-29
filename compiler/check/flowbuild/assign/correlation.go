package assign

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

type ReturnCorrelation struct {
	ValueIndex, ErrorIndex int
	ValueTruthy            bool
}

type GuardedTypeCorrelation struct {
	GuardIndex, TargetIndex int
	GuardOnTruthy           bool
	TargetType              typ.Type
}

// returnRelation encodes each connected return group as its two reachable
// nil-state cases. A group has two cases regardless of its number of slots.
func returnRelation(paths []constraint.Path, inverse, together []ReturnCorrelation, guarded []GuardedTypeCorrelation) constraint.Condition {
	parent := make([]int, len(paths))
	parity := make([]bool, len(paths))
	used := make([]bool, len(paths))
	errorRoot := make([]bool, len(paths))
	for i := range parent {
		parent[i] = i
	}
	var root func(int) (int, bool)
	root = func(i int) (int, bool) {
		if parent[i] == i {
			return i, false
		}
		r, p := root(parent[i])
		parity[i] = parity[i] != p
		parent[i] = r
		return r, parity[i]
	}
	add := func(a, b int, opposite bool) bool {
		if a < 0 || b < 0 || a >= len(paths) || b >= len(paths) || paths[a].IsEmpty() || paths[b].IsEmpty() {
			return true
		}
		used[a], used[b] = true, true
		ra, pa := root(a)
		rb, pb := root(b)
		if ra == rb {
			return (pa != pb) == opposite
		}
		parent[rb] = ra
		parity[rb] = pa != pb != opposite
		errorRoot[ra] = errorRoot[ra] || errorRoot[rb] || opposite
		return true
	}
	for _, pair := range inverse {
		if !add(pair.ErrorIndex, pair.ValueIndex, true) {
			return constraint.TrueCondition()
		}
	}
	for _, pair := range together {
		if !add(pair.ValueIndex, pair.ErrorIndex, false) {
			return constraint.TrueCondition()
		}
	}

	result := constraint.TrueCondition()
	for i := range paths {
		r, _ := root(i)
		if r != i || !used[i] {
			continue
		}
		var present, absent []constraint.Constraint
		for j := range paths {
			memberRoot, flipped := root(j)
			if !used[j] || memberRoot != i {
				continue
			}
			if j == i && errorRoot[i] {
				present = append(present, constraint.Truthy{Path: paths[j]})
				absent = append(absent, constraint.Falsy{Path: paths[j]})
			} else if flipped {
				present = append(present, constraint.IsNil{Path: paths[j]})
				absent = append(absent, constraint.NotNil{Path: paths[j]})
			} else {
				present = append(present, constraint.NotNil{Path: paths[j]})
				absent = append(absent, constraint.IsNil{Path: paths[j]})
			}
		}
		result = constraint.And(result, constraint.Or(constraint.FromConstraints(present...), constraint.FromConstraints(absent...)))
	}
	for _, pair := range inverse {
		if !pair.ValueTruthy || pair.ValueIndex < 0 || pair.ErrorIndex < 0 || pair.ValueIndex >= len(paths) || pair.ErrorIndex >= len(paths) || paths[pair.ValueIndex].IsEmpty() || paths[pair.ErrorIndex].IsEmpty() {
			continue
		}
		result = constraint.And(result, constraint.Or(
			constraint.FromConstraints(constraint.Truthy{Path: paths[pair.ValueIndex]}, constraint.IsNil{Path: paths[pair.ErrorIndex]}),
			constraint.FromConstraints(constraint.Falsy{Path: paths[pair.ValueIndex]}, constraint.Truthy{Path: paths[pair.ErrorIndex]}),
		))
	}
	for _, relation := range guarded {
		if relation.GuardIndex < 0 || relation.TargetIndex < 0 || relation.GuardIndex >= len(paths) || relation.TargetIndex >= len(paths) || paths[relation.GuardIndex].IsEmpty() || paths[relation.TargetIndex].IsEmpty() || relation.TargetType == nil {
			continue
		}
		fallback := constraint.FromConstraints(constraint.Falsy{Path: paths[relation.GuardIndex]})
		if !relation.GuardOnTruthy {
			fallback = constraint.FromConstraints(constraint.Truthy{Path: paths[relation.GuardIndex]})
		}
		target := constraint.FromConstraints(constraint.HasType{Path: paths[relation.TargetIndex], Type: narrow.HashTypeKey(relation.TargetType.Hash())})
		result = constraint.And(result, constraint.Or(fallback, target))
	}
	return result
}
