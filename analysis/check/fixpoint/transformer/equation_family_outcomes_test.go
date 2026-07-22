package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func outcomeFamilyTestBody() lexicalidentity.StableLexicalBodyID {
	return lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("outcome-family-equation")))
}

func outcomeFamilyTerms(kind OperatorKind, entry equation.EntryParameter, mutation string) map[AccessRole]equation.Term {
	contract, err := NewOperatorContract(kind, formal.NewOccurrenceID(outcomeFamilyTestBody(), 1))
	if err != nil {
		panic(err)
	}
	terms := make(map[AccessRole]equation.Term, len(contract.Operands))
	for _, role := range contract.Operands {
		term := equation.ClosedTerm([]byte(string(role) + ":" + mutation))
		if role == AccessEntry {
			term = equation.EntryTerm(entry)
		}
		terms[role] = term
	}
	return terms
}

func TestOutcomeFamilyLoweringCompilesRepresentativeContracts(t *testing.T) {
	compiler, err := OutcomeFamilyCompiler()
	if err != nil {
		t.Fatal(err)
	}
	body := outcomeFamilyTestBody()
	entry := equation.EntryParameter{Body: equation.BodyID(body), Name: "entry"}
	kinds := OutcomeFamilyKinds()
	drafts := make([]equation.Draft, 0, len(kinds))
	for index, kind := range kinds {
		occurrence := OutcomeFamilyOccurrence{
			Kind:       kind,
			Occurrence: formal.NewOccurrenceID(body, uint64(index+1)),
			Target:     equation.Coordinate{Body: equation.BodyID(body), Name: "family-" + string(kind)},
			Entry:      entry,
			Terms:      outcomeFamilyTerms(kind, entry, "representative"),
			Reads:      []ContractSelector{{Role: AccessFlow, Name: "input"}},
			Writes:     []ContractSelector{{Role: AccessState, Name: "output"}},
		}
		draft, _, draftErr := occurrence.Draft()
		if draftErr != nil {
			t.Fatalf("%s Draft: %v", kind, draftErr)
		}
		drafts = append(drafts, draft)
	}
	artifact, err := compiler.Compile(equation.Source{Drafts: drafts})
	if err != nil {
		t.Fatalf("outcome family compile: %v", err)
	}
	if len(artifact.Equations) != len(kinds) {
		t.Fatalf("equations = %d, want %d", len(artifact.Equations), len(kinds))
	}
}

func TestOutcomeFamilyLoweringWalksRepresentativeContributionBody(t *testing.T) {
	program := formalContributionTestProgram(t, semanticContribution{}, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepContribution, contribution: 1}}, next: 2},
		{kind: relationNodeBottom},
	})
	compiler, err := OutcomeFamilyCompiler()
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	artifact, err := program.CompileEquationIR(compiler, func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		calls++
		body := equation.BodyID(occurrence.Body)
		entry := equation.EntryParameter{Body: body, Name: "entry"}
		ordinal := uint64(1)
		if occurrence.Kind == OperatorNonreturning {
			ordinal = 2
		}
		returnDraft, _, bindErr := (OutcomeFamilyOccurrence{
			Kind: occurrence.Kind, Occurrence: formal.NewOccurrenceID(occurrence.Body, ordinal),
			Target: equation.Coordinate{Body: body, Name: "representative-" + string(occurrence.Kind)}, Entry: entry,
			Terms: outcomeFamilyTerms(occurrence.Kind, entry, "body"),
			Reads: []ContractSelector{{Role: AccessFlow, Name: "predecessor"}},
		}).Draft()
		return returnDraft, bindErr
	})
	if err != nil {
		t.Fatalf("CompileEquationIR: %v", err)
	}
	if calls != 2 || len(artifact.Equations) != 2 {
		t.Fatalf("walker calls/artifact = %d/%d, want 2/2", calls, len(artifact.Equations))
	}
}

func TestOutcomeFamilyLeavesForeignKindsFailClosed(t *testing.T) {
	compiler, err := OutcomeFamilyCompiler()
	if err != nil {
		t.Fatal(err)
	}
	body := outcomeFamilyTestBody()
	entry := equation.EntryParameter{Body: equation.BodyID(body), Name: "entry"}
	_, err = compiler.Compile(equation.Source{Drafts: []equation.Draft{{
		Target: equation.Coordinate{Body: equation.BodyID(body), Name: "foreign"}, Entry: entry,
		Occurrence: equation.Occurrence{Kind: string(OperatorEnvironmentWrite), ContractID: equation.ContentID(contentID([]byte("foreign-contract")))},
		Operands: []equation.Operand{
			{Role: string(AccessFlow), Term: equation.ClosedTerm([]byte("flow"))},
			{Role: string(AccessState), Term: equation.ClosedTerm([]byte("state"))},
			{Role: string(AccessGuard), Term: equation.ClosedTerm([]byte("guard"))},
		},
	}}})
	if err == nil {
		t.Fatal("outcome family compiler lowered a foreign kind")
	}
}

