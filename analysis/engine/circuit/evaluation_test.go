package circuit_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/circuit"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/semantic/primitive"
	"github.com/wippyai/go-lua/analysis/semantic/program"
	"github.com/wippyai/go-lua/analysis/semantic/transaction"
)

func TestCircuitMutualRecursiveLoopMatchesPerContextOracle(t *testing.T) {
	circuitProgram, bindings, key, nativeCalls := evaluationFixture(t)
	result, err := circuit.Evaluate(context.Background(), circuitProgram, bindings, circuit.EvaluationConfig{Key: key, Entry: oracleDisjunct(t, "entry", "entry-prov", "entry-alias", "seed"), RouteBinding: func(target program.KnownTarget) (circuit.Disjunct, bool) {
		for _, context := range concreteOracle(t) {
			if context.guard == target.Guard && context.member == target.Member {
				return context.binding, true
			}
		}
		return circuit.Disjunct{}, false
	}})
	if err != nil {
		t.Fatal(err)
	}
	if *nativeCalls != 0 {
		t.Fatalf("circuit executed alternate intrinsic semantics: calls=%d", *nativeCalls)
	}
	want := append([]oracleContext{{binding: oracleDisjunct(t, "entry", "entry-prov", "entry-alias", "seed")}}, concreteOracle(t)...)
	for block, cell := range result.Cells {
		got := cell.Disjuncts()
		if len(got) != len(want) {
			t.Fatalf("block %d disjuncts=%#v", block, got)
		}
		for _, context := range want {
			found := false
			for _, d := range got {
				if d.Binding() == context.binding.Binding() {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("block %d lost correlated binding %q: %#v", block, context.binding.Binding(), got)
			}
		}
	}
	if result.Stats.Cells != 4 || result.Stats.Transfers == 0 || result.Stats.Widens == 0 || result.Stats.Disjuncts < 12 || result.Stats.IntrinsicCalls != 1 || result.Stats.PrecisionLossCells != 0 {
		t.Fatalf("evaluation stats=%#v", result.Stats)
	}
	if len(circuitProgram.BlockNodes()) != 4 || len(circuitProgram.ApplyNodes()) != 2 || len(circuitProgram.LoopNodes()) != 1 || len(circuitProgram.CallSCCNode().Region.Members) != 2 || len(circuitProgram.IntrinsicNodes()) != 1 {
		t.Fatal("circuit did not reify block/apply/LoopMu/CallSCCMu/intrinsic nodes")
	}
}

func TestCircuitCancellationPublishesNothing(t *testing.T) {
	circuitProgram, bindings, key, _ := evaluationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	result, err := circuit.Evaluate(ctx, circuitProgram, bindings, circuit.EvaluationConfig{Key: key, Entry: oracleDisjunct(t, "entry", "entry-prov", "entry-alias", "seed"), RouteBinding: func(target program.KnownTarget) (circuit.Disjunct, bool) { return concreteOracle(t)[0].binding, true }, VisitBlock: func(program.BlockID) { cancel() }})
	if !errors.Is(err, solve.ErrCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	if result.Cells != nil || result.Revisions != nil || result.Stats != (circuit.EvaluationStats{}) {
		t.Fatalf("canceled circuit published %#v", result)
	}
}

type oracleContext struct {
	guard   program.GuardID
	member  program.MemberID
	binding circuit.Disjunct
}

func concreteOracle(t testing.TB) []oracleContext {
	return []oracleContext{{"route-alpha", "alpha", oracleDisjunct(t, "route-alpha", "caller-beta", "alias-alpha", "x-number:y-string")}, {"route-beta", "beta", oracleDisjunct(t, "route-beta", "caller-alpha", "alias-beta", "x-string:y-number")}}
}

func evaluationFixture(t testing.TB) (circuit.Circuit, *circuit.Domain, circuit.CellKey, *int) {
	t.Helper()
	frozen := testTransaction(t)
	calls := 0
	call, err := primitive.NewIntrinsicCall("actor.yield", 1, []byte("reified-only"))
	if err != nil {
		t.Fatal(err)
	}
	binding, err := primitive.NewNativeBinding("actor.yield", 1, "actor-yield.v1", func(primitive.NativeInput) (primitive.NativeOutput, error) {
		calls++
		return primitive.NativeOutput{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	builder := primitive.NewBuilder()
	if err := builder.AddIntrinsic(primitive.IntrinsicDescriptor{ID: "actor.yield", SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if err := builder.BindIntrinsic(binding); err != nil {
		t.Fatal(err)
	}
	descriptor := primitive.ProgramDescriptor{ID: "actor-step", SchemaVersion: 1, Steps: []primitive.Step{primitive.TransactionStep(frozen), primitive.IntrinsicStep(call)}}
	if err := builder.AddProgram(descriptor); err != nil {
		t.Fatal(err)
	}
	for _, role := range []primitive.CoverageRole{primitive.CoveragePrimitive, primitive.CoverageEffect, primitive.CoverageOutput, primitive.CoverageObserver} {
		if err := builder.AddCoverage(primitive.Coverage{ProgramID: "actor-step", LeafID: "1:values:binding", Role: role}); err != nil {
			t.Fatal(err)
		}
	}
	registry, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	proof := sha256.Sum256([]byte("complete"))
	ref := program.Ref(frozen)
	spec := program.Spec{Entry: 1, Members: []program.MemberID{"alpha", "beta"}, Transactions: []program.TransactionRef{ref}, Blocks: []program.Block{{ID: 1, Member: "alpha", Transactions: []program.TransactionRef{ref}}, {ID: 2, Member: "alpha", Transactions: []program.TransactionRef{ref}}, {ID: 3, Member: "beta", Transactions: []program.TransactionRef{ref}}, {ID: 4, Member: "beta", Transactions: []program.TransactionRef{ref}}}, Edges: []program.Edge{{From: 1, To: 2, Guard: "flow-a"}, {From: 2, To: 3, Guard: "call-beta"}, {From: 3, To: 4, Guard: "flow-b"}, {From: 4, To: 1, Guard: "call-alpha"}}, CallSCC: program.CallSCCMu{ID: 1, Members: []program.MemberID{"alpha", "beta"}}, Loops: []program.LoopMu{{ID: 2, SCC: 1, Owner: "alpha", Entry: 1, Blocks: []program.BlockID{1, 2}}}, Routes: []program.MixedTargetRoute{{At: 2, Known: []program.KnownTarget{{Guard: "route-beta", Member: "beta"}}, Completeness: program.TargetsComplete, Proof: proof}, {At: 4, Known: []program.KnownTarget{{Guard: "route-alpha", Member: "alpha"}}, Completeness: program.TargetsComplete, Proof: proof}}}
	semantic, err := program.Freeze(spec)
	if err != nil {
		t.Fatal(err)
	}
	reified, err := circuit.Reify(semantic, registry)
	if err != nil {
		t.Fatal(err)
	}
	partition, err := circuit.NewBindingPartitionPolicy(1, []circuit.ClassID{"apply"}, []circuit.ClassID{"target"}, []circuit.ClassID{"prov"}, []circuit.ClassID{"alias"})
	if err != nil {
		t.Fatal(err)
	}
	key, err := partition.Partition(circuit.PartitionInput{Application: "apply", Target: "target", Provenance: "prov", Alias: "alias"})
	if err != nil {
		t.Fatal(err)
	}
	precision, err := circuit.NewPrecisionPolicy(1, 4, "binding-top", guardIDs("entry", "route-alpha", "route-beta"), guardIDs("entry-prov", "caller-alpha", "caller-beta"), guardIDs("entry-alias", "alias-alpha", "alias-beta"))
	if err != nil {
		t.Fatal(err)
	}
	domain, err := circuit.NewBindingDomain(partition, precision, nil, func(w circuit.BindingID, members []circuit.BindingID) bool {
		return w == "binding-top" && len(members) > 0
	})
	if err != nil {
		t.Fatal(err)
	}
	return reified, domain, key, &calls
}

func testTransaction(t testing.TB) transaction.FrozenTransaction {
	t.Helper()
	builder, err := transaction.NewBuilder([]transaction.Capability{{ID: "values", Kind: transaction.SlotLane}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := transaction.Bind[string](builder, "values", "binding")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := transaction.NewOutcomePolicy(transaction.Commit, transaction.Rollback, transaction.Rollback, transaction.Rollback)
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := builder.BeginOverlay("normal", policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Append(overlay, handle, "assign", []byte("binding")); err != nil {
		t.Fatal(err)
	}
	frozen, err := builder.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}
func oracleDisjunct(t testing.TB, a, p, l, b string) circuit.Disjunct {
	t.Helper()
	app, _ := circuit.NewGuardSet(circuit.GuardID(a))
	prov, _ := circuit.NewGuardSet(circuit.GuardID(p))
	alias, _ := circuit.NewGuardSet(circuit.GuardID(l))
	d, err := circuit.NewDisjunct(app, prov, alias, circuit.BindingID(b))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func guardIDs(ids ...string) []circuit.GuardID {
	out := make([]circuit.GuardID, len(ids))
	for i, id := range ids {
		out[i] = circuit.GuardID(id)
	}
	slices.Sort(out)
	return out
}
