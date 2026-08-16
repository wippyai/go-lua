package typeprojection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestApplyField(t *testing.T) {
	source := typetable.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()

	got, ok := Apply(source, projection.Projection{
		Steps: []projection.Step{projection.Field("name")},
	})
	if !ok {
		t.Fatal("Apply field failed")
	}
	assertProjectionType(t, got, typ.String)
}

func TestApplyCallableReturn(t *testing.T) {
	source := typ.Func().
		Param("value", typ.String).
		Returns(typ.Number, typ.Boolean).
		Build()

	got, ok := Apply(source, projection.Projection{
		Steps: []projection.Step{projection.CallableReturn()},
	})
	if !ok {
		t.Fatal("Apply callable return failed")
	}
	assertProjectionType(t, got, typ.Number)
}

func TestApplyGenericArg(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)

	got, ok := Apply(typ.NewAlias("StringBox", typ.Instantiate(box, typ.String)), projection.Projection{
		Steps: []projection.Step{projection.GenericArg(0)},
	})
	if !ok {
		t.Fatal("Apply generic arg failed")
	}
	assertProjectionType(t, got, typ.String)
}

func TestApplyGenericArgRejectsMissingArg(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)

	if got, ok := Apply(typ.Instantiate(box, typ.String), projection.Projection{
		Steps: []projection.Step{projection.GenericArg(1)},
	}); ok || got != nil {
		t.Fatal("Apply generic missing arg succeeded")
	}
}

func TestApplyInstantiateGeneric(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{param}, typ.NewInterface("Channel", nil))

	got, ok := Apply(typ.NewMeta(typ.String), projection.Projection{
		Steps: []projection.Step{projection.InstantiateGeneric(channel)},
	})
	if !ok {
		t.Fatal("Apply instantiate generic failed")
	}
	assertProjectionType(t, got, typ.Instantiate(channel, typ.String))
}

func TestApplyInstantiateGenericRejectsConstraintMismatch(t *testing.T) {
	param := typ.NewTypeParam("T", typ.Number)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{param}, typ.NewInterface("Channel", nil))

	if got, ok := Apply(typ.NewMeta(typ.String), projection.Projection{
		Steps: []projection.Step{projection.InstantiateGeneric(channel)},
	}); ok || got != nil {
		t.Fatal("Apply instantiate generic constraint mismatch succeeded")
	}
}

func TestApplyChainFieldCallableReturn(t *testing.T) {
	source := typetable.NewRecord().
		Field("make", typ.Func().Returns(typ.Boolean).Build()).
		Build()

	got, ok := Apply(source, projection.Projection{
		Steps: []projection.Step{
			projection.Field("make"),
			projection.CallableReturn(),
		},
	})
	if !ok {
		t.Fatal("Apply field callable return chain failed")
	}
	assertProjectionType(t, got, typ.Boolean)
}

func TestApplySegmentsMixedTraversal(t *testing.T) {
	source := typetable.NewRecord().
		Field("items", typ.NewArray(
			typetable.NewRecord().
				Field("name", typ.String).
				Build(),
		)).
		Build()

	got, ok := ApplySegments(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "items"},
		{Kind: segment.SegmentIndexInt, Index: 2},
		{Kind: segment.SegmentField, Name: "name"},
	})
	if !ok {
		t.Fatal("ApplySegments mixed traversal failed")
	}
	assertProjectionType(t, got, typeexpr.Optional(typ.String))
}

func TestApplySegmentsOptionalReceiverProjectsNil(t *testing.T) {
	source := typeexpr.Optional(typetable.NewRecord().
		Field("answer", typ.LiteralString("ok")).
		Build())

	got, ok := ApplySegments(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "answer"},
	})
	if !ok {
		t.Fatal("ApplySegments optional receiver failed")
	}
	assertProjectionType(t, got, typeexpr.Optional(typ.LiteralString("ok")))
}

func TestApplySegmentsFieldSyntaxReadsMapValue(t *testing.T) {
	slot := typetable.NewRecord().
		Field("value", typetable.NewRecord().Field("path", typ.String).Build()).
		Build()
	source := typetable.NewMap(typ.String, slot)

	got, ok := ApplySegments(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "active"},
		{Kind: segment.SegmentField, Name: "value"},
		{Kind: segment.SegmentField, Name: "path"},
	})
	if !ok {
		t.Fatal("ApplySegments field syntax over map failed")
	}
	assertProjectionType(t, got, typeexpr.Optional(typ.String))
}

