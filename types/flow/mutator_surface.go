package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// RootMutationPoint is the flow-owned projection of any transfer operator that
// mutates a root value. Consumers that only need the solved root surface should
// depend on this product instead of enumerating concrete mutator lanes.
type RootMutationPoint struct {
	Point  cfg.Point
	Target constraint.Path
}

// RootMutationPoints returns canonical mutation points for roots touched by
// map or table mutator transfer operators.
func (in *Inputs) RootMutationPoints(symbols map[cfg.SymbolID]bool) []RootMutationPoint {
	if in == nil {
		return nil
	}
	out := make([]RootMutationPoint, 0, len(in.MapMutatorAssignments)+len(in.TableMutatorAssignments))
	seen := make(map[rootMutationPointKey]struct{})
	add := func(point cfg.Point, target constraint.Path) {
		if point == 0 || target.Symbol == 0 {
			return
		}
		if symbols != nil && !symbols[target.Symbol] {
			return
		}
		key := rootMutationPointKey{point: point, target: target.Key()}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, RootMutationPoint{Point: point, Target: target})
	}
	for _, mutation := range in.MapMutatorAssignments {
		add(mutation.Point, mutation.Target)
	}
	for _, mutation := range in.TableMutatorAssignments {
		add(mutation.Point, mutation.Target)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Point != out[j].Point {
			return out[i].Point < out[j].Point
		}
		return out[i].Target.Less(out[j].Target)
	})
	return out
}

// TransferObservationPoints returns CFG points where transfer may have changed
// solved value state. Consumers that project solved parent state for captured
// environments use these points instead of enumerating concrete transfer lanes:
// alias congruence can update a captured root when the syntactic assignment
// target is a different root.
func (in *Inputs) TransferObservationPoints() []cfg.Point {
	if in == nil {
		return nil
	}
	seen := make(map[cfg.Point]struct{}, len(in.Assignments)+len(in.MapMutatorAssignments)+len(in.TableMutatorAssignments))
	add := func(point cfg.Point) {
		if point == 0 {
			return
		}
		seen[point] = struct{}{}
	}
	for _, assignment := range in.Assignments {
		add(assignment.Point)
	}
	for _, mutation := range in.MapMutatorAssignments {
		add(mutation.Point)
	}
	for _, mutation := range in.TableMutatorAssignments {
		add(mutation.Point)
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]cfg.Point, 0, len(seen))
	for point := range seen {
		out = append(out, point)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

type rootMutationPointKey struct {
	point  cfg.Point
	target constraint.PathKey
}
