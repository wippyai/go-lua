package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
)

func externalCallEquationContentID(seed byte) ContentID {
	var id ContentID
	id[0] = seed
	return id
}

func externalCallEquationBinding(owner equation.BodyID, occurrence formal.OccurrenceID) ExternalCallEquationBinding {
	anchor := externalCallEquationContentID(1)
	reads := []ContractSelector{
		{Role: AccessFlow, Name: "reduction"},
		{Role: AccessGuard, Name: "provider-ready"},
		{Role: AccessPublished, Name: "published-input"},
		{Role: AccessBoundary, Name: "call-boundary"},
		{Role: AccessDiagnostic, Name: "prior-diagnostics"},
		{Role: AccessAllocation, Name: "result-allocation"},
		{Role: AccessState, Name: "factor-lanes"},
	}
	return ExternalCallEquationBinding{
		Occurrence: occurrence,
		Target:     equation.Coordinate{Body: owner, Name: "external-call-output"},
		Entry:      equation.EntryParameter{Body: owner, Name: "entry"},
		Guards:     []equation.Guard{{Body: owner, Encoding: []byte("provider-ready")}},
		Terms: map[AccessRole]equation.Term{
			AccessFlow:       equation.ClosedTerm([]byte("predecessor-flow")),
			AccessPublished:  equation.ClosedTerm([]byte("published-provider-input")),
			AccessBoundary:   equation.ClosedTerm([]byte("sealed-call-boundary")),
			AccessDiagnostic: equation.ClosedTerm([]byte("diagnostic-frame")),
		},
		Reads: reads,
		Writes: []ContractSelector{
			{Role: AccessState, Name: "factor-lanes"},
			{Role: AccessAllocation, Name: "result-allocation"},
			{Role: AccessDiagnostic, Name: "diagnostics"},
			{Role: AccessOutcome, Name: "call-outcome"},
			{Role: AccessBoundary, Name: "call-boundary-publication"},
		},
		GuardAtoms: []string{"provider-ready"},
		Outcomes:   []OutcomeKind{OutcomeNormal, OutcomeSuspension},
		DiagnosticOutputs: []DiagnosticDescriptor{{
			Candidate: "external-call-boundary", Owner: DiagnosticOwnerApplication, SourceAnchor: anchor,
			GuardAtoms: []string{"provider-ready"}, ReadSet: []ContractSelector{reads[3], reads[4]},
			Predicate: "provider-reported", EvidenceRecipe: "sealed-external-call-factor", BoundaryLens: "call-boundary",
		}},
		Dependencies: []ContractDependency{{Kind: "factapply-external-call", ID: anchor}},
	}
}

func TestExternalCallEquationDraftIsCanonicalAndClosed(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	ownerID := base.bodies[0].body
	owner := equation.BodyID(ownerID)
	occurrence := formal.NewOccurrenceID(ownerID, 1)
	leftBinding := externalCallEquationBinding(owner, occurrence)
	left, leftContract, err := NewExternalCallEquationDraft(leftBinding)
	if err != nil {
		t.Fatal(err)
	}
	rightBinding := externalCallEquationBinding(owner, occurrence)
	rightBinding.Reads[0], rightBinding.Reads[len(rightBinding.Reads)-1] = rightBinding.Reads[len(rightBinding.Reads)-1], rightBinding.Reads[0]
	rightBinding.Writes[0], rightBinding.Writes[len(rightBinding.Writes)-1] = rightBinding.Writes[len(rightBinding.Writes)-1], rightBinding.Writes[0]
	right, rightContract, err := NewExternalCallEquationDraft(rightBinding)
	if err != nil {
		t.Fatal(err)
	}
	if leftContract.ContentID() != rightContract.ContentID() || (equation.Artifact{Equations: []equation.Equation{mustExternalCallEquation(t, left)}}).ContentID() != (equation.Artifact{Equations: []equation.Equation{mustExternalCallEquation(t, right)}}).ContentID() {
		t.Fatal("external-call lowering retained declaration order")
	}
	rightBinding.Terms[AccessBoundary] = equation.ClosedTerm([]byte("different-boundary"))
	changed, _, err := NewExternalCallEquationDraft(rightBinding)
	if err != nil {
		t.Fatal(err)
	}
	if (equation.Artifact{Equations: []equation.Equation{mustExternalCallEquation(t, left)}}).ContentID() == (equation.Artifact{Equations: []equation.Equation{mustExternalCallEquation(t, changed)}}).ContentID() {
		t.Fatal("external-call lowering lost semantic operand identity")
	}

	leftBinding.Terms[AccessFlow] = equation.EntryTerm(leftBinding.Entry)
	if _, _, err := NewExternalCallEquationDraft(leftBinding); err == nil {
		t.Fatal("external-call lowering accepted an unclosed operand")
	}
}

