package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestBoundaryFactorSelectionRetainsMixedIdentityTermsExactly(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("boundary-factor-selection-terms"))
	body := lexicalidentity.FunctionBody(namespace, 1)
	concrete := identity.ConcreteTerm(identity.ID{Kind: "test.object", Site: "boundary-selection", Index: 1})
	variable := identity.FormalTerm(identity.NewFormalVar(identity.NewFormalSchemaID(body, 7), formal.Input))
	allocation := identity.AllocationTerm(identity.ManifestAllocationTemplate(body, 2, 3))
	terms := []identity.Term{concrete, variable, allocation}

	selection, err := SealBoundaryFactorSelection(keyspace.New(), nil, terms, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range terms {
		if !selection.closure.ContainsIdentityTerm(term) {
			t.Fatalf("selection omitted identity term %#v", term)
		}
	}
	if selection.closure.allIdentities || len(selection.closure.identities) != len(terms) {
		t.Fatalf("mixed finite selection widened: %#v", selection.closure)
	}

	more := identity.AllocationTerm(identity.ManifestAllocationTemplate(body, 4, 5))
	extended, err := selection.WithIdentities([]identity.Term{more}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !extended.closure.ContainsIdentityTerm(more) || selection.closure.ContainsIdentityTerm(more) {
		t.Fatal("identity-term extension mutated its immutable source or omitted the new term")
	}
}

func TestBoundaryFactorSelectionRejectsInvalidIdentityTermAtomically(t *testing.T) {
	keys := keyspace.New()
	valid := identity.ConcreteTerm(identity.ID{Kind: "test.object", Site: "boundary-selection", Index: 2})
	if selection, err := SealBoundaryFactorSelection(keys, nil, []identity.Term{valid, {}}, false); err == nil || selection.valid() {
		t.Fatalf("invalid identity term was admitted: %#v, %v", selection, err)
	}
	base, err := SealBoundaryFactorSelection(keys, nil, []identity.Term{valid}, false)
	if err != nil {
		t.Fatal(err)
	}
	if extended, err := base.WithIdentities([]identity.Term{{}}, false); err == nil || extended.valid() {
		t.Fatalf("invalid identity extension was admitted: %#v, %v", extended, err)
	}
	if !base.closure.ContainsIdentityTerm(valid) || len(base.closure.identities) != 1 {
		t.Fatal("failed extension mutated the sealed source selection")
	}
}
