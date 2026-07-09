package subtype

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestPrimitiveStrictOrder(t *testing.T) {
	for _, ty := range []typ.Type{typ.Nil, typ.Boolean, typ.Number, typ.Integer, typ.String, typ.Any, typ.Unknown, typ.Never} {
		if !IsSubtype(ty, ty) {
			t.Fatalf("%s should be a subtype of itself", ty)
		}
	}
	if !IsSubtype(typ.Never, typ.String) {
		t.Fatal("never should be bottom")
	}
	if !IsSubtype(typ.String, typ.Any) {
		t.Fatal("any should be top in target position")
	}
	if IsSubtype(typ.Any, typ.String) {
		t.Fatal("any should not be a strict source subtype of string")
	}
	if !IsSubtype(typ.String, typ.Unknown) || IsSubtype(typ.Unknown, typ.String) {
		t.Fatal("unknown should be target-top but not source-bottom")
	}
	if !IsSubtype(typ.Any, typeexpr.Optional(typ.Unknown)) {
		t.Fatal("unknown? should behave as a top target")
	}
}

func TestSubtypeDepthExhaustionFailsClosed(t *testing.T) {
	exhausted := typ.DefaultRecursionDepth + 1
	if (&checker{}).check(typ.String, typ.String, exhausted) {
		t.Fatal("subtype comparison succeeded after recursion-depth exhaustion")
	}

	var sub typ.Type = typ.Number
	var super typ.Type = typ.String
	for i := 0; i < typ.DefaultRecursionDepth+2; i++ {
		sub = typetable.NewRecord().Field("next", sub).Build()
		super = typetable.NewRecord().Field("next", super).Build()
	}
	if IsSubtype(sub, super) {
		t.Fatal("deeply nested mismatched record was a subtype")
	}

	sub = typ.Number
	super = typ.String
	for i := 0; i < typ.DefaultRecursionDepth+2; i++ {
		sub = &typ.Alias{Name: "Sub", Target: sub}
		super = &typ.Alias{Name: "Super", Target: super}
	}
	if IsSubtype(sub, super) {
		t.Fatal("distinct alias chains became subtypes after equality depth exhaustion")
	}
}

func TestAnySourceStrictSubtypeBehavior(t *testing.T) {
	tableTop := typ.NewInterface("table", nil)
	record := typetable.NewRecord().Field("id", typ.String).Build()

	tests := []struct {
		name  string
		super typ.Type
		want  bool
	}{
		{"concrete primitive", typ.String, false},
		{"concrete record", record, false},
		{"builtin table top marker", tableTop, true},
		{"primitive union", typeexpr.Union(typ.String, typ.Number), false},
		{"union with table top marker", typeexpr.Union(typ.String, tableTop), true},
		{"intersection with concrete record", typeexpr.Intersection(tableTop, record), false},
		{"intersection with only accepted tops", typeexpr.Intersection(tableTop, typ.Any), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSubtype(typ.Any, tt.super); got != tt.want {
				t.Fatalf("IsSubtype(any, %s) = %v, want %v", tt.super, got, tt.want)
			}
		})
	}
}

func TestLiteralOptionalUnionAndIntersectionStrict(t *testing.T) {
	if !IsSubtype(typ.LiteralInt(42), typ.Number) {
		t.Fatal("integer literal should be a number")
	}
	if IsSubtype(typ.String, typeexpr.Union(typ.LiteralString("a"), typ.LiteralString("b"))) {
		t.Fatal("base string should not satisfy a literal-only union")
	}
	optNumber := typeexpr.Optional(typ.Number)
	if !IsSubtype(typ.Integer, optNumber) || !IsSubtype(typ.Nil, optNumber) {
		t.Fatal("number? should accept integer and nil")
	}
	if IsSubtype(optNumber, typ.Number) {
		t.Fatal("number? should not satisfy plain number")
	}

	rec := typetable.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
	xOnly := typetable.NewRecord().Field("x", typ.Number).Build()
	yOnly := typetable.NewRecord().Field("y", typ.String).Build()
	if !IsSubtype(rec, typeexpr.Intersection(xOnly, yOnly)) {
		t.Fatal("record with both fields should satisfy both intersection members")
	}
}

