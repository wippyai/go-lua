package collector_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/lower/internal/collector"
	"github.com/wippyai/go-lua/program/source"
	programstatic "github.com/wippyai/go-lua/program/static"
)

// This is intentionally an external-package law: a lowerer can use only the
// owner views and public construction DTOs. It cannot name staticRows,
// provisional keys, or the generic Collector mint operation.
func TestStaticConstructionSurfaceIsUsableExternally(t *testing.T) {
	var c *collector.Collector = collector.New("static-api.lua", 0, bind.GlobalCensus{})
	span := source.Span{File: "static-api.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	body := c.Source().Order().Body(span)
	if body == 0 {
		t.Fatal("Source Body construction failed")
	}
	primitive := c.Static().Types().Primitive(span, programstatic.PrimitiveString)
	if primitive == 0 || keyspace.TermFamily(primitive) != keyspace.FamilyTypePrimitive {
		t.Fatalf("Static Primitive = %v, want TypePrimitive", primitive)
	}
	literal := c.Static().Types().LiteralString(span, "label")
	if literal == 0 || keyspace.TermFamily(literal) != keyspace.FamilyTypeLiteral {
		t.Fatalf("Static LiteralString = %v, want TypeLiteral", literal)
	}
	float := c.Static().Types().LiteralFloat(span, 1.25)
	if float == 0 || keyspace.TermFamily(float) != keyspace.FamilyTypeLiteral {
		t.Fatalf("Static LiteralFloat = %v, want TypeLiteral", float)
	}
	ref := c.Static().References().Unresolved(span, []string{"Type"}, 0)
	if ref == 0 || keyspace.TermFamily(ref) != keyspace.FamilyTypeRef {
		t.Fatalf("Static TypeRef = %v, want TypeRef", ref)
	}
	union := c.Static().Types().Union(span, []keyspace.Term{primitive, literal, float, ref})
	alias := c.Static().Declarations().Alias(span, span, body, "A")
	if union == 0 || alias == 0 || !c.Static().Declarations().AliasParams(alias, nil) || !c.Static().Declarations().AliasTarget(alias, union) {
		t.Fatalf("Static alias construction failed: union=%v alias=%v", union, alias)
	}
	if !c.Source().Order().SetBody(body, alias) || !c.Source().Order().SetEntry(body) {
		t.Fatal("Source order construction failed")
	}
	prepared, err := c.Prepare()
	if err != nil {
		t.Fatalf("Prepare after public Static construction: %v", err)
	}
	assembly, err := prepared.Assemble()
	if err != nil || assembly == nil {
		t.Fatalf("Prepared.Assemble = %v/%v", assembly, err)
	}
	sourceComponent, _, _, _, err := assembly.Take()
	if err != nil || sourceComponent == nil {
		t.Fatalf("Assembly.Take = %v/%v", sourceComponent, err)
	}
	wantExact := []keyspace.LiteralValue{
		{Kind: keyspace.LiteralString, String: "A"},
		{Kind: keyspace.LiteralString, String: "label"},
		{Kind: keyspace.LiteralString, String: "Type"},
	}
	keys := sourceComponent.View().Keys()
	if keys.ExactCount() != len(wantExact) {
		t.Fatalf("Static exact Source count = %d, want %d", keys.ExactCount(), len(wantExact))
	}
	for _, want := range wantExact {
		if key, ok := keys.Find(want); !ok || key == 0 {
			t.Fatalf("Static exact Source admission omitted %#v", want)
		}
	}
}

func TestStaticConstructionRejectsFutureChildOrdinal(t *testing.T) {
	c := collector.New("static-api-future.lua", 0, bind.GlobalCensus{})
	span := source.Span{File: "static-api-future.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	_ = c.Source().Order().Body(span)
	future := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if got := c.Static().Types().Optional(span, future); got != 0 {
		t.Fatalf("Optional accepted future child term %v", got)
	}
	if got := c.Static().Types().Primitive(span, programstatic.PrimitiveString); got != 0 {
		t.Fatalf("collector mutated after future-child rejection with %v", got)
	}
	if _, err := c.Prepare(); err == nil {
		t.Fatal("future-child rejection did not retain a terminal cause")
	}
}
