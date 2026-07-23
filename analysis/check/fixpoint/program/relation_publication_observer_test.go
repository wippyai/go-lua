package program

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRelationPublicationObserverRetainsOnlyThePublicationBoundary(t *testing.T) {
	calls := 0
	corpusSize, passed := 0, 0
	_, err := RunChunk(parseChunk(t, `
local value = "retained"
return value
`), Config{
		Check: body.Config{Registry: standard.Registry()},
		relationPublicationObserver: func(
			frozen *transformer.RelationProgram,
			execution transformer.RelationSolveExecution,
			published formalLexicalPublishedProgram,
		) error {
			calls++
			if frozen == nil || execution == (transformer.RelationSolveExecution{}) {
				t.Fatal("publication observer received no frozen solve")
			}
			if published.root == nil || len(published.results) == 0 {
				t.Fatal("publication observer ran before lexical results were published")
			}
			for _, plan := range frozen.BodyPlanObservations() {
				if !plan.Acyclic {
					continue
				}
				corpusSize++
				result := published.results[plan.Body]
				entry, ok := result.EntryState()
				if !ok {
					t.Fatalf("acyclic body %s has no published entry", plan.Body)
				}
				binding, err := frozen.BindRealRelationBody(plan.Body, entry)
				if err != nil {
					t.Fatalf("bind acyclic body %s: %v", plan.Body, err)
				}
				artifact, err := binding.Compile()
				if err != nil {
					t.Fatalf("compile acyclic body %s: %v", plan.Body, err)
				}
				kernels := make(map[uint64]transformer.RelationOccurrenceKernel)
				for _, occurrence := range binding.Occurrences() {
					occurrence := occurrence
					kernels[occurrence.Ordinal] = func(bound transformer.BoundRelationOccurrence, _ equation.BoundEquation, _ equation.Partition) (equation.TransactionResult, error) {
						if bound.Ordinal != occurrence.Ordinal {
							return equation.TransactionResult{}, fmt.Errorf("foreign occurrence %d", bound.Ordinal)
						}
						return equation.TransactionResult{
							Complete: true,
							Closure:  transformer.PublishedRelationClosure{Values: []equation.Fact{}, Outcomes: []equation.Fact{}, DiagnosticCandidates: []equation.Fact{}, AllocationRekeys: []equation.AllocationRekey{}}.ToOutputClosure(),
							Access: equation.AccessRecord{Payload: transformer.OperatorAccess{
								Kind: occurrence.Kind, Occurrence: formal.NewOccurrenceID(plan.Body, occurrence.Ordinal),
							}},
						}, nil
					}
				}
				registry, err := binding.KernelRegistry(kernels)
				if err != nil {
					t.Fatalf("kernels acyclic body %s: %v", plan.Body, err)
				}
				vm, err := equation.NewAcyclicVM(registry)
				if err != nil {
					t.Fatal(err)
				}
				bound, err := equation.BindEntry(artifact, equation.EntryBinding{Parameter: equation.EntryParameter{Body: equation.BodyID(plan.Body), Name: "entry"}, Value: []byte(fmt.Sprintf("entry/%s", plan.Body))})
				if err != nil {
					t.Fatalf("entry acyclic body %s: %v", plan.Body, err)
				}
				evaluated, err := vm.Evaluate(bound)
				if err != nil {
					t.Fatalf("evaluate acyclic body %s: %v", plan.Body, err)
				}
				production := transformer.PublishedRelationClosure{Values: []equation.Fact{}, Outcomes: []equation.Fact{}, DiagnosticCandidates: []equation.Fact{}, AllocationRekeys: []equation.AllocationRekey{}}
				if !production.ToOutputClosure().Equal(evaluated.Closure) {
					t.Fatalf("closure mismatch for acyclic body %s", plan.Body)
				}
				passed++
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("publication observer calls = %d, want 1", calls)
	}
	if corpusSize == 0 || passed != corpusSize {
		t.Fatalf("CORPUS SIZE %d; PASS RATE %d/%d", corpusSize, passed, corpusSize)
	}
	t.Logf("CORPUS SIZE %d; PASS RATE %d/%d", corpusSize, passed, corpusSize)
}
