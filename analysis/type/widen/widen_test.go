package widen

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestWidenLiterals(t *testing.T) {
	tests := []struct {
		input typ.Type
		want  typ.Type
	}{
		{typ.LiteralBool(true), typ.Boolean},
		{typ.LiteralBool(false), typ.Boolean},
		{typ.LiteralInt(42), typ.Integer},
		{typ.LiteralNumber(3.14), typ.Number},
		{typ.LiteralString("hello"), typ.String},
	}
	for _, tt := range tests {
		if got := Widen(tt.input); got != tt.want {
			t.Fatalf("Widen(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestWidenNonLiterals(t *testing.T) {
	for _, ty := range []typ.Type{typ.Nil, typ.Boolean, typ.Number, typ.String, typ.Any, typ.Never} {
		if Widen(ty) != ty {
			t.Fatalf("Widen(%v) should return the same type", ty)
		}
	}
}

func TestWidenNil(t *testing.T) {
	if Widen(nil) != nil {
		t.Fatal("Widen(nil) should return nil")
	}
}

func TestWidenUnion(t *testing.T) {
	union := typ.NewUnion(typ.LiteralInt(1), typ.LiteralString("x"))

	result := Widen(union)
	resultUnion, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected Union, got %T", result)
	}

	hasInt := false
	hasString := false
	for _, m := range resultUnion.Members {
		if m == typ.Integer {
			hasInt = true
		}
		if m == typ.String {
			hasString = true
		}
	}
	if !hasInt || !hasString {
		t.Fatalf("union members should be widened, got %v", resultUnion.Members)
	}
}

func TestWidenUnionSameLiteralBase(t *testing.T) {
	union := typ.NewUnion(typ.LiteralInt(1), typ.LiteralInt(2))

	if got := Widen(union); got != typ.Integer {
		t.Fatalf("integer literal union should widen to Integer, got %v", got)
	}
}

func TestWidenOptional(t *testing.T) {
	opt := typ.NewOptional(typ.LiteralInt(42))

	resultOpt, ok := Widen(opt).(*typ.Optional)
	if !ok {
		t.Fatalf("expected Optional")
	}
	if resultOpt.Inner != typ.Integer {
		t.Fatalf("optional inner should widen to Integer, got %v", resultOpt.Inner)
	}
}

func TestWidenUnchangedUnionAndOptional(t *testing.T) {
	union := typ.NewUnion(typ.Number, typ.String)
	if got := Widen(union); got != union {
		t.Fatal("union without literals should be unchanged")
	}

	opt := typ.NewOptional(typ.Number)
	if got := Widen(opt); got != opt {
		t.Fatal("optional without literals should be unchanged")
	}
}

func TestWidenForInferenceBasicContainers(t *testing.T) {
	tuple := typ.NewTuple(typ.LiteralInt(1), typ.LiteralString("x"))
	resultTuple, ok := WidenForInference(tuple).(*typ.Tuple)
	if !ok {
		t.Fatalf("expected Tuple")
	}
	if resultTuple.Elements[0] != typ.Integer || resultTuple.Elements[1] != typ.String {
		t.Fatalf("tuple elements should be widened, got %v", resultTuple.Elements)
	}

	arr := typ.NewArray(typ.LiteralInt(42))
	resultArr, ok := WidenForInference(arr).(*typ.Array)
	if !ok {
		t.Fatalf("expected Array")
	}
	if resultArr.Element != typ.Integer {
		t.Fatalf("array element should widen to Integer, got %v", resultArr.Element)
	}

	m := typ.NewMap(typ.LiteralString("key"), typ.LiteralInt(42))
	resultMap, ok := WidenForInference(m).(*typ.Map)
	if !ok {
		t.Fatalf("expected Map")
	}
	if resultMap.Key != typ.String || resultMap.Value != typ.Integer {
		t.Fatalf("map key/value should widen, got %v -> %v", resultMap.Key, resultMap.Value)
	}

	readonly := typ.NewReadonlyMap(typ.LiteralString("key"), typ.LiteralInt(42))
	resultReadonly, ok := WidenForInference(readonly).(*typ.ReadonlyMap)
	if !ok {
		t.Fatalf("expected ReadonlyMap")
	}
	if resultReadonly.Key != typ.String || resultReadonly.Value != typ.Integer {
		t.Fatalf("readonly map key/value should widen, got %v -> %v", resultReadonly.Key, resultReadonly.Value)
	}
}

func TestWidenForInferenceNormalizesNilableTableKeys(t *testing.T) {
	nilableLiteralKey := typ.NewOptional(typ.LiteralString("key"))

	widenedMap := WidenForInference(typ.NewMap(nilableLiteralKey, typ.LiteralInt(42)))
	resultMap, ok := widenedMap.(*typ.Map)
	if !ok {
		t.Fatalf("expected Map, got %T", widenedMap)
	}
	if resultMap.Key != typ.String {
		t.Fatalf("map key should widen and drop nil, got %v", resultMap.Key)
	}

	widenedReadonly := WidenForInference(typ.NewReadonlyMap(nilableLiteralKey, typ.LiteralInt(42)))
	resultReadonly, ok := widenedReadonly.(*typ.ReadonlyMap)
	if !ok {
		t.Fatalf("expected ReadonlyMap, got %T", widenedReadonly)
	}
	if resultReadonly.Key != typ.String {
		t.Fatalf("readonly map key should widen and drop nil, got %v", resultReadonly.Key)
	}

	widenedRecord := WidenForInference(typetable.NewRecord().
		MapComponent(nilableLiteralKey, typ.LiteralInt(42)).
		Build())
	resultRecord, ok := widenedRecord.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", widenedRecord)
	}
	if resultRecord.MapKey != typ.String {
		t.Fatalf("record map key should widen and drop nil, got %v", resultRecord.MapKey)
	}
}

func TestWidenForInferenceNil(t *testing.T) {
	if WidenForInference(nil) != nil {
		t.Fatal("WidenForInference(nil) should return nil")
	}
}

func TestWidenForInferenceRecord(t *testing.T) {
	rec := typetable.NewRecord().
		Field("x", typ.LiteralInt(42)).
		OptField("y", typ.LiteralString("hello")).
		ReadonlyField("z", typ.LiteralBool(true)).
		OptReadonlyField("w", typ.LiteralNumber(1.5)).
		Build()

	resultRec, ok := WidenForInference(rec).(*typ.Record)
	if !ok {
		t.Fatalf("expected Record")
	}

	assertField := func(name string, want typ.Type, optional, readonly bool) {
		t.Helper()
		field := resultRec.GetField(name)
		if field == nil {
			t.Fatalf("missing field %s", name)
		}
		if field.Type != want || field.Optional != optional || field.Readonly != readonly {
			t.Fatalf("field %s = %+v, want type=%v optional=%v readonly=%v", name, field, want, optional, readonly)
		}
	}
	assertField("x", typ.Integer, false, false)
	assertField("y", typ.String, true, false)
	assertField("z", typ.Boolean, false, true)
	assertField("w", typ.Number, true, true)
}

func TestWidenForInferenceRecordStaticMembers(t *testing.T) {
	rec := typetable.NewRecord().
		StaticStringIndex("name", typ.LiteralString("lua")).
		AddStaticMember(typ.StaticMember{
			Kind:     typ.StaticMemberIntIndex,
			Index:    1,
			Type:     typ.LiteralInt(9),
			Optional: true,
			Readonly: true,
		}).
		Build()

	resultRec, ok := WidenForInference(rec).(*typ.Record)
	if !ok {
		t.Fatalf("expected Record")
	}

	name := resultRec.GetStaticStringIndex("name")
	if name == nil || name.Type != typ.String {
		t.Fatalf("static string member should widen to String, got %v", name)
	}
	index := resultRec.GetStaticIntIndex(1)
	if index == nil || index.Type != typ.Integer || !index.Optional || !index.Readonly {
		t.Fatalf("static int member should widen and preserve flags, got %v", index)
	}
}

func TestWidenForInferenceRecordWithMetatableAndMapComponent(t *testing.T) {
	meta := typetable.NewRecord().Field("__index", typ.LiteralString("meta")).Build()
	rec := typetable.NewRecord().
		SetOpen(true).
		Field("name", typ.LiteralString("test")).
		Metatable(meta).
		MapComponent(typ.LiteralString("key"), typ.LiteralInt(1)).
		Build()

	resultRec, ok := WidenForInference(rec).(*typ.Record)
	if !ok {
		t.Fatalf("expected Record")
	}
	if !resultRec.Open {
		t.Fatal("open flag should be preserved")
	}
	if resultRec.Metatable == nil {
		t.Fatal("metatable should be preserved")
	}
	resultMeta, ok := resultRec.Metatable.(*typ.Record)
	if !ok {
		t.Fatalf("expected record metatable, got %T", resultRec.Metatable)
	}
	index := resultMeta.GetField("__index")
	if index == nil || index.Type != typ.String {
		t.Fatalf("metatable field should be widened, got %v", index)
	}
	if !resultRec.HasMapComponent() {
		t.Fatal("map component should be preserved")
	}
	if resultRec.MapKey != typ.String || resultRec.MapValue != typ.Integer {
		t.Fatalf("map component should widen, got %v -> %v", resultRec.MapKey, resultRec.MapValue)
	}
}

func TestWidenForInferenceLargeRecordCollapses(t *testing.T) {
	builder := typetable.NewRecord()
	for i := 0; i < typ.DefaultRecursionDepth+1; i++ {
		builder.Field(fmt.Sprintf("f%d", i), typ.LiteralInt(int64(i)))
	}

	resultMap, ok := WidenForInference(builder.Build()).(*typ.Map)
	if !ok {
		t.Fatalf("expected large record to collapse to Map, got %T", WidenForInference(builder.Build()))
	}
	if resultMap.Key != typ.String || resultMap.Value != typ.Integer {
		t.Fatalf("large record map should be {[string]: integer}, got %v -> %v", resultMap.Key, resultMap.Value)
	}
}

func TestWidenForInferenceUnchangedContainers(t *testing.T) {
	tuple := typ.NewTuple(typ.Number, typ.String)
	if got := WidenForInference(tuple); got != tuple {
		t.Fatal("tuple without literals should be unchanged")
	}

	arr := typ.NewArray(typ.Number)
	if got := WidenForInference(arr); got != arr {
		t.Fatal("array without literals should be unchanged")
	}

	m := typ.NewMap(typ.String, typ.Number)
	if got := WidenForInference(m); got != m {
		t.Fatal("map without literals should be unchanged")
	}

	rec := typetable.NewRecord().Field("x", typ.Number).Build()
	if got := WidenForInference(rec); got != rec {
		t.Fatal("record without literals should be unchanged")
	}
}

func TestWidenForInferenceFunctionNestedLiterals(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.LiteralInt(7)).
		OptParam("flag", typ.LiteralBool(true)).
		Variadic(typ.LiteralString("extra")).
		Returns(
			typetable.NewRecord().
				Field("label", typ.LiteralString("ok")).
				Build(),
		).
		Build()

	resultFn, ok := WidenForInference(fn).(*typ.Function)
	if !ok {
		t.Fatalf("expected Function")
	}
	if resultFn.Params[0].Type != typ.Integer {
		t.Fatalf("expected widened param Integer, got %v", resultFn.Params[0].Type)
	}
	if resultFn.Params[1].Type != typ.Boolean || !resultFn.Params[1].Optional {
		t.Fatalf("expected widened optional param Boolean, got %+v", resultFn.Params[1])
	}
	if resultFn.Variadic != typ.String {
		t.Fatalf("expected widened variadic String, got %v", resultFn.Variadic)
	}

	retRec, ok := resultFn.Returns[0].(*typ.Record)
	if !ok {
		t.Fatalf("expected record return, got %T", resultFn.Returns[0])
	}
	field := retRec.GetField("label")
	if field == nil || field.Type != typ.String {
		t.Fatalf("expected widened record field String, got %v", field)
	}
}

