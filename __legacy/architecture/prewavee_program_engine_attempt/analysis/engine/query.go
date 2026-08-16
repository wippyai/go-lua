package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
)

// Query is one sealed observation of a typed Factor at one exact Program
// occurrence. It is demand authority only: it contributes no Rule semantics,
// cannot inspect a live Solver, and projects only a completed immutable State.
type Query[K ~uint64, V any] struct {
	solver     *Solver
	factor     *Factor[K, V]
	candidate  link.Candidate
	shard      link.Shard
	term       program.Term
	key        K
	coordinate coordinate.Coordinate
	stateSlot  int
	resultSlot int
	bound      bool
}

type queryDeclaration struct {
	candidate   link.Candidate
	shard       link.Shard
	term        program.Term
	slot        func() (int, bool)
	bind        func(coordinate.Coordinate, int, int) bool
	materialize func(facts.Facts) (any, bool)
}

// DeclareQuery registers one typed observation in a shard's existing Entry
// activation before Solver.Seal.
func DeclareQuery[K ~uint64, V any](solver *Solver, factor *Factor[K, V], shard link.Shard, term program.Term, key K) (*Query[K, V], bool) {
	if solver == nil || solver.sealed || factor == nil || factor.solver != solver || !factor.admits(key) || shard == 0 || term == 0 || !solver.validEntryAnchor(shard, term) {
		return nil, false
	}
	return declareQuery(solver, factor, link.Candidate{}, shard, term, key)
}

// DeclareCandidateQuery registers an observation in one exact existing Link
// body Candidate. The Candidate is the structural invocation identity already
// sealed by Link; this constructor does not synthesize boundary transport,
// activation state, or a second coordinate vocabulary.
func DeclareCandidateQuery[K ~uint64, V any](solver *Solver, factor *Factor[K, V], candidate link.Candidate, shard link.Shard, term program.Term, key K) (*Query[K, V], bool) {
	if solver == nil || solver.sealed || factor == nil || factor.solver != solver || !factor.admits(key) || candidate == (link.Candidate{}) || shard == 0 || term == 0 || !solver.validCandidateAnchor(candidate, shard, term) {
		return nil, false
	}
	return declareQuery(solver, factor, candidate, shard, term, key)
}

func declareQuery[K ~uint64, V any](solver *Solver, factor *Factor[K, V], candidate link.Candidate, shard link.Shard, term program.Term, key K) (*Query[K, V], bool) {
	if solver == nil || factor == nil || factor.solver != solver {
		return nil, false
	}
	query := &Query[K, V]{solver: solver, factor: factor, candidate: candidate, shard: shard, term: term, key: key, stateSlot: -1, resultSlot: -1}
	solver.queries = append(solver.queries, queryDeclaration{
		candidate: candidate,
		shard:     shard,
		term:      term,
		slot: func() (int, bool) {
			return factor.slot, factor.slot >= 0
		},
		bind: func(coordinate coordinate.Coordinate, stateSlot, resultSlot int) bool {
			if !coordinate.Valid() || stateSlot < 0 || resultSlot < 0 {
				return false
			}
			// Recompilation after U\E growth must rebind the one existing
			// Query to the same sole Coordinate.  A different handle would be
			// a second demand identity and is rejected rather than silently
			// retargeting an observation.
			if query.bound {
				return query.coordinate == coordinate && query.stateSlot == stateSlot && query.resultSlot == resultSlot
			}
			query.coordinate = coordinate
			query.stateSlot = stateSlot
			query.resultSlot = resultSlot
			query.bound = true
			return true
		},
		materialize: func(root facts.Facts) (any, bool) {
			value, present := factor.binding.Summary(root, key)
			if !present {
				return nil, false
			}
			return value, true
		},
	})
	return query, true
}

// Read observes this Query's pre-materialized typed result. Publication folds
// every feasible support partition while the epoch-local Facts root is live;
// State retains only this immutable value, never Facts, an FDD, a guard, or a
// carrier projection. A pruned or unavailable result is never converted to a
// Factor default or successful diagnostic evidence.
func (query *Query[K, V]) Read(state *State) (V, bool) {
	var zero V
	if query == nil || !query.bound || query.solver == nil || query.factor == nil || query.factor.solver != query.solver || !query.solver.validState(state) {
		return zero, false
	}
	result, ok := state.result(query.stateSlot, query.resultSlot, query.coordinate)
	if !ok || !result.present {
		return zero, false
	}
	value, ok := result.value.(V)
	if !ok {
		return zero, false
	}
	return value, true
}

func (solver *Solver) validateQueries() bool {
	if solver == nil {
		return false
	}
	for _, declaration := range solver.queries {
		valid := solver.validEntryAnchor(declaration.shard, declaration.term)
		if declaration.candidate != (link.Candidate{}) {
			valid = solver.validCandidateAnchor(declaration.candidate, declaration.shard, declaration.term)
		}
		if declaration.slot == nil || declaration.bind == nil || declaration.materialize == nil || !valid {
			return false
		}
		if _, ok := declaration.slot(); !ok {
			return false
		}
	}
	return true
}
