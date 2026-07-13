package program

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestStrictExternalMethodRouteRequiresStaticScalarSignature(t *testing.T) {
	reg := standard.Registry()
	result := factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{})
	tp := typ.NewTypeParam("T", nil)
	for name, sig := range map[string]signature.Function{
		"effectful": {Type: typ.Func().Returns(typ.String).Build(), OperationalEffects: &signature.OperationalEffects{MaySuspend: true}},
		"generic":   {Type: typ.Func().TypeParamRef(tp).Returns(tp).Build()},
	} {
		t.Run(name, func(t *testing.T) {
			op, ok := operationplan.NewSignatureCallOperation(sig)
			if !ok {
				t.Fatal("signature operation rejected before admission test")
			}
			target, ok := operationplan.NewExternalCallSurfaceTarget(op)
			if !ok {
				t.Fatal("external target rejected before admission test")
			}
			method := factflow.NewCallSite(factflow.CallSiteConfig{MethodName: "gsub", ResultTargets: []factflow.CallResultTarget{result}}).View()
			if strictExternalCallSurfaceExact(reg, method, target, op) {
				t.Fatal("contextual external method entered strict static route")
			}
			// Direct external calls retain their existing compiler-owned route;
			// this slice narrows only newly admitted method shape.
			direct := factflow.NewCallSite(factflow.CallSiteConfig{}).View()
			if !strictExternalCallSurfaceExact(reg, direct, target, op) {
				t.Fatal("direct external route was narrowed by method-only gate")
			}
		})
	}

	pure := signature.Function{Type: typ.Func().Returns(typ.String, typ.Integer).Build()}
	op, _ := operationplan.NewSignatureCallOperation(pure)
	target, _ := operationplan.NewExternalCallSurfaceTarget(op)
	method := factflow.NewCallSite(factflow.CallSiteConfig{MethodName: "gsub", ResultTargets: []factflow.CallResultTarget{result}}).View()
	if !strictExternalCallSurfaceExact(reg, method, target, op) {
		t.Fatal("static scalar external method was rejected")
	}
}

func TestStrictExternalMatchRouteRequiresSiteRefinedSignature(t *testing.T) {
	reg := standard.Registry()
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	pattern, _ := factflow.NewStringLiteralValueSource("^__", 0, 0, 0, shape)
	result := factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{})
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		MethodName: "match", ArgumentSources: []factflow.ValueSource{pattern},
		ResultTargets: []factflow.CallResultTarget{result},
	}).View()
	base, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("string.match")
	baseOperation, _ := operationplan.NewSignatureCallOperation(base)
	baseTarget, _ := operationplan.NewExternalCallSurfaceTarget(baseOperation)
	if strictExternalCallSurfaceExact(reg, site, baseTarget, baseOperation) {
		t.Fatal("base argument-sensitive match signature entered strict route")
	}
	refined, exact := effectlowering.RefineStaticStringMethodSignature(reg, base, site)
	if !exact {
		t.Fatal("literal no-capture match signature did not refine")
	}
	operation, _ := operationplan.NewSignatureCallOperation(refined)
	target, _ := operationplan.NewExternalCallSurfaceTarget(operation)
	if !strictExternalCallSurfaceExact(reg, site, target, operation) {
		t.Fatal("site-refined match signature was rejected")
	}
}
