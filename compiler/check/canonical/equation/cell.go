// Package equation builds the combined equation graph of the single canonical
// intraprocedural fixed point and solves it over the one generic worklist in
// types/lattice/solver.
//
// The locked design (Forge journal c735f2d1, seqs #349/#353/#360/#363/#365) is
// a SINGLE fixed point over FunctionState = Points x Contracts, where the two
// halves co-iterate in ONE worklist with ONE convergence test:
//
//   - Points: ordinary forward dataflow. A point cell reads its predecessors'
//     states, applies the per-node transfer function, and emits to its
//     successors.
//   - Contracts: a co-solved DEMAND component that flows BACKWARD to entry. A
//     body use that constrains parameter i emits an obligation into the cell
//     for parameter i; the entry point reads that cell as the value a caller
//     must supply. This is the bidirectional crux: forward value flow and
//     backward demand flow converge together.
//
// Because both halves live in one solver, the widening site is the
// feedback-vertex set of the COMBINED graph: the CFG loop-header FVS (from
// propagate.FeedbackVertexSet) for point cells, plus parameter contract cells
// that can close the entry->body->contract->entry demand cycle. The generic
// solver exact-joins a widening cell's initial fan-in before the cell's first
// transfer visit, then applies delayed widening only to continuing post-visit
// growth, preserving one-shot demand precision without a fake discovery pass.
//
// This package is a clean isolated leaf. It does not touch the legacy flow; the
// real per-node transfer is supplied as an injected NodeTransfer so the sound
// transfer (extracted from types/flow/transfer.go later) plugs in without any
// change here.
package equation

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/lattice"
)

// CellKind discriminates the two cell families of the combined equation graph.
type CellKind uint8

const (
	// PointCell is a per-CFG-point cell carrying a flow.PointState. Its Point
	// field is the CFG point; Param is unused.
	PointCell CellKind = iota
	// ContractCell is a per-parameter cell carrying a paramevidence.ParamContract
	// (the accumulated demand on one parameter). Its Param field is the parameter
	// index; Point is unused.
	ContractCell
)

// Cell is a vertex of the combined equation graph: either a CFG point or a
// parameter-contract slot. It is comparable so the generic solver can key its
// internal maps. The two families never alias — a point cell and a contract cell
// with the same numeric fields are distinct because Kind differs.
type Cell struct {
	Kind  CellKind
	Point cfg.Point
	Param int
}

// pointCellAt is the cell of CFG point p.
func pointCellAt(p cfg.Point) Cell { return Cell{Kind: PointCell, Point: p} }

// contractCellAt is the cell of parameter index i.
func contractCellAt(i int) Cell { return Cell{Kind: ContractCell, Param: i} }

// CellState is the discriminated carrier the solver ranges over: a cell of kind
// PointCell holds a flow.PointState in Point, a cell of kind ContractCell holds
// a paramevidence.ParamContract in Contract. Kind names which field is live.
//
// This is a sum, not a uniform PointState encoding: a point cell's state and a
// contract cell's state are elements of DIFFERENT lattices
// (flow.PointStateDomain vs paramevidence.ParamContractDomain). Carrying them in
// one struct and dispatching the lattice operations by Kind keeps each half in
// its native domain, lets the result assemble directly into
// state.FunctionState{Points, Contracts} with no projection, and needs no
// adapter type. Encoding a contract's product.AbstractValue inside a synthetic
// point's Env would force a parallel key convention and lose the direct
// assembly, so the sum is the honest model.
type CellState struct {
	Kind     CellKind
	Point    flow.PointState
	Contract paramevidence.ParamContract
}

// pointState wraps a PointState as a point-kind CellState.
func pointState(ps flow.PointState) CellState {
	return CellState{Kind: PointCell, Point: ps}
}

// contractState wraps a ParamContract as a contract-kind CellState.
func contractState(c paramevidence.ParamContract) CellState {
	return CellState{Kind: ContractCell, Contract: c}
}

// CellStateDomain is the lattice over CellState. Each operation dispatches BY
// KIND to the kind-specific component domain: a point cell delegates to
// flow.PointStateDomain, a contract cell to paramevidence.ParamContractDomain.
//
// The solver only ever combines two states destined for the same fixed cell,
// whose kind is fixed by construction, so a same-kind combine is the invariant.
// A mixed-kind combine is an internal wiring bug, not a lattice value, so it
// panics rather than silently producing a meaningless join. Bottom/Top are
// kind-parametric and are produced by Initial(cell); the field-less
// CellStateDomain.Bottom()/Top() cannot know a kind, so they are not used by the
// solver (which seeds via Initial) and panic to make any misuse loud.
var CellStateDomain = lattice.Lattice[CellState]{
	Bottom: func() CellState {
		panic("equation: CellStateDomain.Bottom is kind-parametric; seed cells via Initial")
	},
	Top: func() CellState {
		panic("equation: CellStateDomain.Top is kind-parametric; no kindless Top exists")
	},
	Equal: func(a, b CellState) bool {
		requireSameKind(a, b)
		if a.Kind == PointCell {
			return flow.PointStateDomain.Equal(a.Point, b.Point)
		}
		return paramevidence.ParamContractDomain.Equal(a.Contract, b.Contract)
	},
	LessOrEq: func(a, b CellState) bool {
		requireSameKind(a, b)
		if a.Kind == PointCell {
			return flow.PointStateDomain.LessOrEq(a.Point, b.Point)
		}
		return paramevidence.ParamContractDomain.LessOrEq(a.Contract, b.Contract)
	},
	Join: func(a, b CellState) CellState {
		requireSameKind(a, b)
		if a.Kind == PointCell {
			return pointState(flow.PointStateDomain.Join(a.Point, b.Point))
		}
		return contractState(paramevidence.ParamContractDomain.Join(a.Contract, b.Contract))
	},
	// Meet is nil: both component domains are forward-only (PointState and the
	// value-product contract carrier have no greatest lower bound), so the sum
	// has none either, mirroring FunctionStateDomain.
	Meet: nil,
	Widen: func(prev, next CellState) CellState {
		requireSameKind(prev, next)
		if prev.Kind == PointCell {
			return pointState(flow.PointStateDomain.Widen(prev.Point, next.Point))
		}
		return contractState(paramevidence.ParamContractDomain.Widen(prev.Contract, next.Contract))
	},
}

// requireSameKind enforces the same-kind combine invariant. The solver always
// joins/widens a contribution into the cell it targets, whose kind is fixed, so
// a mismatch is a builder bug.
func requireSameKind(a, b CellState) {
	if a.Kind != b.Kind {
		panic("equation: mixed-kind CellState combine (point vs contract); equation-graph wiring bug")
	}
}

// initialFor returns the kind-specific Bottom for a cell, used as the solver's
// Initial seed.
func initialFor(c Cell) CellState {
	switch c.Kind {
	case PointCell:
		return pointState(flow.PointStateDomain.Bottom())
	case ContractCell:
		return contractState(paramevidence.ParamContractDomain.Bottom())
	default:
		panic("equation: unknown CellKind")
	}
}