func TestApplySegmentsMissingUnionMemberBecomesOptional(t *testing.T) {
	file := typetable.NewRecord().
		Field("kind", typ.LiteralString("file")).
		Field("path", typ.String).
		Build()
	timer := typetable.NewRecord().
		Field("kind", typ.LiteralString("timer")).
		Field("seconds", typ.Number).
		Build()
	slot := typetable.NewRecord().
		Field("value", typeexpr.Union(file, timer)).
		Build()
	source := typetable.NewMap(typ.String, slot)

	got, ok := ApplySegments(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "active"},
		{Kind: segment.SegmentField, Name: "value"},
		{Kind: segment.SegmentField, Name: "path"},
	})
	if !ok {
		t.Fatal("ApplySegments missing union member failed")
	}
	assertProjectionType(t, got, typeexpr.Optional(typ.String))
}

func TestApplyWriteSegmentsUsesWritableIndexAtLeaf(t *testing.T) {
	source := typetable.NewRecord().
		Field("items", typ.NewArray(
			typetable.NewRecord().
				Field("name", typ.String).
				Build(),
		)).
		Field("optional_items", typ.NewArray(typeexpr.Optional(typ.Number))).
		Build()

	got, ok := ApplyWriteSegments(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "items"},
		{Kind: segment.SegmentIndexInt, Index: 2},
		{Kind: segment.SegmentField, Name: "name"},
	})
	if !ok {
		t.Fatal("ApplyWriteSegments mixed traversal failed")
	}
	assertProjectionType(t, got, typ.String)

	got, ok = ApplyWriteSegments(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "optional_items"},
		{Kind: segment.SegmentIndexInt, Index: 1},
	})
	if !ok {
		t.Fatal("ApplyWriteSegments optional element failed")
	}
	assertProjectionType(t, got, typeexpr.Optional(typ.Number))
}

func TestApplyWriteSegmentsMeetsUnionRecordFields(t *testing.T) {
	source := typ.MaterializeUnion([]typ.Type{
		typetable.NewRecord().Field("value", typ.Number).Build(),
		typetable.NewRecord().Field("value", typ.String).Build(),
	})

	got, ok := ApplyWriteSegments(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "value"},
	})
	if !ok {
		t.Fatal("ApplyWriteSegments union field failed")
	}
	assertProjectionType(t, got, normalize.IntersectionForMeet(typ.Number, typ.String))
	if subtype.IsSubtype(typ.LiteralString("bad"), got) {
		t.Fatalf("literal string was accepted by union field write meet %v", got)
	}

	got, ok = ApplyWriteSegments(source, []segment.Segment{
		{Kind: segment.SegmentIndexString, Name: "value"},
	})
	if !ok {
		t.Fatal("ApplyWriteSegments union bracket field failed")
	}
	assertProjectionType(t, got, normalize.IntersectionForMeet(typ.Number, typ.String))
	if subtype.IsSubtype(typ.LiteralString("bad"), got) {
		t.Fatalf("literal string was accepted by union bracket write meet %v", got)
	}
}

func TestDynamicWriteValueTypeMeetsClosedRecordStringFields(t *testing.T) {
	source := typetable.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Number).
		Build()

	got, ok := DynamicWriteValueType(source, typ.String)
	if !ok {
		t.Fatal("DynamicWriteValueType closed record string key failed")
	}
	assertProjectionType(t, got, normalize.IntersectionForMeet(typ.Number, typ.String))
	if got == nil || got.String() == "" {
		t.Fatalf("DynamicWriteValueType returned invalid meet: %v", got)
	}
	if subtype.IsSubtype(typ.LiteralString("bad"), got) {
		t.Fatalf("literal string was accepted by dynamic-write meet %v", got)
	}
}

func TestDynamicWriteValueTypeIgnoresAnyRecordSlotsInMeet(t *testing.T) {
	source := typetable.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.Any).
		Build()

	got, ok := DynamicWriteValueType(source, typ.String)
	if !ok {
		t.Fatal("DynamicWriteValueType closed record string key failed")
	}
	assertProjectionType(t, got, typ.String)
}

func TestDynamicWriteValueTypeUsesMapContractDirectly(t *testing.T) {
	source := typ.NewMap(typ.String, typ.Number)

	got, ok := DynamicWriteValueType(source, typ.String)
	if !ok {
		t.Fatal("DynamicWriteValueType map write failed")
	}
	assertProjectionType(t, got, typ.Number)
}

