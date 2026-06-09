package equation

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/propagate"
)

// NodeTransfer is the injected per-node transfer function.
// It is the single seam between this structural equation graph and the sound
// value/condition/numeric transfer supplied by compiler/check/canonical/transfer.
// This package never implements it; the builder takes one and drives it.
//
// Transfer computes the abstract state that holds AT point p, given the joined
// state arriving from p's predecessors (incoming), and reports the obligations
// that p's node imposes on the function's parameters via the supplied demand
// sink. The CFG topology, the predecessor join, the emission to successors, the
// routing of demand into contract cells, and the entry point's reading of the
// contracts are all owned by the builder; the NodeTransfer only describes the
// local node semantics.
//
// Contract:
//   - incoming is the least upper bound of all predecessor point states already
//     computed this iteration (PointStateDomain.Bottom at the entry, before the
//     entry's assumed-contract injection).
//   - entryContracts is the current per-parameter demand assumed at function
//     entry. The builder supplies it only to the entry point transfer, which
//     folds it into the state it returns (it is what a caller is assumed to
//     supply). Non-entry points receive an empty contract map; contract influence
//     reaches them only through the entry out-state and ordinary CFG flow.
//   - demand(i, c) records that p constrains parameter i with contract c. The
//     builder Joins c into ContractCell(i). Calling demand for i<0 is ignored.
//   - The returned PointState is the value of point p; the builder emits it to
//     every successor of p.
//
// Transfer must be monotone in incoming and, at entry, entryContracts so the
// combined system has a least fixed point; the lattice laws then make Solve
// terminate.
type NodeTransfer interface {
	Transfer(
		g *cfg.Graph,
		p cfg.Point,
		incoming flow.PointState,
		entryContracts paramevidence.Contracts,
		entryFacts flow.BoundaryFacts,
		demand func(param int, c paramevidence.ParamContract),
	) flow.PointState
}

// EntryValueSeeder is the optional seam for entry-point values that are already
// elements of the product domain. The equation graph owns when these values
// enter the point-state fixed point; the concrete transfer owns how a parameter
// slot maps to the function's symbol store (Env vs captured Cells).
//
// These are not parameter contracts. Contracts are backward demand facts joined
// into the entry assumption; entry values are forward seeds from caller evidence
// or immutable topology. Declared annotations remain authoritative.
type EntryValueSeeder interface {
	SeedEntryValues(out *flow.PointState, values map[int]product.AbstractValue)
}

// EntryFactSeeder is the optional seam for parameter-relative path facts that
// are already finite flow proofs at the call boundary. The equation graph owns
// when these facts enter the point-state fixed point; the concrete transfer owns
// how parameter-relative boundary paths become this function's symbol paths.
//
// These are forward proof facts, not parameter contracts and not diagnostic
// suppressors. They must enter before the local transfer reads parameter paths so
// ordinary assignment/mutation kill logic can invalidate them inside the body.
type EntryFactSeeder interface {
	SeedEntryFacts(out *flow.PointState, facts flow.BoundaryFacts)
}

// EntrySymbolValueSeeder is the optional seam for entry-point values keyed by
// stable symbols rather than parameter slots. It is used for immutable
// scope/fact-derived bindings such as callback-scoped globals that must enter the
// product state before the local transfer runs. These values are forward facts,
// not declarations injected into the diagnostic bridge after solving.
type EntrySymbolValueSeeder interface {
	SeedEntrySymbolValues(out *flow.PointState, values map[cfg.SymbolID]product.AbstractValue)
}

// EdgeNarrower is the optional per-edge refinement of the flow: the
// path-sensitive narrowing a branch guard proves about its tested value on one of
// its successor edges. It is the second seam between this structural equation graph
// and the sound value-domain narrowing; a NodeTransfer that also implements it
// opts the builder into per-successor narrowing, otherwise the builder joins
// predecessor states unrefined (the prior path-insensitive behavior).
//
// NarrowEdge refines the out-state of predecessor point pred for the successor
// reached by the edge pred -> succ, returning the narrowed state the builder joins
// into succ's incoming. When pred is a branch, the TRUE successor edge carries the
// guard and the FALSE edge its negation; the join at a merge point then drops a
// branch's narrowing (the env-domain LUB recovers the unnarrowed value), so a
// narrowing never survives past its guard. A pred that is not a branch, or an edge
// whose guard the narrower cannot interpret, returns out unchanged.
//
// NarrowEdge must be monotone in out so the combined system keeps its least fixed
// point; the value-domain narrowing primitives it reuses are monotone, so the
// per-edge refinement is.
type EdgeNarrower interface {
	NarrowEdge(g *cfg.Graph, pred, succ cfg.Point, out flow.PointState) flow.PointState
}

// ConditionProjectorProvider optionally supplies the point-condition relevance
// abstraction for the combined solver. The builder applies it as a cell-local
// upper closure after solver Join/Widen and around the local transfer, so
// PointState.Cond cannot retain dead guard vocabulary through the generic
// Kildall accumulator.
type ConditionProjectorProvider interface {
	ConditionProjector() *propagate.ConditionProjector
}

// NodeTransferFunc adapts a plain function to NodeTransfer for callers (and
// tests) that do not need a method receiver.
type NodeTransferFunc func(
	g *cfg.Graph,
	p cfg.Point,
	incoming flow.PointState,
	entryContracts paramevidence.Contracts,
	entryFacts flow.BoundaryFacts,
	demand func(param int, c paramevidence.ParamContract),
) flow.PointState

// Transfer implements NodeTransfer.
func (f NodeTransferFunc) Transfer(
	g *cfg.Graph,
	p cfg.Point,
	incoming flow.PointState,
	entryContracts paramevidence.Contracts,
	entryFacts flow.BoundaryFacts,
	demand func(param int, c paramevidence.ParamContract),
) flow.PointState {
	return f(g, p, incoming, entryContracts, entryFacts, demand)
}
