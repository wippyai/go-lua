package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerAssignmentClaimsUseWIRClaimSourcesForRootWrites(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(x: any)
    local local_cast = x as number
    local local_assert = x!
    local reassigned: any = nil
    reassigned = x as string
end
`)
	body := wirlower.LowerFunction("f", fn, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	localCastPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	localCastSource := mustLocalSource(t, facts, localCastPoint)
	assertWIRConcreteCastAssertion(t, facts, localCastSource, typ.Number, factflow.ValueSourcePath)

	localAssertPoint := requireStmtPoints(t, built, fn.Stmts[1], 1)[0]
	localAssertSource := mustLocalSource(t, facts, localAssertPoint)
	assertWIRAssertion(t, facts, localAssertSource, assertion.NonNil(), factflow.ValueSourcePath)

	reassignPoint := requireStmtPoints(t, built, fn.Stmts[3], 1)[0]
	reassignFact, ok := facts.OrdinaryAssignment(reassignPoint)
	if !ok {
		t.Fatalf("missing ordinary assignment at point %d", reassignPoint)
	}
	assertWIRConcreteCastAssertion(t, facts, reassignFact.Source(), typ.String, factflow.ValueSourcePath)
}

func TestLowerDeclaredAnnotationClaimProducesRuntimeValidationRefinement(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(): ()
    local x: number
    local y: number = 5
end
`)
	body := wirlower.LowerFunction("f", fn, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	noInitPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	noInitSource := mustLocalSource(t, facts, noInitPoint)
	assertWIRConcreteCastAssertion(t, facts, noInitSource, typ.Number, factflow.ValueSourceNil)
	noInitLocal, ok := facts.LocalAssignment(noInitPoint)
	if !ok {
		t.Fatalf("missing local assignment at point %d", noInitPoint)
	}
	if _, ok := noInitLocal.DeclaredAnnotationValue(); !ok {
		t.Fatalf("no-initializer declared local missing DeclaredAnnotationValue")
	}

	// A literal-initialized declared local keeps its literal source precision
	// (TestLowerAnnotatedScalarLiteralAssignmentKeepsLiteralSource): the
	// annotation still reaches DeclaredAnnotationValue, but the source is not
	// wrapped in an ExpressionRefinement since the literal is already more
	// precise than the bare declared type.
	literalPoint := requireStmtPoints(t, built, fn.Stmts[1], 1)[0]
	literalLocal, ok := facts.LocalAssignment(literalPoint)
	if !ok {
		t.Fatalf("missing local assignment at point %d", literalPoint)
	}
	literalSource := literalLocal.Source()
	if literalSource.HasExpr {
		t.Fatalf("literal-initialized declared local source = %#v, want unwrapped literal source", literalSource)
	}
	if literalSource.Kind != factflow.ValueSourceLiteral || literalSource.LiteralKind != factflow.ValueSourceLiteralInteger || literalSource.Int != 5 {
		t.Fatalf("literal-initialized declared local source = %#v, want integer literal 5", literalSource)
	}
	if _, ok := literalLocal.DeclaredAnnotationValue(); !ok {
		t.Fatalf("literal-initialized declared local missing DeclaredAnnotationValue")
	}
}

