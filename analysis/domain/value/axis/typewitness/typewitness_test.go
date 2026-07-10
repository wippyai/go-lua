package typewitness

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/ambient"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestOfPreservesOpenTypeParameter(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	got := Of(param)
	if got.IsTop() || got.IsBottom() {
		t.Fatalf("type parameter witness = %v, want concrete placeholder", got)
	}
	if gotType, ok := got.Type(); !ok || gotType != param {
		t.Fatalf("witness type = %v/%v, want original type parameter", gotType, ok)
	}
}

func TestOfAcceptsClosedOptionalRecord(t *testing.T) {
	record := typetable.NewRecord().Field("value", typ.String).Build()
	optionalRecord := typeexpr.Optional(record)
	got := Of(optionalRecord)
	if got.IsTop() || got.IsBottom() {
		t.Fatalf("closed optional record witness = %v, want concrete", got)
	}
	if gotType, ok := got.Type(); !ok || !typ.TypeEquals(gotType, optionalRecord) {
		t.Fatalf("witness type = %v/%v, want optional record", gotType, ok)
	}
}

func TestOfAcceptsClosedGenericInstantiation(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param},
		typetable.NewRecord().Field("value", param).Build())

	got := Of(typ.Instantiate(box, typ.String))
	if got.IsTop() || got.IsBottom() {
		t.Fatalf("closed generic witness = %v, want concrete", got)
	}
	if gotType, ok := got.Type(); !ok || !typ.TypeEquals(gotType, typ.Instantiate(box, typ.String)) {
		t.Fatalf("witness type = %v/%v, want Box<string>", gotType, ok)
	}
	if got := Of(typ.Instantiate(box, param)); !got.IsTop() {
		t.Fatalf("open structural generic witness = %v, want top", got)
	}
}

func TestOfPreservesOpenOpaqueInstantiation(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	open := typ.Instantiate(ambient.ChannelGeneric(), param)

	got := Of(open)
	if got.IsTop() || got.IsBottom() {
		t.Fatalf("open opaque witness = %v, want symbolic instantiated witness", got)
	}
	if gotType, ok := got.Type(); !ok || !typ.TypeEquals(gotType, open) {
		t.Fatalf("open opaque witness type = %v/%v, want %v", gotType, ok, open)
	}
}

func TestOfPreservesRecordShapeWithNestedUncertainty(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	record := typetable.NewRecord().
		Field("id", typ.String).
		Field("payload", typ.Any).
		Field("value", param).
		Build()

	got := Of(record)
	if got.IsTop() || got.IsBottom() {
		t.Fatalf("record witness = %v, want concrete", got)
	}
	gotType, ok := got.Type()
	if !ok || !typ.TypeEquals(gotType, record) {
		t.Fatalf("witness type = %v/%v, want record shape", gotType, ok)
	}

	gotRecord, ok := gotType.(*typ.Record)
	if !ok {
		t.Fatalf("witness type kind = %T, want *typ.Record", gotType)
	}
	if field := gotRecord.GetField("payload"); field == nil || !typ.TypeEquals(field.Type, typ.Any) {
		t.Fatalf("payload field = %#v, want any", field)
	}
	if field := gotRecord.GetField("value"); field == nil || field.Type != param {
		t.Fatalf("value field = %#v, want original type param", field)
	}
}

func TestOfPreservesFunctionShapeWithNestedUncertainty(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(param).
		Param("value", typ.Unknown).
		Returns(param, typ.Any).
		Build()

	got := Of(fn)
	if got.IsTop() || got.IsBottom() {
		t.Fatalf("function witness = %v, want concrete", got)
	}
	gotType, ok := got.Type()
	if !ok || !typ.TypeEquals(gotType, fn) {
		t.Fatalf("witness type = %v/%v, want function shape", gotType, ok)
	}

	gotFn, ok := gotType.(*typ.Function)
	if !ok {
		t.Fatalf("witness type kind = %T, want *typ.Function", gotType)
	}
	if len(gotFn.Params) != 1 || !typ.TypeEquals(gotFn.Params[0].Type, typ.Unknown) {
		t.Fatalf("function params = %#v, want unknown parameter", gotFn.Params)
	}
	if len(gotFn.Returns) != 2 || gotFn.Returns[0] != param || !typ.TypeEquals(gotFn.Returns[1], typ.Any) {
		t.Fatalf("function returns = %#v, want unresolved param and any", gotFn.Returns)
	}
}