func TestWidenReturnTowerOnlyPreservesFunctionParameters(t *testing.T) {
	param := typ.LiteralInt(7)
	fn := typ.Func().
		Param("x", param).
		Variadic(typ.LiteralString("extra")).
		Returns(typ.LiteralString("ok")).
		Build()

	resultFn, ok := WidenReturnTowerOnly(fn).(*typ.Function)
	if !ok {
		t.Fatalf("expected Function")
	}
	if resultFn.Params[0].Type != param {
		t.Fatalf("parameter should be preserved, got %v", resultFn.Params[0].Type)
	}
	if resultFn.Variadic != typ.String {
		t.Fatalf("variadic should be widened, got %v", resultFn.Variadic)
	}
	if resultFn.Returns[0] != typ.String {
		t.Fatalf("return should be widened, got %v", resultFn.Returns[0])
	}
}

func TestWidenForInferenceGenericFunctionUnchanged(t *testing.T) {
	fn := typ.Func().
		TypeParam("T", typ.Any).
		Param("x", typ.LiteralInt(7)).
		Returns(typ.LiteralString("ok")).
		Build()

	if got := WidenForInference(fn); got != fn {
		t.Fatal("generic function should be preserved to keep binder references intact")
	}
}

func TestWidenForInferenceInterfaceMethodNestedLiterals(t *testing.T) {
	method := typ.Func().
		Param("self", typ.Any).
		Returns(typ.LiteralString("ok")).
		Build()
	iface := typ.NewInterface("", []typ.Method{{Name: "status", Type: method}})

	resultIface, ok := WidenForInference(iface).(*typ.Interface)
	if !ok {
		t.Fatalf("expected Interface")
	}
	if len(resultIface.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(resultIface.Methods))
	}
	if resultIface.Methods[0].Type.Returns[0] != typ.String {
		t.Fatalf("expected widened method return String, got %v", resultIface.Methods[0].Type.Returns[0])
	}
}

func TestWidenForInferenceOtherTypesUnchanged(t *testing.T) {
	for _, ty := range []typ.Type{
		typ.String,
		typ.NewOptional(typ.Number),
		typ.NewUnion(typ.Number, typ.String),
		typ.NewIntersection(typ.Number, typ.String),
	} {
		if got := WidenForInference(ty); got != ty {
			t.Fatalf("WidenForInference(%s) should be unchanged, got %s", ty, got)
		}
	}
}

func TestWidenUsesConstructorUnionRules(t *testing.T) {
	union := typ.NewUnion(typ.LiteralInt(1), typ.Nil)
	result := Widen(union)

	opt, ok := result.(*typ.Optional)
	if !ok {
		t.Fatalf("literal-or-nil union should become optional after widening, got %T", result)
	}
	if opt.Inner != typ.Integer {
		t.Fatalf("optional inner should be Integer, got %v", opt.Inner)
	}
}

func TestWidenUnknownLiteralBaseIsUnchanged(t *testing.T) {
	lit := &typ.Literal{Base: kind.Never, Value: "sentinel"}
	if got := Widen(lit); got != lit {
		t.Fatalf("unknown literal base should be unchanged, got %v", got)
	}
}
