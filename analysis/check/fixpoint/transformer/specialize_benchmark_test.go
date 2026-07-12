package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

// BenchmarkSpecializeMultiRowParameterized exercises the join-heavy caller
// path: both guarded alternatives are feasible and their parameter-derived
// return slots must remain correlated until the final Summary join.
func BenchmarkSpecializeMultiRowParameterized(b *testing.B) {
	reg := standard.Registry()
	builder, certificate := emptyBuilder(b, reg, Shape{Params: 3}, nil)
	arena := builder.Arena()
	condition := arena.Root(Root{Kind: RootParam, Index: 0})
	left := arena.Root(Root{Kind: RootParam, Index: 1})
	right := arena.Root(Root{Kind: RootParam, Index: 2})
	relation, err := builder.Build(certificate, []Row{
		{Guard: arena.Truthy(condition), Ops: []Operation{
			{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: left},
			{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 1, Value: right},
		}},
		{Guard: arena.Falsy(condition), Ops: []Operation{
			{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: right},
			{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 1, Value: left},
		}},
	})
	if err != nil {
		b.Fatal(err)
	}
	cursor, err := NewBindingCursor(relation.Shape(), []product.Value{
		product.Top(),
		typevalue.LiteralString(reg, "left"),
		typevalue.LiteralString(reg, "right"),
	}, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, ok := relation.Specialize(cursor, nil, nil); !ok {
			b.Fatal("fallback")
		}
	}
}
