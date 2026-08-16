package effectlowering

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/type/stringlib"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

func TestExactSameAsReturnArgumentUsesProviderTransformPrecedence(t *testing.T) {
	dependent := signature.Function{
		Type: typ.Func().Param("left", typ.Any).Param("right", typ.Any).Returns(typ.Any).Build(),
		Effect: effect.Empty.With(returns.Return{
			ReturnIndex: 0,
			Transform:   returns.SameAs{Source: effect.ParamRef{Index: -1}},
		}),
	}
	if got, exact := ExactSameAsReturnArgument(dependent, 0, 2); !exact || got != 1 {
		t.Fatalf("dependent return argument = %d/%t, want last argument 1", got, exact)
	}
	nonDependent := signature.Function{
		Type: typ.Func().Param("value", typ.Any).Returns(typ.Any).Build(),
		Effect: effect.Empty.With(returns.Return{
			ReturnIndex: 0,
			Transform:   returns.ElementOf{Source: effect.ParamRef{Index: 0}},
		}),
	}
	if got, exact := ExactSameAsReturnArgument(nonDependent, 0, 1); exact || got != 0 {
		t.Fatalf("non-dependent return argument = %d/%t, want rejected", got, exact)
	}
}

func TestStaticScalarSignatureReturnsAcceptsFormatRejectsBorrowingConcat(t *testing.T) {
	reg := standard.Registry()
	source := signaturelookup.Source{IncludeStdlib: true}
	format, _ := source.Lookup("string.format")
	got, ok := StaticScalarSignatureReturns(reg, nil, format)
	if !ok || len(got) != 1 {
		t.Fatalf("string.format static returns=%#v/%v", got, ok)
	}
	concat, _ := source.Lookup("table.concat")
	if got, ok := StaticScalarSignatureReturns(reg, nil, concat); ok || got != nil {
		t.Fatalf("table.concat borrowing effect compiled as pure: %#v", got)
	}
}

func TestStaticScalarStringMethodReturnsAcceptsOnlySiteExactFiniteScalarUnions(t *testing.T) {
	reg := standard.Registry()
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	pattern, _ := factflow.NewStringLiteralValueSource("^__", 0, 0, 0, shape)
	result := factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{})
	matchSite := factflow.NewCallSite(factflow.CallSiteConfig{MethodName: "match", ArgumentSources: []factflow.ValueSource{pattern}, ResultTargets: []factflow.CallResultTarget{result}}).View()
	base, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("string.match")
	baseHash, baseString := base.Type.Hash(), base.Type.String()
	baseReturns := append([]typ.Type(nil), base.Type.Returns...)
	refined, exact := RefineStaticStringMethodSignature(reg, base, matchSite)
	if !exact || refined.Equals(base) || len(refined.Type.Returns) != 1 || !typ.TypeEquals(refined.Type.Returns[0], typeexpr.Optional(typ.String)) {
		t.Fatalf("refined match signature = %#v/%v", refined, exact)
	}
	if got, ok := StaticScalarStringMethodReturns(reg, nil, base, matchSite); ok || got != nil {
		t.Fatalf("unrefined match signature admitted: %#v", got)
	}
	if got, ok := StaticScalarStringMethodReturns(reg, nil, refined, matchSite); !ok || len(got) != 1 {
		t.Fatalf("site-exact match returns=%#v/%v", got, ok)
	}
	if base.Type.Hash() != baseHash || base.Type.String() != baseString || !sameStaticSignatureTypes(base.Type.Returns, baseReturns) {
		t.Fatal("match refinement mutated base signature storage or cached identity")
	}
	expected := typ.RebuildFunction(typ.FunctionParts{
		TypeParams: base.Type.TypeParams,
		Params:     base.Type.Params,
		Variadic:   base.Type.Variadic,
		Returns:    stringlib.MatchReturnTypes("^__"),
	})
	if refined.Type.Hash() == baseHash || refined.Type.Hash() != expected.Hash() || !typ.TypeEquals(refined.Type, expected) {
		t.Fatalf("refined match identity = %d/%v, want independently rebuilt %d/%v", refined.Type.Hash(), refined.Type, expected.Hash(), expected)
	}

	plainSite := factflow.NewCallSite(factflow.CallSiteConfig{MethodName: "synthetic", ResultTargets: []factflow.CallResultTarget{result}}).View()
	plain := signature.Function{Type: typ.Func().Returns(typ.String, typ.Integer).Build()}
	if got, ok := StaticScalarStringMethodReturns(reg, nil, plain, plainSite); !ok || len(got) != 2 {
		t.Fatalf("ordinary static scalar method returns=%#v/%v", got, ok)
	}

	for name, returns := range map[string]typ.Type{
		"any":             typ.Any,
		"optional":        typeexpr.Optional(typ.String),
		"optional any":    typeexpr.Optional(typ.Any),
		"scalar union":    typeexpr.Union(typ.String, typ.Integer, typ.Nil),
		"composite union": typeexpr.Union(typ.String, typ.NewArray(typ.String)),
	} {
		t.Run(name, func(t *testing.T) {
			sig := signature.Function{Type: typ.Func().Returns(returns).Build()}
			if got, ok := StaticScalarStringMethodReturns(reg, nil, sig, plainSite); ok || got != nil {
				t.Fatalf("contextual union admitted: %#v", got)
			}
		})
	}

	capture, _ := factflow.NewStringLiteralValueSource("(__)", 0, 0, 0, shape)
	captureSite := factflow.NewCallSite(factflow.CallSiteConfig{MethodName: "match", ArgumentSources: []factflow.ValueSource{capture}, ResultTargets: []factflow.CallResultTarget{result}}).View()
	if _, ok := RefineStaticStringMethodSignature(reg, base, captureSite); ok {
		t.Fatal("capturing match pattern gained static signature")
	}
	nonzero := factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 1, 0, pathdom.Path{})
	nonzeroSite := factflow.NewCallSite(factflow.CallSiteConfig{MethodName: "match", ArgumentSources: []factflow.ValueSource{pattern}, ResultTargets: []factflow.CallResultTarget{nonzero}}).View()
	if _, ok := RefineStaticStringMethodSignature(reg, base, nonzeroSite); ok {
		t.Fatal("nonzero match result target gained static signature")
	}
}

func TestStaticFiniteScalarReturnTypeUsesProductiveRecursiveProof(t *testing.T) {
	scalar := typ.NewRecursivePlaceholder("Scalar")
	scalar.SetBody(&typ.Union{Members: []typ.Type{scalar, typ.String}})
	if !staticFiniteScalarReturnType(scalar) {
		t.Fatal("productive recursive scalar union was rejected")
	}

	composite := typ.NewRecursivePlaceholder("Composite")
	composite.SetBody(&typ.Union{Members: []typ.Type{composite, typ.NewArray(typ.String)}})
	if staticFiniteScalarReturnType(composite) {
		t.Fatal("productive recursive composite union was accepted")
	}

	loop := typ.NewRecursive("Loop", func(self typ.Type) typ.Type { return self })
	if staticFiniteScalarReturnType(loop) {
		t.Fatal("cycle-only type manufactured a scalar proof")
	}

	var deep typ.Type = typ.String
	for range 257 {
		deep = &typ.Alias{Name: "Deep", Target: deep}
	}
	if !staticFiniteScalarReturnType(deep) {
		t.Fatal("deep acyclic scalar graph was truncated")
	}
}

func sameStaticSignatureTypes(left, right []typ.Type) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !typ.TypeEquals(left[i], right[i]) {
			return false
		}
	}
	return true
}
