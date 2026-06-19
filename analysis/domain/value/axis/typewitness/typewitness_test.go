package typewitness

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestOfRejectsOpenTypeParameter(t *testing.T) {
	if got := Of(typ.NewTypeParam("T", nil)); !got.IsTop() {
		t.Fatalf("type parameter witness = %v, want top", got)
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
		t.Fatalf("open generic witness = %v, want top", got)
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
