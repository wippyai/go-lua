package engine_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// partitionLawValue is one domain-owned aggregate-root fact.  Its exact
// partitions are deliberately values, not engine keys: an engine key selects
// an aggregate subject, and this value owns the partition abstraction for
// that subject.  summary discards partition precision only by the declared
// mathematical widening, never because the engine ran out of keys or work.
type partitionLawValue struct {
	parts   uint8
	summary bool
}

const (
	partitionLawA uint8 = 1 << iota
	partitionLawB
	partitionLawRetained
)

var (
	partitionLawEmpty         = partitionLawValue{}
	partitionLawExactA        = partitionLawValue{parts: partitionLawA}
	partitionLawExactAB       = partitionLawValue{parts: partitionLawA | partitionLawB}
	partitionLawRetainedValue = partitionLawValue{parts: partitionLawRetained}
	partitionLawSummary       = partitionLawValue{summary: true}
)

func (value partitionLawValue) lessOrEqual(other partitionLawValue) bool {
	if other.summary {
		return true
	}
	return !value.summary && value.parts&^other.parts == 0
}

func partitionLawJoin(left, right partitionLawValue) partitionLawValue {
	if left.summary || right.summary {
		return partitionLawSummary
	}
	return partitionLawValue{parts: left.parts | right.parts}
}

// partitionLawRank is a domain proof witness for the finite widening chain.
// Empty > exact singleton > exact pair > summary.  The exact pair is present
// in the rule result before Widen maps the increasing recurrence to summary.
func partitionLawRank(value partitionLawValue) uint64 {
	if value.summary {
		return 0
	}
	switch value.parts {
	case 0:
		return 3
	case partitionLawA, partitionLawB, partitionLawRetained:
		return 2
	default:
		return 1
	}
}

func partitionLawFactor(t testing.TB, solver *engine.Solver, id byte) *engine.Factor[uint64, partitionLawValue] {
	t.Helper()
	factor, ok := engine.DeclareFactor(solver, engine.FactorConfig[uint64, partitionLawValue]{
		Keys:     engine.KeySpace{End: 3},
		Semantic: partitionLawSemantic(id),
		Lattice: lattice.Lattice[partitionLawValue]{
			Bottom: func() partitionLawValue { return partitionLawEmpty },
			Top:    func() partitionLawValue { return partitionLawSummary },
			Equal:  func(left, right partitionLawValue) bool { return left == right },
			LessOrEq: func(left, right partitionLawValue) bool {
				return left.lessOrEqual(right)
			},
			Join: partitionLawJoin,
			Widen: func(left, right partitionLawValue) partitionLawValue {
				var result partitionLawValue
				switch {
				case left == right:
					result = left
				case left == partitionLawEmpty:
					result = right
				case right == partitionLawEmpty:
					result = left
				default:
					result = partitionLawSummary
				}
				return result
			},
		},
		Default: partitionLawEmpty,
		Fingerprint: func(value partitionLawValue) uint64 {
			if value.summary {
				return 1 << 8
			}
			return uint64(value.parts)
		},
		WidenRank: engine.Measure[uint64, partitionLawValue]{
			Width: 1,
			At: func(_ uint64, value partitionLawValue, _ int) uint64 {
				return partitionLawRank(value)
			},
		},
	})
	if !ok {
		t.Fatal("DeclareFactor rejected the ranked aggregate-partition domain")
	}
	return factor
}

func partitionLawSemantic(id byte) engine.SemanticKey {
	return engine.SemanticKey{ID: program.ContentID{id}, Version: 1}
}

func partitionLawSolver(t testing.TB, source string) (*engine.Solver, *program.Program, link.Shard) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "state-partition-law.lua", Text: []byte(source)})
	if err != nil {
		t.Fatalf("lower Program: %v", err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatalf("seal target: %v", err)
	}
	sealed, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "state-partition-law", Program: p}}})
	if err != nil {
		t.Fatalf("seal Link: %v", err)
	}
	var shard link.Shard
	for index := 0; index < sealed.ShardCount(); index++ {
		candidate, ok := sealed.ShardAt(index)
		if !ok {
			continue
		}
		value, ok := sealed.Program(candidate)
		if value == p && ok {
			shard = candidate
			break
		}
	}
	if shard == 0 {
		t.Fatal("sealed Link did not retain the Program shard")
	}
	solver, err := engine.New(sealed)
	if err != nil {
		t.Fatalf("new Solver: %v", err)
	}
	return solver, p, shard
}

func partitionLawAt[K ~uint64, V any](t testing.TB, solver *engine.Solver, output *engine.Factor[K, V], semantic byte, shard link.Shard, term program.Term, run func(engine.Access[K, V]) bool) *engine.Rule[K, V] {
	t.Helper()
	rule, ok := engine.DeclareRule(solver, output, partitionLawSemantic(semantic), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, term)
	}, run)
	if !ok {
		t.Fatal("DeclareRule(At) rejected a canonical Program term")
	}
	return rule
}

func partitionLawQuery(t testing.TB, solver *engine.Solver, factor *engine.Factor[uint64, partitionLawValue], shard link.Shard, term program.Term, key uint64) *engine.Query[uint64, partitionLawValue] {
	t.Helper()
	query, ok := engine.DeclareQuery(solver, factor, shard, term, key)
	if !ok {
		t.Fatal("DeclareQuery rejected the aggregate-root observation")
	}
	return query
}

