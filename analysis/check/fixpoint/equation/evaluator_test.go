package equation

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
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
	want := OutputClosure{
		Values: []Fact{
			{Key: "copied-store", Value: []byte("caller-entry")},
			{Key: "identity", Value: []byte("caller-entry")},
		},
		Outcomes:         []Fact{{Key: "return", Value: []byte("normal"), Guards: []Guard{{Body: entry.Parameter.Body, Encoding: []byte("not-nil")}}}},
		Diagnostics:      []Fact{{Key: "guard-witness", Value: []byte("not-nil"), Guards: []Guard{{Body: entry.Parameter.Body, Encoding: []byte("not-nil")}}}},
		AllocationRekeys: []AllocationRekey{{From: "formal:table", To: "caller:table"}},
	}
	if !reflect.DeepEqual(evaluation.Closure, want) {
		t.Fatalf("copied-store published closure = %#v, want %#v", evaluation.Closure, want)
	}
}

func TestPartitionFromClosuresMatchesSequentialClosedJoin(t *testing.T) {
	body := testBody(51)
	guard := Guard{Body: body, Encoding: []byte("branch")}
	closures := []OutputClosure{
		{Values: []Fact{{Key: "seed", Value: []byte("one")}}, Diagnostics: []Fact{{Key: "note", Value: []byte("seen"), Guards: []Guard{guard}}}},
		{Values: []Fact{{Key: "next", Value: []byte("two")}, {Key: "seed", Value: []byte("one")}}, Outcomes: []Fact{{Key: "return", Value: []byte("ok")}}, AllocationRekeys: []AllocationRekey{{From: "formal", To: "actual"}}},
	}
	want := OutputClosure{}
	for _, closure := range closures {
		var err error
		want, err = joinClosure(want, closure)
		if err != nil {
			t.Fatalf("sequential closed join: %v", err)
		}
	}
	partition, err := PartitionFromClosuresWithGuards(nil, closures...)
	if err != nil {
		t.Fatalf("aggregate closed join: %v", err)
	}
	if !want.Equal(partition.closure) {
		t.Fatalf("aggregate closure = %#v, want sequential %#v", partition.closure, want)
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
		Values:           []Fact{{Key: "identity", Value: []byte("caller-entry")}, {Key: "copied-store", Value: []byte("caller-entry")}},
		Outcomes:         []Fact{{Key: "return", Value: []byte("normal"), Guards: []Guard{{Body: entry.Parameter.Body, Encoding: []byte("not-nil")}}}},
		Diagnostics:      []Fact{{Key: "guard-witness", Value: []byte("not-nil"), Guards: []Guard{{Body: entry.Parameter.Body, Encoding: []byte("not-nil")}}}},
		AllocationRekeys: []AllocationRekey{{From: "formal:table", To: "caller:table"}},
	}
	cases := []ShadowCase{
		{Name: "identity", Artifact: artifact, Entry: entry, Production: func() (OutputClosure, error) { return production, nil }},
		{Name: "guarded-return", Artifact: artifact, Entry: entry, Production: func() (OutputClosure, error) { return production, nil }},
		{Name: "copied-store", Artifact: artifact, Entry: entry, Production: func() (OutputClosure, error) { return production, nil }},
	}
	for _, shadow := range cases {
		t.Run(shadow.Name, func(t *testing.T) {
			want, err := shadow.Production()
			if err != nil {
				t.Fatalf("production: %v", err)
			}
			bound, err := BindEntry(shadow.Artifact, shadow.Entry)
			if err != nil {
				t.Fatalf("BindEntry: %v", err)
			}
			evaluation, err := stage3VM(t, contracts).Evaluate(bound)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if !want.Equal(evaluation.Closure) {
				t.Fatalf("published closure = %#v, want %#v", evaluation.Closure, want)
			}
		})
	}
	report, err := RunShadow(stage3VM(t, contracts), cases)
	if err != nil {
		t.Fatal(err)
	}
	if report.Cases != len(cases) || report.Passed != len(cases) {
		t.Fatalf("shadow report = %#v, want every per-case output comparison to pass", report)
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
