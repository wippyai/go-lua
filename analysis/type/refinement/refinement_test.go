package refinement

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestContainsFreeTypeParamTreatsClosedInstantiationAsClosed(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typetable.NewRecord().Field("value", tp).Build())

	if ContainsFreeTypeParam(typ.Instantiate(box, typ.String)) {
		t.Fatal("closed Box<string> should not report a free type parameter")
	}
	if !ContainsFreeTypeParam(typ.Instantiate(box, tp)) {
		t.Fatal("Box<T> with symbolic argument should report a free type parameter")
	}
}

func TestContainsFreeTypeParamKeepsFunctionOwnedParamsScoped(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	fn := typ.Func().TypeParamRef(tp).Returns(tp).Build()
	closedFunctionWithFreeSibling := typetable.NewRecord().
		Field("call", fn).
		Field("value", tp).
		Build()

	if !ContainsFreeTypeParam(closedFunctionWithFreeSibling) {
		t.Fatal("free sibling type parameter was hidden by function-owned parameter scan")
	}
	if ContainsFreeTypeParam(fn) {
		t.Fatal("function-owned type parameter was reported as free")
	}
}

func BenchmarkContainsFreeTypeParamAcyclicRepeatedShape(b *testing.B) {
	tp := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typetable.NewRecord().Field("value", tp).Build())
	closed := typ.Instantiate(box, typ.String)
	root := typ.NewTuple(closed, closed, closed, closed)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ContainsFreeTypeParam(root)
	}
}

func BenchmarkContainsFreeTypeParamRecursiveRepeatedShape(b *testing.B) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("next", typeexpr.Optional(self)).
			Field("value", typ.String).
			Build()
	})
	root := typ.NewTuple(node, node, node, node)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ContainsFreeTypeParam(root)
	}
}
