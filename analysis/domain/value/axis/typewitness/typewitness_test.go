package typewitness

import (
	"strconv"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	typeaccess "github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestArtifactRetentionRejectsRecursivePlaceholderBeforeAndAfterSetBody(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, runtimekind.Spec())
	axis.Register(reg, Spec())
	reg.Freeze()
	placeholder := typ.NewRecursivePlaceholder("Mutable")
	stored := product.Set(reg, product.Top(), Key, Of(placeholder))
	if product.RetentionSafe(reg, stored) {
		t.Fatal("open recursive placeholder was admitted to immutable artifact")
	}
	placeholder.SetBody(typ.String)
	if product.RetentionSafe(reg, stored) {
		t.Fatal("SetBody-mutated recursive witness was admitted to immutable artifact")
	}
}

func TestArtifactRetentionAdmitsExactPrimitiveSingletonWitnesses(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, runtimekind.Spec())
	axis.Register(reg, Spec())
	reg.Freeze()
	for _, witness := range []Value{Bottom(), Top(), Of(typ.Nil), Of(typ.Boolean), Of(typ.Integer), Of(typ.Number), Of(typ.String), Of(typ.Never)} {
		stored := product.Set(reg, product.Top(), Key, witness)
		if !product.RetentionSafe(reg, stored) {
			t.Fatalf("is_str leaf witness %v was rejected", witness)
		}
	}
}

func TestArtifactRetentionRejectsCompositeAndNominalWitnesses(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, runtimekind.Spec())
	axis.Register(reg, Spec())
	reg.Freeze()
	param := typ.NewTypeParam("T", nil)
	for name, valueType := range map[string]typ.Type{
		"array":   typ.NewArray(typ.String),
		"record":  typetable.NewRecord().Field("value", typ.String).Build(),
		"alias":   typ.NewAlias("Name", typ.String),
		"generic": typ.NewGeneric("Box", []*typ.TypeParam{param}, typ.NewArray(param)),
		"union":   typ.MaterializeUnion([]typ.Type{typ.Number, typ.String}),
	} {
		t.Run(name, func(t *testing.T) {
			witness := Value{t: valueType}
			stored := product.Set(reg, product.Top(), Key, witness)
			if product.RetentionSafe(reg, stored) {
				t.Fatalf("mutable/composite witness %T crossed immutable artifact boundary", valueType)
			}
		})
	}
}

type forgedStringType struct{ mutable string }

func (*forgedStringType) Kind() kind.Kind              { return kind.String }
func (f *forgedStringType) String() string             { return f.mutable }
func (*forgedStringType) Hash() uint64                 { return 1 }
func (f *forgedStringType) Equals(other typ.Type) bool { return f == other }

func TestArtifactRetentionRejectsKindForgedPrimitiveAndMutableLiteral(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, runtimekind.Spec())
	axis.Register(reg, Spec())
	reg.Freeze()
	forged := &forgedStringType{mutable: "before"}
	if product.RetentionSafe(reg, product.Set(reg, product.Top(), Key, Value{t: forged})) {
		t.Fatal("custom type forged as kind.String crossed artifact boundary")
	}
	forged.mutable = "after"

	literal := &typ.Literal{Base: kind.String, Value: "retention-mutation-fixture"}
	stored := product.Set(reg, product.Top(), Key, Value{t: literal})
	if product.RetentionSafe(reg, stored) {
		t.Fatal("pointer-backed literal crossed artifact boundary before mutation")
	}
	literal.Value = "mutated"
	if product.RetentionSafe(reg, stored) {
		t.Fatal("post-mutation pointer-backed literal crossed artifact boundary")
	}
}

func TestValueStaysSmallAndInternsRecursiveSignatures(t *testing.T) {
	if got := unsafe.Sizeof(Value{}); got != 24 {
		t.Fatalf("typewitness Value size = %d, want 24 bytes", got)
	}
	recursive := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewArray(self)
	})
	first, second := Of(recursive), Of(recursive)
	if first.recursive == nil || first.recursive != second.recursive {
		t.Fatal("equivalent recursive signatures must use one canonical handle")
	}
}

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
		SetOpen(true).
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

