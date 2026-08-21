// runtime_region_newton.go seals the closure basis of one linear recurrence
// Region: the saturated set of its own back-transport powers.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// regionNewton is one Region's sealed closure basis. Its members are the
// distinct relations reachable by composing the Region's back transports one
// or more times, each already carrying the Region's support-axis discharge.
// The empty composition is deliberately absent: it is the Region's own
// constant part, which a head fold already carries as its base, so the basis
// holds exactly the terms that base has to be joined with.
//
// The basis is complete for the Region's operator: because the members are
// closed under composition, every finite iteration of that operator is one of
// them, and joining the constant part moved through all of them is the whole
// climb rather than one step of it. It is sealed only where that claim is
// exact - a Region whose head has no back Group producer, so its recurrence
// is transport alone, and whose publication widens no Factor, so nothing
// outside the transports moves the head.
//
// A Region that does not meet those conditions, or whose transports do not
// return to their own coordinate interface, or whose powers do not settle
// inside the operator's own dimension, simply carries no basis. That is the
// complete meaning of an unavailable basis: Newton does not run for that
// Region and its exact solver path is untouched. Nothing is capped, budgeted,
// or approximated.
type regionNewton struct {
	basis []carrier.ReindexPlan
	// rounds is the number of squaring rounds the saturation needed. Zero
	// means the atom set was already closed and nothing was composed.
	rounds int
}

func (newton regionNewton) available() bool { return len(newton.basis) != 0 }

// saturateRegionNewton closes one atom set under composition by repeated
// squaring: every round composes each member with each member, so the
// reachable power doubles per round and the saturation settles in the
// logarithm of the monoid it generates rather than its size. Members are
// deduped by their published relation identity, so two ways of reaching one
// relation contribute one basis member.
//
// The rounds are bounded by the square of the operator's own dimension, its
// source-coordinate width. Exceeding that bound yields no basis at all: the
// derivation refuses rather than returning a truncated closure, because a
// truncated closure would be an unsound acceleration and a partial answer is
// worth less than none.
func saturateRegionNewton(runtime *carrier.Composition, atoms []carrier.ReindexPlan, late bool) (regionNewton, bool) {
	if runtime == nil || len(atoms) == 0 {
		return regionNewton{}, false
	}
	basis := make([]carrier.ReindexPlan, 0, len(atoms))
	seen := make(map[guard.FormulaID]struct{}, len(atoms))
	admit := func(plan carrier.ReindexPlan) bool {
		if !plan.Valid() || !plan.SelfComposable() {
			return false
		}
		key, keyed := plan.RelationIdentity()
		if !keyed {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return true
		}
		seen[key] = struct{}{}
		basis = append(basis, plan)
		return true
	}
	width, coordinateIdentity := 0, true
	for _, atom := range atoms {
		if !admit(atom) {
			return regionNewton{}, false
		}
		if atom.CoordinateCount() > width {
			width = atom.CoordinateCount()
		}
		coordinateIdentity = coordinateIdentity && atom.CoordinateIdentity()
	}
	if len(basis) == 0 || width == 0 {
		return regionNewton{}, false
	}
	// First collapse: a relation that preserves every coordinate composes with
	// anything that preserves every coordinate to a relation that preserves
	// every coordinate, and on one interface there is only one such relation.
	// The atom set is therefore already its own closure.
	if coordinateIdentity {
		return regionNewton{basis: basis, rounds: 0}, true
	}
	for round := 1; round <= width*width; round++ {
		members := len(basis)
		for left := 0; left < members; left++ {
			for right := 0; right < members; right++ {
				composed, composedOK := composeRegionNewton(runtime, basis[left], basis[right], late)
				if !composedOK || !admit(composed) {
					return regionNewton{}, false
				}
			}
		}
		// Second collapse and the general exit are the same structural fact: a
		// round that reaches no new relation has closed the set. A relation
		// that absorbs its own square settles here on the first round.
		if len(basis) == members {
			return regionNewton{basis: basis, rounds: round}, true
		}
	}
	return regionNewton{}, false
}

func composeRegionNewton(runtime *carrier.Composition, first, second carrier.ReindexPlan, late bool) (carrier.ReindexPlan, bool) {
	if late {
		return runtime.ComposeRuntimeReindex(first, second)
	}
	return runtime.ComposeReindex(first, second)
}

