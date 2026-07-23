package equation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/solve"
)

func cyclicVMFixture(t *testing.T) (CyclicArtifact, EntryBinding, []ContentID) {
	t.Helper()
	body := testBody(101)
	entry := EntryParameter{Body: body, Name: "entry"}
	contracts := []ContentID{testID(101), testID(102)}
	artifact := Artifact{Equations: []Equation{
		{Target: Coordinate{Body: body, Name: "seed"}, Entry: entry, Occurrence: Occurrence{Kind: "entry", ContractID: contracts[0]}, KernelID: "seed", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
		{Target: Coordinate{Body: body, Name: "loop"}, Entry: entry, Occurrence: Occurrence{Kind: "loop-control", ContractID: contracts[1]}, KernelID: "loop", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
	}}
	plan, err := solve.FreezeWTOPlan([]CellID{"seed", "loop"}, []solve.WTOElement[CellID]{{Vertex: "seed"}, {Vertex: "loop", Body: []solve.WTOElement[CellID]{}}}, []solve.WTOInfluence[CellID]{{From: "seed", To: "loop"}, {From: "loop", To: "loop"}})
	if err != nil {
		t.Fatal(err)
	}
	cyclic, err := NewCyclicArtifact(artifact, map[Coordinate]CellID{artifact.Equations[0].Target: "seed", artifact.Equations[1].Target: "loop"}, plan,
		[]SemanticDependency{{From: "seed", To: "loop", Reason: EdgeContractRead}, {From: "loop", To: "loop", Reason: EdgeContractAdvance}},
		[]OutputSelector{{ID: "normal", Cells: []CellID{"loop"}}}, []CellID{"seed", "loop"}, []CellID{"loop"})
	if err != nil {
		t.Fatal(err)
	}
	return cyclic, EntryBinding{Parameter: entry, Value: []byte("caller-entry")}, contracts
}

func TestCyclicVMStabilizesConcretePartitionsAndRecordsWidening(t *testing.T) {
	artifact, entry, contracts := cyclicVMFixture(t)
	registry, err := NewCyclicKernelRegistry([]CyclicKernelBinding{
		{KernelID: "seed", ContractID: contracts[0], Kernel: CyclicKernelFunc(func(_ context.Context, _ BoundCyclicEquation, _ CyclicSnapshot) (TransactionResult, error) {
			return TransactionResult{Complete: true, Closure: OutputClosure{Values: []Fact{{Key: "seed", Value: []byte("concrete")}}}}, nil
		})},
		{KernelID: "loop", ContractID: contracts[1], Kernel: CyclicKernelFunc(func(_ context.Context, _ BoundCyclicEquation, snapshot CyclicSnapshot) (TransactionResult, error) {
			if len(snapshot.Read("seed").Leaves) != 1 {
				return TransactionResult{}, fmt.Errorf("concrete predecessor was not available")
			}
			facts := snapshot.Read("loop").Leaves
			if len(facts) == 0 {
				return TransactionResult{Complete: true, Closure: OutputClosure{Values: []Fact{{Key: "round-0", Value: []byte("v")}}}}, nil
			}
			if len(facts[0].Closure.Values) == 1 {
				return TransactionResult{Complete: true, Closure: OutputClosure{Values: []Fact{{Key: "round-1", Value: []byte("v")}}}}, nil
			}
			return TransactionResult{Complete: true}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewCyclicVM(registry)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindCyclicEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := vm.Evaluate(context.Background(), bound, []string{"normal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(evaluation.Closure.Values) != 3 || len(evaluation.WideningTrace) == 0 {
		t.Fatalf("cyclic evaluation = %#v", evaluation)
	}
	widened := false
	for _, trace := range evaluation.WideningTrace {
		widened = widened || trace.Widened
	}
	if !widened {
		t.Fatalf("widening trace = %#v, want exact widened update", evaluation.WideningTrace)
	}
}

func TestCyclicVMCancellationPublishesNoPartialResult(t *testing.T) {
	artifact, entry, contracts := cyclicVMFixture(t)
	calls := 0
	registry, err := NewCyclicKernelRegistry([]CyclicKernelBinding{
		{KernelID: "seed", ContractID: contracts[0], Kernel: CyclicKernelFunc(func(context.Context, BoundCyclicEquation, CyclicSnapshot) (TransactionResult, error) {
			calls++
			return TransactionResult{Complete: true}, nil
		})},
		{KernelID: "loop", ContractID: contracts[1], Kernel: CyclicKernelFunc(func(context.Context, BoundCyclicEquation, CyclicSnapshot) (TransactionResult, error) {
			calls++
			return TransactionResult{Complete: true}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewCyclicVM(registry)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindCyclicEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	evaluation, err := vm.Evaluate(ctx, bound, []string{"normal"})
	if !errors.Is(err, context.Canceled) || len(evaluation.Closure.Values) != 0 || len(evaluation.WideningTrace) != 0 || evaluation.Transactions != 0 || calls != 0 {
		t.Fatalf("canceled evaluation = %#v, %v; calls=%d", evaluation, err, calls)
	}
}
