package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestExpectedTypeAtSegmentsPreservesNestedInstantiatedGenericArguments(t *testing.T) {
	input := typ.NewTypeParam("T", nil)
	options := typ.NewGeneric("ListenOptions", []*typ.TypeParam{input},
		typetable.NewRecord().
			Field("channel", typ.Instantiate(ambient.ChannelGeneric(), input)).
			Field("decode", typ.Func().Param("raw", typ.Any).Returns(input).Build()).
			Build())
	payload := typeexpr.Union(
		typetable.NewRecord().Field("id", typ.String).Build(),
		typetable.NewRecord().Field("elapsed", typ.Number).Build(),
	)
	root := typ.Instantiate(options, payload)

	got, ok := expectedTypeAtSegments(root, []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}})
	if !ok {
		t.Fatal("expectedTypeAtSegments did not project ListenOptions<T>.channel")
	}
	want := typ.Instantiate(ambient.ChannelGeneric(), payload)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("projected channel type = %s, want %s", formatType(got), formatType(want))
	}
}
