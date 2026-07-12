package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestReturnCorrelationInferenceRequiresSummaryAuthority(t *testing.T) {
	reg := standard.Registry()
	builder, certificate := emptyBuilder(t, reg, Shape{}, nil)
	builder.inferReturnCorrelations = true
	a := builder.Arena()
	first := a.Constant(typevalue.LiteralString(reg, "first"))
	second := a.Constant(typevalue.LiteralString(reg, "second"))
	relation, err := builder.Build(certificate, []Row{{
		Guard: a.True(),
		Ops: []Operation{
			{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: first},
			{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 1, Value: second},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	if got, ok := relation.Specialize(cursor, nil, nil); ok {
		t.Fatalf("uncertified inferred correlations published: %#v", got)
	}
}