func TestLowerReturnClaimsUseWIRClaimSources(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(x: any): (number, any)
    return x as number, x!
end
`)
	body := wirlower.LowerFunction("f", fn, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	returnPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	ret, ok := facts.Return(returnPoint)
	if !ok {
		t.Fatalf("missing return at point %d", returnPoint)
	}
	sources := ret.Sources()
	if len(sources) != 2 {
		t.Fatalf("return sources = %#v, want two", sources)
	}
	if sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("cast return source = %#v, want WIR claim expression source", sources[0])
	}
	if sources[1].Kind != factflow.ValueSourceExpression || !sources[1].HasExpr {
		t.Fatalf("assert return source = %#v, want WIR claim expression source", sources[1])
	}
	xPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "x")
	assertWIRClaimSourcePath(t, facts, sources[0], xPath)
	assertWIRClaimSourcePath(t, facts, sources[1], xPath)
	assertWIRConcreteCastAssertion(t, facts, sources[0], typ.Number, factflow.ValueSourcePath)
	assertWIRAssertion(t, facts, sources[1], assertion.NonNil(), factflow.ValueSourcePath)
}

func TestLowerReturnClaimsUseWIRClaimSourcesWithoutSemanticResult(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(x: any): (number, any)
    return x as number, x!
end
`)
	body := wirlower.LowerFunction("f-no-return-sidecars", fn, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	returnPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	ret, ok := facts.Return(returnPoint)
	if !ok {
		t.Fatalf("missing return at point %d without source return metadata", returnPoint)
	}
	sources := ret.Sources()
	if len(sources) != 2 {
		t.Fatalf("return sources = %#v, want two", sources)
	}
	if sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("cast return source = %#v, want WIR claim expression source", sources[0])
	}
	if sources[1].Kind != factflow.ValueSourceExpression || !sources[1].HasExpr {
		t.Fatalf("assert return source = %#v, want WIR claim expression source", sources[1])
	}
	xPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "x")
	assertWIRClaimSourcePath(t, facts, sources[0], xPath)
	assertWIRClaimSourcePath(t, facts, sources[1], xPath)
	assertWIRConcreteCastAssertion(t, facts, sources[0], typ.Number, factflow.ValueSourcePath)
	assertWIRAssertion(t, facts, sources[1], assertion.NonNil(), factflow.ValueSourcePath)
}

func TestLowerCallArgumentClaimsUseWIRClaimSources(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(x: any): ()
    sink(x as number, x!)
end
`, "sink")
	body := wirlower.LowerFunction("f", fn, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	callPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	site, ok := facts.CallSiteView(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := callSiteArgumentSources(site)
	if len(args) != 2 {
		t.Fatalf("call argument sources = %#v, want two", args)
	}
	if args[0].Kind != factflow.ValueSourceExpression || !args[0].HasExpr {
		t.Fatalf("cast argument source = %#v, want WIR claim expression source", args[0])
	}
	if args[1].Kind != factflow.ValueSourceExpression || !args[1].HasExpr {
		t.Fatalf("assert argument source = %#v, want WIR claim expression source", args[1])
	}
	xPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "x")
	assertWIRClaimSourcePath(t, facts, args[0], xPath)
	assertWIRClaimSourcePath(t, facts, args[1], xPath)
	assertWIRConcreteCastAssertion(t, facts, args[0], typ.Number, factflow.ValueSourcePath)
	assertWIRAssertion(t, facts, args[1], assertion.NonNil(), factflow.ValueSourcePath)
}

func TestLowerCallArgumentClaimsUseWIRClaimSourcesWithoutSemanticResult(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(x: any): ()
    sink(x as number, x!)
end
`, "sink")
	body := wirlower.LowerFunction("f-no-sidecars", fn, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	callPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	site, ok := facts.CallSiteView(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d without cfgbuild CallView", callPoint)
	}
	args := callSiteArgumentSources(site)
	if len(args) != 2 {
		t.Fatalf("call argument sources = %#v, want two", args)
	}
	if args[0].Kind != factflow.ValueSourceExpression || !args[0].HasExpr {
		t.Fatalf("cast argument source = %#v, want WIR claim expression source", args[0])
	}
	if args[1].Kind != factflow.ValueSourceExpression || !args[1].HasExpr {
		t.Fatalf("assert argument source = %#v, want WIR claim expression source", args[1])
	}
	xPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "x")
	assertWIRClaimSourcePath(t, facts, args[0], xPath)
	assertWIRClaimSourcePath(t, facts, args[1], xPath)
	assertWIRConcreteCastAssertion(t, facts, args[0], typ.Number, factflow.ValueSourcePath)
	assertWIRAssertion(t, facts, args[1], assertion.NonNil(), factflow.ValueSourcePath)
}

