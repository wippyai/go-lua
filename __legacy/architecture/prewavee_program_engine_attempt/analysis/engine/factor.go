// Package engine owns the public composition boundary of the formal
// analyzer.  It exposes typed Factors, Rules, immutable States, and Queries;
// storage, dependency indexes, schedules, and coordinates remain private.
package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/program/artifact"
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
}

// Factor is an owner-bound typed abstract map.  It is a declaration and a
// Query subject, not a mutable value store: only Rule execution can write it,
// and only Query can observe a completed State.
type Factor[K ~uint64, V any] struct {
	solver   *Solver
	keys     KeySpace
	semantic SemanticKey

	// binding is replaced only at a successful active-epoch commit. It owns
	// this Factor's typed semantic plane; Factor itself owns no mutable root,
	// vector position, or carrier callback.
	binding factbinding.Binding[K, V]
	slot    int
}

type factorDeclaration struct {
	stageFacts   func(*facts.Schema, *guard.Manager) (stagedFactorBinding, bool)
	semantic     func() SemanticKey
	slot         func() (int, bool)
	hasWidenRank func() bool
	hasNarrow    func() bool
}

// stagedFactorBinding is one private, speculative typed plane installation.
// Building a candidate epoch must not mutate Factor.binding: only the epoch's
// final commit cut adopts this replacement after every dependent compile step
// has succeeded. It owns no Facts root beyond the candidate schema.
type stagedFactorBinding struct {
	initial func(facts.Facts) (facts.Facts, bool)
	commit  func()
}

// DeclareFactor registers one typed Factor before Solver.Seal.  The returned
// capability is accepted only by its exact Solver.  The declaration performs
// no activation, Rule registration, storage publication, or evaluation.
func DeclareFactor[K ~uint64, V any](solver *Solver, config FactorConfig[K, V]) (*Factor[K, V], bool) {
	if solver == nil || solver.sealed || !validFactorConfig(config) {
		return nil, false
	}
	binding, ok := factbinding.New(factbinding.Config[K, V]{
		KeyEnd:      config.Keys.End,
		Default:     config.Default,
		Equal:       config.Lattice.Equal,
		Same:        config.Lattice.Same,
		Fingerprint: config.Fingerprint,
		Join:        config.Lattice.Join,
		Widen:       config.Lattice.Widen,
		Narrow:      config.Lattice.Narrow,
		LessOrEq:    config.Lattice.LessOrEq,
		WidenRank: factbinding.Measure[K, V]{
			Width: config.WidenRank.Width,
			At:    config.WidenRank.At,
		},
		NarrowRank: factbinding.Measure[K, V]{
			Width: config.NarrowRank.Width,
			At:    config.NarrowRank.At,
		},
	})
	if !ok {
		return nil, false
	}
	order := len(solver.factors)
	result := &Factor[K, V]{
		solver:   solver,
		keys:     config.Keys,
		semantic: config.Semantic,
		binding:  binding,
		// Slot is structural declaration order, fixed before any speculative
		// epoch. Rule/query validation must not require an active plane before
		// that epoch has passed every compile gate.
		slot: order,
	}
	solver.factors = append(solver.factors, factorDeclaration{
		stageFacts: func(schema *facts.Schema, guards *guard.Manager) (stagedFactorBinding, bool) {
			candidate := result.binding.Fresh()
			if !candidate.BindFacts(schema, guards) {
				return stagedFactorBinding{}, false
			}
			return stagedFactorBinding{
				initial: candidate.Initial,
				commit: func() {
					result.binding = candidate
					result.slot = order
				},
			}, true
		},
		semantic: func() SemanticKey { return result.semantic },
		slot:     func() (int, bool) { return result.slot, result.slot >= 0 },
		// These are declaration-time proof eligibility facts only. Facts-native
		// joint algebra performs the actual value transition; compile must never
		// recover a Factor lattice or physical store to decide a recurrence.
		hasWidenRank: func() bool { return config.WidenRank.Width > 0 && config.WidenRank.At != nil },
		hasNarrow:    func() bool { return config.Lattice.Narrow != nil },
	})
	return result, true
}

// validFactorConfig preserves the complete domain contract at the one
// declaration boundary. Facts replaces the old physical Factor arena, not its
// algebraic admission laws: absence still denotes Default, and rank witnesses
// remain mandatory exactly where FactorConfig says they are.
func validFactorConfig[K ~uint64, V any](config FactorConfig[K, V]) bool {
	values := config.Lattice
	if values.Bottom == nil || values.Top == nil || values.Equal == nil || values.LessOrEq == nil || values.Join == nil || values.Widen == nil || config.Fingerprint == nil {
		return false
	}
	validRank := func(rank Measure[K, V]) bool {
		return rank.Width == 0 && rank.At == nil || rank.Width > 0 && rank.At != nil
	}
	if !validRank(config.WidenRank) || !validRank(config.NarrowRank) || values.Narrow != nil && (config.NarrowRank.Width == 0 || config.NarrowRank.At == nil) || values.Narrow == nil && (config.NarrowRank.Width != 0 || config.NarrowRank.At != nil) {
		return false
	}
	same := func(left, right V) bool {
		return values.Same != nil && values.Same(left, right) || values.Equal(left, right)
	}
	defaultJoin, defaultWiden := values.Join(config.Default, config.Default), values.Widen(config.Default, config.Default)
	if !same(defaultJoin, config.Default) || !same(defaultWiden, config.Default) || !values.LessOrEq(config.Default, defaultJoin) || !values.LessOrEq(config.Default, defaultWiden) {
		return false
	}
	if values.Narrow != nil {
		defaultNarrow := values.Narrow(config.Default, config.Default)
		if !same(defaultNarrow, config.Default) || !values.LessOrEq(config.Default, defaultNarrow) || !values.LessOrEq(defaultNarrow, config.Default) {
			return false
		}
	}
	return true
}

func (factor *Factor[K, V]) admits(key K) bool {
	return factor != nil && uint64(key) < factor.keys.End
}
