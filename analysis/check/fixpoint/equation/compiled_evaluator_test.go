package equation

import (
	"bytes"
	"testing"
)

func TestFastEvaluatorMatchesAcyclicVMAndClearsScratch(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	compiled, err := CompileArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	vm := stage3VM(t, contracts)
	reference, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	want, err := vm.Evaluate(reference)
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := NewFastEvaluator(vm)
	if err != nil {
		t.Fatal(err)
	}
	scratch, err := NewEvaluatorScratch(compiled)
	if err != nil {
		t.Fatal(err)
	}
	got, err := evaluator.Evaluate(compiled, entry, scratch)
	if err != nil {
		t.Fatal(err)
	}
	if got.Transactions != want.Transactions || !got.Closure.Equal(want.Closure) {
		t.Fatalf("compiled evaluation = %#v, want %#v", got, want)
	}
	if scratch.depth != 0 || scratch.overflow != 0 {
		t.Fatalf("scratch state after evaluation = depth %d overflow %d", scratch.depth, scratch.overflow)
	}
	for _, frame := range scratch.frames {
		for _, operand := range frame.operands {
			if operand.Role != "" || operand.Value != nil {
				t.Fatalf("scratch retained operand %#v", operand)
			}
		}
		for _, guard := range frame.guards {
			if guard.Body.Valid() || guard.Encoding != nil {
				t.Fatalf("scratch retained guard %#v", guard)
			}
		}
		for _, dependency := range frame.dependencies {
			if dependency.Body.Valid() || dependency.Name != "" {
				t.Fatalf("scratch retained dependency %#v", dependency)
			}
		}
	}
}

func TestFastEvaluatorUsesEntryProjectionWithoutRebinding(t *testing.T) {
	artifact, entry, _ := stage3Artifact(t)
	compiled, err := CompileArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	value := []byte("second-caller-entry")
	entry.Value = value
	scratch, err := NewEvaluatorScratch(compiled)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := scratch.acquire(compiled)
	if err != nil {
		t.Fatal(err)
	}
	defer scratch.release(frame)
	var op CompiledOp
	found := false
	for _, candidate := range compiled.ops {
		for _, operand := range compiled.operands[candidate.OperandStart : candidate.OperandStart+candidate.OperandCount] {
			if operand.Kind == OperandEntryProjection {
				op, found = candidate, true
				break
			}
		}
	}
	if !found {
		t.Fatal("compiled plan has no entry projection")
	}
	equation, err := compiled.bindOperation(op, entry, frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(equation.Operands) != 1 || !bytes.Equal(equation.Operands[0].Value, value) || &equation.Operands[0].Value[0] != &value[0] {
		t.Fatalf("entry projection operand = %#v", equation.Operands)
	}
}

func compiledNoOutputFixture(t testing.TB) (CompiledArtifact, EntryBinding, *FastEvaluator, *EvaluatorScratch) {
	t.Helper()
	body := testBody(91)
	entry := EntryParameter{Body: body, Name: "entry"}
	contract := testID(92)
	artifact := Artifact{Equations: []Equation{{
		Target:     Coordinate{Body: body, Name: "hot"},
		Entry:      entry,
		Occurrence: Occurrence{Kind: "hot", ContractID: contract},
		KernelID:   "hot/no-output",
		Operands:   []Operand{{Role: "entry", Term: EntryTerm(entry)}},
	}}}
	compiled, err := CompileArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewKernelRegistry([]KernelBinding{{
		KernelID: "hot/no-output", ContractID: contract,
		Kernel: KernelFunc(func(equation BoundEquation, _ Partition) (TransactionResult, error) {
			if len(equation.Operands) != 1 || !bytes.Equal(equation.Operands[0].Value, []byte("hot-entry")) {
				return TransactionResult{}, ErrIncompleteTransaction
			}
			return TransactionResult{Complete: true}, nil
		}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewAcyclicVM(registry)
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := NewFastEvaluator(vm)
	if err != nil {
		t.Fatal(err)
	}
	scratch, err := NewEvaluatorScratch(compiled)
	if err != nil {
		t.Fatal(err)
	}
	return compiled, EntryBinding{Parameter: entry, Value: []byte("hot-entry")}, evaluator, scratch
}

func TestFastEvaluatorWarmPathAllocs(t *testing.T) {
	compiled, entry, evaluator, scratch := compiledNoOutputFixture(t)
	if _, err := evaluator.Evaluate(compiled, entry, scratch); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(1000, func() {
		result, err := evaluator.Evaluate(compiled, entry, scratch)
		if err != nil || result.Transactions != 1 {
			t.Fatalf("compiled warm evaluation = %#v, %v", result, err)
		}
	})
	if allocs != 0 {
		t.Fatalf("compiled warm evaluation allocated %.2f times/run", allocs)
	}
}

func TestCompiledArtifactInternsArenaTextAtAdmission(t *testing.T) {
	compiled, entry, _, scratch := compiledNoOutputFixture(t)
	op := compiled.ops[0]
	compiled.bytes[op.TargetOffset] = 'X'
	compiled.bytes[op.KernelOffset] = 'X'
	compiled.bytes[compiled.operands[op.OperandStart].RoleOffset] = 'X'

	bound, err := compiled.bindOperation(op, entry, &scratch.frames[0])
	if err != nil {
		t.Fatal(err)
	}
	if bound.Target.Name != "hot" || bound.KernelID != "hot/no-output" || bound.Operands[0].Role != "entry" {
		t.Fatalf("compiled text aliases mutable arena: %#v", bound)
	}
}

func BenchmarkFastEvaluatorWarmNoOutput(b *testing.B) {
	compiled, entry, evaluator, scratch := compiledNoOutputFixture(b)
	if _, err := evaluator.Evaluate(compiled, entry, scratch); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := evaluator.Evaluate(compiled, entry, scratch); err != nil {
			b.Fatal(err)
		}
	}
}