func TestLowerCallArgumentWIRClaimUsesConfiguredTypeResolver(t *testing.T) {
	messageType := typetable.NewRecord().
		Field("topic", typ.String).
		Build()
	fn, bindings, built := parseSemanticFunction(t, `
function f(raw: any): ()
    payload_data(raw as process.Message)
end
`, "payload_data")
	resolver := typeresolve.NewWithExternal(bindings, testExternalTypes{"process.Message": messageType})
	body := wirlower.LowerFunctionWithResolver("f", fn, bindings, built, resolver)
	facts := LowerDetailed(built.Graph, Config{
		Registry:     standard.Registry(),
		TypeResolver: resolver,
		WIR:          body,
	}).Facts

	callPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	site, ok := facts.CallSiteView(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := callSiteArgumentSources(site)
	if len(args) != 1 {
		t.Fatalf("call argument sources = %#v, want one", args)
	}
	if args[0].Kind != factflow.ValueSourceExpression || !args[0].HasExpr {
		t.Fatalf("cast argument source = %#v, want expression source for validated cast", args[0])
	}
	assertWIRConcreteCastAssertion(t, facts, args[0], messageType, factflow.ValueSourcePath)
}

func TestLowerCallArgumentWIRClaimKeepsDisjointRuntimeValidationExpressionSource(t *testing.T) {
	recordType := typetable.NewRecord().
		Field("name", typ.String).
		Build()
	fn, bindings, built := parseSemanticFunction(t, `
function f(y: number): ()
    sink(y as {name: string})
end
`, "sink")
	body := wirlower.LowerFunction("f", fn, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	callPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	site, ok := facts.CallSiteView(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := callSiteArgumentSources(site)
	if len(args) != 1 {
		t.Fatalf("call argument sources = %#v, want one", args)
	}
	if args[0].Kind != factflow.ValueSourceExpression || !args[0].HasExpr {
		t.Fatalf("cast argument source = %#v, want expression source for validated cast", args[0])
	}
	yPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "y")
	assertWIRClaimSourcePath(t, facts, args[0], yPath)
	assertWIRConcreteCastAssertion(t, facts, args[0], recordType, factflow.ValueSourcePath)
}

func TestLowerCallArgumentWIRClaimTypeBeatsSemanticCastType(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(x: any): ()
    local sink = function(value: any) end
    sink(x as string)
end
`)
	callStmt, ok := fn.Stmts[1].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[1])
	}
	callPoint := requireStmtPoints(t, built, callStmt, 1)[0]
	xPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "x")
	sinkPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "sink")

	body := wir.NewBody("synthetic-claim-type-owner")
	stampSyntheticWIRPathSymbols(t, body, bindings, xPath, sinkPath)
	claimTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:    wir.OpClaim,
		Point: callPoint,
		Dst:   claimTemp,
		A:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(xPath))},
		Claim: wir.ClaimCast,
		Type:  body.InternType(typ.Number),
	})
	body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: callPoint,
		Call:  wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(sinkPath))}},
		List:  body.AppendOperands([]wir.Operand{claimTemp}),
	})
	body.SetPointRange(callPoint, start, body.Len())

	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	site, ok := facts.CallSiteView(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := callSiteArgumentSources(site)
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceExpression || !args[0].HasExpr {
		t.Fatalf("call argument sources = %#v, want WIR claim expression source", args)
	}
	assertWIRClaimSourcePath(t, facts, args[0], xPath)
	assertWIRConcreteCastAssertion(t, facts, args[0], typ.Number, factflow.ValueSourcePath)
	if got := expressionRefinementCount(facts); got != 1 {
		t.Fatalf("expression refinements = %d, want only the WIR claim source refinement", got)
	}
}

func TestLowerCallArgumentWIRClaimSegmentedInnerPathComesFromWIR(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(): ()
    local sink = function(value: any) end
    local value = { name = "x" }
    local other = { name = "y" }
    sink(value.name as string)
end
`)
	callStmt, ok := fn.Stmts[3].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[3])
	}
	callPoint := requireStmtPoints(t, built, callStmt, 1)[0]
	sinkPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "sink")
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "value").Field("name")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[2].(*ast.LocalAssignStmt), 0), "other").Field("name")

	body := wir.NewBody("synthetic-segmented-claim-inner")
	stampSyntheticWIRPathSymbols(t, body, bindings, otherPath, sinkPath)
	claimTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:    wir.OpClaim,
		Point: callPoint,
		Dst:   claimTemp,
		A:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
		Claim: wir.ClaimCast,
		Type:  body.InternType(typ.Number),
	})
	body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: callPoint,
		Call:  wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(sinkPath))}},
		List:  body.AppendOperands([]wir.Operand{claimTemp}),
	})
	body.SetPointRange(callPoint, start, body.Len())

	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	site, ok := facts.CallSiteView(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := callSiteArgumentSources(site)
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceExpression || !args[0].HasExpr {
		t.Fatalf("call argument sources = %#v, want WIR claim expression source", args)
	}
	assertWIRClaimSourcePath(t, facts, args[0], otherPath)
	claim, ok := facts.ExpressionRefinement(args[0].ExprRef)
	if !ok {
		t.Fatalf("missing assertion for source ref %d", args[0].ExprRef)
	}
	if got := claim.Mode(); got != factflow.ExpressionRefinementRuntimeValidation {
		t.Fatalf("refinement mode = %v, want %v", got, factflow.ExpressionRefinementRuntimeValidation)
	}
	assertConcreteCastRefinementProduct(t, claim.Refinement(), typ.Number)
	inner := claim.Source()
	if inner.Kind != factflow.ValueSourceExpression || !inner.HasExpr {
		t.Fatalf("WIR claim inner source = %#v, want expression-backed WIR path", inner)
	}
	gotPath, ok := facts.ExpressionPath(inner.ExprRef)
	if !ok || !gotPath.Equal(otherPath) || gotPath.Equal(valuePath) {
		t.Fatalf("WIR claim inner source path = %v/%v, want WIR path %v not semantic path %v", gotPath, ok, otherPath, valuePath)
	}
}

