package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestValueNodeScopeCarriesSelectedResolverToDescendants(t *testing.T) {
	registry := standard.Registry()
	arena := NewArena(registry)
	root := arena.Root(Root{Kind: RootParam})
	term := arena.refineConstraintValue(root, product.Bottom(registry))
	if term == 0 {
		t.Fatal("derived term construction failed")
	}
	selected := valueNodeLeafResolver{
		root: func(Root) (product.Value, bool) { return product.Bottom(registry), true },
	}
	resolver := valueNodeLeafResolver{
		scope: func(current ValueTerm, inherited valueNodeLeafResolver) valueNodeLeafResolver {
			if current == term {
				return selected
			}
			return inherited
		},
		root: func(Root) (product.Value, bool) { return product.Value{}, false },
	}
	if _, exact := arena.evalValueCanonicalWithLeaves(term, resolver); !exact {
		t.Fatal("selected resolver did not carry to the operand descendant")
	}
}
