package transformer

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func effectEquationBinding(t *testing.T, kind OperatorKind, ordinal uint64) EffectEquationBinding {
	t.Helper()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("effect-equation-binding-body")))
	input := formal.NewRoot(owner, 1, formal.Input)
	output := formal.NewRoot(owner, 2, formal.Output)
	body := equation.BodyID(owner)
	return EffectEquationBinding{
		Kind: kind, Occurrence: formal.NewOccurrenceID(owner, ordinal),
		Target: equation.Coordinate{Body: body, Name: "effect-output"}, Entry: equation.EntryParameter{Body: body, Name: "entry"},
		Flow: equation.ClosedTerm([]byte("sealed-flow")), State: equation.ClosedTerm([]byte("sealed-state")),
		Allocation: equation.ClosedTerm([]byte("sealed-allocation")), Guard: equation.ClosedTerm([]byte("sealed-guard")),
		Reads: []ContractSelector{
			{Role: AccessFlow, Name: "reduction-input", Root: input},
			{Role: AccessGuard, Name: "effect-guard"},
			{Role: AccessState, Name: "state-reduction"},
			{Role: AccessAllocation, Name: "allocation-template"},
			{Role: AccessBoundary, Name: "effect-boundary"},
		},
		Writes:        []ContractSelector{{Role: AccessState, Name: "effect-result", Root: output}},
		GuardAtoms:    []string{"effect-guard"},
		WriteAlphabet: []formal.Root{output},
		Dependencies:  []ContractDependency{{Kind: "effect-catalog", ID: contentID([]byte("effect-catalog/v1"))}},
	}
}

func TestEffectEquationLoweringsCompileEachFamily(t *testing.T) {
	for ordinal, kind := range []OperatorKind{
		OperatorPathReplacement, OperatorPathInvalidation, OperatorIndexMutation,
		OperatorAllocationTemplate, OperatorObjectMaterialization,
	} {
		t.Run(string(kind), func(t *testing.T) {
			binding := effectEquationBinding(t, kind, uint64(ordinal+1))
			draft, contract, err := EffectEquationDraft(binding)
			if err != nil {
				t.Fatal(err)
			}
			compiler, err := InstallEffectEquationLowerings(equation.Skeleton())
			if err != nil {
				t.Fatal(err)
			}
			artifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{draft}})
			if err != nil {
				t.Fatal(err)
			}
			if len(artifact.Equations) != 1 || artifact.Equations[0].Occurrence.ContractID != equation.ContentID(contract.ContentID()) {
				t.Fatalf("lowered effect artifact = %#v", artifact)
			}
		})
	}
}

func TestEffectEquationLoweringFailsClosedWithoutHook(t *testing.T) {
	draft, _, err := EffectEquationDraft(effectEquationBinding(t, OperatorPathReplacement, 1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = equation.Skeleton().Compile(equation.Source{Drafts: []equation.Draft{draft}})
	if !errors.Is(err, equation.ErrUnimplementedLowering) {
		t.Fatalf("error = %v, want unimplemented lowering", err)
	}
}

func TestCompileEquationIRWalksRepresentativeEffectBodies(t *testing.T) {
	reg := standard.Registry()
	for _, test := range []struct {
		kind    OperatorKind
		program *RelationProgram
	}{
		{OperatorPathReplacement, formalPathReplacementTestProgram(t, nil, typevalue.LiteralString(reg, "written"))},
		{OperatorPathInvalidation, formalPathInvalidationTestProgram(t, InvalidationScopeDescendants)},
		{OperatorIndexMutation, formalIndexMutationTestProgram(t, typevalue.LiteralString(reg, "key"), typevalue.LiteralString(reg, "value"))},
	} {
		t.Run(string(test.kind), func(t *testing.T) {
			compiler, err := effectTestCompiler()
			if err != nil {
				t.Fatal(err)
			}
			seen := false
			artifact, err := test.program.CompileEquationIR(compiler, func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
				if isEffectEquationKind(occurrence.Kind) {
					if occurrence.Kind == test.kind {
						seen = true
					}
					binding := effectEquationBinding(t, occurrence.Kind, 1)
					body := equation.BodyID(occurrence.Body)
					binding.Occurrence = formal.NewOccurrenceID(occurrence.Body, 1)
					binding.Target = equation.Coordinate{Body: body, Name: string(occurrence.Kind) + "-output"}
					binding.Entry = equation.EntryParameter{Body: body, Name: "entry"}
					binding.Reads[0].Root = formal.NewRoot(occurrence.Body, 1, formal.Input)
					output := formal.NewRoot(occurrence.Body, 2, formal.Output)
					binding.Writes[0].Root = output
					binding.WriteAlphabet = []formal.Root{output}
					draft, _, draftErr := EffectEquationDraft(binding)
					return draft, draftErr
				}
				return nonEffectEquationDraft(occurrence)
			})
			if err != nil {
				t.Fatal(err)
			}
			if !seen || len(artifact.Equations) == 0 {
				t.Fatalf("effect body did not dispatch %q: %#v", test.kind, artifact)
			}
		})
	}
}

func effectTestCompiler() (*equation.Compiler, error) {
	compiler, err := InstallEffectEquationLowerings(equation.Skeleton())
	if err != nil {
		return nil, err
	}
	for _, kind := range FrozenOperatorKinds() {
		if isEffectEquationKind(kind) {
			continue
		}
		compiler, err = compiler.With(string(kind), equation.BindExistingKernel("transformer/test-"+string(kind)+"/v1"))
		if err != nil {
			return nil, err
		}
	}
	return compiler, nil
}

func nonEffectEquationDraft(occurrence RelationEquationOccurrence) (equation.Draft, error) {
	contract, err := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, 1))
	if err != nil {
		return equation.Draft{}, err
	}
	body := equation.BodyID(occurrence.Body)
	operands := make([]equation.Operand, 0, len(contract.Operands))
	for _, role := range contract.Operands {
		operands = append(operands, equation.Operand{Role: string(role), Term: equation.ClosedTerm([]byte(role))})
	}
	return equation.Draft{
		Target:     equation.Coordinate{Body: body, Name: string(occurrence.Kind) + "-output"},
		Entry:      equation.EntryParameter{Body: body, Name: "entry"},
		Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())},
		Operands:   operands,
	}, nil
}

