package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
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

func TestValueNodeConcatCompletesDiagnosedCompositeLeavesWithoutNormalResult(t *testing.T) {
	registry := standard.Registry()
	want := product.Bottom(registry)
	for _, invalid := range []product.Value{
		typevalue.Nil(registry),
		typevalue.LiteralBool(registry, false),
		typevalue.LiteralBool(registry, true),
	} {
		arena := NewArena(registry)
		left := arena.Constant(typevalue.LiteralString(registry, "owner:"))
		concat := arena.StringConcatValue(left, arena.Constant(invalid))
		if concat == 0 {
			t.Fatal("concat construction failed")
		}
		value, exact := arena.evalValueCanonicalWithLeaves(concat, valueNodeLeafResolver{
			completeImpossibleConcat: func() (product.Value, bool) { return want, true },
		})
		if !exact {
			t.Fatalf("diagnosed concat %#v did not retain a composable result", invalid)
		}
		if !product.Equal(registry, value, want) {
			t.Fatalf("diagnosed concat %#v = %#v, want non-normal completion %#v", invalid, value, want)
		}
	}
}
