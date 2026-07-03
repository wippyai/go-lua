package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestObjectLiteralEntryTypeUsesRuntimeKindProof(t *testing.T) {
	reg := standard.Registry()
	value := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	got, ok := ObjectLiteralEntryType(reg, typevalue.NewCache(), value)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ObjectLiteralEntryType = %v/%v, want string from runtime-kind proof", got, ok)
	}
}

func TestObjectLiteralTypeSeparatesDotFieldAndBracketStringMember(t *testing.T) {
	reg := standard.Registry()
	fieldSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1101), HasExpr: true}
	indexSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1102), HasExpr: true}
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("id"), fieldSource),
		factflow.NewObjectEntry(path.NewPlaceholder(0).IndexStr("id"), indexSource),
	})

	values := map[factflow.ValueSource]product.Value{
		fieldSource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
		indexSource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Boolean), typ.Boolean),
	}

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		value, ok := values[source]
		return value, ok
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}

	want := typetable.NewRecord().
		Field("id", typ.String).
		StaticStringIndex("id", typ.Boolean).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal type = %v, want %v", got, want)
	}
}

func TestObjectLiteralTypeDoesNotAdoptExpectedInterfaceWithRequiredMethodsWhenEmpty(t *testing.T) {
	reg := standard.Registry()
	reader := typ.NewInterface("Reader", []typ.Method{{
		Name: "read",
		Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build(),
	}})
	lit := factflow.NewObjectLiteral(nil).
		WithExpected(typevalue.WithWitness(reg, typevalue.FromType(reg, reader), reader))

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(factflow.ValueSource) (product.Value, bool) {
		return product.Value{}, false
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}
	want := typetable.NewRecord().Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal type = %v, want empty record instead of adopting unsatisfied interface", got)
	}
}

func TestObjectLiteralTypeStillAdoptsExpectedMapWhenEmpty(t *testing.T) {
	reg := standard.Registry()
	expected := typ.NewMap(typ.String, typ.Number)
	lit := factflow.NewObjectLiteral(nil).
		WithExpected(typevalue.WithWitness(reg, typevalue.FromType(reg, expected), expected))

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(factflow.ValueSource) (product.Value, bool) {
		return product.Value{}, false
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}
	if !typ.TypeEquals(got, expected) {
		t.Fatalf("object literal type = %v, want expected map", got)
	}
}

func TestObjectLiteralTypeUsesEntryExpectedContractWhenNestedSourceUnresolved(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1201), HasExpr: true}
	entry := factflow.NewObjectEntry(path.NewPlaceholder(0).Field("error").Field("type"), source).
		WithExpected(typeValues.FromTypeWithWitness(reg, typ.String))
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{entry})

	got, ok := ObjectLiteralTypeCached(reg, typeValues, lit, factflow.ValueSourceResolverFunc(func(factflow.ValueSource) (product.Value, bool) {
		return product.Value{}, false
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}
	want := typetable.NewRecord().
		Field("error", typetable.NewRecord().Field("type", typ.String).Build()).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal type = %v, want %v", got, want)
	}
}

func TestObjectLiteralTypeBracketStringDiscriminantDoesNotSelectDotFieldUnionArm(t *testing.T) {
	reg := standard.Registry()
	kindSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1301), HasExpr: true}
	start := typetable.NewRecord().
		Field("kind", typ.LiteralString("start")).
		Field("payload", typ.String).
		Build()
	stop := typetable.NewRecord().
		Field("kind", typ.LiteralString("stop")).
		Field("code", typ.Integer).
		Build()
	expected := typeexpr.Union(start, stop)
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).IndexStr("kind"), kindSource),
	}).WithExpected(typevalue.WithWitness(reg, typevalue.FromType(reg, expected), expected))

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		if source != kindSource {
			return product.Value{}, false
		}
		valueType := typ.LiteralString("stop")
		return typevalue.WithWitness(reg, typevalue.FromType(reg, valueType), valueType), true
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}

	want := typetable.NewRecord().
		StaticStringIndex("kind", typ.LiteralString("stop")).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal type = %v, want static string member record %v", got, want)
	}
	if typ.TypeEquals(got, stop) {
		t.Fatalf("bracket-string kind selected dot-field stop union arm: %v", got)
	}
}

func TestObjectLiteralTypeResolvesEachEntryOnceWhenSelectingExpectedUnionArm(t *testing.T) {
	reg := standard.Registry()
	kindSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1311), HasExpr: true}
	payloadSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1312), HasExpr: true}
	start := typetable.NewRecord().
		Field("kind", typ.LiteralString("start")).
		Field("payload", typ.String).
		Build()
	stop := typetable.NewRecord().
		Field("kind", typ.LiteralString("stop")).
		Field("payload", typ.String).
		Build()
	expected := typeexpr.Union(start, stop)
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("kind"), kindSource),
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("payload"), payloadSource),
	}).WithExpected(typevalue.WithWitness(reg, typevalue.FromType(reg, expected), expected))
	values := map[factflow.ValueSource]product.Value{
		kindSource:    typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("stop")), typ.LiteralString("stop")),
		payloadSource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("body")), typ.LiteralString("body")),
	}
	counts := map[factflow.ValueSource]int{}

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		counts[source]++
		value, ok := values[source]
		return value, ok
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}
	if !typ.TypeEquals(got, stop) {
		t.Fatalf("object literal type = %v, want selected stop record %v", got, stop)
	}
	if counts[kindSource] != 1 {
		t.Fatalf("kind source resolved %d times, want 1", counts[kindSource])
	}
	if counts[payloadSource] != 1 {
		t.Fatalf("payload source resolved %d times, want 1", counts[payloadSource])
	}
}

