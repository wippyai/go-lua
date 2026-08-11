package engine_test

import (
	"context"
	"crypto/sha256"
	"math/bits"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// stage4PackKey is deliberately the one stable semantic pack coordinate in
// these laws. Correlation belongs to the finite relation value below, not to
// a second coordinate or an engine-private product convention.
const stage4PackKey uint64 = 0

// stage4Pairs is a finite binary relation. Each bit is one whole ordered pair,
// so joining branch results cannot manufacture the two absent Cartesian pairs.
type stage4Pairs uint8

const (
	stage4LeftOne  stage4Pairs = 1 << 0 // (left, one)
	stage4RightTwo stage4Pairs = 1 << 3 // (right, two)
	stage4LeftTwo  stage4Pairs = 1 << 1 // absent Cartesian cross-pair
	stage4RightOne stage4Pairs = 1 << 2 // absent Cartesian cross-pair
	stage4AllPairs stage4Pairs = stage4LeftOne | stage4RightTwo | stage4LeftTwo | stage4RightOne
)

func stage4PairLattice() lattice.Lattice[stage4Pairs] {
	return lattice.Lattice[stage4Pairs]{
		Bottom: func() stage4Pairs { return 0 },
		Top:    func() stage4Pairs { return stage4AllPairs },
		Equal: func(left, right stage4Pairs) bool {
			return left == right
		},
		LessOrEq: func(left, right stage4Pairs) bool {
			return left&^right == 0
		},
		Join: func(left, right stage4Pairs) stage4Pairs {
			return left | right
		},
		Meet: func(left, right stage4Pairs) stage4Pairs {
			return left & right
		},
		Widen: func(left, right stage4Pairs) stage4Pairs {
			return left | right
		},
	}
}

// stage4Gate has three ordered states. Its rank is a proof witness for the
// finite self-recursive ascent Bottom < Closed < Open; it is not an iteration
// allowance. The Gate is test-only domain input to the relational Factor.
type stage4Gate uint8

const (
	stage4GateBottom stage4Gate = iota
	stage4GateClosed
	stage4GateOpen
)

func stage4GateLattice() lattice.Lattice[stage4Gate] {
	return lattice.Lattice[stage4Gate]{
		Bottom: func() stage4Gate { return stage4GateBottom },
		Top:    func() stage4Gate { return stage4GateOpen },
		Equal: func(left, right stage4Gate) bool {
			return left == right
		},
		LessOrEq: func(left, right stage4Gate) bool {
			return left <= right
		},
		Join: func(left, right stage4Gate) stage4Gate {
			if left > right {
				return left
			}
			return right
		},
		Meet: func(left, right stage4Gate) stage4Gate {
			if left < right {
				return left
			}
			return right
		},
		Widen: func(left, right stage4Gate) stage4Gate {
			if left > right {
				return left
			}
			return right
		},
	}
}

func stage4Semantic(label string) engine.SemanticKey {
	return engine.SemanticKey{ID: program.ContentID(sha256.Sum256([]byte("stage4-relational-law/" + label))), Version: 1}
}

func stage4PairsFactor(t testing.TB, solver *engine.Solver, label string) *engine.Factor[uint64, stage4Pairs] {
	t.Helper()
	factor, ok := engine.DeclareFactor(solver, engine.FactorConfig[uint64, stage4Pairs]{
		Keys:        engine.KeySpace{End: 1},
		Semantic:    stage4Semantic("factor/" + label),
		Lattice:     stage4PairLattice(),
		Default:     0,
		Fingerprint: func(value stage4Pairs) uint64 { return uint64(value) },
		WidenRank: engine.Measure[uint64, stage4Pairs]{
			Width: 1,
			At: func(_ uint64, value stage4Pairs, _ int) uint64 {
				return uint64(4 - bits.OnesCount8(uint8(value)))
			},
		},
	})
	if !ok {
		t.Fatalf("DeclareFactor(%s)", label)
	}
	return factor
}

func stage4GateFactor(t testing.TB, solver *engine.Solver, label string) *engine.Factor[uint64, stage4Gate] {
	t.Helper()
	factor, ok := engine.DeclareFactor(solver, engine.FactorConfig[uint64, stage4Gate]{
		Keys:        engine.KeySpace{End: 1},
		Semantic:    stage4Semantic("gate/" + label),
		Lattice:     stage4GateLattice(),
		Default:     stage4GateBottom,
		Fingerprint: func(value stage4Gate) uint64 { return uint64(value) },
		WidenRank: engine.Measure[uint64, stage4Gate]{
			Width: 1,
			At: func(_ uint64, value stage4Gate, _ int) uint64 {
				return uint64(stage4GateOpen - value)
			},
		},
	})
	if !ok {
		t.Fatalf("DeclareFactor(%s)", label)
	}
	return factor
}

func stage4BranchProgram(t testing.TB) (*program.Program, *link.Link, link.Shard, program.Term, program.Term, program.Term) {
	t.Helper()
	value, err := programlower.Lower(programlower.Source{
		Name: "stage4-branch.lua",
		Text: []byte(`
if flag then
  local left = 1
else
  local right = 2
end
local after = 3
`),
	})
	if err != nil {
		t.Fatalf("lower branch Program: %v", err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatalf("seal target: %v", err)
	}
	project, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "main", Program: value}}})
	if err != nil {
		t.Fatalf("seal Link: %v", err)
	}
	var shard link.Shard
	for index := 0; index < project.ShardCount(); index++ {
		candidate, ok := project.ShardAt(index)
		if !ok {
			continue
		}
		owned, ok := project.Program(candidate)
		if ok && owned == value {
			shard = candidate
			break
		}
	}
	if shard == 0 {
		t.Fatal("Link did not retain branch Program")
	}
	branch, ok := value.BranchAt(0)
	if !ok {
		t.Fatal("branch Program has no Branch")
	}
	_, _, whenTrue, whenFalse, ok := value.Branch(branch)
	if !ok || whenTrue == 0 || whenFalse == 0 {
		t.Fatal("Branch has no exact arms")
	}
	trueNormal, ok := value.BodyNormalExit(whenTrue)
	if !ok {
		t.Fatal("truthy arm has no normal exit")
	}
	falseNormal, ok := value.BodyNormalExit(whenFalse)
	if !ok {
		t.Fatal("falsy arm has no normal exit")
	}
	rejoin, trueOK := value.OutcomeSuccessor(trueNormal)
	otherRejoin, falseOK := value.OutcomeSuccessor(falseNormal)
	if !trueOK || !falseOK || rejoin == 0 || rejoin != otherRejoin {
		t.Fatalf("branch arms do not have one lexical rejoin: true=%v/%t false=%v/%t", rejoin, trueOK, otherRejoin, falseOK)
	}
	return value, project, shard, whenTrue, whenFalse, rejoin
}

