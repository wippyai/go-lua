package engine_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

func keySignatureFactor(t testing.TB, solver *engine.Solver, id byte, end uint64) *engine.Factor[uint64, localLawBits] {
	t.Helper()
	factor, ok := engine.DeclareFactor(solver, engine.FactorConfig[uint64, localLawBits]{
		Keys:        engine.KeySpace{End: end},
		Semantic:    localLawSemantic(id),
		Lattice:     localLawLattice(),
		Default:     0,
		Fingerprint: func(value localLawBits) uint64 { return uint64(value) },
		WidenRank: engine.Measure[uint64, localLawBits]{
			Width: 1,
			At: func(_ uint64, value localLawBits, _ int) uint64 {
				return uint64(localLawBoth - value)
			},
		},
	})
	if !ok {
		t.Fatal("DeclareFactor rejected direct-key signature Factor")
	}
	return factor
}

// Exact direct keys are the smallest static certificate that distinguishes a
// sibling-key data dependency from a recurrence.  A dynamic Rule remains
// conservative; it cannot use this exception.
func TestExactKeySignatureSeparatesSiblingKeysFromRecurrence(t *testing.T) {
	solver, value, shard := localLawSolver(t, "")
	at := localLawEntry(t, value)
	factor := keySignatureFactor(t, solver, 101, 2)
	producer := localLawDeclareAt(t, solver, factor, 102, shard, at, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Set(1, localLawOne)
	})
	if !engine.WriteExact(producer, 1) {
		t.Fatal("declare exact producer key")
	}
	var input engine.ReadRef[uint64, localLawBits]
	consumer := localLawDeclareAt(t, solver, factor, 103, shard, at, func(access engine.Access[uint64, localLawBits]) bool {
		got, _, valid := engine.ReadAt(access, input, 1)
		return valid && access.Set(0, got)
	})
	if !engine.WriteExact(consumer, 0) {
		t.Fatal("declare exact consumer key")
	}
	var ok bool
	input, ok = engine.ReadExact(consumer, 0, factor, 1)
	if !ok {
		t.Fatal("declare exact sibling read")
	}
	query, ok := engine.DeclareQuery(solver, factor, shard, at, 0)
	if !ok || !solver.Seal() {
		t.Fatal("disjoint exact keys formed a false recurrence")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok {
		t.Fatal("solve exact sibling keys")
	}
	got, present := query.Read(state)
	if !present || got != localLawOne {
		t.Fatalf("exact sibling result = %v/%v, want %v", got, present, localLawOne)
	}
}

func TestExactKeySignatureRetainsTrueUnwitnessedCycles(t *testing.T) {
	t.Run("same key self read", func(t *testing.T) {
		solver, value, shard := localLawSolver(t, "")
		at := localLawEntry(t, value)
		factor := keySignatureFactor(t, solver, 104, 1)
		rule := localLawDeclareAt(t, solver, factor, 105, shard, at, func(engine.Access[uint64, localLawBits]) bool { return true })
		if !engine.WriteExact(rule, 0) {
			t.Fatal("declare exact self write")
		}
		if _, ok := engine.ReadExact(rule, 0, factor, 0); !ok {
			t.Fatal("declare exact self read")
		}
		if _, ok := engine.DeclareQuery(solver, factor, shard, at, 0); !ok {
			t.Fatal("query")
		}
		if solver.Seal() {
			t.Fatal("same exact key self cycle bypassed Program Mu")
		}
	})

	t.Run("same key mutual read", func(t *testing.T) {
		solver, value, shard := localLawSolver(t, "")
		at := localLawEntry(t, value)
		left := keySignatureFactor(t, solver, 106, 1)
		right := keySignatureFactor(t, solver, 107, 1)
		leftRule := localLawDeclareAt(t, solver, left, 108, shard, at, func(engine.Access[uint64, localLawBits]) bool { return true })
		rightRule := localLawDeclareAt(t, solver, right, 109, shard, at, func(engine.Access[uint64, localLawBits]) bool { return true })
		if !engine.WriteExact(leftRule, 0) || !engine.WriteExact(rightRule, 0) {
			t.Fatal("declare exact mutual writes")
		}
		if _, ok := engine.ReadExact(leftRule, 0, right, 0); !ok {
			t.Fatal("declare left exact read")
		}
		if _, ok := engine.ReadExact(rightRule, 0, left, 0); !ok {
			t.Fatal("declare right exact read")
		}
		if _, ok := engine.DeclareQuery(solver, left, shard, at, 0); !ok {
			t.Fatal("query")
		}
		if solver.Seal() {
			t.Fatal("same exact key mutual cycle bypassed Program Mu")
		}
	})
}