func TestObjectLiteralTypeSelectedUnionArmOverridesBroadEntryExpectedFallback(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	okSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1321), HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1322), HasExpr: true}
	success := typetable.NewRecord().
		Field("ok", typ.True).
		Field("value", typ.String).
		Build()
	failure := typetable.NewRecord().
		Field("ok", typ.False).
		Field("error", typ.String).
		Build()
	expected := typeexpr.Union(success, failure)
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("ok"), okSource).
			WithExpected(typeValues.FromTypeWithWitness(reg, typeexpr.Union(typ.False, typ.True))),
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("value"), valueSource).
			WithExpected(typeValues.FromTypeWithWitness(reg, typeexpr.Optional(typ.String))),
	}).WithExpected(typeValues.FromTypeWithWitness(reg, expected))

	got, ok := ObjectLiteralTypeCached(reg, typeValues, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		if source == okSource {
			return typeValues.FromTypeWithWitness(reg, typ.True), true
		}
		return product.Value{}, false
	}))

	if !ok || !typ.TypeEquals(got, success) {
		t.Fatalf("object literal type = %v/%v, want selected success arm %v", got, ok, success)
	}
}

func TestObjectLiteralTypeSelectedUnionArmKeepsNestedEntryExpectedFallback(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	okSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1331), HasExpr: true}
	messageSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1332), HasExpr: true}
	errorType := typetable.NewRecord().
		Field("code", typ.String).
		Field("message", typ.String).
		Build()
	success := typetable.NewRecord().
		Field("ok", typ.True).
		Field("value", typ.String).
		Build()
	failure := typetable.NewRecord().
		Field("ok", typ.False).
		Field("error", errorType).
		Build()
	expected := typeexpr.Union(success, failure)
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("ok"), okSource),
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("error").Field("message"), messageSource).
			WithExpected(typeValues.FromTypeWithWitness(reg, typ.String)),
	}).WithExpected(typeValues.FromTypeWithWitness(reg, expected))

	got, ok := ObjectLiteralTypeCached(reg, typeValues, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		if source == okSource {
			return typeValues.FromTypeWithWitness(reg, typ.False), true
		}
		return product.Value{}, false
	}))

	want := typetable.NewRecord().
		Field("ok", typ.False).
		Field("error", typetable.NewRecord().Field("message", typ.String).Build()).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("object literal type = %v/%v, want nested error.message from entry expected %v", got, ok, want)
	}
}

func TestObjectLiteralTypeAdoptsExpectedRecordFieldAndStaticStringMemberBySegmentKind(t *testing.T) {
	reg := standard.Registry()
	fieldSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1201), HasExpr: true}
	indexSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1202), HasExpr: true}
	expected := typetable.NewRecord().
		Field("field", typ.String).
		StaticStringIndex("member", typ.Boolean).
		Build()
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("field"), fieldSource),
		factflow.NewObjectEntry(path.NewPlaceholder(0).IndexStr("member"), indexSource),
	}).WithExpected(typevalue.WithWitness(reg, typevalue.FromType(reg, expected), expected))

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(factflow.ValueSource) (product.Value, bool) {
		return product.Value{}, false
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}
	if !typ.TypeEquals(got, expected) {
		t.Fatalf("object literal type = %v, want %v", got, expected)
	}
}

func TestObjectLiteralTypePreservesNestedWitnessShape(t *testing.T) {
	reg := standard.Registry()
	payloadParam := typ.NewTypeParam("T", nil)

	payloadType := typetable.NewRecord().
		Field("kind", typ.String).
		Field("payload", typ.Any).
		Build()
	handlerType := typ.Func().
		TypeParamRef(payloadParam).
		Param("value", typ.Unknown).
		Returns(payloadParam, typ.String).
		Build()

	payloadSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(401), HasExpr: true}
	handlerSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(402), HasExpr: true}
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("traits").Field("payload"), payloadSource),
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("traits").Field("handler"), handlerSource),
	})

	values := map[factflow.ValueSource]product.Value{
		payloadSource: typevalue.WithWitness(reg, typevalue.FromType(reg, payloadType), payloadType),
		handlerSource: typevalue.WithWitness(reg, typevalue.FromType(reg, handlerType), handlerType),
	}

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		value, ok := values[source]
		return value, ok
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}

	want := typetable.NewRecord().
		Field("traits", typetable.NewRecord().
			Field("handler", handlerType).
			Field("payload", payloadType).
			Build()).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal type = %v, want %v", got, want)
	}
}