func stage4DeclareAt(t testing.TB, solver *engine.Solver, factor *engine.Factor[uint64, stage4Pairs], label string, shard link.Shard, term program.Term, run func(engine.Access[uint64, stage4Pairs]) bool) *engine.Rule[uint64, stage4Pairs] {
	t.Helper()
	rule, ok := engine.DeclareRule(solver, factor, stage4Semantic("rule/"+label), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, term)
	}, run)
	if !ok {
		t.Fatalf("DeclareRule(%s)", label)
	}
	return rule
}

func stage4DeclareGateAt(t testing.TB, solver *engine.Solver, factor *engine.Factor[uint64, stage4Gate], label string, shard link.Shard, term program.Term, run func(engine.Access[uint64, stage4Gate]) bool) *engine.Rule[uint64, stage4Gate] {
	t.Helper()
	rule, ok := engine.DeclareRule(solver, factor, stage4Semantic("rule/"+label), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, term)
	}, run)
	if !ok {
		t.Fatalf("DeclareRule(%s)", label)
	}
	return rule
}

func stage4DeclarePairFrom(t testing.TB, solver *engine.Solver, factor *engine.Factor[uint64, stage4Pairs], label string, shard link.Shard, edge program.Edge, run func(engine.Access[uint64, stage4Pairs]) bool) *engine.Rule[uint64, stage4Pairs] {
	t.Helper()
	rule, ok := engine.DeclareRule(solver, factor, stage4Semantic("rule/"+label), func(binding *engine.RuleBinding) bool {
		return binding.From(shard, edge)
	}, run)
	if !ok {
		t.Fatalf("DeclareRule(%s)", label)
	}
	return rule
}

