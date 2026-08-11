package static

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func TestStaticContentIDTypesFieldLedger(t *testing.T) {
	cases := []contentDelta{
		{"primitive.kind", contentTypesComponent, func(c *Component) { c.types.primitive[0].Kind = PrimitiveString }},
		{"literal.kind", contentTypesComponent, func(c *Component) { c.types.literal[0].Kind = keyspace.LiteralInteger }},
		{"literal.exact", contentTypesComponent, func(c *Component) { c.types.literal[0].Exact = 77 }},
		{"literal.float-bits", contentTypesComponent, func(c *Component) { c.types.literal[0].FloatBits = 3 }},
		{"optional.inner", contentTypesComponent, func(c *Component) { c.types.optional[0].Inner = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) }},
		{"union.member", contentTypesComponent, func(c *Component) {
			c.types.terms[c.types.union[0].Start] = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3)
		}},
		{"union.arity", contentTypesComponent, func(c *Component) { c.types.union[0].End = c.types.union[0].Start }},
		{"union.order", contentTypesComponent, swapUnionMembers},
		{"intersection.member", contentTypesComponent, func(c *Component) {
			c.types.terms[c.types.intersection[0].Start] = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"intersection.arity", contentTypesComponent, func(c *Component) { c.types.intersection[0].End = c.types.intersection[0].Start }},
		{"intersection.order", contentTypesComponent, swapIntersectionMembers},
		{"generic.base", contentTypesComponent, func(c *Component) { c.types.generic[0].base = keyspace.MakeTerm(keyspace.FamilyTypeRef, 2) }},
		{"generic.arg", contentTypesComponent, func(c *Component) {
			c.types.terms[c.types.generic[0].args.Start] = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"generic.arity", contentTypesComponent, func(c *Component) { c.types.generic[0].args.End = c.types.generic[0].args.Start }},
		{"array.element", contentTypesComponent, func(c *Component) { c.types.array[0].Element = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) }},
		{"array.read-only", contentTypesComponent, func(c *Component) { c.types.array[0].ReadOnly = !c.types.array[0].ReadOnly }},
		{"map.key", contentTypesComponent, func(c *Component) { c.types.mapType[0].Key = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) }},
		{"map.value", contentTypesComponent, func(c *Component) { c.types.mapType[0].Value = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) }},
		{"map.read-only", contentTypesComponent, func(c *Component) { c.types.mapType[0].ReadOnly = !c.types.mapType[0].ReadOnly }},
		{"record.field", contentTypesComponent, func(c *Component) {
			c.types.fields[c.types.record[0].fields.Start] = keyspace.MakeTerm(keyspace.FamilyTypeField, 2)
		}},
		{"record.arity", contentTypesComponent, func(c *Component) { c.types.record[0].fields.End = c.types.record[0].fields.Start }},
		{"record.read-only", contentTypesComponent, func(c *Component) { c.types.record[0].readOnly = !c.types.record[0].readOnly }},
		{"field.key", contentTypesComponent, func(c *Component) { c.types.field[0].Key = 88 }},
		{"field.type", contentTypesComponent, func(c *Component) { c.types.field[0].Type = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) }},
		{"field.optional", contentTypesComponent, func(c *Component) { c.types.field[0].Optional = !c.types.field[0].Optional }},
	}
	runContentDeltaLedger(t, cases)
}

func contentTypesComponent(t *testing.T) *Component {
	t.Helper()
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 2)
	if !ok {
		t.Fatal("CoordinateFromParts rejected content fixture")
	}
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 5
	counts[keyspace.FamilyTypeLiteral] = 1
	counts[keyspace.FamilyTypeOptional] = 1
	counts[keyspace.FamilyTypeUnion] = 1
	counts[keyspace.FamilyTypeIntersection] = 1
	counts[keyspace.FamilyTypeRef] = 2
	counts[keyspace.FamilyTypeGeneric] = 1
	counts[keyspace.FamilyTypeArray] = 1
	counts[keyspace.FamilyTypeMap] = 1
	counts[keyspace.FamilyTypeRecord] = 1
	counts[keyspace.FamilyTypeField] = 1
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyCell] = 1
	primitive := func(ordinal uint32) keyspace.Term { return keyspace.MakeTerm(keyspace.FamilyTypePrimitive, ordinal) }
	input := Input{Counts: counts,
		Types: TypesInput{
			Primitive:    []Primitive{{Kind: PrimitiveNil}, {Kind: PrimitiveNumber}, {Kind: PrimitiveString}, {Kind: PrimitiveBoolean}, {Kind: PrimitiveNever}},
			Literal:      []Literal{{Kind: keyspace.LiteralString, Exact: 7}},
			Optional:     []Optional{{Inner: primitive(1)}},
			Union:        []Union{{Members: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1), primitive(2)}}},
			Intersection: []Intersection{{Members: []keyspace.Term{primitive(3), primitive(4)}}},
			Generic:      []Generic{{Base: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1), Args: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeUnion, 1)}}},
			Array:        []Array{{Element: keyspace.MakeTerm(keyspace.FamilyTypeGeneric, 1), ReadOnly: true}},
			Map:          []Map{{Key: keyspace.MakeTerm(keyspace.FamilyTypeRef, 2), Value: keyspace.MakeTerm(keyspace.FamilyTypeArray, 1)}},
			Field:        []Field{{Key: 9, Type: keyspace.MakeTerm(keyspace.FamilyTypeMap, 1), Optional: true}},
			Record:       []Record{{Fields: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeField, 1)}, ReadOnly: true}},
		},
		References: ReferencesInput{TypeRef: []TypeRef{
			{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)},
			{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{2, 3}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{4}},
		}},
		Declarations: DeclarationsInput{Alias: []TypeAlias{{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: primitive(5), Name: 10, NameCoordinate: coordinate}}},
	}
	return staticContentComponent(t, input)
}

func swapUnionMembers(component *Component) {
	range_ := component.types.union[0]
	component.types.terms[range_.Start], component.types.terms[range_.Start+1] = component.types.terms[range_.Start+1], component.types.terms[range_.Start]
}

func swapIntersectionMembers(component *Component) {
	range_ := component.types.intersection[0]
	component.types.terms[range_.Start], component.types.terms[range_.Start+1] = component.types.terms[range_.Start+1], component.types.terms[range_.Start]
}