func TestWidenUsesOpenRecordForDisjointStableFields(t *testing.T) {
	prev := typetable.NewRecord().Field("id", typ.String).Build()
	next := typetable.NewRecord().Field("name", typ.String).Build()

	got := Widen(Of(prev), Of(next))
	gotType, ok := got.Type()
	if !ok {
		t.Fatalf("Widen(disjoint records) = top, want open record")
	}
	want := typetable.NewRecord().
		OptField("id", typ.String).
		OptField("name", typ.String).
		SetOpen(true).
		Build()
	if !typ.TypeEquals(gotType, want) {
		t.Fatalf("Widen(disjoint records) = %v, want %v", gotType, want)
	}
	if !LessOrEq(Of(prev), got) || !LessOrEq(Of(next), got) {
		t.Fatalf("Widen(disjoint records) = %v is not an upper bound", got)
	}
}

func TestWidenUpperBoundLawAcrossReachableBranches(t *testing.T) {
	withMap := func(value typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("status", typ.LiteralString("ready")).
			MapComponent(typ.String, value).
			Build()
	}
	withMetatable := func(metatable typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("status", typ.String).
			Metatable(metatable).
			Build()
	}

	tests := []struct {
		name string
		prev Value
		next Value
	}{
		{name: "bottom", prev: Bottom(), next: Of(typ.String)},
		{name: "next bottom", prev: Of(typ.String), next: Bottom()},
		{name: "top", prev: Top(), next: Of(typ.String)},
		{name: "already ordered", prev: Of(typ.Integer), next: Of(typ.LiteralInt(1))},
		{name: "primitive family", prev: Of(typ.LiteralInt(0)), next: Of(typ.LiteralInt(1))},
		{name: "integer number family", prev: Of(typ.Integer), next: Of(typ.Number)},
		{name: "array elements", prev: Of(typ.NewArray(typ.LiteralString("a"))), next: Of(typ.NewArray(typ.LiteralString("b")))},
		{name: "array tuple", prev: Of(typ.NewArray(typ.LiteralString("a"))), next: Of(typ.NewTuple(typ.LiteralString("b"), typ.LiteralString("c")))},
		{name: "empty record array", prev: Of(typetable.NewRecord().Build()), next: Of(typ.NewArray(typ.LiteralString("item")))},
		{name: "record field", prev: Of(typetable.NewRecord().Field("status", typ.LiteralString("old")).Build()), next: Of(typetable.NewRecord().Field("status", typ.LiteralString("new")).Build())},
		{name: "record field growth", prev: Of(typetable.NewRecord().Field("status", typ.LiteralString("old")).Build()), next: Of(typetable.NewRecord().Field("status", typ.LiteralString("new")).Field("error", typ.String).Build())},
		{name: "record static member", prev: Of(typetable.NewRecord().Field("status", typ.String).StaticStringIndex("run", typ.LiteralString("old")).Build()), next: Of(typetable.NewRecord().Field("status", typ.String).StaticStringIndex("run", typ.LiteralString("new")).Build())},
		{name: "record static member growth", prev: Of(typetable.NewRecord().Field("status", typ.String).StaticStringIndex("run", typ.String).Build()), next: Of(typetable.NewRecord().Field("status", typ.String).StaticStringIndex("run", typ.String).StaticStringIndex("stop", typ.String).Build())},
		{name: "record map value", prev: Of(withMap(typ.LiteralString("old"))), next: Of(withMap(typ.LiteralString("new")))},
		{name: "record map shape", prev: Of(typetable.NewRecord().Field("status", typ.LiteralString("ready")).Build()), next: Of(withMap(typ.String))},
		{name: "record open shape", prev: Of(typetable.NewRecord().Field("status", typ.LiteralString("old")).Build()), next: Of(typetable.NewRecord().Field("status", typ.LiteralString("new")).SetOpen(true).Build())},
		{name: "fallback top", prev: Of(withMetatable(typ.String)), next: Of(withMetatable(typ.Number))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widened := Widen(tt.prev, tt.next)
			if !LessOrEq(tt.prev, widened) {
				t.Fatalf("prev %v is not below Widen(prev,next) = %v", tt.prev, widened)
			}
			if !LessOrEq(tt.next, widened) {
				t.Fatalf("next %v is not below Widen(prev,next) = %v", tt.next, widened)
			}
		})
	}
}

