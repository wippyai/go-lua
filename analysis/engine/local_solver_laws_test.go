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

// localLawBits is a four-element finite join-semilattice represented as a
// bit set.  Its tiny height makes the expected fixed points in these laws
// precise without relying on engine implementation details.
type localLawBits uint8

const (
	localLawOne  localLawBits = 1
	localLawTwo  localLawBits = 2
	localLawBoth localLawBits = localLawOne | localLawTwo
)

func localLawLattice() lattice.Lattice[localLawBits] {
	return lattice.Lattice[localLawBits]{
		Bottom: func() localLawBits { return 0 },
		Top:    func() localLawBits { return localLawBoth },
		Equal: func(left, right localLawBits) bool {
			return left == right
		},
		LessOrEq: func(left, right localLawBits) bool {
			return left|right == right
		},
		Join: func(left, right localLawBits) localLawBits {
			return left | right
		},
		Widen: func(left, right localLawBits) localLawBits {
			return left | right
		},
	}
}

func localLawSemantic(id byte) engine.SemanticKey {
	return engine.SemanticKey{ID: program.ContentID{id}, Version: 1}
}

// localLawFactor deliberately has no optional Lattice.Narrow. Program/Link-
// proven recurrences therefore use forward widening only and never cause the
// scheduler to invent a narrowing pass.
func localLawFactor(t *testing.T, solver *engine.Solver, id byte) *engine.Factor[uint64, localLawBits] {
	t.Helper()
	factor, ok := engine.DeclareFactor(solver, engine.FactorConfig[uint64, localLawBits]{
		Keys:        engine.KeySpace{End: 1},
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
		t.Fatal("DeclareFactor rejected the finite local-law domain")
	}
	return factor
}

// localLawNarrowFactor is the same finite semantic domain with an explicit,
// ranked Cousot narrowing contract.  Support retraction is a narrowing
// transition, so the cyclic-Prune law must opt into that contract rather than
// asking the engine to invent a value narrowing for a forward-only Factor.
func localLawNarrowFactor(t *testing.T, solver *engine.Solver, id byte) *engine.Factor[uint64, localLawBits] {
	t.Helper()
	valueLattice := localLawLattice()
	valueLattice.Narrow = func(_ localLawBits, next localLawBits) localLawBits {
		return next
	}
	factor, ok := engine.DeclareFactor(solver, engine.FactorConfig[uint64, localLawBits]{
		Keys:        engine.KeySpace{End: 1},
		Semantic:    localLawSemantic(id),
		Lattice:     valueLattice,
		Default:     0,
		Fingerprint: func(value localLawBits) uint64 { return uint64(value) },
		WidenRank: engine.Measure[uint64, localLawBits]{
			Width: 1,
			At: func(_ uint64, value localLawBits, _ int) uint64 {
				return uint64(localLawBoth - value)
			},
		},
		NarrowRank: engine.Measure[uint64, localLawBits]{
			Width: 1,
			At: func(_ uint64, value localLawBits, _ int) uint64 {
				return uint64(value)
			},
		},
	})
	if !ok {
		t.Fatal("DeclareFactor rejected the ranked local-law domain")
	}
	return factor
}

func localLawProgram(t *testing.T, source string) *program.Program {
	t.Helper()
	value, err := programlower.Lower(programlower.Source{
		Name: "local-solver-law.lua",
		Text: []byte(source),
	})
	if err != nil {
		t.Fatalf("lower Program: %v", err)
	}
	return value
}

func localLawLink(t *testing.T, value *program.Program) (*link.Link, link.Shard) {
	t.Helper()
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatalf("seal empty target Contract: %v", err)
	}
	sealed, err := link.Seal(&link.Spec{
		Target:  contract,
		Modules: []link.Module{{Name: "local-solver-law", Program: value}},
	})
	if err != nil {
		t.Fatalf("seal Link: %v", err)
	}
	for index := 0; index < sealed.ShardCount(); index++ {
		shard, ok := sealed.ShardAt(index)
		if !ok {
			continue
		}
		candidate, ok := sealed.Program(shard)
		if ok && candidate == value {
			return sealed, shard
		}
	}
	t.Fatal("sealed Link did not retain its Program")
	return nil, 0
}