func TestLowerCallArgumentWIRClaimRootLocalInnerStaysPathSource(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(): ()
    local sink = function(value: any) end
    local suites = {}
    sink(suites as any)
end
`)
	callStmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	callPoint := requireStmtPoints(t, built, callStmt, 1)[0]
	sinkPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "sink")
	suitesPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "suites")

	body := wir.NewBody("synthetic-root-local-claim-inner")
	stampSyntheticWIRPathSymbols(t, body, bindings, suitesPath, sinkPath)
	claimTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:    wir.OpClaim,
		Point: callPoint,
		Dst:   claimTemp,
		A:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(suitesPath))},
		Claim: wir.ClaimCast,
		Type:  body.InternType(typ.Any),
	})
	body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: callPoint,
		Call:  wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(sinkPath))}},
		List:  body.AppendOperands([]wir.Operand{claimTemp}),
	})
	body.SetPointRange(callPoint, start, body.Len())

	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	site, ok := facts.CallSiteView(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := callSiteArgumentSources(site)
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceExpression || !args[0].HasExpr {
		t.Fatalf("call argument sources = %#v, want WIR claim expression source", args)
	}
	assertWIRClaimSourcePath(t, facts, args[0], suitesPath)
	claim, ok := facts.ExpressionRefinement(args[0].ExprRef)
	if !ok {
		t.Fatalf("missing assertion for source ref %d", args[0].ExprRef)
	}
	inner := claim.Source()
	if inner.Kind != factflow.ValueSourcePath || inner.PathKey != suitesPath.Key() || inner.HasExpr {
		t.Fatalf("WIR claim inner source = %#v, want root local path source %s", inner, suitesPath.Key())
	}
}

func TestLowerClaimWrappedCallBindingsUseWIRClaims(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
type Message = {topic: string}
local inbox = make() as Message
local ready = check()!
`, "make", "check")
	body := wirlower.Lower("chunk", stmts, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	castLocal := stmts[1].(*ast.LocalAssignStmt)
	castPoints := requireStmtPoints(t, built, castLocal, 2)
	castSource := mustLocalSource(t, facts, castPoints[1])
	if castSource.Kind != factflow.ValueSourceCall || !castSource.HasExpr || !castSource.HasCallPoint || castSource.CallPoint != castPoints[0] {
		t.Fatalf("cast-wrapped call source = %#v, want call source at point %d", castSource, castPoints[0])
	}
	castClaim, ok := facts.ExpressionRefinement(castSource.ExprRef)
	if !ok {
		t.Fatalf("missing WIR cast refinement for source ref %d", castSource.ExprRef)
	}
	if got := castClaim.Mode(); got != factflow.ExpressionRefinementRuntimeValidation {
		t.Fatalf("cast refinement mode = %v, want runtime validation", got)
	}
	if got := product.Get(standard.Registry(), castClaim.Refinement(), assertion.Key); !got.Has(assertion.RuntimeClaim) || !got.Has(assertion.TypeClaim) {
		t.Fatalf("cast assertion = %s, want runtime type claim", got)
	}
	if inner := castClaim.Source(); inner.Kind != factflow.ValueSourceCall || inner.HasExpr || inner.ExprRef != 0 || inner.CallPoint != castPoints[0] {
		t.Fatalf("cast refinement source = %#v, want WIR inner call source at point %d", inner, castPoints[0])
	}

	assertLocal := stmts[2].(*ast.LocalAssignStmt)
	assertPoints := requireStmtPoints(t, built, assertLocal, 2)
	assertSource := mustLocalSource(t, facts, assertPoints[1])
	assertClaim, ok := facts.ExpressionRefinement(assertSource.ExprRef)
	if !ok {
		t.Fatalf("missing WIR non-nil refinement for source ref %d", assertSource.ExprRef)
	}
	if got := product.Get(standard.Registry(), assertClaim.Refinement(), assertion.Key); !got.Has(assertion.NonNilClaim) {
		t.Fatalf("non-nil assertion = %s, want non-nil claim", got)
	}
	if inner := assertClaim.Source(); inner.Kind != factflow.ValueSourceCall || inner.HasExpr || inner.ExprRef != 0 || inner.CallPoint != assertPoints[0] {
		t.Fatalf("non-nil refinement source = %#v, want WIR inner call source at point %d", inner, assertPoints[0])
	}
}

