// Package engine owns the public composition boundary of the formal
// analyzer.  It exposes typed Factors, Rules, immutable States, and Queries;
// storage, dependency indexes, schedules, and coordinates remain private.
package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/analysis/engine/internal/dependency"
	"github.com/wippyai/go-lua/analysis/engine/internal/factor"
	"github.com/wippyai/go-lua/analysis/engine/internal/fiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program/artifact"
)

// KeySpace is one Factor's sealed, finite direct dependency-key universe.
// End denotes the exact universe [0, End). It is a semantic bound, never a
// resource budget, iteration limit, or approximation. A domain that needs
// dynamic or deep support encodes its canonical partitions and any support
// widening inside V; direct engine keys never grow during solving.
type KeySpace struct {
	End uint64
}

// Measure is a key-aware, well-founded termination witness. On every strict
// Widen or Narrow transition its lexicographic value must descend.
// Width is fixed at Factor declaration; At must be pure and deterministic.
type Measure[K ~uint64, V any] struct {
	Width int
	At    func(key K, value V, component int) uint64
}

// SemanticKey is the one cache identity vocabulary shared with Program
// artifacts.  There is deliberately no engine-local mirror.
type SemanticKey = artifact.SemanticKey

func availableSemanticKey(key SemanticKey) bool { return key.ID.Available() && key.Version != 0 }

// FactorConfig declares the complete immutable semantic contract for one
// typed Factor.  Factor order is the composition's sealed schema order: the
// same composition must declare Factors deterministically, and changing that
// ordered semantic identity vector invalidates derived engine artifacts.  A
// Factor is global domain state, so Program shard and Term belong to Rule
// endpoints rather than Factor identity.
//
// Fingerprint is required for collision-tolerant semantic terminal sharing.
// Equal remains the authority: equal values must fingerprint equally, but a
// matching fingerprint never proves equality by itself.
type FactorConfig[K ~uint64, V any] struct {
	Keys KeySpace
	// Semantic identifies this Factor's complete domain semantics and revision.
	// Equation artifacts persist this identity only—not Factor values, evidence,
	// or a value codec. An unavailable key simply makes this Factor ineligible
	// for a persisted equation cache; it never creates a guessed fallback
	// identity.
	Semantic SemanticKey

	Lattice     lattice.Lattice[V]
	Default     V
	Fingerprint func(V) uint64
	// WidenRank proves strict Widen transitions descend at a compiled Mu. It
	// may be omitted only when this Factor occurs in no cyclic compiled tuple;
	// Seal rejects a cycle containing an unranked Factor before any Rule runs.
	WidenRank Measure[K, V]
	// NarrowRank is required exactly when Lattice.Narrow is supplied. A
	// recurrence SCC enters narrowing only when every sealed Factor has both.
	NarrowRank Measure[K, V]
	// Formal declares that this Factor's Rule-produced body columns are already
	// expressed in the canonical formal namespace of a selected Body. Only
	// such columns may be replayed from a read-projected body specialization.
	//
	// This is a domain declaration, not an engine guess about values. Factors
	// that retain application-local allocation, cell, continuation, or other
	// structural freshness keep the zero default and are always preserved from
	// the current Candidate fiber on a specialization hit.
	Formal bool
}

// Factor is an owner-bound typed abstract map.  It is a declaration and a
// Query subject, not a mutable value store: only Rule execution can write it,
// and only Query can observe a completed State.
type Factor[K ~uint64, V any] struct {
	solver      *Solver
	arena       *factor.Arena[K, V]
	join        func(V, V) V
	admits      func(K) bool
	formal      bool
	same        func(V, V) bool
	fingerprint func(V) uint64
	semantic    SemanticKey

	// binding is assigned exactly once while Solver seals declarations in
	// canonical semantic order.  It is never exposed as a Factor identity.
	binding fiber.Binding[coordinate.Coordinate, K, V]
	bound   bool
	slot    int
	live    *factorRuntime[K, V]
}

type factorDeclaration struct {
	bind             func(*fiber.Bank) bool
	semantic         func() SemanticKey
	initial          func(*guard.Manager) (stateSlot, bool)
	joinContribution func(*transaction, *fiber.Draft, fiber.Leaf, fiber.Leaf) bool
	changed          func(*transaction, coordinate.Coordinate, guard.Guard, fiber.Leaf, fiber.Leaf) (bool, bool)
	widen            func(*transaction, coordinate.Coordinate, guard.Guard, *fiber.Draft, fiber.Leaf, fiber.Leaf) bool
	narrow           func(*transaction, coordinate.Coordinate, guard.Guard, *fiber.Draft, fiber.Leaf, fiber.Leaf) bool
	lessOrEq         func(*transaction, fiber.Leaf, fiber.Leaf) (bool, bool)
}