func TestFunctionParameterReturnAndArity(t *testing.T) {
	sub := typ.Func().Param("x", typ.Number).Returns(typ.Integer).Build()
	super := typ.Func().Param("x", typ.Integer).Returns(typ.Number).Build()
	if !IsSubtype(sub, super) {
		t.Fatal("function should accept a broader parameter and return a narrower value")
	}
	if IsSubtype(super, sub) {
		t.Fatal("reverse function relation should fail")
	}

	requiresTwo := typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Nil).Build()
	requiresOne := typ.Func().Param("x", typ.Number).Returns(typ.Nil).Build()
	if IsSubtype(requiresTwo, requiresOne) {
		t.Fatal("function requiring more arguments should not satisfy a one-argument target")
	}
	withLateRequired := typ.Func().OptParam("x", typ.Number).Param("y", typ.Number).Returns(typ.Nil).Build()
	if IsSubtype(withLateRequired, requiresOne) {
		t.Fatal("required parameter after optional still contributes to minimum arity")
	}
	if !IsSubtype(typ.Func().Build(), typ.Func().Returns(typeexpr.Optional(typ.Unknown)).Build()) {
		t.Fatal("missing Lua return should satisfy nilable return slot")
	}
}

func TestFreshMethodReceiverCanWidenToBaseReceiver(t *testing.T) {
	animal := typetable.NewRecord().Field("name", typ.String).Build()
	dog := typetable.NewRecord().Field("name", typ.String).Field("fetch", typ.Func().Param("self", typ.Any).Returns(typ.String).Build()).Build()
	animalSpeak := typ.Func().Param("self", animal).Returns(typ.String).Build()
	dogSpeak := typ.Func().Param("self", dog).Returns(typ.String).Build()

	if !IsFreshAssignable(dog, animal) {
		t.Fatal("fresh Dog receiver should satisfy Animal receiver")
	}
	if !IsFreshAssignable(animalSpeak, dogSpeak) {
		t.Fatal("Animal.speak should be assignable to Dog.speak")
	}
}

func TestFreshRecursiveRecordMethodReceiversCanWidenAcrossEquivalentAliases(t *testing.T) {
	buildLogger := func() *typ.Recursive {
		return typ.NewRecursive("Logger", func(self typ.Type) typ.Type {
			return typetable.NewRecord().
				Field("level", typ.String).
				Field("log", typ.Func().Param("self", self).Param("msg", typ.String).Build()).
				Field("debug", typ.Func().Param("self", self).Param("msg", typ.String).Build()).
				Build()
		})
	}
	sourceSelf := buildLogger()
	target := buildLogger()
	source := typetable.NewRecord().
		Field("level", typ.String).
		Field("log", typ.Func().Param("self", sourceSelf).Param("msg", typ.String).Build()).
		Field("debug", typ.Func().Param("self", sourceSelf).Param("msg", typ.String).Build()).
		Build()

	if !IsFreshAssignable(source, target) {
		t.Fatal("fresh recursive method receiver should satisfy an equivalent declared recursive alias")
	}
}

func TestRecordWidthDepthReadonlyAndMutable(t *testing.T) {
	named := typetable.NewRecord().Field("name", typ.String).Build()
	namedAge := typetable.NewRecord().Field("name", typ.String).Field("age", typ.Number).Build()
	if !IsSubtype(namedAge, named) || IsSubtype(named, namedAge) {
		t.Fatal("record width check should require target fields only")
	}

	staticName := typetable.NewRecord().StaticStringIndex("name", typ.String).Build()
	if !IsSubtype(staticName, named) {
		t.Fatal("exact static string member should satisfy an equivalent dot-field read")
	}
	if !IsSubtype(named, staticName) {
		t.Fatal("dot field should satisfy an equivalent exact static string member read")
	}
	badStaticName := typetable.NewRecord().StaticStringIndex("name", typ.Number).Build()
	if IsSubtype(badStaticName, named) {
		t.Fatal("static string member with wrong type should not satisfy dot-field")
	}

	mutableInteger := typetable.NewRecord().Field("x", typ.Integer).Build()
	mutableNumber := typetable.NewRecord().Field("x", typ.Number).Build()
	if !IsSubtype(mutableInteger, mutableNumber) {
		t.Fatal("mutable integer field should widen to number")
	}

	dog := typ.LiteralString("dog")
	animal := typeexpr.Union(dog, typ.LiteralString("cat"))
	mutableDog := typetable.NewRecord().Field("pet", dog).Build()
	mutableAnimal := typetable.NewRecord().Field("pet", animal).Build()
	if IsSubtype(mutableDog, mutableAnimal) || IsSubtype(mutableAnimal, mutableDog) {
		t.Fatal("literal union tags in mutable fields should stay invariant")
	}

	readonlyAnimal := typetable.NewRecord().ReadonlyField("pet", animal).Build()
	if !IsSubtype(mutableDog, readonlyAnimal) {
		t.Fatal("mutable narrower field should satisfy readonly wider target")
	}
	if IsSubtype(readonlyAnimal, mutableAnimal) {
		t.Fatal("readonly field should not satisfy mutable target")
	}
}

