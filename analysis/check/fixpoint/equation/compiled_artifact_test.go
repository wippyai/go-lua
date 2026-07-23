package equation

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/solve"
)

func TestCompileArtifactBuildsConservativeCompactPlan(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	compiled, err := CompileArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.ContentID() != artifact.ContentID() || compiled.Layout() != (CompiledCellLayout{OperationCount: 3, OperandCount: 3, GuardCount: 1}) {
		t.Fatalf("compiled layout = %#v, id = %x", compiled.Layout(), compiled.ContentID())
	}
	operands := compiled.Operands()
	entryOperand := -1
	for index, operand := range operands {
		if operand.Kind == OperandEntryProjection {
			entryOperand = index
		}
	}
	if entryOperand < 0 || operands[entryOperand].Kind != OperandEntryProjection {
		t.Fatalf("operand kinds = %#v", operands)
	}
	if got := string(compiled.OperandBytes(operands[entryOperand])); got != entry.Parameter.Name {
		t.Fatalf("entry spelling = %q", got)
	}
	for _, operand := range operands {
		if operand.Kind == OperandCanonicalConstant && string(compiled.OperandBytes(operand)) == "identity" {
			goto foundIdentity
		}
	}
	t.Fatalf("compiled constants omitted identity: %#v", operands)
foundIdentity:
	if blocks := compiled.Blocks(); len(blocks) != 1 || blocks[0].OpCount != 3 {
		t.Fatalf("blocks = %#v", blocks)
	}
	reconstructed, err := compiled.ReferenceArtifact()
	if err != nil || string(reconstructed.CanonicalBytes()) != string(artifact.CanonicalBytes()) {
		t.Fatalf("reconstructed = %#v, %v", reconstructed, err)
	}
	report, err := RunCompiledDifferential(stage3VM(t, contracts), []CompiledDifferentialCase{{Name: "fixture", Artifact: artifact, Compiled: compiled, Entry: entry}})
	if err != nil || report.Passed != 1 {
		t.Fatalf("differential = %#v, %v", report, err)
	}
}

func TestCompileArtifactRejectsCyclicProgram(t *testing.T) {
	artifact, entry, _ := stage3Artifact(t)
	artifact.Equations[0].Dependencies = []Coordinate{{Body: entry.Parameter.Body, Name: "guarded-return"}}
	artifact.Equations[1].Dependencies = []Coordinate{{Body: entry.Parameter.Body, Name: "identity"}}
	if _, err := CompileArtifact(artifact); err == nil {
		t.Fatal("cyclic artifact was compiled as a straight-line plan")
	}
}

func TestCompileCyclicArtifactKeepsFrozenWTOAndTraceOracle(t *testing.T) {
	artifact, entry, contracts := cyclicVMFixture(t)
	compiled, err := CompileCyclicArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	wantPlan := []solve.WTOElement[CellID]{
		{Vertex: "seed"},
		{Vertex: "loop", Body: []solve.WTOElement[CellID]{}},
	}
	if !reflect.DeepEqual(artifact.Plan.Elements(), wantPlan) || !reflect.DeepEqual(compiled.frozen.Plan.Elements(), wantPlan) {
		t.Fatalf("frozen WTO differs: source=%#v compiled=%#v want=%#v", artifact.Plan.Elements(), compiled.frozen.Plan.Elements(), wantPlan)
	}
	wantBlocks := []CompiledWTOBlock{
		{Operation: 1, ChildStart: 1, ChildCount: 0},
		{Operation: 0, ChildStart: 2, ChildCount: 0},
	}
	if !reflect.DeepEqual(compiled.Blocks(), wantBlocks) {
		t.Fatalf("compiled WTO blocks = %#v, want %#v", compiled.Blocks(), wantBlocks)
	}
	registry, err := NewCyclicKernelRegistry([]CyclicKernelBinding{
		{KernelID: "seed", ContractID: contracts[0], Kernel: CyclicKernelFunc(func(_ context.Context, _ BoundCyclicEquation, _ CyclicSnapshot) (TransactionResult, error) {
			return TransactionResult{Complete: true, Closure: OutputClosure{Values: []Fact{{Key: "seed", Value: []byte("concrete")}}}}, nil
		})},
		{KernelID: "loop", ContractID: contracts[1], Kernel: CyclicKernelFunc(func(_ context.Context, _ BoundCyclicEquation, snapshot CyclicSnapshot) (TransactionResult, error) {
			if len(snapshot.Read("loop").Leaves) == 0 {
				return TransactionResult{Complete: true, Closure: OutputClosure{Values: []Fact{{Key: "round-0", Value: []byte("v")}}}}, nil
			}
			if len(snapshot.Read("loop").Leaves[0].Closure.Values) == 1 {
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
	baseline, err := vm.Evaluate(context.Background(), mustBindCyclicEntry(t, artifact, entry), []string{"normal"})
	if err != nil {
		t.Fatal(err)
	}
	compiledEvaluation, err := vm.Evaluate(context.Background(), mustBindCyclicEntry(t, compiled.frozen, entry), []string{"normal"})
	if err != nil {
		t.Fatal(err)
	}
	wantClosure := OutputClosure{Values: []Fact{
		{Key: "round-0", Value: []byte("v")},
		{Key: "round-1", Value: []byte("v")},
		{Key: "seed", Value: []byte("concrete")},
	}}
	if !baseline.Closure.Equal(wantClosure) {
		t.Fatalf("source cyclic closure = %#v, want %#v", baseline.Closure, wantClosure)
	}
	wantTrace := []struct {
		Cell    CellID
		Visit   int
		Widened bool
	}{
		{Cell: "loop", Visit: 0, Widened: false},
		{Cell: "loop", Visit: 1, Widened: true},
		{Cell: "loop", Visit: 2, Widened: true},
	}
	traceShape := func(traces []WideningTrace) []struct {
		Cell    CellID
		Visit   int
		Widened bool
	} {
		shape := make([]struct {
			Cell    CellID
			Visit   int
			Widened bool
		}, len(traces))
		for index, trace := range traces {
			shape[index] = struct {
				Cell    CellID
				Visit   int
				Widened bool
			}{Cell: trace.Cell, Visit: trace.Visit, Widened: trace.Widened}
		}
		return shape
	}
	gotTrace := traceShape(baseline.WideningTrace)
	if !reflect.DeepEqual(gotTrace, wantTrace) {
		t.Fatalf("source widening schedule = %#v, want %#v", gotTrace, wantTrace)
	}
	compiledTrace := traceShape(compiledEvaluation.WideningTrace)
	if !compiledEvaluation.Closure.Equal(baseline.Closure) || !reflect.DeepEqual(compiledTrace, wantTrace) {
		t.Fatalf("compiled cyclic evaluation differs: source=%#v compiled=%#v", baseline, compiledEvaluation)
	}
	for index, sourceTrace := range baseline.WideningTrace {
		if !sourceTrace.Equal(compiledEvaluation.WideningTrace[index]) {
			t.Fatalf("widening trace[%d] differs: source=%#v compiled=%#v", index, sourceTrace, compiledEvaluation.WideningTrace[index])
		}
	}
}