func TestApplyConstructorSegmentsTreatsNonLeafPrefixesAsPresent(t *testing.T) {
	source := typetable.NewRecord().
		Field("error", typeexpr.Optional(typetable.NewRecord().
			Field("type", typ.String).
			Field("code", typeexpr.Optional(typ.Any)).
			Build())).
		Build()

	got, ok := ApplyConstructorSegments(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "error"},
		{Kind: segment.SegmentField, Name: "type"},
	})
	if !ok {
		t.Fatal("ApplyConstructorSegments nested field failed")
	}
	assertProjectionType(t, got, typ.String)

	got, ok = ApplyConstructorSegments(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "error"},
		{Kind: segment.SegmentField, Name: "code"},
	})
	if !ok {
		t.Fatal("ApplyConstructorSegments optional leaf failed")
	}
	assertProjectionType(t, got, typeexpr.Optional(typ.Any))
}

func TestExpectedConstructorEntryTypeTreatsArrayLeafAsPresent(t *testing.T) {
	source := typ.NewArray(typ.String)

	got, ok := ApplyConstructorSegments(source, []segment.Segment{
		{Kind: segment.SegmentIndexInt, Index: 1},
	})
	if !ok {
		t.Fatal("ApplyConstructorSegments array entry failed")
	}
	assertProjectionType(t, got, typeexpr.Optional(typ.String))

	got, ok = ExpectedConstructorEntryType(source, []segment.Segment{
		{Kind: segment.SegmentIndexInt, Index: 1},
	})
	if !ok {
		t.Fatal("ExpectedConstructorEntryType array entry failed")
	}
	assertProjectionType(t, got, typ.String)
}

func TestExpectedConstructorEntryTypePreservesDeclaredNilableFieldPayload(t *testing.T) {
	source := typetable.NewRecord().
		Field("nest", typeexpr.Optional(typ.String)).
		OptField("label", typ.String).
		Build()

	got, ok := ExpectedConstructorEntryType(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "nest"},
	})
	if !ok {
		t.Fatal("ExpectedConstructorEntryType nilable field failed")
	}
	assertProjectionType(t, got, typeexpr.Optional(typ.String))

	got, ok = ExpectedConstructorEntryType(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "label"},
	})
	if !ok {
		t.Fatal("ExpectedConstructorEntryType optional field failed")
	}
	assertProjectionType(t, got, typeexpr.Optional(typ.String))
}

func TestExpectedConstructorEntryTypeSkipsUnionMembersWithoutExplicitEntry(t *testing.T) {
	response := typetable.NewRecord().
		Field("status", typ.Integer).
		Field("body", typ.String).
		Field("headers", typetable.NewMap(typ.String, typ.String)).
		Build()
	success := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", response).
		Build()
	failure := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	source := typeexpr.Union(success, failure)

	got, ok := ExpectedConstructorEntryType(source, []segment.Segment{
		{Kind: segment.SegmentField, Name: "value"},
		{Kind: segment.SegmentField, Name: "headers"},
	})
	if !ok {
		t.Fatal("ExpectedConstructorEntryType union member explicit entry failed")
	}
	want := typetable.NewMap(typ.String, typ.String)
	assertProjectionType(t, got, want)
}

func TestPresentConstructorRootUnwrapsOptionalTableContract(t *testing.T) {
	record := typetable.NewRecord().Field("id", typ.String).Build()
	got := PresentConstructorRoot(typeexpr.Optional(record))
	assertProjectionType(t, got, record)
}

func TestApplySegmentsRejectsUnsupportedKind(t *testing.T) {
	if got, ok := ApplySegments(typ.String, []segment.Segment{{Kind: segment.SegmentKind(99)}}); ok || got != nil {
		t.Fatal("ApplySegments unsupported kind succeeded")
	}
}

func TestSegmentKeyType(t *testing.T) {
	tests := []struct {
		name string
		seg  segment.Segment
		want typ.Type
	}{
		{
			name: "field",
			seg:  segment.Segment{Kind: segment.SegmentField, Name: "name"},
			want: typ.LiteralString("name"),
		},
		{
			name: "string index",
			seg:  segment.Segment{Kind: segment.SegmentIndexString, Name: "raw-key"},
			want: typ.LiteralString("raw-key"),
		},
		{
			name: "integer index",
			seg:  segment.Segment{Kind: segment.SegmentIndexInt, Index: 3},
			want: typ.LiteralInt(3),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SegmentKeyType(tt.seg)
			if !ok {
				t.Fatal("SegmentKeyType failed")
			}
			assertProjectionType(t, got, tt.want)
		})
	}

	if got, ok := SegmentKeyType(segment.Segment{Kind: segment.SegmentKind(99)}); ok || got != nil {
		t.Fatal("SegmentKeyType unsupported segment succeeded")
	}
}

func assertProjectionType(t *testing.T, got typ.Type, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}
