package transformer

import (
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"testing"
)

func registryTestOwner(seed byte) lexicalidentity.StableLexicalBodyID {
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = seed
	return owner
}

func TestFormalCoordinateRegistryLexicalClassesAreImmutable(t *testing.T) {
	owner := registryTestOwner(1)
	input, middle := formal.NewRoot(owner, 1, formal.Input), formal.NewRoot(owner, 1, formal.Middle)
	class := formal.NewLexicalClassID(owner, 1)
	builder := newFormalCoordinateRegistryBuilder(owner)
	if err := builder.addClass(input, class); err != nil {
		t.Fatal(err)
	}
	if err := builder.addClass(middle, class); err != nil {
		t.Fatal(err)
	}
	if err := builder.addClass(input, formal.NewLexicalClassID(owner, 2)); err == nil {
		t.Fatal("accepted mutable lexical class")
	}
	registry, err := builder.freeze()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := registry.class(input)
	if !ok || got != class {
		t.Fatalf("input class = %#v, %v", got, ok)
	}
	members := registry.classMembers(class)
	members[0] = formal.Root{}
	if got := registry.classMembers(class); len(got) != 2 || got[0] == (formal.Root{}) {
		t.Fatal("class members escaped registry")
	}
}

func TestFormalCoordinateRegistryAliasesRemainGuardScoped(t *testing.T) {
	owner := registryTestOwner(2)
	left, right := formal.NewRoot(owner, 1, formal.Input), formal.NewRoot(owner, 2, formal.Input)
	builder := newFormalCoordinateRegistryBuilder(owner)
	for index, root := range []formal.Root{left, right} {
		if err := builder.addClass(root, formal.NewLexicalClassID(owner, uint64(index+1))); err != nil {
			t.Fatal(err)
		}
	}
	guard := formalGuardScope{occurrence: formal.NewOccurrenceID(owner, 7), branch: 1}
	if err := builder.addAlias(left, right, guard); err != nil {
		t.Fatal(err)
	}
	registry, err := builder.freeze()
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.aliasesAt(guard); len(got) != 1 {
		t.Fatalf("guarded aliases = %d, want 1", len(got))
	}
	if got := registry.aliasesAt(formalGuardScope{occurrence: guard.occurrence, branch: 2}); len(got) != 0 {
		t.Fatalf("alias leaked: %#v", got)
	}
}

func TestFormalCoordinateRegistryWriteAlphabetsAreOccurrenceLocal(t *testing.T) {
	owner := registryTestOwner(3)
	first, second := formal.NewRoot(owner, 1, formal.Output), formal.NewRoot(owner, 2, formal.Output)
	builder := newFormalCoordinateRegistryBuilder(owner)
	for index, root := range []formal.Root{first, second} {
		if err := builder.addClass(root, formal.NewLexicalClassID(owner, uint64(index+1))); err != nil {
			t.Fatal(err)
		}
	}
	one, two := formal.NewOccurrenceID(owner, 1), formal.NewOccurrenceID(owner, 2)
	if err := builder.addAlphabet(one, []formal.Root{first}); err != nil {
		t.Fatal(err)
	}
	if err := builder.addAlphabet(two, []formal.Root{second}); err != nil {
		t.Fatal(err)
	}
	registry, err := builder.freeze()
	if err != nil {
		t.Fatal(err)
	}
	alphabet, ok := registry.alphabet(one)
	if !ok || !alphabet.contains(first) || alphabet.contains(second) {
		t.Fatal("first occurrence alphabet leaked authority")
	}
	if _, ok := registry.alphabet(formal.NewOccurrenceID(owner, 3)); ok {
		t.Fatal("unregistered occurrence acquired alphabet")
	}
}