func stage4Query(t testing.TB, solver *engine.Solver, factor *engine.Factor[uint64, stage4Pairs], shard link.Shard, term program.Term) *engine.Query[uint64, stage4Pairs] {
	t.Helper()
	query, ok := engine.DeclareQuery(solver, factor, shard, term, stage4PackKey)
	if !ok {
		t.Fatal("DeclareQuery")
	}
	return query
}

func stage4GateQuery(t testing.TB, solver *engine.Solver, factor *engine.Factor[uint64, stage4Gate], shard link.Shard, term program.Term) *engine.Query[uint64, stage4Gate] {
	t.Helper()
	query, ok := engine.DeclareQuery(solver, factor, shard, term, stage4PackKey)
	if !ok {
		t.Fatal("DeclareQuery")
	}
	return query
}

func stage4EntryEdge(t testing.TB, value *program.Program) (program.Term, program.Edge) {
	t.Helper()
	entry, ok := value.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	count, ok := value.ActivationEdgeCount(entry)
	if !ok {
		t.Fatal("Entry has no activation Edge range")
	}
	for index := 0; index < count; index++ {
		edge, ok := value.ActivationEdgeAt(entry, index)
		if !ok {
			t.Fatalf("ActivationEdgeAt(%d)", index)
		}
		if edge.From() == entry {
			return entry, edge
		}
	}
	t.Fatal("Entry has no outgoing Program Edge")
	return 0, program.Edge{}
}

func stage4Solve(t testing.TB, solver *engine.Solver) *engine.State {
	t.Helper()
	if !solver.Seal() {
		t.Fatal("Solver.Seal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solver.Solve")
	}
	return state
}

func TestStage4LawStablePackKeepsBranchPairsCorrelated(t *testing.T) {
	_, project, shard, whenTrue, whenFalse, rejoin := stage4BranchProgram(t)
	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	pairs := stage4PairsFactor(t, solver, "branch-pairs")
	stage4DeclareAt(t, solver, pairs, "truthy-pair", shard, whenTrue, func(access engine.Access[uint64, stage4Pairs]) bool {
		return access.Set(stage4PackKey, stage4LeftOne)
	})
	stage4DeclareAt(t, solver, pairs, "falsy-pair", shard, whenFalse, func(access engine.Access[uint64, stage4Pairs]) bool {
		return access.Set(stage4PackKey, stage4RightTwo)
	})
	query := stage4Query(t, solver, pairs, shard, rejoin)

	state := stage4Solve(t, solver)
	got, present := query.Read(state)
	if !present || got != stage4LeftOne|stage4RightTwo {
		t.Fatalf("rejoined pack relation = %#x/%t, want only correlated pairs %#x", got, present, stage4LeftOne|stage4RightTwo)
	}
	if got&stage4LeftTwo != 0 || got&stage4RightOne != 0 {
		t.Fatalf("rejoined pack relation invented Cartesian pairs: %#x", got)
	}
}

