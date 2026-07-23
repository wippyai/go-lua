package equation

import (
	"context"
	"testing"
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
	if len(compiled.Blocks()) != 2 {
		t.Fatalf("compiled WTO blocks = %#v", compiled.Blocks())
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
	t.Logf("blocks=%#v plan=%#v baseline=%#v compiled=%#v", compiled.Blocks(), artifact.Plan.Elements(), baseline, compiledEvaluation)
	if err := RunCompiledCyclicDifferential(context.Background(), vm, artifact, compiled, entry, []string{"normal"}); err != nil {
		t.Fatal(err)
	}
}
