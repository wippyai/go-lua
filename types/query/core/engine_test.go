package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewEngine(t *testing.T) {
	e := NewEngine()
	if e == nil {
		t.Fatal("NewEngine returned nil")
	}
	if e.fieldQ == nil {
		t.Error("fieldQ should be initialized")
	}
	if e.methodQ == nil {
		t.Error("methodQ should be initialized")
	}
	if e.indexQ == nil {
		t.Error("indexQ should be initialized")
	}
}

func TestNewEngineWithStdlib(t *testing.T) {
	cfg := StdlibConfig{
		MethodProviders: map[kind.Kind]*typ.Record{
			kind.String: typ.NewRecord().Field("len", typ.Func().Returns(typ.Integer).Build()).Build(),
		},
	}
	e := NewEngineWithStdlib(cfg)
	if e == nil {
		t.Fatal("NewEngineWithStdlib returned nil")
	}
	if len(e.methodProviders) != 1 {
		t.Error("method providers should be set")
	}
}

func TestEngine_Field_Record(t *testing.T) {
	e := NewEngine()
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	fieldType, ok := e.Field(nil, rec, "x")
	if !ok {
		t.Fatal("Field should find 'x'")
	}
	if fieldType != typ.Number {
		t.Errorf("got %v, want number", fieldType)
	}
}

func TestEngine_Field_AliasDiscriminatedUnionCommonField(t *testing.T) {
	e := NewEngine()
	ctx := db.NewQueryContext(db.New())
	typeA := typ.NewAlias("A", typ.NewRecord().
		Field("tag", typ.LiteralString("a")).
		Field("value", typ.String).
		Build())
	typeB := typ.NewAlias("B", typ.NewRecord().
		Field("tag", typ.LiteralString("b")).
		Field("value", typ.Number).
		Build())
	union := typ.NewUnion(typeA, typeB)

	got, ok := e.Field(ctx, union, "value")
	if !ok {
		t.Fatal("expected alias union common field to resolve through query engine")
	}
	want := typ.NewUnion(typ.Number, typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("Engine.Field(A|B, value) = %v, want %v", got, want)
	}
}

func TestEngine_Field_PartialRecordUnionKeepsMissingFieldOptionality(t *testing.T) {
	e := NewEngine()
	ctx := db.NewQueryContext(db.New())
	action := typ.NewUnion(
		typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("x", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("b")).Field("y", typ.String).Build(),
	)
	okVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", action).
		Build()
	errVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()

	got, ok := e.Field(ctx, typ.NewUnion(okVariant, errVariant), "value")
	want := typ.NewOptional(action)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("Engine.Field(VR, value) = %v, %v; want %v,true", got, ok, want)
	}
}

func TestEngine_Field_NotFound(t *testing.T) {
	e := NewEngine()
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	_, ok := e.Field(nil, rec, "y")
	if ok {
		t.Error("Field should not find 'y'")
	}
}

func TestEngine_Field_RecursiveAliasResolvesLikeFreeFunction(t *testing.T) {
	e := NewEngine()
	ctx := db.NewQueryContext(db.New())

	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("status_code", typ.Number).
			OptField("next", self).
			Build()
	})
	alias := typ.NewAlias("Response", node)
	if !typ.ContainsRecursive(alias) {
		t.Fatal("setup invalid: recursive alias expected")
	}

	if _, ok := Field(alias, "status_code"); !ok {
		t.Fatal("querycore.Field(recursive alias, status_code) = false, want true")
	}
	if _, ok := e.Field(ctx, alias, "status_code"); !ok {
		t.Fatal("Engine.Field(recursive alias, status_code) = false, want true")
	}
}

func TestEngine_Field_MetatableDivergentOpenRecordsDoNotShareInternRef(t *testing.T) {
	e := NewEngine()
	ctx := db.NewQueryContext(db.New())

	readerMeta := typ.NewRecord().
		Field("__index", typ.NewRecord().OptField("session_id", typ.String).Build()).
		Build()
	reader := typ.NewRecord().SetOpen(true).Metatable(readerMeta).Build()

	queryMeta := typ.NewRecord().
		Field("__index", typ.NewRecord().
			OptField("_error", typ.String).
			OptField("_session_id", typ.String).
			OptField("_type_filter", typ.String).
			Build()).
		Build()
	query := typ.NewRecord().SetOpen(true).Metatable(queryMeta).Build()

	if typ.SameProductFamily(reader, query) {
		t.Fatal("records with divergent metatables must not be the same product family")
	}

	// Interning the reader first must not poison the query record's field cache.
	if _, ok := e.Field(ctx, reader, "session_id"); !ok {
		t.Fatal("Engine.Field(reader, session_id) = false, want true")
	}
	if _, ok := Field(query, "_error"); !ok {
		t.Fatal("querycore.Field(query, _error) = false, want true")
	}
	if _, ok := e.Field(ctx, query, "_error"); !ok {
		t.Fatal("Engine.Field(query, _error) = false, want true")
	}
}

func TestEngine_Index_Array(t *testing.T) {
	e := NewEngine()
	arr := typ.NewArray(typ.String)
	elemType, ok := e.Index(nil, arr, typ.Integer)
	if !ok {
		t.Fatal("Index should succeed for array")
	}
	// A sequence index is nil-eligible until a length proof removes nil.
	if !ContainsNil(elemType) {
		t.Errorf("got %v, want optional string", elemType)
	}
}

func TestEngine_Index_Map(t *testing.T) {
	e := NewEngine()
	m := typ.NewMap(typ.String, typ.Number)
	valType, ok := e.Index(nil, m, typ.String)
	if !ok {
		t.Fatal("Index should succeed for map")
	}
	if valType == nil {
		t.Error("expected non-nil value type")
	}
}

