package sibling

import (
	"reflect"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// HasPhiCandidate reports whether a correlated call result reaches a phi.
// Most functions with phi nodes do not need another flow solve.
func HasPhiCandidate(graph *cfg.Graph, inputs *flow.Inputs) bool {
	if graph == nil || inputs == nil || len(inputs.SiblingAssignments) == 0 {
		return false
	}
	for _, phi := range graph.PhiNodes() {
		for _, op := range phi.Operands {
			source := inputs.SiblingAssignments[flow.SiblingKey{Symbol: phi.Target.Symbol, VersionID: op.Version.ID}]
			if source != nil && (len(source.Correlations) > 0 || len(source.CoCorrelations) > 0 || len(source.GuardedCorrelations) > 0) {
				return true
			}
		}
	}
	return false
}

// PropagatePhi keeps a multi-return relationship when the same pair of
// variables is joined from corresponding assignments on every live incoming
// edge. A pair with an unrelated write on any live edge is not correlated.
func PropagatePhi(graph *cfg.Graph, inputs *flow.Inputs, solution *flow.Solution) bool {
	if graph == nil || inputs == nil || solution == nil || len(inputs.SiblingAssignments) == 0 {
		return false
	}
	phis := graph.PhiNodes()
	byPoint := make(map[cfg.Point]map[cfg.SymbolID]*cfg.PhiInfo)
	for i := range phis {
		phi := &phis[i]
		if byPoint[phi.Point] == nil {
			byPoint[phi.Point] = make(map[cfg.SymbolID]*cfg.PhiInfo)
		}
		byPoint[phi.Point][phi.Target.Symbol] = phi
	}
	dead := make(map[cfg.Point]bool)
	checked := make(map[cfg.Point]bool)
	impossible := func(p cfg.Point) bool {
		if !checked[p] {
			checked[p] = true
			dead[p] = literalConditionImpossibleAt(solution, inputs, p)
			if dead[p] {
				if inputs.DeadPoints == nil {
					inputs.DeadPoints = make(map[cfg.Point]bool)
				}
				inputs.DeadPoints[p] = true
			}
		}
		return dead[p]
	}
	changed := false
	for pass := 0; pass < len(phis); pass++ {
		added := false
		for i := range phis {
			phi := &phis[i]
			key := flow.SiblingKey{Symbol: phi.Target.Symbol, VersionID: phi.Target.ID}
			if inputs.SiblingAssignments[key] != nil {
				continue
			}
			for _, op := range phi.Operands {
				if impossible(op.From) {
					continue
				}
				source := inputs.SiblingAssignments[flow.SiblingKey{Symbol: phi.Target.Symbol, VersionID: op.Version.ID}]
				if source == nil {
					continue
				}
				for _, partner := range source.Symbols {
					if partner == 0 || partner == phi.Target.Symbol {
						continue
					}
					other := byPoint[phi.Point][partner]
					if other == nil || !matchingPhiPair(phi, other, inputs, impossible) {
						continue
					}
					joined := &flow.SiblingAssignment{
						Symbols:             append([]cfg.SymbolID(nil), source.Symbols...),
						Names:               append([]string(nil), source.Names...),
						Correlations:        append([]flow.ReturnCorrelation(nil), source.Correlations...),
						CoCorrelations:      append([]flow.ReturnCorrelation(nil), source.CoCorrelations...),
						GuardedCorrelations: append([]flow.GuardedTypeCorrelation(nil), source.GuardedCorrelations...),
					}
					inputs.SiblingAssignments[key] = joined
					inputs.SiblingAssignments[flow.SiblingKey{Symbol: partner, VersionID: other.Target.ID}] = joined
					added, changed = true, true
					break
				}
				if added {
					break
				}
			}
		}
		if !added {
			break
		}
	}
	return changed
}

func matchingPhiPair(a, b *cfg.PhiInfo, inputs *flow.Inputs, impossible func(cfg.Point) bool) bool {
	if a == nil || b == nil || a.Point != b.Point || len(a.Operands) != len(b.Operands) {
		return false
	}
	partnerByFrom := make(map[cfg.Point]cfg.PhiOperand, len(b.Operands))
	for _, op := range b.Operands {
		partnerByFrom[op.From] = op
	}
	var common *flow.SiblingAssignment
	live := 0
	for _, op := range a.Operands {
		if impossible(op.From) {
			continue
		}
		partnerOp, ok := partnerByFrom[op.From]
		if !ok {
			return false
		}
		left := inputs.SiblingAssignments[flow.SiblingKey{Symbol: a.Target.Symbol, VersionID: op.Version.ID}]
		right := inputs.SiblingAssignments[flow.SiblingKey{Symbol: b.Target.Symbol, VersionID: partnerOp.Version.ID}]
		if left == nil || left != right {
			return false
		}
		if common == nil {
			common = left
		} else if !sameCorrelations(common, left) {
			return false
		}
		live++
	}
	return live > 0
}

func sameCorrelations(a, b *flow.SiblingAssignment) bool {
	return reflect.DeepEqual(a.Symbols, b.Symbols) &&
		reflect.DeepEqual(a.Correlations, b.Correlations) &&
		reflect.DeepEqual(a.CoCorrelations, b.CoCorrelations) &&
		reflect.DeepEqual(a.GuardedCorrelations, b.GuardedCorrelations)
}

// literalConditionImpossibleAt recognizes an exhausted finite literal domain.
// It uses the type before the guard, so it can detect an impossible path even
// when normal narrowing conservatively preserves the old type at Never.
func literalConditionImpossibleAt(solution *flow.Solution, inputs *flow.Inputs, p cfg.Point) bool {
	condition := solution.ConditionAt(p)
	if condition.IsFalse() {
		return true
	}
	for i := 0; i < condition.NumDisjuncts(); i++ {
		byPath := make(map[constraint.PathKey]typ.Type)
		impossible := false
		for _, c := range condition.DisjunctConstraints(i) {
			var path constraint.Path
			var apply func(typ.Type) typ.Type
			switch v := c.(type) {
			case constraint.Truthy:
				path, apply = v.Path, narrow.ToTruthy
			case constraint.Falsy:
				path, apply = v.Path, narrow.ToFalsy
			case constraint.NotHasType:
				if v.Type.Kind != narrow.TypeKeyHash {
					continue
				}
				excluded := inputs.TypeKeys[v.Type.Hash]
				if _, ok := excluded.(*typ.Literal); !ok {
					continue
				}
				path = v.Path
				apply = func(t typ.Type) typ.Type { return narrow.ExcludeType(t, excluded) }
			default:
				continue
			}
			if path.IsEmpty() {
				continue
			}
			key := path.Key()
			current, found := byPath[key]
			if !found {
				current = solution.TypeAt(p, path)
			}
			if current == nil || current.Kind().IsPlaceholder() {
				continue
			}
			current = apply(current)
			byPath[key] = current
			if typ.IsNever(current) {
				impossible = true
				break
			}
		}
		if !impossible {
			return false
		}
	}
	return condition.NumDisjuncts() > 0
}