func assertWIRClaimSourcePath(t *testing.T, facts factflow.Facts, source factflow.ValueSource, want path.Path) {
	t.Helper()
	if !source.HasExpr || source.ExprRef == 0 {
		t.Fatalf("source = %#v, want expression source", source)
	}
	got, ok := facts.ExpressionPath(source.ExprRef)
	if !ok || !got.Equal(want) {
		t.Fatalf("WIR claim source ref %d path = %v/%v, want %v", source.ExprRef, got, ok, want)
	}
	refinement, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("WIR claim source ref %d has no refinement", source.ExprRef)
	}
	owned, ok := refinement.ResultPath()
	if !ok || !owned.Equal(want) {
		t.Fatalf("WIR claim result path = %v/%v, want owned %v", owned, ok, want)
	}
}

func assertWIRAssertion(t *testing.T, facts factflow.Facts, source factflow.ValueSource, want assertion.Value, wantInnerKind factflow.ValueSourceKind) {
	t.Helper()
	claim, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing assertion for source ref %d", source.ExprRef)
	}
	assertClaimRefinementProduct(t, claim.Refinement(), want)
	inner := claim.Source()
	if inner.ExprRef != 0 || inner.HasExpr || inner.Kind != wantInnerKind {
		t.Fatalf("WIR assertion inner source = %#v, outer %#v", inner, source)
	}
}

func assertWIRConcreteCastAssertion(t *testing.T, facts factflow.Facts, source factflow.ValueSource, want typ.Type, wantInnerKind factflow.ValueSourceKind) {
	t.Helper()
	claim, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing assertion for source ref %d", source.ExprRef)
	}
	if got := claim.Mode(); got != factflow.ExpressionRefinementRuntimeValidation {
		t.Fatalf("refinement mode = %v, want %v", got, factflow.ExpressionRefinementRuntimeValidation)
	}
	assertConcreteCastRefinementProduct(t, claim.Refinement(), want)
	inner := claim.Source()
	if inner.ExprRef != 0 || inner.HasExpr || inner.Kind != wantInnerKind {
		t.Fatalf("WIR assertion inner source = %#v, outer %#v", inner, source)
	}
}

