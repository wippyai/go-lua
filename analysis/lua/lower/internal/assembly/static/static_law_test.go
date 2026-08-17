package static_test

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	assembly "github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

func staticViews(t *testing.T, c *assembly.Collector) (programsource.View, programstatic.View) {
	t.Helper()
	published, err := c.Publish()
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return published.Source(), published.Static()
}

func staticFixture(name string) (*assembly.Collector, keyspace.Term) {
	c := assembly.New(name, 0, bind.GlobalCensus{})
	body := c.Body(programsource.Span{File: name})
	return c, body
}

func completeStatic(t *testing.T, c *assembly.Collector, body keyspace.Term, roots ...keyspace.Term) (programsource.View, programstatic.View) {
	t.Helper()
	if body == 0 || !c.SetBody(body, roots...) || !c.SetEntry(body) {
		t.Fatal("Source completion failed")
	}
	return staticViews(t, c)
}

func TestStaticFreezeResolvesOnlyThroughSourcePreimage(t *testing.T) {
	c, body := staticFixture("static-law.lua")
	span := programsource.Span{File: "static-law.lua"}
	primitive := c.Primitive(span, programstatic.PrimitiveString)
	literal := c.LiteralString(span, "literal")
	field := c.Field(span, "field", primitive, false)
	record := c.Record(span, []keyspace.Term{field}, false)
	union := c.Union(span, []keyspace.Term{literal, record})
	declarationSpan := programsource.Span{File: "static-law.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	alias := c.Alias(declarationSpan, declarationSpan, body, "Root")
	if primitive == 0 || literal == 0 || field == 0 || record == 0 || union == 0 || alias == 0 ||
		!c.AliasParams(alias, nil) || !c.AliasTarget(alias, union) {
		t.Fatal("static construction failed")
	}
	sourceView, staticView := completeStatic(t, c, body, alias)
	if key, ok := sourceView.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "literal"}); !ok || key == 0 {
		t.Fatal("Static literal did not resolve through Source exact preimage")
	}
	if staticView.Types().Primitives().Count() != 1 || staticView.Types().Literals().Count() != 1 ||
		staticView.Types().Fields().Count() != 1 || staticView.Types().Records().Count() != 1 {
		t.Fatalf("static counts = primitive=%d literal=%d field=%d record=%d",
			staticView.Types().Primitives().Count(), staticView.Types().Literals().Count(),
			staticView.Types().Fields().Count(), staticView.Types().Records().Count())
	}
}

func TestStaticFreezeRejectsMissingPayloadAndNaN(t *testing.T) {
	c, body := staticFixture("static-invalid.lua")
	span := programsource.Span{File: "static-invalid.lua"}
	if got := c.LiteralFloatBits(span, math.Float64bits(math.NaN())); got != 0 {
		t.Fatalf("NaN literal admitted as %v", got)
	}
	if c.Body(span) != 0 {
		t.Fatal("invalid static payload did not terminalize construction")
	}
	if published, err := c.Publish(); err == nil || published != nil {
		t.Fatalf("Publish after invalid static payload = %#v/%v", published, err)
	}
	_ = body
}

