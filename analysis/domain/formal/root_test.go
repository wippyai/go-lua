package formal

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestRootRetainsCompleteOwnerOrdinalAndVocabulary(t *testing.T) {
	first := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-root-a")), 1)
	second := first
	second[len(second)-1] ^= 1
	wide := uint64(math.MaxUint32) + 9
	roots := []Root{
		NewRoot(first, wide, Input),
		NewRoot(first, wide, Output),
		NewRoot(first, wide+1, Input),
		NewRoot(second, wide, Input),
	}
	seen := make(map[Root]struct{}, len(roots))
	for _, root := range roots {
		if !root.Valid() || root.Owner() == (lexicalidentity.StableLexicalBodyID{}) || root.Ordinal() < wide {
			t.Fatalf("invalid or truncated root %#v", root)
		}
		seen[root] = struct{}{}
	}
	if len(seen) != len(roots) {
		t.Fatal("distinct complete formal descriptors collided")
	}
	for left := range roots {
		for right := range roots {
			if (Compare(roots[left], roots[right]) == 0) != (roots[left] == roots[right]) {
				t.Fatalf("canonical order equality drift at %d/%d", left, right)
			}
		}
	}
}