func localLawSolver(t *testing.T, source string) (*engine.Solver, *program.Program, link.Shard) {
	t.Helper()
	value := localLawProgram(t, source)
	sealed, shard := localLawLink(t, value)
	solver, err := engine.New(sealed)
	if err != nil {
		t.Fatalf("new Solver: %v", err)
	}
	return solver, value, shard
}

func localLawEntry(t *testing.T, value *program.Program) program.Term {
	t.Helper()
	entry, ok := value.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	return entry
}

func localLawActivationEdge(t *testing.T, value *program.Program, activation program.Term, accept func(program.Edge) bool) program.Edge {
	t.Helper()
	count, ok := value.ActivationEdgeCount(activation)
	if !ok {
		t.Fatalf("ActivationEdgeCount(%v) failed", activation)
	}
	for index := 0; index < count; index++ {
		edge, ok := value.ActivationEdgeAt(activation, index)
		if !ok {
			t.Fatalf("ActivationEdgeAt(%v, %d) failed", activation, index)
		}
		if accept(edge) {
			return edge
		}
	}
	t.Fatal("Program has no matching activation Edge")
	return program.Edge{}
}

func localLawDeclareAt[K ~uint64, V any](t *testing.T, solver *engine.Solver, factor *engine.Factor[K, V], semantic byte, shard link.Shard, at program.Term, run func(engine.Access[K, V]) bool) *engine.Rule[K, V] {
	t.Helper()
	rule, ok := engine.DeclareRule(solver, factor, localLawSemantic(semantic), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, at)
	}, run)
	if !ok {
		t.Fatal("DeclareRule(At) rejected a canonical Program occurrence")
	}
	return rule
}

func localLawDeclareFrom[K ~uint64, V any](t *testing.T, solver *engine.Solver, factor *engine.Factor[K, V], semantic byte, shard link.Shard, edge program.Edge, run func(engine.Access[K, V]) bool) *engine.Rule[K, V] {
	t.Helper()
	rule, ok := engine.DeclareRule(solver, factor, localLawSemantic(semantic), func(binding *engine.RuleBinding) bool {
		return binding.From(shard, edge)
	}, run)
	if !ok {
		t.Fatal("DeclareRule(From) rejected a canonical Program Edge")
	}
	return rule
}

func localLawQuery(t *testing.T, solver *engine.Solver, factor *engine.Factor[uint64, localLawBits], shard link.Shard, at program.Term) *engine.Query[uint64, localLawBits] {
	t.Helper()
	query, ok := engine.DeclareQuery(solver, factor, shard, at, 0)
	if !ok {
		t.Fatal("DeclareQuery rejected a canonical Program occurrence")
	}
	return query
}

func localLawSealAndSolve(t *testing.T, solver *engine.Solver) *engine.State {
	t.Helper()
	if !solver.Seal() {
		t.Fatal("Seal rejected the declared local solver law")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("Solve did not publish a State")
	}
	return state
}

func localLawRead(t *testing.T, query *engine.Query[uint64, localLawBits], state *engine.State) localLawBits {
	t.Helper()
	value, present := query.Read(state)
	if !present {
		t.Fatal("Query has no semantic value in the published State")
	}
	return value
}