func TestJoinPreservesDistinctLiteralAlternatives(t *testing.T) {
	tests := []struct {
		name  string
		left  typ.Type
		right typ.Type
		want  typ.Type
	}{
		// Distinct literals are preserved as a canonical union; collapsing them to
		// the family base would make Join non-associative once a literal union is
		// itself a reachable witness (e.g. a discriminant tag such as "a" | "b").
		{name: "integer literals", left: typ.LiteralInt(0), right: typ.LiteralInt(1), want: typ.MaterializeUnion([]typ.Type{typ.LiteralInt(0), typ.LiteralInt(1)})},
		{name: "string literals", left: typ.LiteralString("ack"), right: typ.LiteralString("nak"), want: typ.MaterializeUnion([]typ.Type{typ.LiteralString("ack"), typ.LiteralString("nak")})},
		{name: "integer and number literal", left: typ.LiteralInt(1), right: typ.LiteralNumber(1.5), want: typ.MaterializeUnion([]typ.Type{typ.LiteralInt(1), typ.LiteralNumber(1.5)})},
		// A literal is absorbed by its own family base.
		{name: "integer literal absorbed by integer", left: typ.LiteralInt(7), right: typ.Integer, want: typ.Integer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Join(Of(tt.left), Of(tt.right))
			gotType, ok := got.Type()
			if !ok || !typ.TypeEquals(gotType, tt.want) {
				t.Fatalf("Join(%v,%v) = %v/%v, want %v", tt.left, tt.right, gotType, ok, tt.want)
			}
		})
	}
}

func TestMeetRejectsDistinctStringLiteralWitnesses(t *testing.T) {
	got := Meet(Of(typ.LiteralString("string")), Of(typ.LiteralString("boolean")))
	if !got.IsBottom() {
		gotType, gotOK := got.Type()
		t.Fatalf("Meet(distinct string literals) = %v/%v, want bottom", gotType, gotOK)
	}
}

func TestMeetPreservesStructurallyEqualOptionalRecords(t *testing.T) {
	leftRecord := typetable.NewRecord().
		Field("code", typ.String).
		Field("message", typ.String).
		Build()
	rightRecord := typetable.NewRecord().
		Field("code", typ.String).
		Field("message", typ.String).
		Build()
	left := typeexpr.Optional(leftRecord)
	right := typeexpr.Optional(rightRecord)
	if !typ.TypeEquals(left, right) {
		t.Fatalf("test setup produced unequal records: %v vs %v", left, right)
	}

	got := Meet(Of(left), Of(right))
	gotType, ok := got.Type()
	if !ok || !typ.TypeEquals(gotType, left) {
		t.Fatalf("Meet(structurally equal optional records) = %v/%v, want %v", gotType, ok, left)
	}
}

func TestMeetPreservesRecordInsideOptionalRecord(t *testing.T) {
	record := typetable.NewRecord().
		Field("code", typ.String).
		Field("message", typ.String).
		Build()
	optional := typeexpr.Optional(record)

	got := Meet(Of(optional), Of(record))
	gotType, ok := got.Type()
	if !ok || !typ.TypeEquals(gotType, record) {
		t.Fatalf("Meet(optional record, present record) = %v/%v, want %v", gotType, ok, record)
	}
}