func TestStaticIndexRecordSatisfiesContainerTargets(t *testing.T) {
	indexed := typetable.NewRecord().StaticIntIndex(1, typ.String).Build()
	if IsSubtype(indexed, typ.NewArray(typ.String)) {
		t.Fatal("exact integer slots should not become strict array subtypes")
	}
	if !IsFreshAssignable(indexed, typ.NewArray(typ.String)) {
		t.Fatal("fresh record with exact integer slot should assign to array target")
	}
	if IsFreshAssignable(indexed, typ.NewArray(typ.Number)) {
		t.Fatal("fresh record with exact integer slot must still check array element type")
	}

}

func TestFreshRecordExplicitNilAssignableToNilableField(t *testing.T) {
	status := typeexpr.Union(
		typ.LiteralString("queued"),
		typ.LiteralString("started"),
		typ.LiteralString("completed"),
		typ.LiteralString("failed"),
	)
	source := typetable.NewRecord().
		Field("error", typ.Nil).
		Field("status", typ.LiteralString("queued")).
		Build()
	target := typetable.NewRecord().
		Field("error", typeexpr.Optional(typ.String)).
		Field("status", status).
		Build()

	if !IsFreshAssignable(source, target) {
		t.Fatal("fresh record with explicit nil field should assign to nilable field and literal-union field")
	}
	if !IsFreshAssignable(source, typeexpr.Optional(target)) {
		t.Fatal("fresh record with explicit nil field should assign through optional record target")
	}

	projectionSource := typetable.NewRecord().
		Field("id", typ.LiteralString("queued-1")).
		Field("status", typ.LiteralString("queued")).
		Field("output", typ.Nil).
		Field("error_code", typ.Nil).
		Field("retryable", typ.Nil).
		Field("updated_at", typ.LiteralInt(1)).
		Build()
	projectionTarget := typetable.NewRecord().
		Field("id", typ.String).
		Field("status", status).
		Field("output", typeexpr.Optional(typ.String)).
		Field("error_code", typeexpr.Optional(typ.String)).
		Field("retryable", typeexpr.Optional(typ.Boolean)).
		Field("updated_at", typeexpr.Optional(typ.Number)).
		Build()
	if !IsFreshAssignable(projectionSource, projectionTarget) {
		t.Fatal("fresh record should allow explicit nils and literals for optional record-field contracts")
	}
}

func TestRecordStaticMembersAreChecked(t *testing.T) {
	sub := typetable.NewRecord().StaticStringIndex("id", typ.Integer).Build()
	super := typetable.NewRecord().StaticStringIndex("id", typ.Number).Build()
	if !IsSubtype(sub, super) {
		t.Fatal("static member type should be checked")
	}
	if IsSubtype(typetable.NewRecord().Build(), super) {
		t.Fatal("missing required static member should fail closed")
	}
}

func TestRecordReadableFieldAndStaticMemberShareOptionalReadonlyPolicy(t *testing.T) {
	readonlyStaticSource := typetable.NewRecord().
		AddStaticMember(typ.StaticMember{
			Kind:     typ.StaticMemberStringIndex,
			Name:     "id",
			Type:     typ.String,
			Readonly: true,
		}).
		Build()
	mutableFieldTarget := typetable.NewRecord().Field("id", typ.String).Build()
	if IsSubtype(readonlyStaticSource, mutableFieldTarget) {
		t.Fatal("readonly static string member must not satisfy mutable dot-field target")
	}

	readonlyFieldSource := typetable.NewRecord().ReadonlyField("id", typ.String).Build()
	mutableStaticTarget := typetable.NewRecord().StaticStringIndex("id", typ.String).Build()
	if IsSubtype(readonlyFieldSource, mutableStaticTarget) {
		t.Fatal("readonly dot field must not satisfy mutable static string target")
	}

	optionalStaticSource := typetable.NewRecord().
		AddStaticMember(typ.StaticMember{
			Kind:     typ.StaticMemberStringIndex,
			Name:     "id",
			Type:     typ.String,
			Optional: true,
		}).
		Build()
	if IsSubtype(optionalStaticSource, mutableFieldTarget) {
		t.Fatal("optional static string member must not satisfy required dot-field target")
	}

	optionalFieldSource := typetable.NewRecord().OptField("id", typ.String).Build()
	if IsSubtype(optionalFieldSource, mutableStaticTarget) {
		t.Fatal("optional dot field must not satisfy required static string target")
	}
}