func TestWidenArbitrarilyNamedRecordFieldsProducesAnUpperBound(t *testing.T) {
	prev := Of(recordWithGeneratedFields(typ.LiteralString("cold"), 0))
	next := Of(recordWithGeneratedFields(typ.LiteralString("hot"), 257))

	widened := Widen(prev, next)
	if !LessOrEq(prev, widened) || !LessOrEq(next, widened) {
		t.Fatalf("Widen(%v,%v) = %v is not an upper bound", prev, next, widened)
	}
	record, ok := widened.Type()
	if !ok {
		t.Fatalf("Widen(record growth) = top, want an open record upper bound")
	}
	got, ok := record.(*typ.Record)
	if !ok || !got.Open {
		t.Fatalf("Widen(record growth) = %v, want open record", record)
	}
	for index := 0; index < 257; index++ {
		field := got.GetField(generatedRecordFieldName(index))
		if field == nil || !field.Optional || !typ.TypeEquals(field.Type, typ.String) {
			t.Fatalf("generated field %q = %#v, want optional string", generatedRecordFieldName(index), field)
		}
	}
}

func TestWidenRecordFieldGrowthStabilizesWithoutFieldBudget(t *testing.T) {
	current := Widen(
		Of(recordWithGeneratedFields(typ.LiteralString("cold"), 0)),
		Of(recordWithGeneratedFields(typ.LiteralString("hot"), 1)),
	)
	if current.IsTop() {
		t.Fatal("initial record field growth widened to top")
	}

	for count := 2; count <= 257; count++ {
		next := Of(recordWithGeneratedFields(typ.LiteralString("hot"), count))
		widened := Widen(current, next)
		if !Equal(widened, current) {
			t.Fatalf("record widening grew at %d fields: %v -> %v", count, current, widened)
		}
		if !LessOrEq(next, widened) {
			t.Fatalf("record with %d generated fields is not below stable widened value %v", count, widened)
		}
	}
}

func TestWidenNestedArrayShapeGrowthStabilizes(t *testing.T) {
	current := Widen(Of(nestedArray(typ.String, 1)), Of(nestedArray(typ.String, 2)))
	if current.IsTop() {
		t.Fatal("nested array growth widened to top instead of retaining the array witness")
	}
	for depth := 3; depth <= 128; depth++ {
		next := Of(nestedArray(typ.String, depth))
		widened := Widen(current, next)
		if !Equal(widened, current) {
			t.Fatalf("nested array widening grew at depth %d: %v -> %v", depth, current, widened)
		}
		if !LessOrEq(next, widened) {
			t.Fatalf("nested array at depth %d is not below stable widened value %v", depth, widened)
		}
	}
}

func TestLessOrEqContainsOptionalInnerEvidence(t *testing.T) {
	recordA := typetable.NewRecord().Field("a", typ.String).Build()
	openRecord := typetable.NewRecord().
		OptField("a", typ.String).
		OptField("b", typ.String).
		SetOpen(true).
		Build()
	optionalLiterals := typ.MaterializeOptional(typ.MaterializeUnion([]typ.Type{
		typ.LiteralString("a"),
		typ.LiteralString("b"),
	}))

	tests := []struct {
		name   string
		source typ.Type
		target typ.Type
	}{
		{name: "nil", source: typ.Nil, target: typ.MaterializeOptional(typ.String)},
		{name: "record", source: typ.MaterializeOptional(recordA), target: typ.MaterializeOptional(openRecord)},
		{name: "array", source: typ.MaterializeOptional(typ.NewArray(typ.String)), target: typ.MaterializeOptional(typ.NewArray(typ.Any))},
		{name: "union", source: optionalLiterals, target: typ.MaterializeOptional(typ.String)},
		{name: "optional into explicit union", source: typ.MaterializeOptional(recordA), target: typ.MaterializeUnion([]typ.Type{typ.Nil, openRecord})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !LessOrEq(Of(tt.source), Of(tt.target)) {
				t.Fatalf("optional source %v is not contained in target %v", tt.source, tt.target)
			}
		})
	}
}

func TestOfCanonicalizesOptionalAndExplicitNilUnion(t *testing.T) {
	record := typetable.NewRecord().Field("id", typ.String).Build()
	optional := Of(typ.MaterializeOptional(record))
	explicit := Of(typ.MaterializeUnion([]typ.Type{typ.Nil, record}))
	if !Equal(optional, explicit) {
		t.Fatalf("Of(optional %v) = %v, Of(nil union %v) = %v; equivalent witnesses must compare equal", record, optional, record, explicit)
	}
	if !LessOrEq(optional, explicit) || !LessOrEq(explicit, optional) {
		t.Fatalf("optional and explicit nil-union witnesses must be mutually contained: %v, %v", optional, explicit)
	}
}

