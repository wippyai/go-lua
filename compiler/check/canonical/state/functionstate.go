// Package state defines the carrier of the single canonical fixed point: the
// per-function abstract state ranged over by the generic lattice solver.
//
// The canonical flow computes ONE intraprocedural fixed point per function over
// FunctionState = Points x Contracts, a reduced product of two component
// domains:
//
//   - Points: the per-program-point states (env, path condition, relational
//     numeric), keyed by CFG point. This is ordinary forward dataflow.
//   - Contracts: the per-parameter contract demand. Body uses emit obligations
//     into the contract cell for their parameter; the entry point reads the
//     accumulated obligation as the value a caller must supply. This is a
//     co-solved DEMAND component flowing back to entry, not a nested layer:
//     one worklist and one convergence test range over both halves, so the
//     widening site is the feedback-vertex set of the COMBINED graph (CFG points
//     plus contract cells), per the locked design.
//
// FunctionStateDomain is the componentwise reduced product of the two halves'
// lattices. As with flow.PointStateDomain, the product is populated by direct
// componentwise delegation — no adapter type wraps the components — so it
// inherits their laws (monotone Join, ACC-under-Widen) by the product-of-
// lattices construction.
package state

import (
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/lattice"
	latticeproduct "github.com/wippyai/go-lua/types/lattice/product"
)

// FunctionState is the unified per-function abstract state of the single
// canonical fixed point.
type FunctionState struct {
	// Points maps each CFG point to its PointState. An absent point denotes
	// flow.PointStateDomain.Bottom() (no information), per MapLattice semantics.
	// A point's state is its OUT-state: the value LEAVING the point after its
	// node transfer (the form summary projection reads for return slots).
	Points map[cfg.Point]flow.PointState
	// Contracts maps each parameter index to its accumulated contract demand.
	// An absent index denotes no obligation (paramevidence.ParamContractDomain
	// Bottom). Body uses Join obligations in; the entry env reads them.
	Contracts paramevidence.Contracts

	// InPoints maps each reachable CFG point to its IN-state: the join over its
	// reachable predecessors' edge-narrowed out-states (the entry point's own
	// seeded state for the entry). It is DERIVED solver output the builder fills
	// from the solved cell map, not a lattice component: the FunctionStateDomain
	// Join/Widen/Equal/LessOrEq laws ignore it (a join of two states does not
	// recompute in-states), so it stays out of the convergence test and the law
	// suite. The diagnostic surface reads it as the single source of truth for a
	// point's entering state, so the edge-narrowing the solver applied at a merge
	// is observed exactly once, with no re-derivation. An absent point is
	// unreachable and yields the Bottom in-state.
	InPoints map[cfg.Point]flow.PointState
}

// pointsDomain lifts flow.PointStateDomain pointwise over CFG points: an absent
// point is PointState Bottom, Join/Widen are pointwise, and a point whose state
// is Bottom is canonicalized to absence.
var pointsDomain = latticeproduct.MapLattice[cfg.Point](flow.PointStateDomain)

// FunctionStateDomain is the abstract domain of FunctionState: the componentwise
// reduced product of pointsDomain and paramevidence.ContractDomain.
//
// Meet is nil: both component domains are forward-only (PointState via its value
// and numeric components, ContractDomain via the value product), so the product
// has no greatest lower bound and the LawSuite skips the meet-side laws.
var FunctionStateDomain = lattice.Lattice[FunctionState]{
	Bottom: func() FunctionState {
		return FunctionState{
			Points:    pointsDomain.Bottom(),
			Contracts: paramevidence.ContractDomain.Bottom(),
		}
	},
	Top: func() FunctionState {
		return FunctionState{
			Points:    pointsDomain.Top(),
			Contracts: paramevidence.ContractDomain.Top(),
		}
	},
	Equal: func(a, b FunctionState) bool {
		return pointsDomain.Equal(a.Points, b.Points) &&
			paramevidence.ContractDomain.Equal(a.Contracts, b.Contracts)
	},
	LessOrEq: func(a, b FunctionState) bool {
		return pointsDomain.LessOrEq(a.Points, b.Points) &&
			paramevidence.ContractDomain.LessOrEq(a.Contracts, b.Contracts)
	},
	Join: func(a, b FunctionState) FunctionState {
		return FunctionState{
			Points:    pointsDomain.Join(a.Points, b.Points),
			Contracts: paramevidence.ContractDomain.Join(a.Contracts, b.Contracts),
		}
	},
	Meet: nil,
	Widen: func(prev, next FunctionState) FunctionState {
		return FunctionState{
			Points:    pointsDomain.Widen(prev.Points, next.Points),
			Contracts: paramevidence.ContractDomain.Widen(prev.Contracts, next.Contracts),
		}
	},
}
