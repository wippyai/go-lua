package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestValueAgainstTypeProvesRecursiveUnionAliasShape(t *testing.T) {
	json := typ.NewRecursive("Json", func(self typ.Type) typ.Type {
		return typeexpr.Union(typ.Number, typ.String, typ.NewMap(typ.String, self))
	})
	nested := mustShapeTable(t, shapefact.Table{Closed: true, Members: []shapefact.Member{{
		Suffix: ".c", Present: true, Value: `scalar/string/"deep"`,
	}}})
	value := mustShapeTable(t, shapefact.Table{Closed: true, Members: []shapefact.Member{
		{Suffix: ".a", Present: true, Value: "scalar/number/1"},
		{Suffix: ".b", Present: true, Value: string(nested)},
	}})

	if got := valueAgainstType(value, json); got != shapeProven {
		t.Fatalf("recursive union shape relation = %v, want proven", got)
	}
}

func TestValueAgainstTypeProvesMutualRecursiveAliasOptionalShape(t *testing.T) {
	tree := typ.NewRecursivePlaceholder("Tree")
	node := typ.NewRecursivePlaceholder("TreeNode")
	tree.SetBody(typetable.NewRecord().Field("root", typeexpr.Optional(node)).Build())
	node.SetBody(typetable.NewRecord().
		Field("label", typ.String).
		Field("owner", tree).
		Field("children", typ.NewArray(node)).
		OptField("parent", node).
		Build())
	value := mustShapeTable(t, shapefact.Table{Closed: true, Members: []shapefact.Member{{
		Suffix: ".root", Present: false,
	}}})

	if got := valueAgainstType(value, tree); got != shapeProven {
		t.Fatalf("mutual recursive optional shape relation = %v, want proven", got)
	}
}

func TestValueAgainstTypeRejectsUnguardedRecursiveAlias(t *testing.T) {
	loop := typ.NewRecursivePlaceholder("Loop")
	loop.SetBody(loop)
	value := mustShapeTable(t, shapefact.Table{Closed: true})

	if got := valueAgainstType(value, loop); got != shapeUnknown {
		t.Fatalf("unguarded recursive shape relation = %v, want unknown", got)
	}
}

func mustShapeTable(t *testing.T, table shapefact.Table) []byte {
	t.Helper()
	encoded, ok := shapefact.EncodeTable(table)
	if !ok {
		t.Fatal("EncodeTable rejected test shape")
	}
	return encoded
}
