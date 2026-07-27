package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
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

func TestFormalCoordinateRegistryAllowsAnExplicitEmptyRootVocabulary(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	owner := registryTestOwner(4)
	rekey, err := domain.SealCoordinateFormalRootRekey(owner, keyspace.New(), keyspace.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := freezeFormalCoordinateRegistry(domain, rekey)
	if err != nil {
		t.Fatal(err)
	}
	if registry == nil || len(registry.classes) != 0 || len(registry.members) != 0 || len(registry.alphabets) != 0 {
		t.Fatalf("empty root vocabulary registry = %#v", registry)
	}
}

func TestFormalCoordinateRegistryAdvanceInvalidatesOnlyAliasSupport(t *testing.T) {
	owner := registryTestOwner(5)
	left := formal.NewRoot(owner, 1, formal.Input)
	right := formal.NewRoot(owner, 2, formal.Input)
	other := formal.NewRoot(owner, 3, formal.Input)
	leftClass := formal.NewLexicalClassID(owner, 1)
	rightClass := formal.NewLexicalClassID(owner, 2)
	otherClass := formal.NewLexicalClassID(owner, 3)
	builder := newFormalCoordinateRegistryBuilder(owner)
	for _, binding := range []struct {
		root  formal.Root
		class formal.LexicalClassID
	}{{left, leftClass}, {right, rightClass}, {other, otherClass}} {
		if err := builder.addClass(binding.root, binding.class); err != nil {
			t.Fatal(err)
		}
	}
	guard := formalGuardScope{occurrence: formal.NewOccurrenceID(owner, 1), branch: 1}
	if err := builder.addAliasSupported(left, right, guard, []formal.LexicalClassID{leftClass, rightClass}); err != nil {
		t.Fatal(err)
	}
	advanceOccurrence := formal.NewOccurrenceID(owner, 2)
	if err := builder.addAdvance(advanceOccurrence, []formal.LexicalClassID{rightClass}); err != nil {
		t.Fatal(err)
	}
	registry, err := builder.freeze()
	if err != nil {
		t.Fatal(err)
	}
	alias := registry.aliasesAt(guard)[0]
	advance, ok := registry.advance(advanceOccurrence)
	if !ok || !registry.aliasInvalidated(alias, advance) {
		t.Fatal("advance did not invalidate its supported alias")
	}
	if err := builder.addAdvance(formal.NewOccurrenceID(owner, 3), []formal.LexicalClassID{otherClass}); err != nil {
		t.Fatal(err)
	}
	otherRegistry, err := builder.freeze()
	if err != nil {
		t.Fatal(err)
	}
	otherAdvance, _ := otherRegistry.advance(formal.NewOccurrenceID(owner, 3))
	if otherRegistry.aliasInvalidated(otherRegistry.aliasesAt(guard)[0], otherAdvance) {
		t.Fatal("advance globally closed a class-adjacent alias")
	}
}

func TestFormalCoordinateRegistryCanonicalContentIgnoresConstructionOrder(t *testing.T) {
	owner := registryTestOwner(6)
	first := formal.NewRoot(owner, 1, formal.Input)
	second := formal.NewRoot(owner, 2, formal.Output)
	firstClass := formal.NewLexicalClassID(owner, 1)
	secondClass := formal.NewLexicalClassID(owner, 2)
	build := func(reverse bool) *formalCoordinateRegistry {
		builder := newFormalCoordinateRegistryBuilder(owner)
		bindings := []struct {
			root  formal.Root
			class formal.LexicalClassID
		}{{first, firstClass}, {second, secondClass}}
		if reverse {
			bindings[0], bindings[1] = bindings[1], bindings[0]
		}
		for _, binding := range bindings {
			if err := builder.addClass(binding.root, binding.class); err != nil {
				t.Fatal(err)
			}
		}
		if err := builder.addAlias(first, second, formalGuardScope{occurrence: formal.NewOccurrenceID(owner, 1), branch: 1}); err != nil {
			t.Fatal(err)
		}
		if err := builder.addAlphabet(formal.NewOccurrenceID(owner, 2), []formal.Root{second}); err != nil {
			t.Fatal(err)
		}
		if err := builder.addAdvance(formal.NewOccurrenceID(owner, 3), []formal.LexicalClassID{firstClass}); err != nil {
			t.Fatal(err)
		}
		registry, err := builder.freeze()
		if err != nil {
			t.Fatal(err)
		}
		return registry
	}
	left, right := build(false), build(true)
	if string(left.CanonicalBytes()) != string(right.CanonicalBytes()) || left.ContentID() != right.ContentID() {
		t.Fatal("canonical registry content depends on construction order")
	}
}
