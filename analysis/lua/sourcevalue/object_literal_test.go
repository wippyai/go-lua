package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

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

	got, ok := ObjectLiteralType(reg, lit, func(source factflow.ValueSource) (product.Value, bool) {
		value, ok := values[source]
		return value, ok
	})
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

	got, ok := ObjectLiteralType(reg, lit, func(factflow.ValueSource) (product.Value, bool) {
		return product.Value{}, false
	})
	if !ok {
		t.Fatal("objectLiteralType returned false")
	}
	if !typ.TypeEquals(got, expected) {
		t.Fatalf("object literal type = %v, want %v", got, expected)
	}

	fieldOnly := typetable.NewRecord().Field("member", typ.String).Build()
	if _, ok := expectedRecordField(true, fieldOnly, []segment.Segment{{Kind: segment.SegmentIndexString, Name: "member"}}); ok {
		t.Fatal("bracket-string entry unexpectedly adopted dot-field contextual type")
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

	got, ok := ObjectLiteralType(reg, lit, func(source factflow.ValueSource) (product.Value, bool) {
		value, ok := values[source]
		return value, ok
	})
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

func TestObjectLiteralTypeDoesNotUseTopOriginEntryAsStructuralProof(t *testing.T) {
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

			got, ok := ObjectLiteralType(reg, lit, func(source factflow.ValueSource) (product.Value, bool) {
				value, ok := values[source]
				return value, ok
			})
			if !ok {
				t.Fatal("objectLiteralType returned false")
			}

			want := typetable.NewRecord().Build()
			if !typ.TypeEquals(got, want) {
				t.Fatalf("object literal type = %v, want %v", got, want)
			}
		})
	}
}

func TestObjectLiteralTypeDoesNotAdoptExpectedFieldForTopOriginEntry(t *testing.T) {
	reg := standard.Registry()
	idSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(711), HasExpr: true}
	expected := typetable.NewRecord().Field("id", typ.String).Build()
	lit := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntry(path.NewPlaceholder(0).Field("id"), idSource),
	}).WithExpected(typevalue.WithWitness(reg, typevalue.FromType(reg, expected), expected))

	got, ok := ObjectLiteralType(reg, lit, func(source factflow.ValueSource) (product.Value, bool) {
		return product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop()), true
	})
	if !ok {
		t.Fatal("objectLiteralType returned false")
	}

	want := typetable.NewRecord().Build()
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

	got, ok := ObjectLiteralType(reg, lit, func(source factflow.ValueSource) (product.Value, bool) {
		if source == channelSource {
			return typevalue.WithWitness(reg, typevalue.FromType(reg, channelType), channelType), true
		}
		return product.Value{}, false
	})
	if !ok {
		t.Fatal("objectLiteralType returned false")
	}

	want := typetable.NewRecord().Field("channel", channelType).Build()
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
	pageWitness := typetable.NewRecord().Field("route", typ.String).Build()
	values := map[factflow.ValueSource]product.Value{
		pageSource:  typevalue.WithWitness(reg, typevalue.FromType(reg, pageType), pageType),
		idSource:    product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop()),
		routeSource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
	}

	got, ok := ObjectLiteralType(reg, lit, func(source factflow.ValueSource) (product.Value, bool) {
		value, ok := values[source]
		return value, ok
	})
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

	got, ok := ObjectLiteralType(reg, lit, func(source factflow.ValueSource) (product.Value, bool) {
		value, ok := values[source]
		return value, ok
	})
	if !ok {
		t.Fatal("objectLiteralType returned false")
	}

	want := typ.NewTuple(typ.Number, typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("object literal sequence type = %v, want %v", got, want)
	}
}
