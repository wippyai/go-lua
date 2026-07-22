package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCompileEquationIRWalksEnvironmentWriteThroughExemplar(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	formalEnvironmentWriteSealRootCarrier(t, base)
	arena := base.bodies[0].relation.code.terms
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepEnvironmentWrite, slot: statekey.SymbolValue(101), value: arena.Root(Root{Kind: RootParam, Index: 0})}}, next: 2},
		{kind: relationNodeBottom},
	})
	compiler, err := equation.Skeleton().With("environment-write", equation.BindExistingKernel("transformer/formal-environment-write/v1"))
	if err != nil {
		t.Fatal(err)
	}
	// This tiny existing relation also has its structural nonreturning terminal.
	// It is bound only to let the test reach the environment-write exemplar;
	// Skeleton itself remains fail-closed for every non-exemplar family.
	compiler, err = compiler.With("nonreturning", equation.BindExistingKernel("transformer/formal-nonreturning/v1"))
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	artifact, err := program.CompileEquationIR(compiler, func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		calls++
		contract, contractErr := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, uint64(calls)))
		if contractErr != nil {
			return equation.Draft{}, contractErr
		}
		body := equation.BodyID(occurrence.Body)
		entry := equation.EntryParameter{Body: body, Name: "entry"}
		operands := make([]equation.Operand, 0, len(contract.Operands))
		for _, role := range contract.Operands {
			operands = append(operands, equation.Operand{Role: string(role), Term: equation.ClosedTerm([]byte(role))})
		}
		return equation.Draft{Target: equation.Coordinate{Body: body, Name: string(occurrence.Kind) + "-output"}, Entry: entry, Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())}, Operands: operands}, nil
	})
	if err != nil {
		t.Fatalf("CompileEquationIR: %v", err)
	}
	if calls != 2 || len(artifact.Equations) != 2 {
		t.Fatalf("walker calls/artifact = %d/%d, want 2/2", calls, len(artifact.Equations))
	}
	var environment equation.Equation
	for _, lowered := range artifact.Equations {
		if lowered.Occurrence.Kind == "environment-write" {
			environment = lowered
		}
	}
	if environment.KernelID != "transformer/formal-environment-write/v1" {
		t.Fatalf("environment kernel = %q", environment.KernelID)
	}
}