func TestOutcomeFamilyArtifactIdentityIsCanonicalAndSemantic(t *testing.T) {
	compiler, err := OutcomeFamilyCompiler()
	if err != nil {
		t.Fatal(err)
	}
	body := outcomeFamilyTestBody()
	entry := equation.EntryParameter{Body: equation.BodyID(body), Name: "entry"}
	makeDraft := func(kind OperatorKind, ordinal uint64, mutation string) equation.Draft {
		draft, _, draftErr := (OutcomeFamilyOccurrence{
			Kind: kind, Occurrence: formal.NewOccurrenceID(body, ordinal),
			Target: equation.Coordinate{Body: equation.BodyID(body), Name: string(kind)}, Entry: entry,
			Terms: outcomeFamilyTerms(kind, entry, mutation),
		}).Draft()
		if draftErr != nil {
			t.Fatal(draftErr)
		}
		return draft
	}
	outcome, contribution := makeDraft(OperatorOutcome, 1, "one"), makeDraft(OperatorContribution, 2, "one")
	left, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{outcome, contribution}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{contribution, outcome}})
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() != right.ContentID() {
		t.Fatal("outcome family artifact retained declaration order")
	}
	changed, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{makeDraft(OperatorOutcome, 1, "changed"), contribution}})
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() == changed.ContentID() {
		t.Fatal("outcome family artifact lost semantic operand identity")
	}
}

func TestOutcomeFamilyAuditIsFailClosed(t *testing.T) {
	body := outcomeFamilyTestBody()
	class := formal.NewLexicalClassID(body, 1)
	dependency := ContractDependency{Kind: "frozen-kernel", ID: contentID([]byte("outcome-family-kernel"))}
	diagnostic := DiagnosticDescriptor{Candidate: "candidate", Owner: DiagnosticOwnerCalleeCheck,
		SourceAnchor: contentID([]byte("anchor")), Predicate: "reportable", EvidenceRecipe: "evidence"}
	for index, kind := range OutcomeFamilyKinds() {
		draft, contract, err := (OutcomeFamilyOccurrence{
			Kind: kind, Occurrence: formal.NewOccurrenceID(body, uint64(index+1)),
			Target: equation.Coordinate{Body: equation.BodyID(body), Name: "audit-" + string(kind)},
			Entry:  equation.EntryParameter{Body: equation.BodyID(body), Name: "entry"},
			Terms:  outcomeFamilyTerms(kind, equation.EntryParameter{Body: equation.BodyID(body), Name: "entry"}, "audit"),
			Reads:  []ContractSelector{{Role: AccessFlow, Name: "read"}}, Writes: []ContractSelector{{Role: AccessState, Name: "write"}},
			Advances: []formal.LexicalClassID{class}, Outcomes: []OutcomeKind{OutcomeNormal},
			DiagnosticOutputs: []DiagnosticDescriptor{diagnostic}, Dependencies: []ContractDependency{dependency},
		}).Draft()
		if err != nil || draft.Occurrence.ContractID == (equation.ContentID{}) {
			t.Fatalf("%s contract: %v", kind, err)
		}
		access := OperatorAccess{Kind: kind, Occurrence: contract.Occurrence, Reads: contract.Reads, Writes: contract.Writes,
			Advances: contract.Advances, Outcomes: contract.Outcomes, Diagnostics: []string{diagnostic.Candidate}, Dependencies: contract.Dependencies}
		execution := equation.Execution{Complete: true, Published: true, Access: equation.AccessRecord{Payload: access}}
		if err := AuditOutcomeFamilyExecution(contract, execution); err != nil {
			t.Fatalf("%s declared audit: %v", kind, err)
		}
		access.Reads = append(access.Reads, ContractSelector{Role: AccessFlow, Name: "undeclared"})
		execution.Access.Payload = access
		if err := AuditOutcomeFamilyExecution(contract, execution); err == nil {
			t.Fatalf("%s audit accepted undeclared read", kind)
		}
	}
	partial := equation.Execution{Complete: false, Published: true}
	if err := equation.RunAndVerify(func() (equation.Execution, error) { return partial, nil }, func(equation.AccessRecord) error { return nil }); err == nil {
		t.Fatal("outcome family audit accepted partial publication")
	}
}