// A Program edge with no bound domain Rule is the identity transport for the
// complete joint fiber, not merely the queried Factor.
func TestLocalLawProgramEdgeWithoutDomainRuleCarriesJointFiber(t *testing.T) {
	solver, value, shard := localLawSolver(t, "local kept = 1")
	entry := localLawEntry(t, value)
	edge := localLawActivationEdge(t, value, entry, func(edge program.Edge) bool {
		return edge.From() == entry
	})
	left := localLawFactor(t, solver, 1)
	right := localLawFactor(t, solver, 2)
	localLawDeclareAt(t, solver, left, 11, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Set(0, localLawOne)
	})
	localLawDeclareAt(t, solver, right, 12, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Set(0, localLawTwo)
	})
	leftQuery := localLawQuery(t, solver, left, shard, edge.To())
	rightQuery := localLawQuery(t, solver, right, shard, edge.To())

	state := localLawSealAndSolve(t, solver)
	if got := localLawRead(t, leftQuery, state); got != localLawOne {
		t.Fatalf("left Factor after zero-rule Edge = %d, want %d", got, localLawOne)
	}
	if got := localLawRead(t, rightQuery, state); got != localLawTwo {
		t.Fatalf("right Factor after zero-rule Edge = %d, want %d", got, localLawTwo)
	}
}

// Pruning a local terminal removes the entire current support.  A later
// sibling Rule therefore cannot leak a write through that terminal.
func TestLocalLawPruneRemovesSupportWithoutSiblingLeak(t *testing.T) {
	solver, value, shard := localLawSolver(t, "local kept = 1")
	entry := localLawEntry(t, value)
	edge := localLawActivationEdge(t, value, entry, func(edge program.Edge) bool {
		return edge.From() == entry
	})
	pruner := localLawFactor(t, solver, 3)
	sibling := localLawFactor(t, solver, 4)
	localLawDeclareAt(t, solver, pruner, 13, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Prune()
	})
	localLawDeclareAt(t, solver, sibling, 14, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Set(0, localLawBoth)
	})
	query := localLawQuery(t, solver, sibling, shard, edge.To())

	state := localLawSealAndSolve(t, solver)
	if got, present := query.Read(state); present || got != 0 {
		t.Fatalf("sibling write survived local Prune: %d/%t", got, present)
	}
}

// A function body that has no selected Candidate is an existing Program
// occurrence but never an implicit root seed.
// A Rule failure rejects the whole candidate transaction, including writes
// made before the callback reports failure.
func TestLocalLawRejectedTransactionPublishesNoPartialState(t *testing.T) {
	solver, value, shard := localLawSolver(t, "")
	entry := localLawEntry(t, value)
	factor := localLawFactor(t, solver, 6)
	localLawDeclareAt(t, solver, factor, 16, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		if !access.Set(0, localLawBoth) {
			return false
		}
		return false
	})
	query := localLawQuery(t, solver, factor, shard, entry)
	if !solver.Seal() {
		t.Fatal("Seal rejected the transaction-rejection law")
	}

	state, ok := solver.Solve(context.Background(), nil)
	if ok || state != nil {
		t.Fatalf("rejected transaction published State %p/%t", state, ok)
	}
	if got, present := query.Read(state); present || got != 0 {
		t.Fatalf("rejected transaction exposed a partial Query result: %d/%t", got, present)
	}
	state, ok = solver.Solve(context.Background(), nil)
	if ok || state != nil {
		t.Fatalf("rejected transaction left retry-visible State %p/%t", state, ok)
	}
}

// A self-read is one finite compiled equation even when its Program location
// has no source-control backedge. Its ranked Factor converges to the semantic
// fixed point rather than relying on a guessed Program recurrence boundary.
func TestLocalLawSelfReadConverges(t *testing.T) {
	solver, value, shard := localLawSolver(t, "")
	entry := localLawEntry(t, value)
	factor := localLawFactor(t, solver, 7)
	var read engine.ReadRef[uint64, localLawBits]
	rule := localLawDeclareAt(t, solver, factor, 17, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
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
		t.Fatal("Read did not bind the self-recursive Factor")
	}
	query := localLawQuery(t, solver, factor, shard, entry)
	state := localLawSealAndSolve(t, solver)
	if got := localLawRead(t, query, state); got != localLawBoth {
		t.Fatalf("self-recursive fixed point = %d, want %d", got, localLawBoth)
	}
}

