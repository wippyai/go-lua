package identity

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestRelationalIdentityTermIsTypedAndStructurallyFinite(t *testing.T) {
	body := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-term-schema")), 1)
	in := formal.NewRoot(body, 7, formal.Input)
	out := formal.NewRoot(body, 7, formal.Output)
	if !in.Valid() || !out.Valid() || in == out ||
		in.Owner() != out.Owner() || in.Ordinal() != out.Ordinal() {
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

func TestFormalIdentityTermRetainsNeutralRootExactly(t *testing.T) {
	body := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-term-root")), 3)
	for _, vocabulary := range []formal.Vocabulary{formal.Input, formal.Middle, formal.Output} {
		root := formal.NewRoot(body, 1<<40+29, vocabulary)
		term := FormalTerm(root)
		got, ok := term.Formal()
		if !ok || got != root ||
			got.Owner() != body ||
			got.Ordinal() != root.Ordinal() ||
			got.Vocabulary() != vocabulary {
			t.Fatalf("formal term root = %#v/%t, want exact %#v", got, ok, root)
		}
	}
}

func TestFormalIdentitySubstitutionPreservesBottomSingletonTop(t *testing.T) {
	body := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-term-substitution")), 1)
	vars := []formal.Root{
		formal.NewRoot(body, 1, formal.Input),
		formal.NewRoot(body, 2, formal.Input),
		formal.NewRoot(body, 3, formal.Input),
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

func TestFormalIdentitySubstitutionKeysByCompleteNeutralRoot(t *testing.T) {
	firstBody := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-substitution-owner-a")), 1)
	secondBody := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-substitution-owner-b")), 1)
	roots := []formal.Root{
		formal.NewRoot(firstBody, 9, formal.Input),
		formal.NewRoot(firstBody, 10, formal.Input),
		formal.NewRoot(firstBody, 9, formal.Output),
		formal.NewRoot(secondBody, 9, formal.Input),
	}
	ids := []ID{
		{Kind: "test.object", Site: "owner-a-ordinal-9-input", Index: 1},
		{Kind: "test.object", Site: "owner-a-ordinal-10-input", Index: 2},
		{Kind: "test.object", Site: "owner-a-ordinal-9-output", Index: 3},
		{Kind: "test.object", Site: "owner-b-ordinal-9-input", Index: 4},
	}
	bindings := make([]Binding, len(roots))
	for index := range roots {
		bindings[index] = Binding{Variable: roots[index], Image: Singleton(ids[index])}
	}
	substitution, ok := NewSubstitution(bindings)
	if !ok {
		t.Fatal("complete-root substitution rejected")
	}
	for index, root := range roots {
		got, found := substitution.Substitute(FormalTerm(root))
		if !found || !Equal(got, Singleton(ids[index])) {
			t.Fatalf("root %d image = %v/%t, want %v", index, got, found, ids[index])
		}
	}
}

func TestCanonicalIdentityTermDistinguishesOwnerVocabularyAndAlternative(t *testing.T) {
	firstBody := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-term-owner-a")), 1)
	secondBody := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-term-owner-b")), 1)
	values := []Value{
		SingletonTerm(FormalTerm(formal.NewRoot(firstBody, 1, formal.Input))),
		SingletonTerm(FormalTerm(formal.NewRoot(firstBody, 1, formal.Output))),
		SingletonTerm(FormalTerm(formal.NewRoot(secondBody, 1, formal.Input))),
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
	first := FormalTerm(formal.NewRoot(firstBody, 1, formal.Input))
	second := FormalTerm(formal.NewRoot(secondBody, 1, formal.Input))
	if first == second || Less(first, second) == Less(second, first) {
		t.Fatal("formal identity order omitted distinct lexical owners")
	}
	firstAllocation := AllocationTerm(ManifestAllocationTemplate(firstBody, 1, 1))
	secondAllocation := AllocationTerm(ManifestAllocationTemplate(secondBody, 1, 1))
	if firstAllocation == secondAllocation || Less(firstAllocation, secondAllocation) == Less(secondAllocation, firstAllocation) {
		t.Fatal("allocation identity order omitted distinct lexical owners")
	}
}