func TestQueryTypeRefsCanonicalizeStructuralTypesForMapKeys(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	left := typ.NewRecord().
		Field("items", typ.NewArray(typ.NewRecord().Field("id", typ.String).Build())).
		Build()
	right := typ.NewRecord().
		Field("items", typ.NewArray(typ.NewRecord().Field("id", typ.String).Build())).
		Build()

	leftRef := internTypeRef(ctx, left)
	rightRef := internTypeRef(ctx, right)
	if leftRef == nil || rightRef == nil || leftRef != rightRef {
		t.Fatalf("expected equal structural types to share query ref, got %p and %p", leftRef, rightRef)
	}

	keys := map[fieldKey]bool{{t: leftRef, name: "items"}: true}
	if !keys[fieldKey{t: rightRef, name: "items"}] {
		t.Fatal("expected boxed type refs to be usable as stable query keys")
	}
}

func TestQueryTypeRefsKeepAliasWrapperDistinctFromTarget(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	target := typ.NewRecord().Field("value", typ.String).Build()
	alias := typ.NewAlias("A", target)

	aliasRef := internTypeRef(ctx, alias)
	targetRef := internTypeRef(ctx, target)
	if aliasRef == nil || targetRef == nil {
		t.Fatal("expected query refs")
	}
	if aliasRef == targetRef {
		t.Fatal("alias and target must not share a query ref; wrapper lookup delegates to target")
	}
}

func TestQueryTypeRefsUseProductFamilyForRecursiveProducts(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})

	leftRef := internTypeRef(ctx, left)
	leftAgain := internTypeRef(ctx, left)
	rightRef := internTypeRef(ctx, right)
	if leftRef == nil || leftAgain == nil || rightRef == nil {
		t.Fatal("expected query refs")
	}
	if leftRef != leftAgain {
		t.Fatal("same recursive node should share a query ref")
	}
	if leftRef == rightRef {
		t.Fatal("distinct recursive product families must not be interned together")
	}
}

func TestQueryTypeRefsShareEquivalentRecursiveProductFamilies(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("children", typ.NewArray(self)).
			Build()
	})

	leftRef := internTypeRef(ctx, left)
	rightRef := internTypeRef(ctx, right)
	if leftRef == nil || rightRef == nil {
		t.Fatal("expected query refs")
	}
	if leftRef != rightRef {
		t.Fatalf("equivalent recursive product families should share a query ref, got %p and %p", leftRef, rightRef)
	}
}

func TestEngine_UnaryOp_Not(t *testing.T) {
	e := NewEngine()
	result := e.UnaryOp(nil, "not", typ.Boolean)
	if result != typ.Boolean {
		t.Errorf("not boolean should be boolean, got %v", result)
	}
}

func TestEngine_UnaryOp_Minus(t *testing.T) {
	e := NewEngine()
	result := e.UnaryOp(nil, "-", typ.Number)
	if result != typ.Number {
		t.Errorf("-number should be number, got %v", result)
	}
}

func TestEngine_BinaryOp_Add(t *testing.T) {
	e := NewEngine()
	result := e.BinaryOp(nil, typ.Number, "+", typ.Number)
	if result != typ.Number {
		t.Errorf("number + number should be number, got %v", result)
	}
}

func TestEngine_BinaryOp_Concat(t *testing.T) {
	e := NewEngine()
	result := e.BinaryOp(nil, typ.String, "..", typ.String)
	if result != typ.String {
		t.Errorf("string .. string should be string, got %v", result)
	}
}

func TestEngine_Callable_Function(t *testing.T) {
	e := NewEngine()
	fn := typ.Func().Returns(typ.String).Build()
	result, ok := e.Callable(nil, fn)
	if !ok {
		t.Fatal("function should be callable")
	}
	if result == nil {
		t.Error("should return function type")
	}
}

func TestEngine_IsSubtype_RecursiveAliasRecordUnion(t *testing.T) {
	e := NewEngine()
	ctx := db.NewQueryContext(db.New())

	rec := typ.NewRecursivePlaceholder("Message")
	msgAlias := typ.NewAlias("Message", rec)
	rec.SetBody(typ.NewRecord().
		Field("_topic", typ.String).
		Field("topic", typ.Func().Param("self", rec).Returns(typ.String).Build()).
		Build())

	msgCh := typ.NewAlias("MsgCh", typ.NewRecord().Field("__tag", typ.LiteralString("msg")).Build())
	timerCh := typ.NewAlias("TimerCh", typ.NewRecord().Field("__tag", typ.LiteralString("timer")).Build())
	timer := typ.NewRecord().Field("elapsed", typ.Number).Build()

	result := typ.NewUnion(
		typ.NewRecord().
			Field("channel", msgCh).
			Field("value", msgAlias).
			Field("ok", typ.Boolean).
			Build(),
		typ.NewRecord().
			Field("channel", timerCh).
			Field("value", timer).
			Field("ok", typ.Boolean).
			Build(),
	)

	synthesized := typ.NewRecord().
		Field("channel", msgCh).
		Field("value", typ.NewRecord().
			Field("_topic", typ.String).
			Field("topic", typ.Func().Param("s", msgAlias).Returns(typ.String).Build()).
			Build()).
		Field("ok", typ.True).
		Build()

	if !e.IsSubtype(ctx, synthesized, result) {
		t.Fatal("engine subtype should accept recursive alias record inside union member")
	}
}

func TestEngine_Callable_NonFunction(t *testing.T) {
	e := NewEngine()
	_, ok := e.Callable(nil, typ.String)
	if ok {
		t.Error("string should not be callable")
	}
}
