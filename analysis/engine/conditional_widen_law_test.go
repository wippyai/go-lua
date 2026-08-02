package engine_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

// unrankedLocalLawFactor is lawful only on an acyclic compiled equation. Its
// ordinary lattice operations remain complete; it deliberately withholds the
// Mu-specific stabilization witness.
func unrankedLocalLawFactor(t *testing.T, solver *engine.Solver, id byte) *engine.Factor[uint64, localLawBits] {
	t.Helper()
	factor, ok := engine.DeclareFactor(solver, engine.FactorConfig[uint64, localLawBits]{
		Keys:        engine.KeySpace{End: 1},
		Semantic:    localLawSemantic(id),
		Lattice:     localLawLattice(),
		Default:     0,
		Fingerprint: func(value localLawBits) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("DeclareFactor rejected an unranked acyclic Factor")
	}
	return factor
}

// An unranked Factor needs no convergence proof when its demanded equation is
// acyclic: it is evaluated once and ordinary Rule transfer produces the source
// fact directly.
func TestUnrankedFactorAcyclicSourceSolves(t *testing.T) {
	solver, value, shard := localLawSolver(t, "")
	entry := localLawEntry(t, value)
	factor := unrankedLocalLawFactor(t, solver, 90)
	localLawDeclareAt(t, solver, factor, 91, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Set(0, localLawOne)
	})
	query := localLawQuery(t, solver, factor, shard, entry)

	if !solver.Seal() {
		t.Fatal("Seal rejected an acyclic unranked Factor")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solve rejected an acyclic unranked Factor")
	}
	if got := localLawRead(t, query, state); got != localLawOne {
		t.Fatalf("acyclic unranked source fact = %d, want %d", got, localLawOne)
	}
}

// Any local self-read makes the compiled equation cyclic even when Program
// control has no backedge. Seal must reject the absent Mu proof before a Rule
// callback can execute, and Solve must have no alternate execution path.
func TestUnrankedFactorCyclicEquationFailsClosedBeforeExecution(t *testing.T) {
	solver, value, shard := localLawSolver(t, "")
	entry := localLawEntry(t, value)
	factor := unrankedLocalLawFactor(t, solver, 92)
	ran := false
	var read engine.ReadRef[uint64, localLawBits]
	rule := localLawDeclareAt(t, solver, factor, 93, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		ran = true
		_, _, valid := engine.ReadAt(access, read, 0)
		return valid && access.Set(0, localLawOne)
	})
	var ok bool
	read, ok = engine.Read(rule, 0, factor)
	if !ok {
		t.Fatal("Read did not bind the unranked self dependency")
	}
	_ = localLawQuery(t, solver, factor, shard, entry)

	if solver.Seal() {
		t.Fatal("Seal accepted a cyclic tuple containing an unranked Factor")
	}
	if ran {
		t.Fatal("cyclic unranked Rule ran during Seal")
	}
	if state, ok := solver.Solve(context.Background(), nil); ok || state != nil {
		t.Fatalf("Solve executed an unranked cyclic equation: %p/%t", state, ok)
	}
	if ran {
		t.Fatal("cyclic unranked Rule ran during Solve")
	}
}

// Fiber values are complete Factor products. An unranked sibling therefore
// belongs to the same cyclic tuple even when the cyclic Rule reads and writes
// only the ranked column; Seal must not silently skip that column's Widen.
func TestCyclicTupleRejectsUnrankedSiblingFactor(t *testing.T) {
	solver, value, shard := localLawSolver(t, "")
	entry := localLawEntry(t, value)
	ranked := localLawFactor(t, solver, 94)
	_ = unrankedLocalLawFactor(t, solver, 95)
	var read engine.ReadRef[uint64, localLawBits]
	rule := localLawDeclareAt(t, solver, ranked, 96, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		_, _, valid := engine.ReadAt(access, read, 0)
		return valid && access.Set(0, localLawOne)
	})
	var ok bool
	read, ok = engine.Read(rule, 0, ranked)
	if !ok {
		t.Fatal("Read did not bind the ranked self dependency")
	}
	_ = localLawQuery(t, solver, ranked, shard, entry)

	if solver.Seal() {
		t.Fatal("Seal omitted an unranked sibling from a cyclic Factor tuple")
	}
}

// The same recurrence is accepted when the Factor supplies the checked
// widening measure, preserving the existing convergent Mu semantics.
func TestRankedFactorCyclicEquationStillConverges(t *testing.T) {
	solver, value, shard := localLawSolver(t, "")
	entry := localLawEntry(t, value)
	factor := localLawFactor(t, solver, 94)
	var read engine.ReadRef[uint64, localLawBits]
	rule := localLawDeclareAt(t, solver, factor, 95, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		prior, present, valid := engine.ReadAt(access, read, 0)
		if !valid {
			return false
		}
		next := localLawOne
		if present {
			next = prior | localLawTwo
		}
		return access.Set(0, next)
	})
	var ok bool
	read, ok = engine.Read(rule, 0, factor)
	if !ok {
		t.Fatal("Read did not bind the ranked self dependency")
	}
	query := localLawQuery(t, solver, factor, shard, entry)
	state := localLawSealAndSolve(t, solver)
	if got := localLawRead(t, query, state); got != localLawBoth {
		t.Fatalf("ranked cyclic fixed point = %d, want %d", got, localLawBoth)
	}
}