func TestWidenCollapsesScalarLiteralGrowthToPrimitiveFamily(t *testing.T) {
	tests := []struct {
		name string
		prev typ.Type
		next typ.Type
		want typ.Type
	}{
		{name: "integer literal to integer", prev: typ.LiteralInt(0), next: typ.Integer, want: typ.Integer},
		{name: "integer literal union to integer", prev: typ.MaterializeUnion([]typ.Type{typ.LiteralInt(0), typ.LiteralInt(1)}), next: typ.Integer, want: typ.Integer},
		{name: "integer to number", prev: typ.Integer, next: typ.Number, want: typ.Number},
		{name: "string literals to string", prev: typ.LiteralString("a"), next: typ.MaterializeUnion([]typ.Type{typ.LiteralString("a"), typ.LiteralString("b")}), want: typ.String},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Widen(Of(tt.prev), Of(tt.next))
			gotType, ok := got.Type()
			if !ok || !typ.TypeEquals(gotType, tt.want) {
				t.Fatalf("Widen(%v,%v) = %v/%v, want %v", tt.prev, tt.next, gotType, ok, tt.want)
			}
		})
	}
}

func TestWidenPreservesStableRecordShape(t *testing.T) {
	prev := typetable.NewRecord().Field("id", typ.LiteralString("a")).Build()
	next := typetable.NewRecord().Field("id", typ.LiteralString("b")).Build()

	got := Widen(Of(prev), Of(next))
	gotType, ok := got.Type()
	want := typetable.NewRecord().Field("id", typ.String).Build()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("Widen(record literals) = %v/%v, want %v", gotType, ok, want)
	}
}

func TestWidenPreservesArrayAccumulatorGrowth(t *testing.T) {
	empty := typetable.NewRecord().Build()
	next := typ.NewArray(typ.LiteralString("id"))

	got := Widen(Of(empty), Of(next))
	gotType, ok := got.Type()
	want := typ.NewArray(typ.String)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("Widen(empty table,array literal) = %v/%v, want %v", gotType, ok, want)
	}
}

func TestJoinTreatsEmptyRecordAsArrayMember(t *testing.T) {
	empty := typetable.NewRecord().Build()
	array := typ.NewArray(typ.String)

	got := Join(Of(empty), Of(array))
	gotType, ok := got.Type()
	if !ok || !typ.TypeEquals(gotType, array) {
		t.Fatalf("Join(empty table,array) = %v/%v, want %v", gotType, ok, array)
	}
}

func TestWidenPreservesArrayElementGrowth(t *testing.T) {
	prev := typ.NewArray(typ.LiteralString("a"))
	next := typ.NewArray(typ.LiteralString("b"))

	got := Widen(Of(prev), Of(next))
	gotType, ok := got.Type()
	want := typ.NewArray(typ.String)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("Widen(array literals) = %v/%v, want %v", gotType, ok, want)
	}
}

func TestWidenStableRecordShapeKeepsUnchangedFields(t *testing.T) {
	prev := typetable.NewRecord().
		Field("node_order", typ.NewArray(typ.String)).
		Field("last_node_id", typ.Nil).
		Build()
	next := typetable.NewRecord().
		Field("node_order", typ.NewArray(typ.String)).
		Field("last_node_id", typ.String).
		Build()

	got := Widen(Of(prev), Of(next))
	gotType, ok := got.Type()
	want := typetable.NewRecord().
		Field("node_order", typ.NewArray(typ.String)).
		Field("last_node_id", typeexpr.Optional(typ.String)).
		Build()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("Widen(record field update) = %v/%v, want %v", gotType, ok, want)
	}
}

func TestWidenStableRecordShapeKeepsBranchOnlyFieldsOptional(t *testing.T) {
	prev := typetable.NewRecord().
		Field("status", typ.LiteralString("running")).
		Field("migrations_failed", typ.LiteralInt(0)).
		Build()
	next := typetable.NewRecord().
		Field("status", typ.LiteralString("error")).
		Field("migrations_failed", typ.LiteralInt(1)).
		Field("error", typ.String).
		Build()

	got := Widen(Of(prev), Of(next))
	gotType, ok := got.Type()
	want := typetable.NewRecord().
		OptField("error", typ.String).
		Field("migrations_failed", typ.Integer).
		Field("status", typ.String).
		Build()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("Widen(record branch-only field) = %v/%v, want %v", gotType, ok, want)
	}
}