func TestWidenOptionalRecordFieldIsAnUpperBound(t *testing.T) {
	prev := typetable.NewRecord().
		Field("x", typ.MaterializeOptional(typetable.NewRecord().Field("a", typ.String).Build())).
		Build()
	next := typetable.NewRecord().
		Field("x", typ.MaterializeOptional(typetable.NewRecord().Field("b", typ.String).Build())).
		Build()

	widened := Widen(Of(prev), Of(next))
	if !LessOrEq(Of(prev), widened) || !LessOrEq(Of(next), widened) {
		t.Fatalf("Widen(%v,%v) = %v is not an upper bound", prev, next, widened)
	}
}

func TestWidenReconcilesDotAndStaticStringKeys(t *testing.T) {
	staticRecord := func(name string, value typ.Type, optional, readonly bool) typ.Type {
		return typetable.NewRecord().AddStaticMember(typ.StaticMember{
			Kind:     typ.StaticMemberStringIndex,
			Name:     name,
			Type:     value,
			Optional: optional,
			Readonly: readonly,
		}).Build()
	}
	tests := []struct {
		name    string
		prev    typ.Type
		next    typ.Type
		want    typ.Type
		wantTop bool
	}{
		{
			name: "field then static",
			prev: typetable.NewRecord().Field("id", typ.String).Build(),
			next: staticRecord("id", typ.Number, false, false),
			want: typetable.NewRecord().Field("id", typ.Any).Build(),
		},
		{
			name: "static then field",
			prev: staticRecord("id", typ.Number, false, false),
			next: typetable.NewRecord().Field("id", typ.String).Build(),
			want: typetable.NewRecord().Field("id", typ.Any).Build(),
		},
		{
			name: "optional field then required static",
			prev: typetable.NewRecord().OptField("id", typ.String).Build(),
			next: staticRecord("id", typ.String, false, false),
			want: typetable.NewRecord().OptField("id", typ.String).Build(),
		},
		{
			name: "required static then optional field",
			prev: staticRecord("id", typ.String, false, false),
			next: typetable.NewRecord().OptField("id", typ.String).Build(),
			want: typetable.NewRecord().OptField("id", typ.String).Build(),
		},
		{
			name: "readonly field then readonly static",
			prev: typetable.NewRecord().ReadonlyField("id", typ.Integer).Build(),
			next: staticRecord("id", typ.Number, false, true),
			want: typetable.NewRecord().ReadonlyField("id", typ.Number).Build(),
		},
		{
			name: "readonly static then readonly field",
			prev: staticRecord("id", typ.Integer, false, true),
			next: typetable.NewRecord().ReadonlyField("id", typ.Number).Build(),
			want: typetable.NewRecord().ReadonlyField("id", typ.Number).Build(),
		},
		{
			name:    "readonly mismatch",
			prev:    typetable.NewRecord().ReadonlyField("id", typ.String).Build(),
			next:    staticRecord("id", typ.String, false, false),
			wantTop: true,
		},
		{
			name:    "readonly mismatch reverse",
			prev:    staticRecord("id", typ.String, false, false),
			next:    typetable.NewRecord().ReadonlyField("id", typ.String).Build(),
			wantTop: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev, next := Of(tt.prev), Of(tt.next)
			widened := Widen(prev, next)
			if tt.wantTop {
				if !widened.IsTop() {
					t.Fatalf("Widen(%v,%v) = %v, want top", tt.prev, tt.next, widened)
				}
				return
			}
			got, ok := widened.Type()
			if !ok || !typ.TypeEquals(got, tt.want) {
				t.Fatalf("Widen(%v,%v) = %v/%v, want %v", tt.prev, tt.next, got, ok, tt.want)
			}
			if !LessOrEq(prev, widened) || !LessOrEq(next, widened) {
				t.Fatalf("Widen(%v,%v) = %v is not an upper bound", tt.prev, tt.next, widened)
			}
		})
	}
}