func TestObjectLiteralTypePreservesTopOriginEntryPresenceWithoutConcreteProof(t *testing.T) {
	for _, tt := range []struct {
		name     string
		evidence evidence.Value
	}{
		{name: "explicit top", evidence: evidence.ExplicitTop()},
		{name: "gradual top", evidence: evidence.GradualTop()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reg := standard.Registry()
			idSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(701), HasExpr: true}
			lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
				factflow.NewObjectEntry(path.NewPlaceholder(0).Field("id"), idSource),
			})
			values := map[factflow.ValueSource]product.Value{
				idSource: product.Set(reg, product.Top(), evidence.Key, tt.evidence),
			}

			got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
				value, ok := values[source]
				return value, ok
			}))
			if !ok {
				t.Fatal("objectLiteralType returned false")
			}

			want := typetable.NewRecord().Field("id", typ.Unknown).Build()
			if !typ.TypeEquals(got, want) {
				t.Fatalf("object literal type = %v, want %v", got, want)
			}
		})
	}
}

func TestObjectLiteralTypePreservesResolvedUntypedEntryPresence(t *testing.T) {
	reg := standard.Registry()
	contentSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(721), HasExpr: true}
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("content"), contentSource),
	})

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		if source == contentSource {
			return product.NewWithPresence(reg, product.ShapeTop, presence.Present()), true
		}
		return product.Value{}, false
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}

	want := typetable.NewRecord().Field("content", typ.Unknown).Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal type = %v, want %v", got, want)
	}
}

func TestObjectLiteralTypeDoesNotAdoptExpectedFieldForTopOriginEntry(t *testing.T) {
	reg := standard.Registry()
	idSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(711), HasExpr: true}
	expected := typetable.NewRecord().Field("id", typ.String).Build()
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("id"), idSource),
	}).WithExpected(typevalue.WithWitness(reg, typevalue.FromType(reg, expected), expected))

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		return product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop()), true
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}

	want := typetable.NewRecord().Field("id", typ.Unknown).Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal type = %v, want %v", got, want)
	}
}

func TestObjectLiteralTypeKeepsProvenSiblingWhenNestedEntryUnresolved(t *testing.T) {
	reg := standard.Registry()
	channelGeneric, ok := ambient.Lookup(ambient.Channel)
	if !ok {
		t.Fatal("ambient Channel not found")
	}
	channelType := typ.Instantiate(channelGeneric.(*typ.Generic), typ.String)
	channelSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(901), HasExpr: true}
	witnessSource := factflow.ValueSource{Kind: factflow.ValueSourceCall, ExprRef: factflow.ExprRef(902), HasExpr: true}
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("channel"), channelSource),
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("schema").Field("witness"), witnessSource),
	})

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		if source == channelSource {
			return typevalue.WithWitness(reg, typevalue.FromType(reg, channelType), channelType), true
		}
		return product.Value{}, false
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}

	want := typetable.NewRecord().
		Field("channel", channelType).
		Field("schema", typetable.NewRecord().Field("witness", typ.Unknown).Build()).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal type = %v, want %v", got, want)
	}
}

func TestObjectLiteralTypeBuildsNestedSequenceWithTopOriginField(t *testing.T) {
	reg := standard.Registry()
	pageSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(801), HasExpr: true}
	idSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(802), HasExpr: true}
	routeSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(803), HasExpr: true}
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).IndexInt(1), pageSource),
		factflow.NewObjectEntry(path.NewPlaceholder(0).IndexInt(1).Field("id"), idSource),
		factflow.NewObjectEntry(path.NewPlaceholder(0).IndexInt(1).Field("route"), routeSource),
	})

	pageType := typetable.NewRecord().Field("id", typ.Any).Field("route", typ.String).Build()
	pageWitness := typetable.NewRecord().Field("id", typ.Unknown).Field("route", typ.String).Build()
	values := map[factflow.ValueSource]product.Value{
		pageSource:  typevalue.WithWitness(reg, typevalue.FromType(reg, pageType), pageType),
		idSource:    product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop()),
		routeSource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
	}

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		value, ok := values[source]
		return value, ok
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}

	want := typ.NewTuple(pageWitness)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal type = %v, want %v", got, want)
	}
}

func TestObjectLiteralTypePreservesPureSequenceAsTuple(t *testing.T) {
	reg := standard.Registry()
	firstSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(501), HasExpr: true}
	secondSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(502), HasExpr: true}
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).IndexInt(1), firstSource),
		factflow.NewObjectEntry(path.NewPlaceholder(0).IndexInt(2), secondSource),
	})

	values := map[factflow.ValueSource]product.Value{
		firstSource:  typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number),
		secondSource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
	}

	got, ok := ObjectLiteralType(reg, lit, factflow.ValueSourceResolverFunc(func(source factflow.ValueSource) (product.Value, bool) {
		value, ok := values[source]
		return value, ok
	}))

	if !ok {
		t.Fatal("objectLiteralType returned false")
	}

	want := typ.NewTuple(typ.Number, typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal sequence type = %v, want %v", got, want)
	}
}