func TestWidenStableRecordShapeKeepsBranchOnlyStaticMembersOptional(t *testing.T) {
	prev := typetable.NewRecord().
		Field("status", typ.LiteralString("running")).
		StaticStringIndex("run", typ.Func().Returns(typ.Nil).Build()).
		Build()
	next := typetable.NewRecord().
		Field("status", typ.LiteralString("ready")).
		StaticStringIndex("run", typ.Func().Returns(typ.Nil).Build()).
		StaticStringIndex("stop", typ.Func().Returns(typ.Nil).Build()).
		Build()

	got := Widen(Of(prev), Of(next))
	gotType, ok := got.Type()
	rec, recOK := gotType.(*typ.Record)
	if !ok || !recOK {
		t.Fatalf("Widen(record branch-only static member) = %v/%v, want record", gotType, ok)
	}
	stop := rec.GetStaticStringIndex("stop")
	if stop == nil || !stop.Optional {
		t.Fatalf("stop member = %#v, want optional branch-only static member", stop)
	}
}

func TestWidenStableRecordShapeWithMethodSurface(t *testing.T) {
	prev := typetable.NewRecord().
		Field("node_order", typ.NewArray(typ.String)).
		Field("last_node_id", typ.Nil).
		Build()
	next := typetable.NewRecord().
		Field("node_order", typ.NewArray(typ.String)).
		Field("last_node_id", typ.String).
		Build()
	prev = typetable.NewRecord().
		Field("node_order", typ.NewArray(typ.String)).
		Field("last_node_id", typ.Nil).
		StaticStringIndex("create_node", typ.Func().Param("self", prev).Returns(typ.String, typ.Nil).Build()).
		Build()
	next = typetable.NewRecord().
		Field("node_order", typ.NewArray(typ.String)).
		Field("last_node_id", typ.String).
		StaticStringIndex("create_node", typ.Func().Param("self", next).Returns(typ.String, typ.Nil).Build()).
		Build()

	got := Widen(Of(prev), Of(next))
	gotType, ok := got.Type()
	if !ok {
		t.Fatalf("Widen(record method surface) = top, want preserved record")
	}
	rec, ok := gotType.(*typ.Record)
	if !ok || rec.GetField("node_order") == nil || rec.GetStaticStringIndex("create_node") == nil {
		t.Fatalf("Widen(record method surface) = %v, want fields and method surface preserved", gotType)
	}
}

func TestWidenStableRecordShapeWithMethodSurfaceAcrossMultipleIterations(t *testing.T) {
	recordWithState := func(last typ.Type) typ.Type {
		base := typetable.NewRecord().
			Field("node_order", typ.NewArray(typ.String)).
			Field("last_node_id", last).
			Build()
		return typetable.NewRecord().
			Field("node_order", typ.NewArray(typ.String)).
			Field("last_node_id", last).
			StaticStringIndex("create_node", typ.Func().Param("self", base).Returns(typ.String, typ.Nil).Build()).
			Build()
	}

	got := Widen(Of(recordWithState(typ.Nil)), Of(recordWithState(typ.String)))
	got = Widen(got, Of(recordWithState(typ.LiteralString("next"))))

	gotType, ok := got.Type()
	if !ok {
		t.Fatalf("repeated Widen(record method surface) = top, want preserved record")
	}
	rec, ok := gotType.(*typ.Record)
	if !ok || rec.GetField("node_order") == nil || rec.GetStaticStringIndex("create_node") == nil {
		t.Fatalf("repeated Widen(record method surface) = %v, want fields and method surface preserved", gotType)
	}
}

func TestWidenFallsBackToTopForIncompatibleRecordShape(t *testing.T) {
	prev := typetable.NewRecord().Field("id", typ.String).Build()
	next := typetable.NewRecord().Field("name", typ.String).Build()

	if got := Widen(Of(prev), Of(next)); !got.IsTop() {
		t.Fatalf("Widen(incompatible records) = %v, want top", got)
	}
}
