package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
)

func operatorContractFixture(t *testing.T) OperatorContract {
	t.Helper()
	owner := registryTestOwner(41)
	occurrence := formal.NewOccurrenceID(owner, 1)
	input := formal.NewRoot(owner, 1, formal.Input)
	output := formal.NewRoot(owner, 2, formal.Output)
	class := formal.NewLexicalClassID(owner, 1)
	contract, err := NewOperatorContract(OperatorRootAssignment, occurrence)
	if err != nil {
		t.Fatal(err)
	}
	contract.Reads = []ContractSelector{{Role: AccessGuard, Name: "assignment-guard"}, {Role: AccessFlow, Name: "predecessor", Root: input}}
	contract.Writes = []ContractSelector{{Role: AccessState, Name: "target", Root: output}}
	contract.GuardAtoms = []string{"assignment-guard"}
	contract.Advances = []formal.LexicalClassID{class}
	contract.AliasSupport = []formal.LexicalClassID{class}
	contract.WriteAlphabet = []formal.Root{output}
	contract.Outcomes = []OutcomeKind{OutcomeNormal}
	contract.DiagnosticOutputs = []DiagnosticDescriptor{{
		Candidate:      "assignment-precondition",
		Owner:          DiagnosticOwnerApplication,
		SourceAnchor:   contentID([]byte("assignment-source")),
		GuardAtoms:     []string{"assignment-guard"},
		ReadSet:        []ContractSelector{{Role: AccessGuard, Name: "assignment-guard"}},
		Predicate:      "assignment-is-valid",
		EvidenceRecipe: "assignment-evidence-v1",
		BoundaryLens:   "formal-target-to-caller-target",
	}}
	contract.Dependencies = []ContractDependency{{Kind: "registry", ID: contentID([]byte("registry"))}}
	return contract
}

func TestFrozenOperatorContractCatalogCoversEveryFormalStepCapability(t *testing.T) {
	seen := make(map[OperatorKind]struct{})
	for _, kind := range FrozenOperatorKinds() {
		if _, duplicate := seen[kind]; duplicate {
			t.Fatalf("duplicate frozen operator kind %q", kind)
		}
		seen[kind] = struct{}{}
	}
	for capability := formalRelationStepCapabilityApply; capability <= formalRelationStepCapabilityExternalCall; capability++ {
		kind, ok := operatorKindForStepCapability(capability)
		if !ok {
			t.Fatalf("formal capability %d has no frozen operator contract", capability)
		}
		if _, ok := seen[kind]; !ok {
			t.Fatalf("formal capability %d maps to unregistered operator kind %q", capability, kind)
		}
	}
}

func TestFrozenOperatorContractCatalogHasCanonicalContentIdentity(t *testing.T) {
	first, second := FrozenOperatorContractCatalog(), FrozenOperatorContractCatalog()
	if first.ContentID() != second.ContentID() || string(first.CanonicalBytes()) != string(second.CanonicalBytes()) {
		t.Fatal("frozen operator catalog content is not deterministic")
	}
}

func TestOperatorContractCanonicalContentIgnoresDeclarationOrder(t *testing.T) {
	first := operatorContractFixture(t)
	second := operatorContractFixture(t)
	second.Reads[0], second.Reads[1] = second.Reads[1], second.Reads[0]
	if got, want := string(first.CanonicalBytes()), string(second.CanonicalBytes()); got != want {
		t.Fatal("canonical operator contract bytes depend on declaration order")
	}
	if first.ContentID() != second.ContentID() {
		t.Fatal("canonical operator contract content identity depends on declaration order")
	}
}

func TestOperatorContractVerifierRejectsUndeclaredAccesses(t *testing.T) {
	contract := operatorContractFixture(t)
	access := OperatorAccess{
		Kind:       contract.Kind,
		Occurrence: contract.Occurrence,
		Reads:      append([]ContractSelector(nil), contract.Reads...),
		Writes:     append([]ContractSelector(nil), contract.Writes...),
		Advances:   append([]formal.LexicalClassID(nil), contract.Advances...),
		Outcomes:   append([]OutcomeKind(nil), contract.Outcomes...),
		Diagnostics: []string{
			"assignment-precondition",
		},
		Dependencies: append([]ContractDependency(nil), contract.Dependencies...),
	}
	if err := contract.VerifyAccess(access); err != nil {
		t.Fatalf("declared access rejected: %v", err)
	}
	access.Reads = append(access.Reads, ContractSelector{Role: AccessState, Name: "hidden-state-read"})
	if err := contract.VerifyAccess(access); err == nil {
		t.Fatal("undeclared read was accepted")
	}
	access.Reads = access.Reads[:len(access.Reads)-1]
	access.Writes = append(access.Writes, ContractSelector{Role: AccessState, Name: "class-adjacent-write"})
	if err := contract.VerifyAccess(access); err == nil {
		t.Fatal("undeclared write was accepted")
	}
}

func TestOperatorContractWriteAlphabetDoesNotCloseOverLexicalClass(t *testing.T) {
	contract := operatorContractFixture(t)
	classAdjacent := formal.NewRoot(contract.Occurrence.Owner(), 3, formal.Output)
	contract.Writes = append(contract.Writes, ContractSelector{Role: AccessState, Name: "class-adjacent", Root: classAdjacent})
	if _, err := canonicalOperatorContract(contract); err == nil {
		t.Fatal("write alphabet was closed over an unlisted class-adjacent root")
	}
}

func TestOperatorContractRequiresApplicationBoundaryLens(t *testing.T) {
	contract := operatorContractFixture(t)
	contract.DiagnosticOutputs[0].BoundaryLens = ""
	if _, err := canonicalOperatorContract(contract); err == nil {
		t.Fatal("application diagnostic without a boundary lens was accepted")
	}
}