func TestFreshAssignableRecordRequiresTargetFields(t *testing.T) {
	source := typetable.NewRecord().
		Field("elapsed", typ.Number).
		Build()
	target := typetable.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Number).
		Build()

	if IsFreshAssignable(source, target) {
		t.Fatal("fresh record widening must not ignore missing required target fields")
	}
}

func TestFreshAssignableRecordAllowsMissingOptionalTargetFields(t *testing.T) {
	source := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	target := typetable.NewRecord().
		Field("id", typ.String).
		OptField("note", typeexpr.Optional(typ.String)).
		Build()

	if !IsFreshAssignable(source, target) {
		t.Fatal("fresh record widening should allow absent optional target fields")
	}
}

func TestReadonlyMapViews(t *testing.T) {
	if !IsSubtype(typ.NewMap(typ.String, typ.Integer), typ.NewReadonlyMap(typ.String, typ.Number)) {
		t.Fatal("mutable map should satisfy covariant readonly view")
	}
	if IsSubtype(typ.NewReadonlyMap(typ.String, typ.Number), typ.NewMap(typ.String, typ.Number)) {
		t.Fatal("readonly map view must not grant mutable map capability")
	}

	rec := typetable.NewRecord().
		Field("id", typ.Integer).
		OptField("name", typeexpr.Optional(typ.String)).
		Build()
	view := typ.NewReadonlyMap(typ.String, typeexpr.Union(typ.Number, typ.String))
	if !IsSubtype(rec, view) {
		t.Fatal("record present entries should satisfy readonly map view")
	}
}

func TestMapAdaptersNormalizeNilableKeys(t *testing.T) {
	nilableString := typeexpr.Optional(typ.String)

	if !IsSubtype(typ.NewMap(nilableString, typ.Integer), typ.NewReadonlyMap(typ.String, typ.Number)) {
		t.Fatal("mutable map should normalize its key for readonly map view checks")
	}

	recordView := typetable.NewRecord().MapComponent(nilableString, typ.Integer).Build()
	if !IsSubtype(typ.NewMap(typ.String, typ.Integer), recordView) {
		t.Fatal("record map component should normalize its key for map-to-record checks")
	}
}

func TestMapToRecordRejectsMutableLiteralUnionFieldWidening(t *testing.T) {
	dog := typ.LiteralString("dog")
	cat := typ.LiteralString("cat")
	pet := typeexpr.Union(typ.Nil, dog, cat)

	recordView := typetable.NewRecord().
		Field("pet", pet).
		MapComponent(typ.String, dog).
		Build()

	if IsSubtype(typ.NewMap(typ.String, dog), recordView) {
		t.Fatal("mutable map-to-record check must reject optional literal-union field widening")
	}
}

func TestTableShapesAndMapViews(t *testing.T) {
	if !IsSubtype(typetable.NewRecord().Build(), typ.NewArray(typ.Number)) {
		t.Fatal("empty record should satisfy array shape")
	}
	if !IsSubtype(typ.NewTuple(typ.Integer, typ.Integer), typ.NewArray(typ.Number)) {
		t.Fatal("tuple elements should satisfy array element type")
	}
	if !IsSubtype(typ.NewArray(typ.String), typ.NewMap(typ.Integer, typ.String)) {
		t.Fatal("array should satisfy compatible integer-key map")
	}
	if IsSubtype(typ.NewMap(typ.String, typ.Any), typetable.NewRecord().Build()) {
		t.Fatal("map should not satisfy an empty record target")
	}
}

func TestFreshAssignableRecordLiteralAdoptsTableTopField(t *testing.T) {
	source := typetable.NewRecord().
		Field("model", typ.LiteralString("x")).
		Field("options", typetable.NewRecord().Build()).
		Build()
	target := typetable.NewRecord().
		Field("model", typ.String).
		Field("options", typetable.BuiltinTopMarker()).
		Build()

	if !IsFreshAssignable(source, target) {
		t.Fatal("fresh record literal should let nested empty table adopt table field contract")
	}
}

func TestFreshAssignableRecordLiteralAdoptsEmptyArrayField(t *testing.T) {
	contentPart := typetable.NewRecord().
		Field("type", typ.String).
		Field("text", typ.String).
		Build()
	source := typetable.NewRecord().
		Field("content", typetable.NewRecord().Build()).
		Build()
	target := typetable.NewRecord().
		Field("content", typ.NewArray(contentPart)).
		Build()

	if !IsFreshAssignable(source, target) {
		t.Fatal("fresh record literal should let nested empty table adopt array field contract")
	}
}

