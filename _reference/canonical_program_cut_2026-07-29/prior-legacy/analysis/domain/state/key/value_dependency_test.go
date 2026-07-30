package key

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestValueDependencyAlternativesAreClosedAndDisjoint(t *testing.T) {
	var owner lexicalidentity.StableLexicalBodyID
	owner[0], owner[len(owner)-1] = 1, 2
	input := formal.NewRoot(owner, 7, formal.Input)
	output := formal.NewRoot(owner, 7, formal.Output)

	concrete := ConcreteDependency(SymbolValue(3))
	if got, ok := concrete.Concrete(); !ok || got != SymbolValue(3) {
		t.Fatalf("Concrete = %v/%v", got, ok)
	}
	if got, ok := concrete.Formal(); ok || got.Valid() {
		t.Fatalf("concrete Formal = %v/%v", got, ok)
	}
	for _, root := range []formal.Root{input, output} {
		dependency := FormalDependency(root)
		if got, ok := dependency.Formal(); !ok || got != root {
			t.Fatalf("Formal(%v) = %v/%v", root, got, ok)
		}
		if got, ok := dependency.Concrete(); ok || got != 0 {
			t.Fatalf("formal Concrete = %v/%v", got, ok)
		}
	}
	if FormalDependency(input) == FormalDependency(output) {
		t.Fatal("IN and OUT roots collapsed")
	}
	if (ValueDependency{}).Valid() || ConcreteDependency(0).Valid() || FormalDependency(formal.Root{}).Valid() {
		t.Fatal("invalid dependency admitted")
	}
}