func mustExternalCallEquation(t *testing.T, draft equation.Draft) equation.Equation {
	t.Helper()
	compiler, err := equation.Skeleton().With(string(OperatorExternalCall), ExternalCallEquationLowerer())
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{draft}})
	if err != nil {
		t.Fatal(err)
	}
	return artifact.Equations[0]
}

func TestCompileEquationIRWalksExternalCallThroughFactapplyLowering(t *testing.T) {
	program, _, _, _ := formalCallOutcomeFiberFixture(t)
	compiler, err := equation.Skeleton().With(string(OperatorExternalCall), ExternalCallEquationLowerer())
	if err != nil {
		t.Fatal(err)
	}
	// The representative relation has its structural nonreturning terminal;
	// it is bound only so the external-call occurrence can reach its hook.
	compiler, err = compiler.With(string(OperatorNonreturning), equation.BindExistingKernel("transformer/formal-nonreturning/v1"))
	if err != nil {
		t.Fatal(err)
	}
	var externalCalls int
	artifact, err := program.CompileEquationIR(compiler, func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		body := equation.BodyID(occurrence.Body)
		if occurrence.Kind == OperatorExternalCall {
			externalCalls++
			draft, _, bindErr := NewExternalCallEquationDraft(externalCallEquationBinding(body, formal.NewOccurrenceID(occurrence.Body, 1)))
			return draft, bindErr
		}
		contract, bindErr := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, 2))
		if bindErr != nil {
			return equation.Draft{}, bindErr
		}
		terms := make(map[AccessRole]equation.Term, len(contract.Operands))
		for _, role := range contract.Operands {
			terms[role] = equation.ClosedTerm([]byte(role))
		}
		return equation.Draft{Target: equation.Coordinate{Body: body, Name: "nonreturning-output"}, Entry: equation.EntryParameter{Body: body, Name: "entry"}, Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())}, Operands: []equation.Operand{{Role: string(contract.Operands[0]), Term: terms[contract.Operands[0]]}, {Role: string(contract.Operands[1]), Term: terms[contract.Operands[1]]}}}, nil
	})
	if err != nil {
		t.Fatalf("CompileEquationIR: %v", err)
	}
	if externalCalls != 1 || len(artifact.Equations) != 2 {
		t.Fatalf("external calls/equations = %d/%d, want 1/2", externalCalls, len(artifact.Equations))
	}
	for _, lowered := range artifact.Equations {
		if lowered.Occurrence.Kind == string(OperatorExternalCall) && lowered.KernelID != ExternalCallEquationKernelID {
			t.Fatalf("external-call kernel = %q", lowered.KernelID)
		}
	}
}

func TestExternalCallLoweringAuditsCompleteFactapplyAccess(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	owner := base.bodies[0].body
	_, contract, err := NewExternalCallEquationDraft(externalCallEquationBinding(equation.BodyID(owner), formal.NewOccurrenceID(owner, 1)))
	if err != nil {
		t.Fatal(err)
	}
	access := OperatorAccess{
		Kind: contract.Kind, Occurrence: contract.Occurrence,
		Reads: append([]ContractSelector(nil), contract.Reads...), Writes: append([]ContractSelector(nil), contract.Writes...),
		Outcomes: append([]OutcomeKind(nil), contract.Outcomes...), Dependencies: append([]ContractDependency(nil), contract.Dependencies...),
		Diagnostics: []string{contract.DiagnosticOutputs[0].Candidate},
	}
	execution := Execution{Complete: true, Published: true, Access: equation.AccessRecord{Payload: access}}
	if err := VerifyExternalCallLoweredAccess(contract, execution); err != nil {
		t.Fatalf("external-call audit: %v", err)
	}
	access.Reads = append(access.Reads, ContractSelector{Role: AccessBoundary, Name: "undeclared-boundary"})
	execution.Access.Payload = access
	if err := VerifyExternalCallLoweredAccess(contract, execution); err == nil {
		t.Fatal("external-call audit accepted undeclared read")
	}
	execution = Execution{Published: true, Access: equation.AccessRecord{Payload: OperatorAccess{Kind: contract.Kind, Occurrence: contract.Occurrence}}}
	if err := VerifyExternalCallLoweredAccess(contract, execution); err == nil {
		t.Fatal("external-call lowering published an incomplete transaction")
	}
}
