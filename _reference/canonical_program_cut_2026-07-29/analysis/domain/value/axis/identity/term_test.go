package identity

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestRelationalIdentityTermIsTypedAndStructurallyFinite(t *testing.T) {
	body := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-term-schema")), 1)
	schema := NewFormalSchemaID(body, 7)
	in := NewFormalVar(schema, formal.Input)
	out := in.In(formal.Output)
	if !schema.Valid() || !in.Valid() || !out.Valid() || in == out || in.Schema() != out.Schema() {
		t.Fatal("vocabulary rename did not preserve one finite structural schema coordinate")
	}
	concreteID := ID{Kind: "test.object", Site: "identity-term", Index: 1}
	template := ManifestAllocationTemplate(body, 2, 3)
	terms := []Term{ConcreteTerm(concreteID), FormalTerm(in), AllocationTerm(template)}
	for index, term := range terms {
		if !term.Valid() || term.Kind() != TermKind(index+1) {
			t.Fatalf("term %d = %#v", index, term)
		}
	}
	if _, concrete := terms[2].Concrete(); concrete {
		t.Fatal("allocation template was admitted through the concrete alternative")
	}
}

func TestFormalIdentitySubstitutionPreservesBottomSingletonTop(t *testing.T) {
	body := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-term-substitution")), 1)
	vars := []FormalVar{
		NewFormalVar(NewFormalSchemaID(body, 1), formal.Input),
		NewFormalVar(NewFormalSchemaID(body, 2), formal.Input),
		NewFormalVar(NewFormalSchemaID(body, 3), formal.Input),
	}
	exact := ID{Kind: "test.object", Site: "identity-substitution", Index: 1}
	substitution, ok := NewSubstitution([]Binding{
		{Variable: vars[0], Image: Bottom()},
		{Variable: vars[1], Image: Singleton(exact)},
		{Variable: vars[2], Image: Top()},
	})
	if !ok {
		t.Fatal("valid substitution rejected")
	}
	images := []Value{Bottom(), Singleton(exact), Top()}
	for index, variable := range vars {
		got, found := substitution.Substitute(FormalTerm(variable))
		if !found || !Equal(got, images[index]) {
			t.Fatalf("image %d = %v/%v, want %v", index, got, found, images[index])
		}
	}
	if got, found := substitution.Substitute(ConcreteTerm(exact)); !found || !Equal(got, Singleton(exact)) {
		t.Fatalf("concrete identity = %v/%v", got, found)
	}
	template := ManifestAllocationTemplate(body, 1, 1)
	if _, found := substitution.Substitute(AllocationTerm(template)); found {
		t.Fatal("formal substitution instantiated an allocation template")
	}
	if _, ok := NewSubstitution([]Binding{{Variable: vars[0], Image: Bottom()}, {Variable: vars[0], Image: Top()}}); ok {
		t.Fatal("duplicate formal binding admitted")
	}
}

func TestCanonicalIdentityTermDistinguishesOwnerVocabularyAndAlternative(t *testing.T) {
	firstBody := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-term-owner-a")), 1)
	secondBody := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-term-owner-b")), 1)
	firstSchema := NewFormalSchemaID(firstBody, 1)
	values := []Value{
		SingletonTerm(FormalTerm(NewFormalVar(firstSchema, formal.Input))),
		SingletonTerm(FormalTerm(NewFormalVar(firstSchema, formal.Output))),
		SingletonTerm(FormalTerm(NewFormalVar(NewFormalSchemaID(secondBody, 1), formal.Input))),
		SingletonTerm(AllocationTerm(ManifestAllocationTemplate(firstBody, 1, 1))),
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		encoded := string(canonicalBytes(t, value))
		if _, duplicate := seen[encoded]; duplicate {
			t.Fatal("distinct typed identity terms shared canonical bytes")
		}
		seen[encoded] = struct{}{}
	}
}

func TestIdentityTermStructuralOrderIncludesLexicalOwner(t *testing.T) {
	firstBody := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-term-order-a")), 1)
	secondBody := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-term-order-b")), 1)
	first := FormalTerm(NewFormalVar(NewFormalSchemaID(firstBody, 1), formal.Input))
	second := FormalTerm(NewFormalVar(NewFormalSchemaID(secondBody, 1), formal.Input))
	if first == second || Less(first, second) == Less(second, first) {
		t.Fatal("formal identity order omitted distinct lexical owners")
	}
	firstAllocation := AllocationTerm(ManifestAllocationTemplate(firstBody, 1, 1))
	secondAllocation := AllocationTerm(ManifestAllocationTemplate(secondBody, 1, 1))
	if firstAllocation == secondAllocation || Less(firstAllocation, secondAllocation) == Less(secondAllocation, firstAllocation) {
		t.Fatal("allocation identity order omitted distinct lexical owners")
	}
}