// A completed State has already validated and compacted the selected branch
// leaves at publication. Repeated readers must therefore obtain the same
// joined fact without rebuilding the guarded carrier or allocating observer
// scratch; concurrent readers share only immutable published data.
func TestStage4LawPublishedBranchReadIsStableConcurrentAndAllocationFree(t *testing.T) {
	_, project, shard, whenTrue, whenFalse, rejoin := stage4BranchProgram(t)
	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	pairs := stage4PairsFactor(t, solver, "published-branch-read")
	stage4DeclareAt(t, solver, pairs, "truthy", shard, whenTrue, func(access engine.Access[uint64, stage4Pairs]) bool {
		return access.Set(stage4PackKey, stage4LeftOne)
	})
	stage4DeclareAt(t, solver, pairs, "falsy", shard, whenFalse, func(access engine.Access[uint64, stage4Pairs]) bool {
		return access.Set(stage4PackKey, stage4RightTwo)
	})
	query := stage4Query(t, solver, pairs, shard, rejoin)
	state := stage4Solve(t, solver)
	want := stage4LeftOne | stage4RightTwo
	if got, present := query.Read(state); !present || got != want {
		t.Fatalf("published branch result = %#x/%t, want %#x", got, present, want)
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		got, present := query.Read(state)
		if !present || got != want {
			panic("published branch observation changed")
		}
	}); allocations != 0 {
		t.Fatalf("published guarded Query.Read allocations = %f, want 0", allocations)
	}
	const readers = 16
	const reads = 128
	errs := make(chan struct{}, readers)
	var group sync.WaitGroup
	for range readers {
		group.Add(1)
		go func() {
			defer group.Done()
			for range reads {
				got, present := query.Read(state)
				if !present || got != want {
					errs <- struct{}{}
					return
				}
			}
		}()
	}
	group.Wait()
	close(errs)
	if _, failed := <-errs; failed {
		t.Fatal("concurrent published branch observation changed")
	}
}

// A dead relation subject is a removed guarded row, not a default value or a
// tombstone inside V. One typed liveness Fact chooses that sole pair Rule's
// output; a distinct branch subject remains exact at the same pack key.
func TestStage4LawDeadRelationSubjectIsRemovedWithoutWideningLiveSubject(t *testing.T) {
	_, project, shard, whenTrue, whenFalse, rejoin := stage4BranchProgram(t)
	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	pairs := stage4PairsFactor(t, solver, "subject-lifetime")
	liveness := stage4GateFactor(t, solver, "dead-subject-liveness")
	stage4DeclareGateAt(t, solver, liveness, "dead-subject-closed", shard, whenTrue, func(access engine.Access[uint64, stage4Gate]) bool {
		return access.Set(stage4PackKey, stage4GateClosed)
	})
	var livenessRead engine.ReadRef[uint64, stage4Gate]
	deadSubject := stage4DeclareAt(t, solver, pairs, "dead-subject-cleanup", shard, whenTrue, func(access engine.Access[uint64, stage4Pairs]) bool {
		value, present, valid := engine.ReadAt(access, livenessRead, stage4PackKey)
		if !valid {
			return false
		}
		if !present || value == stage4GateClosed {
			return access.Prune()
		}
		return access.Set(stage4PackKey, stage4LeftOne)
	})
	var ok bool
	livenessRead, ok = engine.Read(deadSubject, 0, liveness)
	if !ok {
		t.Fatal("Read dead subject liveness")
	}
	stage4DeclareAt(t, solver, pairs, "live-subject-value", shard, whenFalse, func(access engine.Access[uint64, stage4Pairs]) bool {
		return access.Set(stage4PackKey, stage4RightTwo)
	})
	query := stage4Query(t, solver, pairs, shard, rejoin)

	state := stage4Solve(t, solver)
	got, present := query.Read(state)
	if !present || got != stage4RightTwo {
		t.Fatalf("live subject after sibling subject removal = %#x/%t, want exactly %#x", got, present, stage4RightTwo)
	}
	if got&stage4LeftOne != 0 || got&stage4LeftTwo != 0 || got&stage4RightOne != 0 {
		t.Fatalf("dead subject or Cartesian residue survived exact cleanup: %#x", got)
	}
}
