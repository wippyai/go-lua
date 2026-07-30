package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestStage2EquationCompilerOwnsEveryFrozenOperationKind(t *testing.T) {
	compiler, err := Stage2EquationCompiler()
	if err != nil {
		t.Fatal(err)
	}
	kinds := FrozenOperatorKinds()
	if len(kinds) != len(equation.FrozenKinds) {
		t.Fatalf("transformer/equation catalog size = %d/%d", len(kinds), len(equation.FrozenKinds))
	}
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("stage-2-equation-completeness")))
	drafts := make([]equation.Draft, 0, len(kinds))
	for index, kind := range kinds {
		if string(kind) != equation.FrozenKinds[index] {
			t.Fatalf("catalog kind %d = %q, want %q", index, kind, equation.FrozenKinds[index])
		}
		contract, contractErr := NewOperatorContract(kind, formal.NewOccurrenceID(owner, uint64(index+1)))
		if contractErr != nil {
			t.Fatal(contractErr)
		}
		body := equation.BodyID(owner)
		entry := equation.EntryParameter{Body: body, Name: "entry"}
		operands := make([]equation.Operand, 0, len(contract.Operands))
		for _, role := range contract.Operands {
			term := equation.ClosedTerm([]byte(string(kind) + "/" + string(role)))
			if role == AccessEntry {
				term = equation.EntryTerm(entry)
			}
			operands = append(operands, equation.Operand{Role: string(role), Term: term})
		}
		drafts = append(drafts, equation.Draft{
			Target: equation.Coordinate{Body: body, Name: string(kind) + "-output"}, Entry: entry,
			Occurrence: equation.Occurrence{Kind: string(kind), ContractID: equation.ContentID(contract.ContentID())}, Operands: operands,
		})
	}
	artifact, err := compiler.Compile(equation.Source{Drafts: drafts})
	if err != nil {
		t.Fatalf("stage-2 completeness compile: %v", err)
	}
	if len(artifact.Equations) != len(kinds) {
		t.Fatalf("lowered equations = %d, want %d", len(artifact.Equations), len(kinds))
	}
	for index, lowered := range artifact.Equations {
		if lowered.Occurrence.Kind != string(kinds[index]) || lowered.KernelID != stage2EquationBindings[index].kernel {
			t.Fatalf("lowering %d = %#v, want kind=%q kernel=%q", index, lowered, kinds[index], stage2EquationBindings[index].kernel)
		}
	}
}
