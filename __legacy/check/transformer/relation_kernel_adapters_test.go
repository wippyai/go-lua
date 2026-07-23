package transformer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestRelationKernelRegistryCallsOnlyContractBoundCanonicalKernels(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	body := lexicalidentity.RootBody(namespace)
	program, err := FreezeRelationProgram([]RelationProgramUnit{formalTemplateFreezeUnit(t, body)}, testAcyclicCallTopology(t, body))
	if err != nil {
		t.Fatal(err)
	}
	binding, err := program.BindRealRelationBody(body, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := binding.Compile()
	if err != nil {
		t.Fatal(err)
	}
	calls := make(map[uint64]int)
	kernels := make(map[uint64]RelationOccurrenceKernel)
	for _, occurrence := range binding.Occurrences() {
		ordinal := occurrence.Ordinal
		kernels[ordinal] = func(boundOccurrence BoundRelationOccurrence, _ equation.BoundEquation, _ equation.Partition) (equation.TransactionResult, error) {
			calls[ordinal]++
			return equation.TransactionResult{
				Complete: true,
				Closure:  equation.OutputClosure{Values: []equation.Fact{{Key: fmt.Sprintf("occurrence-%d", ordinal), Value: []byte(boundOccurrence.Kind)}}},
				Access:   equation.AccessRecord{Payload: OperatorAccess{Kind: boundOccurrence.Kind, Occurrence: formalOccurrenceID(body, ordinal)}},
			}, nil
		}
	}
	registry, err := binding.KernelRegistry(kernels)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := equation.NewAcyclicVM(registry)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := equation.BindEntry(artifact, equation.EntryBinding{Parameter: equation.EntryParameter{Body: equation.BodyID(body), Name: "entry"}, Value: []byte("real-entry")})
	if err != nil {
		t.Fatal(err)
	}
	evaluated, err := vm.Evaluate(bound)
	if err != nil {
		t.Fatal(err)
	}
	if evaluated.Transactions != len(kernels) || len(calls) != len(kernels) {
		t.Fatalf("transactions/canonical calls = %d/%d, want %d", evaluated.Transactions, len(calls), len(kernels))
	}
}

func TestRelationKernelRegistryRejectsAlteredRuntimeOperand(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	body := lexicalidentity.RootBody(namespace)
	program, err := FreezeRelationProgram([]RelationProgramUnit{formalTemplateFreezeUnit(t, body)}, testAcyclicCallTopology(t, body))
	if err != nil {
		t.Fatal(err)
	}
	binding, err := program.BindRealRelationBody(body, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := binding.Compile()
	if err != nil {
		t.Fatal(err)
	}
	kernels := make(map[uint64]RelationOccurrenceKernel)
	for _, occurrence := range binding.Occurrences() {
		kernels[occurrence.Ordinal] = func(BoundRelationOccurrence, equation.BoundEquation, equation.Partition) (equation.TransactionResult, error) {
			return equation.TransactionResult{Complete: true, Access: equation.AccessRecord{Payload: OperatorAccess{}}}, nil
		}
	}
	registry, err := binding.KernelRegistry(kernels)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := equation.BindEntry(artifact, equation.EntryBinding{Parameter: equation.EntryParameter{Body: equation.BodyID(body), Name: "entry"}, Value: []byte("real-entry")})
	if err != nil {
		t.Fatal(err)
	}
	altered := false
	for equationIndex := range bound.Equations {
		for operandIndex := range bound.Equations[equationIndex].Operands {
			if bound.Equations[equationIndex].Operands[operandIndex].Role != string(AccessEntry) {
				bound.Equations[equationIndex].Operands[operandIndex].Value = []byte("foreign-slot")
				altered = true
				break
			}
		}
		if altered {
			break
		}
	}
	if !altered {
		t.Fatal("fixture had no non-entry operand")
	}
	vm, err := equation.NewAcyclicVM(registry)
	if err != nil {
		t.Fatal(err)
	}
	_, err = vm.Evaluate(bound)
	if err == nil || !strings.Contains(err.Error(), "unbound") {
		t.Fatalf("altered operand error = %v", err)
	}
}

func formalOccurrenceID(body lexicalidentity.StableLexicalBodyID, ordinal uint64) formal.OccurrenceID {
	return formal.NewOccurrenceID(body, ordinal)
}