func TestLessOrEqPreservesDotFieldPrecedenceOverStaticStringMember(t *testing.T) {
	conflicted := typetable.NewRecord().
		OptField("id", typ.String).
		AddStaticMember(typ.StaticMember{
			Kind: typ.StaticMemberStringIndex,
			Name: "id",
			Type: typ.String,
		}).
		Build()

	got, ok := typeaccess.Field(conflicted, "id")
	want := typ.MaterializeOptional(typ.String)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("Field(conflicted, id) = %v/%v, want %v", got, ok, want)
	}
	required := typetable.NewRecord().Field("id", typ.String).Build()
	if LessOrEq(Of(conflicted), Of(required)) {
		t.Fatalf("optional dot-field evidence %v was accepted under required target %v", conflicted, required)
	}
}

func TestWidenCoversOpenAndMapLookupEvidence(t *testing.T) {
	mapOnly := typetable.NewRecord().MapComponent(typ.String, typ.Number).Build()
	mapAndStatic := typetable.NewRecord().
		StaticStringIndex("id", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()
	tests := []struct {
		name string
		prev typ.Type
		next typ.Type
	}{
		{
			name: "open record missing field",
			prev: typetable.NewRecord().Field("id", typ.String).SetOpen(true).Build(),
			next: typetable.NewRecord().SetOpen(true).Build(),
		},
		{name: "map record missing static key", prev: mapOnly, next: mapAndStatic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev, next := Of(tt.prev), Of(tt.next)
			widened := Widen(prev, next)
			if !LessOrEq(prev, widened) || !LessOrEq(next, widened) {
				t.Fatalf("Widen(%v,%v) = %v is not an upper bound", tt.prev, tt.next, widened)
			}
			widenedType, ok := widened.Type()
			if !ok {
				t.Fatalf("Widen(%v,%v) = top, want open record upper bound", tt.prev, tt.next)
			}
			got, ok := typeaccess.Field(widenedType, "id")
			if !ok || !typ.TypeEquals(got, typ.Any) {
				t.Fatalf("Field(Widen(%v,%v), id) = %v/%v, want any", tt.prev, tt.next, got, ok)
			}
		})
	}
}

func TestLessOrEqContainsPrimitiveFamilyWideningInNestedShapes(t *testing.T) {
	tests := []struct {
		name   string
		source typ.Type
		target typ.Type
	}{
		{name: "integer number", source: typ.Integer, target: typ.Number},
		{name: "integer literal number", source: typ.LiteralInt(1), target: typ.Number},
		{name: "optional integer number", source: typ.MaterializeOptional(typ.Integer), target: typ.MaterializeOptional(typ.Number)},
		{name: "array integer number", source: typ.NewArray(typ.Integer), target: typ.NewArray(typ.Number)},
		{name: "record integer number", source: typetable.NewRecord().Field("value", typ.Integer).Build(), target: typetable.NewRecord().Field("value", typ.Number).Build()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !LessOrEq(Of(tt.source), Of(tt.target)) {
				t.Fatalf("source %v is not contained in target %v", tt.source, tt.target)
			}
		})
	}
}

func TestLessOrEqOrdinaryRecordDoesNotAllocate(t *testing.T) {
	source := Of(typetable.NewRecord().Field("id", typ.String).Build())
	target := Of(typetable.NewRecord().
		Field("id", typ.String).
		OptField("name", typ.String).
		Build())
	if !LessOrEq(source, target) {
		t.Fatal("test setup: ordinary record must be contained by optional extension")
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		if !LessOrEq(source, target) {
			t.Fatal("ordinary record containment changed during allocation check")
		}
	}); allocations != 0 {
		t.Fatalf("LessOrEq(ordinary record) allocations = %v, want 0", allocations)
	}
}

func recordWithGeneratedFields(status typ.Type, count int) typ.Type {
	fields := make([]typ.Field, 0, count+1)
	fields = append(fields, typ.Field{Name: "status", Type: status})
	for index := 0; index < count; index++ {
		fields = append(fields, typ.Field{Name: generatedRecordFieldName(index), Type: typ.String})
	}
	return typ.RebuildRecord(typ.RecordParts{Fields: fields})
}

func generatedRecordFieldName(index int) string {
	return "generated/field-" + strconv.FormatInt(int64(index*7919+17), 36) + "-" + strconv.FormatInt(int64((index*97)%257), 10)
}

func nestedArray(element typ.Type, depth int) typ.Type {
	for range depth {
		element = typ.NewArray(element)
	}
	return element
}