func TestEffectEquationArtifactIdentityIsSemantic(t *testing.T) {
	first := effectEquationBinding(t, OperatorPathReplacement, 1)
	second := first
	second.Reads = append([]ContractSelector(nil), first.Reads...)
	second.Reads[0], second.Reads[4] = second.Reads[4], second.Reads[0]
	firstDraft, _, err := EffectEquationDraft(first)
	if err != nil {
		t.Fatal(err)
	}
	secondDraft, _, err := EffectEquationDraft(second)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := InstallEffectEquationLowerings(equation.Skeleton())
	if err != nil {
		t.Fatal(err)
	}
	left, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{firstDraft}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{secondDraft}})
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() != right.ContentID() {
		t.Fatal("effect equation identity retained declaration order")
	}
	second.Guard = equation.ClosedTerm([]byte("other-sealed-guard"))
	changedDraft, _, err := EffectEquationDraft(second)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{changedDraft}})
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() == changed.ContentID() {
		t.Fatal("effect equation identity ignored a semantic operand")
	}
}

func TestEffectEquationLoweringAuditsDeclaredAccessAndRejectsHiddenAccess(t *testing.T) {
	for ordinal, kind := range []OperatorKind{
		OperatorPathReplacement, OperatorPathInvalidation, OperatorIndexMutation,
		OperatorAllocationTemplate, OperatorObjectMaterialization,
	} {
		t.Run(string(kind), func(t *testing.T) {
			_, contract, err := EffectEquationDraft(effectEquationBinding(t, kind, uint64(ordinal+1)))
			if err != nil {
				t.Fatal(err)
			}
			access := OperatorAccess{
				Kind: contract.Kind, Occurrence: contract.Occurrence,
				Reads: append([]ContractSelector(nil), contract.Reads...), Writes: append([]ContractSelector(nil), contract.Writes...),
				Advances: append([]formal.LexicalClassID(nil), contract.Advances...), Outcomes: append([]OutcomeKind(nil), contract.Outcomes...),
				Dependencies: append([]ContractDependency(nil), contract.Dependencies...),
			}
			execution := Execution{Complete: true, Published: true, Access: equation.AccessRecord{Payload: access}}
			if err := VerifyLoweredOperatorAccess(contract, execution); err != nil {
				t.Fatalf("declared effect access: %v", err)
			}
			access.Reads = append(access.Reads, ContractSelector{Role: AccessState, Name: "undeclared-state-read"})
			execution.Access.Payload = access
			if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
				t.Fatal("undeclared effect read was accepted")
			}
		})
	}
}

func TestEffectEquationLoweringRejectsPartialPublication(t *testing.T) {
	_, contract, err := EffectEquationDraft(effectEquationBinding(t, OperatorPathReplacement, 1))
	if err != nil {
		t.Fatal(err)
	}
	execution := Execution{Complete: false, Published: true, Access: equation.AccessRecord{Payload: OperatorAccess{Kind: contract.Kind, Occurrence: contract.Occurrence}}}
	if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
		t.Fatal("partial effect transaction was published")
	}
}