func TestStaticRowsFillsAreOneShotAndClaimsCanonical(t *testing.T) {
	const name = "static-fills.lua"
	span := programsource.Span{File: name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	t.Run("canonical rows", func(t *testing.T) {
		c, body := staticFixture(name)
		alias := c.Alias(span, span, body, "Alias")
		param := c.TypeParam(span, alias, "T")
		primitive := c.Primitive(span, programstatic.PrimitiveString)
		if alias == 0 || param == 0 || primitive == 0 || !c.TypeParamConstraint(param, 0) ||
			!c.AliasParams(alias, []keyspace.Term{param}) || !c.AliasTarget(alias, primitive) {
			t.Fatal("Alias construction failed")
		}
		_, staticView := completeStatic(t, c, body, alias)
		if staticView.Declarations().Aliases().Count() != 1 || staticView.Declarations().TypeParams().Count() != 1 {
			t.Fatal("canonical declaration rows were not published")
		}
	})
	t.Run("duplicate fill terminalizes", func(t *testing.T) {
		c, body := staticFixture(name)
		alias := c.Alias(span, span, body, "Alias")
		param := c.TypeParam(span, alias, "T")
		if alias == 0 || param == 0 || !c.TypeParamConstraint(param, 0) || !c.AliasParams(alias, []keyspace.Term{param}) {
			t.Fatal("Alias setup failed")
		}
		if c.AliasParams(alias, []keyspace.Term{param}) {
			t.Fatal("duplicate Alias parameter fill was accepted")
		}
		if published, err := c.Publish(); err == nil || published != nil {
			t.Fatalf("terminal duplicate published: %#v/%v", published, err)
		}
	})
}

func TestStaticClaimStateMachineSeparatesOneShotAndFill(t *testing.T) {
	const name = "static-claims.lua"
	span := programsource.Span{File: name}
	t.Run("publishes filled claim", func(t *testing.T) {
		c, body := staticFixture(name)
		operand := c.Bool(span, body, true)
		target := c.Primitive(span, programstatic.PrimitiveString)
		claim := c.DeclareValueClaim(span, body, kind.ValueClaimTypeColonColon, operand)
		values := c.Values(span, body, []keyspace.Term{claim}, 0)
		ret := c.Return(span, body, values)
		if operand == 0 || target == 0 || claim == 0 || values == 0 || ret == 0 || !c.FillValueClaimTarget(claim, target) {
			t.Fatal("ValueClaim declaration/fill failed")
		}
		_, staticView := completeStatic(t, c, body, ret)
		if staticView.Operands().Claims().Count() != 1 {
			t.Fatalf("Static claim count = %d, want one", staticView.Operands().Claims().Count())
		}
	})
	t.Run("duplicate fill terminalizes", func(t *testing.T) {
		c, body := staticFixture(name)
		operand := c.Bool(span, body, true)
		target := c.Primitive(span, programstatic.PrimitiveString)
		claim := c.DeclareValueClaim(span, body, kind.ValueClaimTypeColonColon, operand)
		if claim == 0 || !c.FillValueClaimTarget(claim, target) {
			t.Fatal("ValueClaim setup failed")
		}
		if c.FillValueClaimTarget(claim, target) {
			t.Fatal("duplicate ValueClaim fill was accepted")
		}
		if published, err := c.Publish(); err == nil || published != nil {
			t.Fatalf("terminal duplicate published: %#v/%v", published, err)
		}
	})
}

func TestStaticDeclarationAndAssertionKeepSeparateCoordinates(t *testing.T) {
	c, body := staticFixture("static-coordinates.lua")
	span := programsource.Span{File: "static-coordinates.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	alias := c.Alias(span, span, body, "Alias")
	primitive := c.Primitive(span, programstatic.PrimitiveString)
	assertion := c.TypeAsserts(span, span, "T", false, 0, primitive)
	if alias == 0 || primitive == 0 || assertion == 0 || !c.AliasParams(alias, nil) || !c.AliasTarget(alias, assertion) {
		t.Fatal("Alias/assertion setup failed")
	}
	_, staticView := completeStatic(t, c, body, alias)
	if staticView.Declarations().Aliases().Count() != 1 || staticView.Signatures().Assertions().Count() != 1 {
		t.Fatal("declaration/assertion rows lost their separate coordinates")
	}
}

func TestStaticPublicationDuplicateIsDelegatedToStaticBuild(t *testing.T) {
	c, body := staticFixture("static-publication.lua")
	span := programsource.Span{File: "static-publication.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	cell := c.Cell(span, body)
	value := c.Bool(span, body, true)
	values := c.Values(span, body, []keyspace.Term{value}, 0)
	assign := c.Assign(span, body, []keyspace.Term{cell}, []programsource.Span{span}, values)
	primitive := c.Primitive(span, programstatic.PrimitiveString)
	alias := c.Alias(span, span, body, "Alias")
	ref := c.Declaration(span, []string{"Alias"}, body, alias)
	if cell == 0 || value == 0 || values == 0 || assign == 0 || primitive == 0 || alias == 0 ||
		!c.AliasParams(alias, nil) || !c.AliasTarget(alias, primitive) || ref == 0 {
		t.Fatal("publication setup failed")
	}
	if first := c.Type(span, assign, 0, ref); first == 0 {
		t.Fatal("first publication was rejected")
	} else if second := c.Type(span, assign, 0, ref); second != 0 {
		t.Fatalf("duplicate publication = %v, want rejection", second)
	}
}

func TestStaticClaimDeclarationRequiresTargetBeforeFreeze(t *testing.T) {
	c, body := staticFixture("static-claim-target.lua")
	span := programsource.Span{File: "static-claim-target.lua"}
	operand := c.Bool(span, body, true)
	claim := c.DeclareValueClaim(span, body, kind.ValueClaimTypeColonColon, operand)
	if operand == 0 || claim == 0 {
		t.Fatal("claim declaration failed")
	}
	if c.FillValueClaimTarget(claim, 0) {
		t.Fatal("zero claim target was accepted")
	}
	if published, err := c.Publish(); err == nil || published != nil {
		t.Fatalf("Publish accepted unfilled claim target: %#v/%v", published, err)
	}
}

func TestCollectorStaticRolesRejectWrongFamiliesAndKeepTypeOfOperandOpen(t *testing.T) {
	const name = "collector-static-role-admission.lua"
	span := programsource.Span{File: name}
	setup := func() (*assembly.Collector, keyspace.Term, keyspace.Term, keyspace.Term, keyspace.Term) {
		c := assembly.New(name, 0, bind.GlobalCensus{})
		body := c.Body(span)
		primitive := c.Primitive(span, programstatic.PrimitiveString)
		stringTerm := c.String(span, body, "operand")
		cell := c.Cell(span, body)
		return c, body, primitive, stringTerm, cell
	}
	c, _, _, stringTerm, _ := setup()
	if got := c.Optional(span, stringTerm); got != 0 {
		t.Fatalf("Optional accepted non-Node String %v", got)
	}
	c, _, primitive, _, _ := setup()
	if got := c.Declaration(span, []string{"Alias"}, 0, primitive); got != 0 {
		t.Fatalf("TypeRef Declaration accepted Primitive target %v", got)
	}
	c, _, primitive, _, _ = setup()
	if got := c.TypeParam(span, primitive, "T"); got != 0 {
		t.Fatalf("TypeParam accepted Primitive owner %v", got)
	}
	c, _, _, stringTerm, cell := setup()
	if got := c.Annotation(span, cell, stringTerm, "note"); got != 0 {
		t.Fatalf("Annotation accepted String target %v", got)
	}
	c, _, primitive, stringTerm, _ = setup()
	if got := c.TypeOf(span, primitive, stringTerm); got != 0 {
		t.Fatalf("TypeOf accepted Primitive scope %v", got)
	}
	c, _, _, stringTerm, cell = setup()
	if got := c.TypeOf(span, cell, stringTerm); got == 0 {
		t.Fatal("TypeOf rejected current non-Import operand")
	}
	reserved := assembly.New(name, 1, bind.GlobalCensus{})
	reservedBody := reserved.Body(span)
	reservedCell := reserved.Cell(span, reservedBody)
	if got := reserved.TypeOf(span, reservedCell, keyspace.MakeTerm(keyspace.FamilyImport, 1)); got != 0 {
		t.Fatalf("TypeOf accepted reserved Import operand %v", got)
	}
}

func TestStaticConstructionSurfacePublishesStaticRows(t *testing.T) {
	c := assembly.New("static-api.lua", 0, bind.GlobalCensus{})
	span := programsource.Span{File: "static-api.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	body := c.Body(span)
	if body == 0 {
		t.Fatal("Source Body construction failed")
	}
	primitive := c.Primitive(span, programstatic.PrimitiveString)
	if primitive == 0 || keyspace.TermFamily(primitive) != keyspace.FamilyTypePrimitive {
		t.Fatalf("Static Primitive = %v, want TypePrimitive", primitive)
	}
	literal := c.LiteralString(span, "label")
	if literal == 0 || keyspace.TermFamily(literal) != keyspace.FamilyTypeLiteral {
		t.Fatalf("Static LiteralString = %v, want TypeLiteral", literal)
	}
	float := c.LiteralFloat(span, 1.25)
	if float == 0 || keyspace.TermFamily(float) != keyspace.FamilyTypeLiteral {
		t.Fatalf("Static LiteralFloat = %v, want TypeLiteral", float)
	}
	ref := c.Unresolved(span, []string{"Type"}, 0)
	if ref == 0 || keyspace.TermFamily(ref) != keyspace.FamilyTypeRef {
		t.Fatalf("Static TypeRef = %v, want TypeRef", ref)
	}
	union := c.Union(span, []keyspace.Term{primitive, literal, float, ref})
	alias := c.Alias(span, span, body, "A")
	if union == 0 || alias == 0 || !c.AliasParams(alias, nil) || !c.AliasTarget(alias, union) {
		t.Fatalf("Static alias construction failed: union=%v alias=%v", union, alias)
	}
	if !c.SetBody(body, alias) || !c.SetEntry(body) {
		t.Fatal("Source order construction failed")
	}
	published, err := c.Publish()
	if err != nil {
		t.Fatalf("Publish after public Static construction: %v", err)
	}
	if published == nil {
		t.Fatal("Publish returned no Program")
	}
	keys := published.Source().Keys()
	wantExact := []keyspace.LiteralValue{
		{Kind: keyspace.LiteralString, String: "A"},
		{Kind: keyspace.LiteralString, String: "label"},
		{Kind: keyspace.LiteralString, String: "Type"},
	}
	if keys.ExactCount() != len(wantExact) {
		t.Fatalf("Static exact Source count = %d, want %d", keys.ExactCount(), len(wantExact))
	}
	for _, want := range wantExact {
		if key, ok := keys.Find(want); !ok || key == 0 {
			t.Fatalf("Static exact Source admission omitted %#v", want)
		}
	}
	if got := published.Static().Declarations().Aliases().Count(); got != 1 {
		t.Fatalf("Static alias count = %d, want one", got)
	}
}

func TestStaticConstructionRejectsFutureChildOrdinal(t *testing.T) {
	c := assembly.New("static-api-future.lua", 0, bind.GlobalCensus{})
	span := programsource.Span{File: "static-api-future.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	_ = c.Body(span)
	future := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if got := c.Optional(span, future); got != 0 {
		t.Fatalf("Optional accepted future child term %v", got)
	}
	if got := c.Primitive(span, programstatic.PrimitiveString); got != 0 {
		t.Fatalf("collector mutated after future-child rejection with %v", got)
	}
	if _, err := c.Publish(); err == nil {
		t.Fatal("future-child rejection did not retain a terminal cause")
	}
}