// DeclareFactor registers one typed Factor before Solver.Seal.  The returned
// capability is accepted only by its exact Solver.  The declaration performs
// no activation, Rule registration, storage publication, or evaluation.
func DeclareFactor[K ~uint64, V any](solver *Solver, config FactorConfig[K, V]) (*Factor[K, V], bool) {
	if solver == nil || solver.sealed {
		return nil, false
	}
	arena, ok := factor.New(factor.Config[K, V]{
		KeyRange:    factor.KeyRange{End: config.Keys.End},
		Lattice:     config.Lattice,
		Default:     config.Default,
		Fingerprint: config.Fingerprint,
		WidenRank: factor.Measure[K, V]{
			Width: config.WidenRank.Width,
			At:    config.WidenRank.At,
		},
		NarrowRank: factor.Measure[K, V]{
			Width: config.NarrowRank.Width,
			At:    config.NarrowRank.At,
		},
	})
	if !ok {
		return nil, false
	}
	hasWidenRank := config.WidenRank.Width > 0 && config.WidenRank.At != nil
	order := len(solver.factors)
	result := &Factor[K, V]{
		solver: solver,
		arena:  arena,
		join:   config.Lattice.Join,
		formal: config.Formal,
		same: func(left, right V) bool {
			return config.Lattice.Same != nil && config.Lattice.Same(left, right) || config.Lattice.Equal(left, right)
		},
		fingerprint: config.Fingerprint,
		semantic:    config.Semantic,
		admits:      func(key K) bool { return uint64(key) < config.Keys.End },
		slot:        -1,
	}
	var narrow func(*transaction, coordinate.Coordinate, guard.Guard, *fiber.Draft, fiber.Leaf, fiber.Leaf) bool
	if config.Lattice.Narrow != nil {
		narrow = func(transaction *transaction, coordinate coordinate.Coordinate, condition guard.Guard, draft *fiber.Draft, left, right fiber.Leaf) bool {
			return transaction != nil && result.live != nil && result.live.transaction == transaction && result.binding.Narrow(result.live.scratch, coordinate, condition, draft, left, right)
		}
	}
	var widen func(*transaction, coordinate.Coordinate, guard.Guard, *fiber.Draft, fiber.Leaf, fiber.Leaf) bool
	if hasWidenRank {
		widen = func(transaction *transaction, coordinate coordinate.Coordinate, condition guard.Guard, draft *fiber.Draft, left, right fiber.Leaf) bool {
			return transaction != nil && result.live != nil && result.live.transaction == transaction && result.binding.Widen(result.live.scratch, coordinate, condition, draft, left, right)
		}
	}
	solver.factors = append(solver.factors, factorDeclaration{
		bind: func(bank *fiber.Bank) bool {
			binding, valid := fiber.Bind[coordinate.Coordinate](bank, arena)
			if !valid || result.bound {
				return false
			}
			result.binding = binding
			result.bound = true
			result.slot = order
			return true
		},
		semantic: func() SemanticKey { return result.semantic },
		initial: func(guards *guard.Manager) (stateSlot, bool) {
			index, valid := dependency.New[coordinate.Coordinate](arena, guards)
			if !valid {
				return stateSlot{}, false
			}
			return factorSlot(result, index), true
		},
		joinContribution: func(transaction *transaction, draft *fiber.Draft, left, right fiber.Leaf) bool {
			return transaction != nil && result.live != nil && result.live.transaction == transaction && result.binding.JoinContribution(result.live.scratch, draft, left, right)
		},
		changed: func(transaction *transaction, coordinate coordinate.Coordinate, condition guard.Guard, left, right fiber.Leaf) (bool, bool) {
			if transaction == nil || result.live == nil || result.live.transaction != transaction {
				return false, false
			}
			return result.binding.Changed(result.live.scratch, coordinate, condition, left, right)
		},
		widen:  widen,
		narrow: narrow,
		lessOrEq: func(transaction *transaction, left, right fiber.Leaf) (bool, bool) {
			if transaction == nil || result.live == nil || result.live.transaction != transaction {
				return false, false
			}
			return result.binding.LessOrEq(result.live.scratch, left, right)
		},
	})
	return result, true
}