func TestExactKeySignatureFailsClosedAtAccess(t *testing.T) {
	t.Run("undeclared write", func(t *testing.T) {
		solver, value, shard := localLawSolver(t, "")
		at := localLawEntry(t, value)
		factor := keySignatureFactor(t, solver, 110, 2)
		rule := localLawDeclareAt(t, solver, factor, 111, shard, at, func(access engine.Access[uint64, localLawBits]) bool {
			return access.Set(1, localLawOne)
		})
		if !engine.WriteExact(rule, 0) {
			t.Fatal("declare exact output")
		}
		if _, ok := engine.DeclareQuery(solver, factor, shard, at, 0); !ok || !solver.Seal() {
			t.Fatal("seal undeclared write law")
		}
		if state, ok := solver.Solve(context.Background(), nil); ok || state != nil {
			t.Fatal("undeclared direct write reached State")
		}
	})

	t.Run("undeclared exact read", func(t *testing.T) {
		solver, value, shard := localLawSolver(t, "")
		at := localLawEntry(t, value)
		factor := keySignatureFactor(t, solver, 112, 2)
		producer := localLawDeclareAt(t, solver, factor, 113, shard, at, func(access engine.Access[uint64, localLawBits]) bool {
			return access.Set(0, localLawOne)
		})
		if !engine.WriteExact(producer, 0) {
			t.Fatal("declare producer")
		}
		var input engine.ReadRef[uint64, localLawBits]
		consumer := localLawDeclareAt(t, solver, factor, 114, shard, at, func(access engine.Access[uint64, localLawBits]) bool {
			_, _, valid := engine.ReadAt(access, input, 1)
			return valid && access.Set(1, localLawOne)
		})
		if !engine.WriteExact(consumer, 1) {
			t.Fatal("declare consumer")
		}
		var ok bool
		input, ok = engine.ReadExact(consumer, 0, factor, 0)
		if !ok {
			t.Fatal("declare exact read")
		}
		if _, ok := engine.DeclareQuery(solver, factor, shard, at, 1); !ok || !solver.Seal() {
			t.Fatal("seal undeclared read law")
		}
		if state, ok := solver.Solve(context.Background(), nil); ok || state != nil {
			t.Fatal("undeclared direct read reached State")
		}
	})

	t.Run("Carry cannot bypass exact writes", func(t *testing.T) {
		solver, value, shard := localLawSolver(t, "")
		at := localLawEntry(t, value)
		factor := keySignatureFactor(t, solver, 115, 2)
		producer := localLawDeclareAt(t, solver, factor, 116, shard, at, func(access engine.Access[uint64, localLawBits]) bool {
			return access.Set(1, localLawOne)
		})
		if !engine.WriteExact(producer, 1) {
			t.Fatal("declare producer")
		}
		var input engine.ReadRef[uint64, localLawBits]
		consumer := localLawDeclareAt(t, solver, factor, 117, shard, at, func(access engine.Access[uint64, localLawBits]) bool {
			return engine.Carry(access, input)
		})
		if !engine.WriteExact(consumer, 0) {
			t.Fatal("declare exact carry output")
		}
		var ok bool
		input, ok = engine.ReadExact(consumer, 0, factor, 1)
		if !ok {
			t.Fatal("declare carry read")
		}
		if _, ok := engine.DeclareQuery(solver, factor, shard, at, 0); !ok || !solver.Seal() {
			t.Fatal("seal carry law")
		}
		if state, ok := solver.Solve(context.Background(), nil); ok || state != nil {
			t.Fatal("Carry bypassed exact write signature")
		}
	})
}

func TestExactKeySignatureKeepsKeyZeroAndRejectsDuplicates(t *testing.T) {
	solver, value, shard := localLawSolver(t, "")
	at := localLawEntry(t, value)
	factor := keySignatureFactor(t, solver, 118, 2)
	rule := localLawDeclareAt(t, solver, factor, 119, shard, at, func(engine.Access[uint64, localLawBits]) bool { return true })
	if !engine.WriteExact(rule, 1) || !engine.WriteExact(rule, 0) || engine.WriteExact(rule, 0) {
		t.Fatal("exact write set did not admit key zero in sorted deduplicated form")
	}
	if _, ok := engine.ReadExact(rule, 0, factor, 1); !ok {
		t.Fatal("exact key one read")
	}
	if _, ok := engine.ReadExact(rule, 0, factor, 0); !ok {
		t.Fatal("exact key zero read")
	}
	if _, ok := engine.ReadExact(rule, 0, factor, 0); ok {
		t.Fatal("duplicate exact read accepted")
	}
}
