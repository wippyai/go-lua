package transformer

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

const formalCallResultsEquationKernel = "transformer/formal-call-results/v1"

// callResultsEquationContract declares the complete family surface shared by
// the sealed N0 materialization and N3 postcondition factor programs.  The
// formal tuple remains the existing factapply kernel's carrier; these names
// only identify the already-sealed semantic surfaces for audit.
func callResultsEquationContract(occurrence formal.OccurrenceID) (OperatorContract, error) {
	contract, err := NewOperatorContract(OperatorCallResults, occurrence)
	if err != nil {
		return OperatorContract{}, err
	}
	contract.Reads = []ContractSelector{
		{Role: AccessFlow, Name: "predecessor"},
		{Role: AccessGuard, Name: "phase-guard"},
		{Role: AccessState, Name: "formal-values-and-lanes"},
	}
	contract.Writes = []ContractSelector{{Role: AccessState, Name: "formal-values-and-lanes"}}
	return contract, nil
}

func callResultsEquationDraft(occurrence RelationEquationOccurrence, ordinal uint64) (equation.Draft, error) {
	contract, err := callResultsEquationContract(formal.NewOccurrenceID(occurrence.Body, ordinal))
	if err != nil {
		return equation.Draft{}, err
	}
	body := equation.BodyID(occurrence.Body)
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	operands := make([]equation.Operand, 0, len(contract.Operands))
	for _, role := range contract.Operands {
		// These are closed names for the tuple surfaces sealed by the relation
		// template, not State values or a fallback evaluator.
		operands = append(operands, equation.Operand{Role: string(role), Term: equation.ClosedTerm([]byte("call-results/" + string(role)))})
	}
	return equation.Draft{
		Target:     equation.Coordinate{Body: body, Name: "call-results-output"},
		Entry:      entry,
		Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())},
		Operands:   operands,
	}, nil
}

func TestCompileEquationIRWalksCallResultsThroughExistingKernel(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepCallResults, resultPhase: factapply.CallResultPhaseMaterialize}}, next: 2},
		{kind: relationNodeBottom},
	})
	compiler, err := equation.Skeleton().With("call-results", equation.BindExistingKernel(formalCallResultsEquationKernel))
	if err != nil {
		t.Fatal(err)
	}
	// The structural Bottom terminal is a separate frozen occurrence.  Bind it
	// only so this representative body reaches the call-results hook.
	compiler, err = compiler.With("nonreturning", equation.BindExistingKernel("transformer/formal-nonreturning/v1"))
	if err != nil {
		t.Fatal(err)
	}
	var calls uint64
	artifact, err := program.CompileEquationIR(compiler, func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		calls++
		if occurrence.Kind == OperatorCallResults {
			return callResultsEquationDraft(occurrence, calls)
		}
		contract, contractErr := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, calls))
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
	for _, lowered := range artifact.Equations {
		if lowered.Occurrence.Kind == string(OperatorCallResults) {
			if lowered.KernelID != formalCallResultsEquationKernel {
				t.Fatalf("call-results kernel = %q", lowered.KernelID)
			}
			return
		}
	}
	t.Fatal("call-results occurrence was not lowered")
}

func TestCallResultsLoweringAuditsRecordedExecution(t *testing.T) {
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("call-results-equation-audit")))
	occurrence := formal.NewOccurrenceID(owner, 1)
	contract, err := callResultsEquationContract(occurrence)
	if err != nil {
		t.Fatal(err)
	}
	access := OperatorAccess{Kind: OperatorCallResults, Occurrence: occurrence, Reads: append([]ContractSelector(nil), contract.Reads...), Writes: append([]ContractSelector(nil), contract.Writes...)}
	execution := Execution{Complete: true, Published: true, Access: equation.AccessRecord{Payload: access}}
	if err := VerifyLoweredOperatorAccess(contract, execution); err != nil {
		t.Fatalf("call-results audit: %v", err)
	}

	access.Reads = append(access.Reads, ContractSelector{Role: AccessState, Name: "undeclared-factor"})
	execution.Access.Payload = access
	if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
		t.Fatal("call-results audit accepted undeclared read")
	}

	execution = Execution{Complete: false, Published: true, Access: equation.AccessRecord{Payload: OperatorAccess{Kind: OperatorCallResults, Occurrence: occurrence}}}
	if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
		t.Fatal("call-results lowering published a partial transaction")
	}
}

func TestCallResultsEquationIdentityTracksContractAndOperands(t *testing.T) {
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("call-results-equation-identity")))
	occurrence := RelationEquationOccurrence{Body: owner, Kind: OperatorCallResults}
	left, err := callResultsEquationDraft(occurrence, 1)
	if err != nil {
		t.Fatal(err)
	}
	right := left
	right.Operands = append([]equation.Operand(nil), left.Operands...)
	right.Operands[0].Term = equation.ClosedTerm([]byte("call-results/changed-flow"))
	compiler, err := equation.Skeleton().With("call-results", equation.BindExistingKernel(formalCallResultsEquationKernel))
	if err != nil {
		t.Fatal(err)
	}
	leftArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{left}})
	if err != nil {
		t.Fatal(err)
	}
	rightArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{right}})
	if err != nil {
		t.Fatal(err)
	}
	if leftArtifact.ContentID() == rightArtifact.ContentID() {
		t.Fatal("call-results artifact ignored a semantic operand")
	}
	contract, err := callResultsEquationContract(formal.NewOccurrenceID(owner, 1))
	if err != nil {
		t.Fatal(err)
	}
	contract.Reads = append(contract.Reads, ContractSelector{Role: AccessDiagnostic, Name: "call-result-diagnostic"})
	changedContract := left
	changedContract.Occurrence.ContractID = equation.ContentID(contract.ContentID())
	changedArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{changedContract}})
	if err != nil {
		t.Fatal(err)
	}
	if leftArtifact.ContentID() == changedArtifact.ContentID() {
		t.Fatal("call-results artifact ignored contract content")
	}
	reordered := contract
	reordered.Reads = append([]ContractSelector(nil), contract.Reads...)
	for leftIndex, rightIndex := 0, len(reordered.Reads)-1; leftIndex < rightIndex; leftIndex, rightIndex = leftIndex+1, rightIndex-1 {
		reordered.Reads[leftIndex], reordered.Reads[rightIndex] = reordered.Reads[rightIndex], reordered.Reads[leftIndex]
	}
	if contract.ContentID() != reordered.ContentID() {
		t.Fatal("call-results contract retained declaration order")
	}
	canonicalOrder := left
	canonicalOrder.Occurrence.ContractID = equation.ContentID(reordered.ContentID())
	canonicalOrderArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{canonicalOrder}})
	if err != nil {
		t.Fatal(err)
	}
	if changedArtifact.ContentID() != canonicalOrderArtifact.ContentID() {
		t.Fatal("call-results artifact retained contract declaration order")
	}

	if _, err := equation.Skeleton().Compile(equation.Source{Drafts: []equation.Draft{left}}); !errors.Is(err, equation.ErrUnimplementedLowering) {
		t.Fatalf("uninstalled call-results lowering error = %v", err)
	}
}