func TestLowerClaimsToSidecarsWithoutProofRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	typeRead := ident("x")
	anyRead := ident("x")
	nonNilRead := ident("x")
	typeCast := &ast.CastExpr{Expr: typeRead, Type: primitiveType("number"), Syntax: ast.CastSyntaxAs}
	anyCast := &ast.CastExpr{Expr: anyRead, Type: primitiveType("any"), Syntax: ast.CastSyntaxColonColon}
	nonNil := &ast.NonNilAssertExpr{Expr: nonNilRead}
	local := localAssign([]string{"a", "b", "c"}, typeCast, anyCast, nonNil)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	facts := lowerChunkFactsWithWIR(t, "claim-sidecars", stmts, built, bindings, standard.Registry())
	points := requireStmtPoints(t, built, local, 3)
	typeSource := mustLocalSource(t, facts, points[0])
	anySource := mustLocalSource(t, facts, points[1])
	nonNilSource := mustLocalSource(t, facts, points[2])

	assertWIRConcreteCastAssertion(t, facts, typeSource, typ.Number, factflow.ValueSourcePath)
	assertWIRAssertion(t, facts, anySource, assertion.Any(), factflow.ValueSourcePath)
	assertWIRAssertion(t, facts, nonNilSource, assertion.NonNil(), factflow.ValueSourcePath)
	if len(facts.BranchRefinements(points[2])) != 0 {
		t.Fatalf("x! assignment produced branch/presence refinement")
	}
}

func TestLowerClaimsPreserveCastSyntaxVariantsWithoutProofRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	asTypeRead := ident("x")
	colonTypeRead := ident("x")
	asAnyRead := ident("x")
	colonAnyRead := ident("x")
	asTypeCast := &ast.CastExpr{Expr: asTypeRead, Type: primitiveType("number"), Syntax: ast.CastSyntaxAs}
	colonTypeCast := &ast.CastExpr{Expr: colonTypeRead, Type: primitiveType("number"), Syntax: ast.CastSyntaxColonColon}
	asAnyCast := &ast.CastExpr{Expr: asAnyRead, Type: primitiveType("any"), Syntax: ast.CastSyntaxAs}
	colonAnyCast := &ast.CastExpr{Expr: colonAnyRead, Type: primitiveType("any"), Syntax: ast.CastSyntaxColonColon}
	local := localAssign([]string{"a", "b", "c", "d"}, asTypeCast, colonTypeCast, asAnyCast, colonAnyCast)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	facts := lowerChunkFactsWithWIR(t, "claim-syntax-variants", stmts, built, bindings, standard.Registry())
	points := requireStmtPoints(t, built, local, 4)
	cases := []struct {
		name  string
		point cfg.Point
		want  assertion.Value
		typ   typ.Type
	}{
		{name: "as type", point: points[0], want: assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim), typ: typ.Number},
		{name: "colon type", point: points[1], want: assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim), typ: typ.Number},
		{name: "as any", point: points[2], want: assertion.Any()},
		{name: "colon any", point: points[3], want: assertion.Any()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := mustLocalSource(t, facts, tc.point)
			if tc.typ != nil {
				assertWIRConcreteCastAssertion(t, facts, source, tc.typ, factflow.ValueSourcePath)
				refinement, ok := facts.ExpressionRefinement(source.ExprRef)
				if !ok {
					t.Fatalf("missing assertion for source ref %d", source.ExprRef)
				}
				if got := refinement.Mode(); got != factflow.ExpressionRefinementRuntimeValidation {
					t.Fatalf("refinement mode = %v, want %v", got, factflow.ExpressionRefinementRuntimeValidation)
				}
			} else {
				assertWIRAssertion(t, facts, source, tc.want, factflow.ValueSourcePath)
			}
		})
	}
}