// regionNewtonAddressable is the exact class the closure basis is sealed for.
// Both halves are read from already-sealed rows: the immutable WTO
// classification says whether the head's recurrence is transport alone, and
// the sealed publication scope says whether that recurrence widens a Factor.
// Neither is a heuristic and neither is a measure.
func regionNewtonAddressable(region equation.RegionView, widen carrier.MergeScope) bool {
	return region.BackHeadProducerCount() == 0 && widen.FactorFree()
}

// sealRegionNewton derives one Region's closure basis at bind time. It
// returns an unavailable basis, not a failure, for every Region outside the
// addressable class: those Regions keep exactly the solver path they have.
// It fails only when the sealed rows it reads are inconsistent, which is the
// same contract every other Region binding in this file carries.
func sealRegionNewton(graph *equation.Graph, region equation.RegionView, head equation.Point, runtime *carrier.Composition, plans runtimeReindexes, widen carrier.MergeScope, discharge regionDischarge, late bool) (regionNewton, bool) {
	if graph == nil || runtime == nil || !head.Available() {
		return regionNewton{}, false
	}
	if !regionNewtonAddressable(region, widen) {
		return regionNewton{}, true
	}
	atoms, adjoined, atomsOK := regionNewtonAtoms(graph, region, head, plans, discharge, late, runtime)
	if !atomsOK {
		return regionNewton{}, false
	}
	if !adjoined || len(atoms) == 0 {
		return regionNewton{}, true
	}
	newton, saturated := saturateRegionNewton(runtime, atoms, late)
	if !saturated {
		return regionNewton{}, true
	}
	return newton, true
}

// regionNewtonAtoms lowers the Region's back ingress into the generators of
// its closure. A Region in the addressable class closes its head through
// transports only, so its generators are exactly the back environment edges,
// the head self-transports the immutable recurrence graph keeps beside them,
// and the back Factor edges - the same rows the binder classifies as back
// ingress. Each generator is the transport followed by the Region's
// support-axis discharge, because the head publishes the discharged value on
// every iteration and the closure must describe that same operator.
//
// The second result is adjunction: false means one transport does not adjoin
// the head's own discharge, so it is not a generator of this Region's
// operator and the Region leaves the addressable class rather than
// contributing a wrong generator. The third result is consistency of the
// sealed rows themselves, which is a binding failure.
func regionNewtonAtoms(graph *equation.Graph, region equation.RegionView, head equation.Point, plans runtimeReindexes, discharge regionDischarge, late bool, runtime *carrier.Composition) ([]carrier.ReindexPlan, bool, bool) {
	atoms := make([]carrier.ReindexPlan, 0, region.BackEnvironmentEdgeCount()+region.BackFactorEdgeCount())
	// appendAtom reports adjunction and row consistency in that order.
	appendAtom := func(input equation.Input) (bool, bool) {
		plan, planOK := plans.plan(input.Reindex())
		if !planOK || !plan.Valid() {
			return false, false
		}
		if !discharge.available() {
			atoms = append(atoms, plan)
			return true, true
		}
		discharged, composed := composeRegionNewton(runtime, plan, discharge.plan, late)
		if !composed {
			return false, true
		}
		atoms = append(atoms, discharged)
		return true, true
	}
	for index := 0; index < region.BackEnvironmentEdgeCount(); index++ {
		edge, edgeOK := region.BackEnvironmentEdgeAt(index)
		if !edgeOK {
			return nil, false, false
		}
		adjoined, consistent := appendAtom(edge.Input())
		if !consistent {
			return nil, false, false
		}
		if !adjoined {
			return nil, false, true
		}
	}
	for edgeIndex := 0; edgeIndex < graph.EnvironmentEdgeTotal(); edgeIndex++ {
		edge, edgeOK := graph.EnvironmentEdgeAtIndex(edgeIndex)
		if !edgeOK {
			return nil, false, false
		}
		if !edge.TransportOnly() || edge.Target() != head {
			continue
		}
		adjoined, consistent := appendAtom(edge.Input())
		if !consistent {
			return nil, false, false
		}
		if !adjoined {
			return nil, false, true
		}
	}
	for index := 0; index < region.BackFactorEdgeCount(); index++ {
		edge, edgeOK := region.BackFactorEdgeAt(index)
		if !edgeOK {
			return nil, false, false
		}
		adjoined, consistent := appendAtom(edge.Input())
		if !consistent {
			return nil, false, false
		}
		if !adjoined {
			return nil, false, true
		}
	}
	return atoms, true, true
}
