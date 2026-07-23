package equation

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func stage3Artifact(t *testing.T) (Artifact, EntryBinding, []ContentID) {
	t.Helper()
	body := testBody(31)
	entry := EntryParameter{Body: body, Name: "entry"}
	contracts := []ContentID{testID(41), testID(42), testID(43)}
	artifact := Artifact{Equations: []Equation{
		{Target: Coordinate{Body: body, Name: "identity"}, Entry: entry, Occurrence: Occurrence{Kind: "entry", ContractID: contracts[0]}, KernelID: "canonical/identity", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
		{Target: Coordinate{Body: body, Name: "guarded-return"}, Entry: entry, Guards: []Guard{{Body: body, Encoding: []byte("not-nil")}}, Occurrence: Occurrence{Kind: "outcome", ContractID: contracts[1]}, KernelID: "canonical/guarded-return", Operands: []Operand{{Role: "flow", Term: ClosedTerm([]byte("identity"))}}},
		{Target: Coordinate{Body: body, Name: "copied-store"}, Entry: entry, Dependencies: []Coordinate{{Body: body, Name: "identity"}}, Occurrence: Occurrence{Kind: "environment-write", ContractID: contracts[2]}, KernelID: "canonical/copied-store", Operands: []Operand{{Role: "store", Term: ClosedTerm([]byte("source"))}}},
	}}
	if artifact.CanonicalBytes() == nil {
		t.Fatal("stage-3 fixture artifact is invalid")
	}
	return artifact, EntryBinding{Parameter: entry, Value: []byte("caller-entry")}, contracts
}

func stage3VM(t *testing.T, contracts []ContentID) *AcyclicVM {
	t.Helper()
	registry, err := NewKernelRegistry([]KernelBinding{
		{KernelID: "canonical/identity", ContractID: contracts[0], Kernel: KernelFunc(func(equation BoundEquation, _ Partition) (TransactionResult, error) {
			if len(equation.Operands) != 1 || !bytes.Equal(equation.Operands[0].Value, []byte("caller-entry")) {
				return TransactionResult{}, fmt.Errorf("entry was not closed")
			}
			return TransactionResult{Complete: true, Closure: OutputClosure{Values: []Fact{{Key: "identity", Value: equation.Operands[0].Value}}}}, nil
		})},
		{KernelID: "canonical/guarded-return", ContractID: contracts[1], Kernel: KernelFunc(func(equation BoundEquation, _ Partition) (TransactionResult, error) {
			if len(equation.Guards) != 1 || string(equation.Guards[0].Encoding) != "not-nil" {
				return TransactionResult{}, fmt.Errorf("guard was not retained")
			}
			return TransactionResult{Complete: true, Closure: OutputClosure{Outcomes: []Fact{{Key: "return", Value: []byte("normal")}}, Diagnostics: []Fact{{Key: "guard-witness", Value: []byte("not-nil")}}}}, nil
		})},
		{KernelID: "canonical/copied-store", ContractID: contracts[2], Kernel: KernelFunc(func(_ BoundEquation, partition Partition) (TransactionResult, error) {
			values := partition.Values()
			if len(values) != 1 || values[0].Key != "identity" || !bytes.Equal(values[0].Value, []byte("caller-entry")) {
				return TransactionResult{}, fmt.Errorf("completed partition was not supplied")
			}
			return TransactionResult{Complete: true, Closure: OutputClosure{Values: []Fact{{Key: "copied-store", Value: values[0].Value}}, AllocationRekeys: []AllocationRekey{{From: "formal:table", To: "caller:table"}}}}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewAcyclicVM(registry)
	if err != nil {
		t.Fatal(err)
	}
	return vm
}

func TestAcyclicBoundEvaluatorIdentityWitness(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := stage3VM(t, contracts).Evaluate(bound)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Transactions != 3 || len(evaluation.Closure.Values) != 2 || string(evaluation.Closure.Values[0].Value) != "caller-entry" {
		t.Fatalf("identity witness closure = %#v", evaluation)
	}
}

func TestAcyclicBoundEvaluatorGuardedReturnWitness(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := stage3VM(t, contracts).Evaluate(bound)
	if err != nil {
		t.Fatal(err)
	}
	if len(evaluation.Closure.Outcomes) != 1 || evaluation.Closure.Outcomes[0].Key != "return" || !bytes.Equal(evaluation.Closure.Outcomes[0].Value, []byte("normal")) ||
		len(evaluation.Closure.Diagnostics) != 1 || evaluation.Closure.Diagnostics[0].Key != "guard-witness" {
		t.Fatalf("guarded-return witness closure = %#v", evaluation.Closure)
	}
}

func TestAcyclicBoundEvaluatorCopiedStoreWitness(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := stage3VM(t, contracts).Evaluate(bound)
	if err != nil {
		t.Fatal(err)
	}
	if len(evaluation.Closure.AllocationRekeys) != 1 || evaluation.Closure.AllocationRekeys[0] != (AllocationRekey{From: "formal:table", To: "caller:table"}) ||
		!bytes.Equal(evaluation.Closure.Values[0].Value, evaluation.Closure.Values[1].Value) {
		t.Fatalf("copied-store witness closure = %#v", evaluation.Closure)
	}
}

func TestAcyclicBoundEvaluatorDoesNotPublishPartialTransaction(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	registry, err := NewKernelRegistry([]KernelBinding{
		{KernelID: "canonical/identity", ContractID: contracts[0], Kernel: KernelFunc(func(BoundEquation, Partition) (TransactionResult, error) {
			return TransactionResult{Complete: true, Closure: OutputClosure{Values: []Fact{{Key: "would-leak", Value: []byte("x")}}}}, nil
		})},
		{KernelID: "canonical/guarded-return", ContractID: contracts[1], Kernel: KernelFunc(func(BoundEquation, Partition) (TransactionResult, error) {
			return TransactionResult{Complete: false, Closure: OutputClosure{Outcomes: []Fact{{Key: "must-not-publish", Value: []byte("x")}}}}, nil
		})},
		{KernelID: "canonical/copied-store", ContractID: contracts[2], Kernel: KernelFunc(func(BoundEquation, Partition) (TransactionResult, error) {
			return TransactionResult{Complete: true}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewAcyclicVM(registry)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Evaluate(bound); !errors.Is(err, ErrIncompleteTransaction) {
		t.Fatalf("partial transaction error = %v", err)
	}
}

func TestAcyclicBoundEvaluatorRejectsMissingContractBoundKernel(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	registry, err := NewKernelRegistry([]KernelBinding{{KernelID: "canonical/identity", ContractID: contracts[0], Kernel: KernelFunc(func(BoundEquation, Partition) (TransactionResult, error) {
		return TransactionResult{Complete: true}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewAcyclicVM(registry)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Evaluate(bound); err == nil {
		t.Fatal("missing contract-bound kernel was accepted")
	}
}

func TestAcyclicBoundEvaluatorRejectsCyclicArtifact(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	artifact.Equations[0].Dependencies = []Coordinate{{Body: entry.Parameter.Body, Name: "guarded-return"}}
	artifact.Equations[1].Dependencies = []Coordinate{{Body: entry.Parameter.Body, Name: "identity"}}
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage3VM(t, contracts).Evaluate(bound); err == nil {
		t.Fatal("cyclic artifact was accepted")
	}
}

func TestAcyclicBoundEvaluatorShadowDifferentialCorpus(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	production := OutputClosure{
		Values:   []Fact{{Key: "identity", Value: []byte("caller-entry")}, {Key: "copied-store", Value: []byte("caller-entry")}},
		Outcomes: []Fact{{Key: "return", Value: []byte("normal")}}, Diagnostics: []Fact{{Key: "guard-witness", Value: []byte("not-nil")}},
		AllocationRekeys: []AllocationRekey{{From: "formal:table", To: "caller:table"}},
	}
	cases := []ShadowCase{
		{Name: "identity", Artifact: artifact, Entry: entry, Production: func() (OutputClosure, error) { return production, nil }},
		{Name: "guarded-return", Artifact: artifact, Entry: entry, Production: func() (OutputClosure, error) { return production, nil }},
		{Name: "copied-store", Artifact: artifact, Entry: entry, Production: func() (OutputClosure, error) { return production, nil }},
	}
	report, err := RunShadow(stage3VM(t, contracts), cases)
	if err != nil {
		t.Fatal(err)
	}
	if report.Cases != 3 || report.Passed != 3 {
		t.Fatalf("shadow report = %#v", report)
	}
}

func TestAcyclicBoundEvaluatorShadowRejectsPublishedDifference(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	_, err := RunShadow(stage3VM(t, contracts), []ShadowCase{{
		Name: "published-difference", Artifact: artifact, Entry: entry,
		Production: func() (OutputClosure, error) {
			return OutputClosure{Values: []Fact{{Key: "identity", Value: []byte("wrong-entry")}}}, nil
		},
	}})
	if err == nil {
		t.Fatal("shadow accepted unequal published output")
	}
}