func TestLowerParsedCastClaimsOnlyProduceClaimRefinements(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local x = 0
local a, b, c, d = x as number, x :: number, x as any, x :: any
`)

	facts := lowerChunkFactsWithWIR(t, "parsed-cast-claims", stmts, built, bindings, standard.Registry())
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	local := mustLocalStmt(t, stmts, 1)
	points := requireStmtPoints(t, built, local, 4)
	cases := []struct {
		name  string
		point cfg.Point
		want  assertion.Value
		typ   typ.Type
	}{
		{name: "as number", point: points[0], want: assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim), typ: typ.Number},
		{name: "colon number", point: points[1], want: assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim), typ: typ.Number},
		{name: "as any", point: points[2], want: assertion.Any()},
		{name: "colon any", point: points[3], want: assertion.Any()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := mustLocalSource(t, facts, tc.point)
			if tc.typ != nil {
				assertWIRConcreteCastAssertion(t, facts, source, tc.typ, factflow.ValueSourcePath)
			} else {
				assertWIRAssertion(t, facts, source, tc.want, factflow.ValueSourcePath)
			}
		})
	}
	for _, point := range built.Graph.RPO() {
		if len(facts.BranchRefinements(point)) != 0 {
			t.Fatalf("parsed source cast emitted branch refinement at point %d", point)
		}
	}
}

func TestLowerStructuralCastClaimIsRuntimeValidation(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
type Payload = { id: string }
local raw = {}
local payload = raw :: Payload
`)

	reg := standard.Registry()
	facts := lowerChunkFactsWithWIR(t, "structural-cast-claim", stmts, built, bindings, reg)
	local := mustLocalStmt(t, stmts, 2)
	source := mustLocalSource(t, facts, requireStmtPoints(t, built, local, 1)[0])
	claim, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing structural cast assertion for source ref %d", source.ExprRef)
	}
	got := product.Get(reg, claim.Refinement(), assertion.Key)
	want := assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim)
	if !assertion.Equal(got, want) {
		t.Fatalf("structural cast assertion = %s, want %s", got, want)
	}
	witness := product.Get(reg, claim.Refinement(), typewitness.Key)
	gotType, ok := witness.Type()
	if !ok || gotType.Kind() != kind.Record {
		t.Fatalf("structural cast witness = %v/%v, want record", witness, ok)
	}
}