func TestFreshAssignableNestedGenericUnionPayloadArrayLiteral(t *testing.T) {
	renderPayload := typetable.NewRecord().
		Field("kind", typ.LiteralString("render")).
		Field("template", typ.String).
		Build()
	indexPayload := typetable.NewRecord().
		Field("kind", typ.LiteralString("index")).
		Field("terms", typ.NewArray(typ.String)).
		Build()
	payload := typeexpr.Union(renderPayload, indexPayload)
	param := typ.NewTypeParam("T", nil)
	envelope := typ.NewGeneric("Envelope", []*typ.TypeParam{param},
		typetable.NewRecord().Field("payload", param).Build(),
	)
	target := typetable.NewRecord().
		Field("kind", typ.LiteralString("dispatch")).
		Field("envelope", typ.Instantiate(envelope, payload)).
		Build()
	source := typetable.NewRecord().
		Field("kind", typ.LiteralString("dispatch")).
		Field("envelope", typetable.NewRecord().
			Field("payload", typetable.NewRecord().
				Field("kind", typ.LiteralString("index")).
				Field("terms", typ.NewTuple(typ.LiteralString("lua"), typ.LiteralString("types"), typ.LiteralString("cache"))).
				Build()).
			Build()).
		Build()

	if !IsFreshAssignable(source, target) {
		t.Fatalf("fresh nested payload literal should widen tuple terms to string[] through the selected union arm")
	}
}

func TestFreshAssignableNestedRecordStaticMembersToOptionalMapComponent(t *testing.T) {
	source := typetable.NewRecord().
		Field("enabled", typ.LiteralBool(true)).
		Field("description_suffix", typ.LiteralString(" (runs specialized agent in parallel)")).
		Field("default_schema", typetable.NewRecord().
			Field("type", typ.LiteralString("object")).
			Field("properties", typetable.NewRecord().
				Field("message", typetable.NewRecord().
					Field("type", typ.LiteralString("string")).
					Field("description", typ.LiteralString("The message to forward to the agent")).
					Build()).
				Build()).
			Field("required", typ.NewTuple(typ.LiteralString("message"))).
			Build()).
		Build()
	target := typetable.NewRecord().
		Field("enabled", typ.Boolean).
		Field("description_suffix", typ.String).
		Field("default_schema", typetable.NewRecord().
			Field("type", typ.String).
			OptField("properties", typeexpr.Optional(typ.NewMap(typ.String, typ.Any))).
			OptField("required", typeexpr.Optional(typ.Any)).
			Build()).
		Build()

	if !IsFreshAssignable(source, typeexpr.Optional(target)) {
		t.Fatal("fresh nested record with static members should satisfy optional map-component field contract")
	}
}

func TestBuiltinTableTopMarkerSubtyping(t *testing.T) {
	tableTop := typ.NewInterface("table", nil)

	accepted := []typ.Type{
		typetable.NewRecord().Field("x", typ.Number).Build(),
		typetable.NewMap(typ.String, typ.Number),
		typetable.NewReadonlyMap(typ.String, typ.Number),
		typ.NewArray(typ.String),
		typ.NewTuple(typ.String),
		typ.NewInterface("NamedTableLike", nil),
		typeexpr.Intersection(typetable.NewRecord().Build(), typ.NewInterface("NamedTableLike", nil)),
		typ.Any,
	}
	for _, sub := range accepted {
		if !IsSubtype(sub, tableTop) {
			t.Fatalf("%v should satisfy builtin table top marker", sub)
		}
	}

	rejected := []typ.Type{typ.String, typ.Number, typ.Unknown}
	for _, sub := range rejected {
		if IsSubtype(sub, tableTop) {
			t.Fatalf("%v should not satisfy builtin table top marker", sub)
		}
	}
}

func TestGenericConstraintsAndInstantiation(t *testing.T) {
	tp := typ.NewTypeParam("T", typ.Number)
	if !IsSubtype(tp, typ.Number) || !IsSubtype(typ.Integer, tp) {
		t.Fatal("type parameter constraint should be checked both directions")
	}

	param := typ.NewTypeParam("U", nil)
	body := typetable.NewRecord().Field("value", param).Build()
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, body)
	boxString := typ.Instantiate(box, typ.String)
	if !IsSubtype(boxString, typetable.NewRecord().Field("value", typ.String).Build()) {
		t.Fatal("instantiated generic should expand for structural comparison")
	}
	if IsSubtype(typ.Instantiate(box, typ.String), typ.Instantiate(box, typ.Number)) {
		t.Fatal("instantiated type arguments should be invariant")
	}
}