func TestLocalLawMutualReadConverges(t *testing.T) {
	solver, value, shard := localLawSolver(t, "")
	entry := localLawEntry(t, value)
	left := localLawFactor(t, solver, 8)
	right := localLawFactor(t, solver, 9)
	var leftInput engine.ReadRef[uint64, localLawBits]
	var rightInput engine.ReadRef[uint64, localLawBits]
	leftRule := localLawDeclareAt(t, solver, left, 18, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		prior, present, valid := engine.ReadAt(access, leftInput, 0)
		if !valid {
			return false
		}
		next := localLawOne
		if present {
			next = prior | localLawOne
		}
		return access.Set(0, next)
	})
	rightRule := localLawDeclareAt(t, solver, right, 19, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		prior, present, valid := engine.ReadAt(access, rightInput, 0)
		if !valid {
			return false
		}
		next := localLawTwo
		if present {
			next = prior | localLawTwo
		}
		return access.Set(0, next)
	})
	var ok bool
	leftInput, ok = engine.Read(leftRule, 0, right)
	if !ok {
		t.Fatal("Read did not bind the left Rule's right-factor input")
	}
	rightInput, ok = engine.Read(rightRule, 0, left)
	if !ok {
		t.Fatal("Read did not bind the right Rule's left-factor input")
	}
	leftQuery := localLawQuery(t, solver, left, shard, entry)
	rightQuery := localLawQuery(t, solver, right, shard, entry)
	state := localLawSealAndSolve(t, solver)
	if got := localLawRead(t, leftQuery, state); got != localLawBoth {
		t.Fatalf("left mutual fixed point = %d, want %d", got, localLawBoth)
	}
	if got := localLawRead(t, rightQuery, state); got != localLawBoth {
		t.Fatalf("right mutual fixed point = %d, want %d", got, localLawBoth)
	}
}

// A local Rule can remove the only seeded row of a cyclic action component.
// The declared self read makes the Program action cyclic; the Rule itself
// deliberately has no relation application.  The result must therefore be
// absent, rather than the prior present-zero carrier row that an ordinary
// left-biased Zip would retain.
func TestLocalLawCyclicPruneRetractsSeededSupport(t *testing.T) {
	solver, value, shard := localLawSolver(t, "")
	entry := localLawEntry(t, value)
	factor := localLawNarrowFactor(t, solver, 11)
	pruner := localLawDeclareAt(t, solver, factor, 22, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Prune()
	})
	// This is a scheduling dependency only.  It establishes one local cyclic
	// action component without introducing an active Link Relation.
	if _, ok := engine.Read(pruner, 0, factor); !ok {
		t.Fatal("Read did not bind the cyclic local input")
	}
	query := localLawQuery(t, solver, factor, shard, entry)
	state := localLawSealAndSolve(t, solver)
	if got, present := query.Read(state); present || got != 0 {
		t.Fatalf("cyclic Prune retained seeded support: %d/%t", got, present)
	}
}

// A Program Mu supplies the exact Guard reset on its own Edge, while the
// compiled equation independently converges a local Factor self-read at the
// transfer target.
func TestLocalLawProgramMuAndSelfReadConverge(t *testing.T) {
	solver, value, shard := localLawSolver(t, `
while true do
  if unknown then break end
end
`)
	entry := localLawEntry(t, value)
	recurrence := localLawActivationEdge(t, value, entry, func(edge program.Edge) bool {
		_, ok := edge.Mu()
		return ok
	})
	at := recurrence.To()
	factor := localLawFactor(t, solver, 23)
	var read engine.ReadRef[uint64, localLawBits]
	rule := localLawDeclareAt(t, solver, factor, 24, shard, at, func(access engine.Access[uint64, localLawBits]) bool {
		_, _, valid := engine.ReadAt(access, read, 0)
		return valid && access.Set(0, localLawOne)
	})
	var ok bool
	read, ok = engine.Read(rule, 0, factor)
	if !ok {
		t.Fatal("Read did not bind the unrelated local self dependency")
	}
	query, ok := engine.DeclareQuery(solver, factor, shard, at, 0)
	if !ok {
		t.Fatal("DeclareQuery(unrelated local self dependency)")
	}
	state := localLawSealAndSolve(t, solver)
	if got := localLawRead(t, query, state); got != localLawOne {
		t.Fatalf("Program-Mu local self fixed point = %d, want %d", got, localLawOne)
	}
}