func TestLowerWIRClaimConditionPublishesRefinementWithoutSemanticSidecar(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local x: any = 0
if x as number then end
`)
	body := wirlower.Lower("claim-condition-no-sidecar", stmts, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	if got := facts.BranchRefinements(point); len(got) != 0 {
		t.Fatalf("WIR claim condition emitted branch refinements at point %d: %#v", point, got)
	}
	source, ok := facts.BranchConditionSource(point)
	if !ok {
		t.Fatalf("WIR claim condition missing condition source at point %d", point)
	}
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		t.Fatalf("WIR claim condition source = %#v, want expression-backed source", source)
	}
	if source.Adjusted {
		t.Fatalf("WIR claim condition source = %#v, want unadjusted branch-condition source", source)
	}
	assertWIRConcreteCastAssertion(t, facts, source, typ.Number, factflow.ValueSourcePath)
}

func TestLowerNestedClaimsPreserveOuterIdentityAndInnerFlow(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	read := ident("x")
	nonNil := &ast.NonNilAssertExpr{Expr: read}
	cast := &ast.CastExpr{Expr: nonNil, Type: primitiveType("number")}
	local := localAssign([]string{"a"}, cast)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	facts := lowerChunkFactsWithWIR(t, "nested-claims", stmts, built, bindings, standard.Registry())
	source := mustLocalSource(t, facts, requireStmtPoints(t, built, local, 1)[0])
	outer, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing outer assertion for source %#v", source)
	}
	if want := concreteCastAssertionForType(typ.Number); !assertion.Equal(refinementAssertion(t, outer), want) {
		t.Fatalf("outer assertion = %s, want %s", refinementAssertion(t, outer), want)
	}
	innerSource := outer.Source()
	if innerSource.ExprRef == source.ExprRef || innerSource.ExprRef == 0 {
		t.Fatalf("outer assertion did not point at distinct inner expr ref: outer=%#v inner=%#v", source, innerSource)
	}
	inner, ok := facts.ExpressionRefinement(innerSource.ExprRef)
	if !ok {
		t.Fatalf("missing inner non-nil claim for source %#v", innerSource)
	}
	if got := refinementAssertion(t, inner); !assertion.Equal(got, assertion.NonNil()) {
		t.Fatalf("inner assertion = %s, want non-nil", got)
	}
}

func TestLowerClaimWrappedCallPreservesProducerAndClaim(t *testing.T) {
	fooIdent := ident("foo")
	fooCall := &ast.FuncCallExpr{Func: fooIdent}
	fooCast := &ast.CastExpr{Expr: fooCall, Type: primitiveType("number")}
	local := localAssign([]string{"x"}, fooCast)
	barCall := &ast.FuncCallExpr{Func: ident("bar")}
	barCast := &ast.CastExpr{Expr: barCall, Type: primitiveType("string")}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{barCast}}
	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	readyCast := &ast.CastExpr{Expr: readyCall, Type: primitiveType("boolean")}
	ifStmt := &ast.IfStmt{Condition: readyCast}
	stmts := []ast.Stmt{local, ifStmt, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"foo", "bar", "ready"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatal("BuildChunk returned nil")
	}
	body := wirlower.Lower("wrapped-claim-return", stmts, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	localPoints := requireStmtPoints(t, built, local, 2)
	site, ok := facts.CallSiteView(localPoints[0])
	if !ok {
		t.Fatal("missing assertion-wrapped assignment call site")
	}
	innerRef, ok := site.Expr()
	if !ok || innerRef == 0 {
		t.Fatalf("inner call-site expr ref = %d/%v", innerRef, ok)
	}
	localSource := mustLocalSource(t, facts, localPoints[1])
	if localSource.Kind != factflow.ValueSourceCall || localSource.ExprRef == innerRef || localSource.CallPoint != localPoints[0] || !localSource.HasCallPoint {
		t.Fatalf("local wrapped call source = %#v, inner ref %d", localSource, innerRef)
	}
	claim, ok := facts.ExpressionRefinement(localSource.ExprRef)
	if !ok {
		t.Fatalf("missing assertion sidecar for outer ref %d", localSource.ExprRef)
	}
	if want := concreteCastAssertionForType(typ.Number); !assertion.Equal(refinementAssertion(t, claim), want) {
		t.Fatalf("outer assertion = %s, want %s", refinementAssertion(t, claim), want)
	}
	innerSource := claim.Source()
	if innerSource.Kind != factflow.ValueSourceCall || innerSource.CallPoint != localPoints[0] || !innerSource.HasCallPoint {
		t.Fatalf("assertion inner source = %#v, want call source at point %d", innerSource, localPoints[0])
	}

	returnPoints := requireStmtPoints(t, built, ret, 2)
	returnFact, ok := facts.Return(returnPoints[1])
	if !ok {
		t.Fatal("missing wrapped return fact")
	}
	returnSources := returnFact.Sources()
	if len(returnSources) != 1 || returnSources[0].Kind != factflow.ValueSourceExpression || !returnSources[0].HasExpr {
		t.Fatalf("wrapped return source = %#v, want cast expression source", returnSources)
	}
	returnClaim, ok := facts.ExpressionRefinement(returnSources[0].ExprRef)
	if !ok {
		t.Fatalf("missing wrapped return assertion for ref %d", returnSources[0].ExprRef)
	}
	if want := concreteCastAssertionForType(typ.String); !assertion.Equal(refinementAssertion(t, returnClaim), want) {
		t.Fatalf("return assertion = %s, want %s", refinementAssertion(t, returnClaim), want)
	}
	returnInner := returnClaim.Source()
	if returnInner.Kind != factflow.ValueSourceCall || returnInner.CallPoint != returnPoints[0] || !returnInner.HasCallPoint {
		t.Fatalf("return assertion inner source = %#v, want call source at point %d", returnInner, returnPoints[0])
	}

	ifPoints := requireStmtPoints(t, built, ifStmt, 2)
	conditionSource, ok := facts.BranchConditionSource(ifPoints[1])
	if !ok {
		t.Fatal("missing wrapped condition source")
	}
	if conditionSource.Kind != factflow.ValueSourceExpression || !conditionSource.HasExpr {
		t.Fatalf("wrapped condition source = %#v, want expression source", conditionSource)
	}
	conditionClaim, ok := facts.ExpressionRefinement(conditionSource.ExprRef)
	if !ok {
		t.Fatalf("missing wrapped condition assertion for ref %d", conditionSource.ExprRef)
	}
	conditionInner := conditionClaim.Source()
	if conditionInner.Kind != factflow.ValueSourceCall || conditionInner.CallPoint != ifPoints[0] || !conditionInner.HasCallPoint {
		t.Fatalf("condition assertion inner source = %#v, want call source at point %d", conditionInner, ifPoints[0])
	}
}