func partitionLawSolve(t testing.TB, solver *engine.Solver) *engine.State {
	t.Helper()
	if !solver.Seal() {
		t.Fatal("Seal rejected the partition law")
	}
	// Solve is intentionally called through the flag-day cancellation-aware
	// public boundary.  These laws must never reach into transaction state.
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solve did not publish State")
	}
	return state
}

func partitionLawRead(t testing.TB, query *engine.Query[uint64, partitionLawValue], state *engine.State) partitionLawValue {
	t.Helper()
	value, present := query.Read(state)
	if !present {
		t.Fatal("Query has no aggregate-root value")
	}
	return value
}

// A finite aggregate self-read is one compiled equation. Its own ranked
// widening, not a scheduler budget or source-control annotation, produces
// the domain-owned aggregate summary.
func TestStatePartitionLawSelfReadWidensAggregate(t *testing.T) {
	solver, p, shard := partitionLawSolver(t, "")
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	factor := partitionLawFactor(t, solver, 1)
	const aggregateRoot uint64 = 1
	var read engine.ReadRef[uint64, partitionLawValue]
	rule := partitionLawAt(t, solver, factor, 11, shard, entry, func(access engine.Access[uint64, partitionLawValue]) bool {
		_, present, valid := engine.ReadAt(access, read, aggregateRoot)
		if !valid {
			return false
		}
		next := partitionLawExactA
		if present {
			next = partitionLawExactAB
		}
		return access.Set(aggregateRoot, next)
	})
	var readOK bool
	read, readOK = engine.Read(rule, 0, factor)
	if !readOK {
		t.Fatal("Read rejected the ranked self recurrence")
	}
	if _, ok := engine.DeclareQuery(solver, factor, shard, entry, aggregateRoot); !ok {
		t.Fatal("DeclareQuery(aggregate self-read)")
	}
	query := partitionLawQuery(t, solver, factor, shard, entry, aggregateRoot)
	state := partitionLawSolve(t, solver)
	if got := partitionLawRead(t, query, state); got != partitionLawSummary {
		t.Fatalf("aggregate self recurrence = %#v, want summary %#v", got, partitionLawSummary)
	}
}

// Exact partition lifetime is owned by V.  A source aggregate contains A and
// B; a cleanup Rule, justified by an exact liveness read, writes back only A
// at the successor.  The unrelated aggregate is carried unchanged.  Neither
// A nor B is an engine key, and no whole-Factor clear is allowed.
func TestStatePartitionLawCleanupRemovesDeadPartitionAndRetainsOtherSubjects(t *testing.T) {
	solver, p, shard := partitionLawSolver(t, "local staged = 1")
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	count, ok := p.ActivationEdgeCount(entry)
	if !ok {
		t.Fatal("Program has no activation Edge range")
	}
	var successor program.Term
	for index := 0; index < count; index++ {
		edge, ok := p.ActivationEdgeAt(entry, index)
		if ok && edge.From() == entry {
			successor = edge.To()
			break
		}
	}
	if successor == 0 {
		t.Fatal("Entry has no causal successor")
	}
	pack := partitionLawFactor(t, solver, 2)
	liveness := partitionLawFactor(t, solver, 3)
	const aggregateRoot uint64 = 1
	const unrelatedRoot uint64 = 2
	partitionLawAt(t, solver, pack, 21, shard, entry, func(access engine.Access[uint64, partitionLawValue]) bool {
		return access.Set(aggregateRoot, partitionLawExactAB) && access.Set(unrelatedRoot, partitionLawRetainedValue)
	})
	partitionLawAt(t, solver, liveness, 22, shard, entry, func(access engine.Access[uint64, partitionLawValue]) bool {
		return access.Set(aggregateRoot, partitionLawExactA)
	})
	var liveRead engine.ReadRef[uint64, partitionLawValue]
	cleanup := partitionLawAt(t, solver, pack, 23, shard, successor, func(access engine.Access[uint64, partitionLawValue]) bool {
		live, present, valid := engine.ReadAt(access, liveRead, aggregateRoot)
		if !valid || !present || live != partitionLawExactA {
			return false
		}
		return access.Set(aggregateRoot, partitionLawExactA)
	})
	var readOK bool
	liveRead, readOK = engine.Read(cleanup, 0, liveness)
	if !readOK {
		t.Fatal("Read rejected exact liveness evidence")
	}
	beforeCleanup := partitionLawQuery(t, solver, pack, shard, entry, aggregateRoot)
	live := partitionLawQuery(t, solver, pack, shard, successor, aggregateRoot)
	unrelated := partitionLawQuery(t, solver, pack, shard, successor, unrelatedRoot)

	state := partitionLawSolve(t, solver)
	if got := partitionLawRead(t, beforeCleanup, state); got != partitionLawExactAB {
		t.Fatalf("source aggregate before cleanup = %#v, want exact A+B", got)
	}
	if got := partitionLawRead(t, live, state); got != partitionLawExactA {
		t.Fatalf("live aggregate after dead partition cleanup = %#v, want exact A", got)
	}
	if got := partitionLawRead(t, unrelated, state); got != partitionLawRetainedValue {
		t.Fatalf("unrelated aggregate after dead partition cleanup = %#v, want retained subject", got)
	}
}