// Program's Mu edge discharges the stale choice of the loop body before the
// next turn.  The next turn can therefore take the break edge and contribute
// the recurrence value alongside the direct first-turn break result.
func TestLocalLawProgramMuEdgeRecurrenceDischargesSemantically(t *testing.T) {
	solver, value, shard := localLawSolver(t, `
while true do
  if unknown then break end
end
`)
	entry := localLawEntry(t, value)
	recurrence := localLawActivationEdge(t, value, entry, func(edge program.Edge) bool {
		_, ok := edge.Mu()
		return ok
	})
	if count, ok := recurrence.MuDecisionCount(); !ok || count == 0 {
		t.Fatalf("Program Mu edge has no decision discharge interface: %d/%t", count, ok)
	}
	exit, ok := value.BodyNormalExit(entry)
	if !ok {
		t.Fatal("loop Program has no normal exit")
	}
	factor := localLawFactor(t, solver, 10)
	localLawDeclareAt(t, solver, factor, 20, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Set(0, localLawOne)
	})
	localLawDeclareFrom(t, solver, factor, 21, shard, recurrence, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Set(0, localLawTwo)
	})
	query := localLawQuery(t, solver, factor, shard, exit)

	state := localLawSealAndSolve(t, solver)
	if got := localLawRead(t, query, state); got != localLawBoth {
		t.Fatalf("Mu recurrence did not discharge into the next loop turn: got %d, want %d", got, localLawBoth)
	}
}

// A Rule capability is a callback-scoped value, not an object that can be
// retained and used to reopen a completed transaction. Copying it is allowed
// by the language, but both copies expire at the callback boundary.
func TestLocalLawRetainedAccessExpiresAtCallbackBoundary(t *testing.T) {
	solver, value, shard := localLawSolver(t, "local kept = 1")
	entry := localLawEntry(t, value)
	factor := localLawFactor(t, solver, 31)
	var retained engine.Access[uint64, localLawBits]
	localLawDeclareAt(t, solver, factor, 32, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		retained = access
		return access.Set(0, localLawOne)
	})
	query := localLawQuery(t, solver, factor, shard, entry)
	state := localLawSealAndSolve(t, solver)
	if got := localLawRead(t, query, state); got != localLawOne {
		t.Fatalf("published value = %d, want %d", got, localLawOne)
	}
	if retained.Set(0, localLawTwo) {
		t.Fatal("retained Rule Access reopened a completed callback")
	}
}

// Reusing the private Product frame for a later Rule must not revive a value
// retained from an earlier Rule. The declared read supplies the causal order;
// the observable law is that only the later Rule can publish its own result.
func TestLocalLawRetainedAccessDoesNotReviveForSiblingRule(t *testing.T) {
	solver, value, shard := localLawSolver(t, "local kept = 1")
	entry := localLawEntry(t, value)
	first := localLawFactor(t, solver, 33)
	second := localLawFactor(t, solver, 34)
	var retained engine.Access[uint64, localLawBits]
	localLawDeclareAt(t, solver, first, 35, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		retained = access
		return access.Set(0, localLawOne)
	})
	resurrected := false
	later := localLawDeclareAt(t, solver, second, 36, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		resurrected = retained.Set(0, localLawTwo)
		return access.Set(0, localLawTwo)
	})
	if _, ok := engine.Read(later, 0, first); !ok {
		t.Fatal("Read did not establish the later Rule dependency")
	}
	query := localLawQuery(t, solver, second, shard, entry)
	state := localLawSealAndSolve(t, solver)
	if resurrected {
		t.Fatal("retained Rule Access revived for a later Rule callback")
	}
	if got := localLawRead(t, query, state); got != localLawTwo {
		t.Fatalf("later Rule result = %d, want %d", got, localLawTwo)
	}
}
