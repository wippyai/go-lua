package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := site.ArgumentSources()
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
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d without cfgbuild CallView", callPoint)
	}
	args := site.ArgumentSources()
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
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := site.ArgumentSources()
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
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := site.ArgumentSources()
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
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := site.ArgumentSources()
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
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := site.ArgumentSources()
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
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing call site at point %d", callPoint)
	}
	args := site.ArgumentSources()
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

func TestLowerParsedAnyClaimCastsMarkUntrustedTop(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local x = 0
local a, b, c, d = x as any, x :: any, x as unknown, x :: unknown
`)

	reg := standard.Registry()
	facts := lowerChunkFactsWithWIR(t, "parsed-any-claims", stmts, built, bindings, standard.Registry())
	local := mustLocalStmt(t, stmts, 1)
	points := requireStmtPoints(t, built, local, 4)
	base := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	for _, point := range points {
		source := mustLocalSource(t, facts, point)
		refinement, ok := facts.ExpressionRefinement(source.ExprRef)
		if !ok {
			t.Fatalf("missing any claim refinement for source ref %d", source.ExprRef)
		}
		if got := product.Get(reg, refinement.Refinement(), assertion.Key); !assertion.Equal(got, assertion.Any()) {
			t.Fatalf("refinement assertion = %s, want any", got)
		}
		if got := product.Get(reg, refinement.Refinement(), evidence.Key); !evidence.Equal(got, evidence.ExplicitTop()) {
			t.Fatalf("refinement evidence = %s, want explicit-top", got)
		}
	}
	xSym := mustLocalAt(t, bindings, mustLocalStmt(t, stmts, 0), 0)
	input := state.State{}.WriteValue(reg, key.SymbolValue(xSym), base)

	factApply := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts: facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			KeySpace: keyspace.New(),
		}),
	})
	for _, point := range points {
		out := factApply(transfer.NodeContext{Registry: reg, Point: point}, input)
		fact, ok := facts.LocalAssignment(point)
		if !ok {
			t.Fatalf("missing local assignment at point %d", point)
		}
		assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
		want := product.Set(reg, base, assertion.Key, assertion.Any())
		want = product.Set(reg, want, evidence.Key, evidence.ExplicitTop())
		if !product.Equal(reg, assigned, want) {
			t.Fatalf("assigned value changed axes other than assertion.Any and explicit-top at point %d", point)
		}
		if got := product.Get(reg, assigned, assertion.Key); !assertion.Equal(got, assertion.Any()) {
			t.Fatalf("assigned assertion = %s, want any", got)
		}
		if got := product.PresenceOf(assigned); !presence.Equal(got, presence.Present()) {
			t.Fatalf("assigned presence = %s, want present", got)
		}
		if got := product.Get(reg, assigned, runtimekind.Key); !runtimekind.Equal(got, runtimekind.Singleton(runtimekind.Table)) {
			t.Fatalf("assigned runtime kind = %s, want table", got)
		}
		if got := product.Get(reg, assigned, evidence.Key); !evidence.Equal(got, evidence.ExplicitTop()) {
			t.Fatalf("assigned evidence = %s, want explicit-top", got)
		}
	}
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

func TestLowerClaimRefinementsApplyIndicatorsWithoutMutatingBaseValues(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	typeRead := ident("x")
	anyRead := ident("x")
	nonNilRead := ident("x")
	typeCast := &ast.CastExpr{Expr: typeRead, Type: primitiveType("number")}
	anyCast := &ast.CastExpr{Expr: anyRead, Type: primitiveType("any")}
	nonNil := &ast.NonNilAssertExpr{Expr: nonNilRead}
	local := localAssign([]string{"a", "b", "c"}, typeCast, anyCast, nonNil)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	reg, err := standard.RegistryWithAxes(testLowerSparseAxisSpec().Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes error = %v", err)
	}
	facts := lowerChunkFactsWithWIR(t, "claim-refinement-indicators", stmts, built, bindings, reg)
	points := requireStmtPoints(t, built, local, 3)
	xSym := mustLocalAt(t, bindings, decl, 0)
	type sourceCase struct {
		name              string
		point             cfg.Point
		base              product.Value
		wantClaim         assertion.Value
		wantPresence      presence.Value
		wantRuntimeKind   runtimekind.Value
		checkRuntimeKind  bool
		checkNoRefinement bool
		checkEvidence     bool
		wantEvidence      evidence.Value
	}
	cases := []sourceCase{
		{
			name:         "type",
			point:        points[0],
			base:         product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), testLowerSparseAxisKey, testLowerSparseAxisLow),
			wantClaim:    concreteCastAssertionForType(typ.Number),
			wantPresence: presence.Present(),
		},
		{
			name:              "any",
			point:             points[1],
			base:              product.Set(reg, product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), testLowerSparseAxisKey, testLowerSparseAxisLow), runtimekind.Key, runtimekind.Singleton(runtimekind.Table)),
			wantClaim:         assertion.Any(),
			wantPresence:      presence.Present(),
			wantRuntimeKind:   runtimekind.Singleton(runtimekind.Table),
			checkRuntimeKind:  true,
			checkNoRefinement: true,
			checkEvidence:     true,
			wantEvidence:      evidence.ExplicitTop(),
		},
		{
			name:         "non-nil",
			point:        points[2],
			base:         product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Maybe()), testLowerSparseAxisKey, testLowerSparseAxisLow),
			wantClaim:    assertion.NonNil(),
			wantPresence: presence.Present(),
		},
	}
	for i := range cases {
		source := mustLocalSource(t, facts, cases[i].point)
		if _, ok := facts.ExpressionRefinement(source.ExprRef); !ok {
			t.Fatalf("%s refinement missing", cases[i].name)
		}
	}

	factApply := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts: facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			KeySpace: keyspace.New(),
		}),
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.checkRuntimeKind {
				if got := product.PresenceOf(tc.base); !presence.Equal(got, tc.wantPresence) {
					t.Fatalf("base input presence = %s, want %s", got, tc.wantPresence)
				}
				if got := product.Get(reg, tc.base, runtimekind.Key); !runtimekind.Equal(got, tc.wantRuntimeKind) {
					t.Fatalf("base input runtime kind = %s, want %s", got, tc.wantRuntimeKind)
				}
			}
			input := state.State{}.WriteValue(reg, key.SymbolValue(xSym), tc.base)
			out := factApply(transfer.NodeContext{Registry: reg, Point: tc.point}, input)
			fact, ok := facts.LocalAssignment(tc.point)
			if !ok {
				t.Fatalf("missing local assignment at point %d", tc.point)
			}
			assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
			if got := product.Get(reg, assigned, assertion.Key); !assertion.Equal(got, tc.wantClaim) {
				t.Fatalf("assigned assertion = %s, want %s", got, tc.wantClaim)
			}
			if got := product.PresenceOf(assigned); !presence.Equal(got, tc.wantPresence) {
				t.Fatalf("assigned presence = %s, want %s", got, tc.wantPresence)
			}
			if tc.checkRuntimeKind {
				if got := product.Get(reg, assigned, runtimekind.Key); !runtimekind.Equal(got, tc.wantRuntimeKind) {
					t.Fatalf("assigned runtime kind = %s, want %s", got, tc.wantRuntimeKind)
				}
			}
			if got := product.Get(reg, assigned, testLowerSparseAxisKey); got != testLowerSparseAxisLow {
				t.Fatalf("assigned sparse axis = %v, want %v", got, testLowerSparseAxisLow)
			}
			if tc.checkEvidence {
				if got := product.Get(reg, assigned, evidence.Key); !evidence.Equal(got, tc.wantEvidence) {
					t.Fatalf("assigned evidence = %s, want %s", got, tc.wantEvidence)
				}
			}
			if tc.checkNoRefinement {
				if len(facts.BranchRefinements(tc.point)) != 0 {
					t.Fatalf("%s assignment produced branch refinement", tc.name)
				}
			}
			if got := product.Get(reg, tc.base, assertion.Key); !assertion.Equal(got, assertion.Top()) {
				t.Fatalf("base value mutated with assertion = %s", got)
			}
			if tc.checkRuntimeKind {
				if got := product.Get(reg, tc.base, runtimekind.Key); !runtimekind.Equal(got, tc.wantRuntimeKind) {
					t.Fatalf("base runtime kind = %s, want %s", got, tc.wantRuntimeKind)
				}
			}
			if got := product.Get(reg, tc.base, testLowerSparseAxisKey); got != testLowerSparseAxisLow {
				t.Fatalf("base sparse axis = %v, want %v", got, testLowerSparseAxisLow)
			}
		})
	}
}

func TestLowerNestedClaimRefinementsApplyCombinedIndicators(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	read := ident("x")
	nonNil := &ast.NonNilAssertExpr{Expr: read}
	cast := &ast.CastExpr{Expr: nonNil, Type: primitiveType("number")}
	local := localAssign([]string{"a"}, cast)
	stmts := []ast.Stmt{decl, local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	reg := standard.Registry()
	facts := lowerChunkFactsWithWIR(t, "nested-claim-indicators", stmts, built, bindings, standard.Registry())
	point := requireStmtPoints(t, built, local, 1)[0]
	source := mustLocalSource(t, facts, point)
	outer, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing outer assertion refinement")
	}
	if _, ok := facts.ExpressionRefinement(outer.Source().ExprRef); !ok {
		t.Fatalf("missing inner assertion refinement")
	}
	base := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	xSym := mustLocalAt(t, bindings, decl, 0)
	factApply := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts: facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			KeySpace: keyspace.New(),
		}),
	})

	input := state.State{}.WriteValue(reg, key.SymbolValue(xSym), base)
	out := factApply(transfer.NodeContext{Registry: reg, Point: point}, input)
	fact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment at point %d", point)
	}
	assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
	got := product.Get(reg, assigned, assertion.Key)
	if !got.Has(assertion.TypeClaim) || !got.Has(assertion.NonNilClaim) {
		t.Fatalf("nested assertion = %s, want type and non-nil indicators", got)
	}
	if got := product.Get(reg, base, assertion.Key); !assertion.Equal(got, assertion.Top()) {
		t.Fatalf("base value mutated with assertion = %s", got)
	}
}

func TestLowerNestedAnyClaimRefinementsKeepUntrustedTop(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local x = 0
local a, b = (x as any) as number, (x :: any) :: number
`)

	reg := standard.Registry()
	facts := lowerChunkFactsWithWIR(t, "nested-any-claim-indicators", stmts, built, bindings, reg)
	local := mustLocalStmt(t, stmts, 1)
	points := requireStmtPoints(t, built, local, 2)
	for _, point := range points {
		source := mustLocalSource(t, facts, point)
		outer, ok := facts.ExpressionRefinement(source.ExprRef)
		if !ok {
			t.Fatalf("missing outer assertion refinement for source ref %d", source.ExprRef)
		}
		assertClaimRefinementProduct(t, outer.Refinement(), concreteCastAssertionForType(typ.Number))
		inner := outer.Source()
		innerRefinement, ok := facts.ExpressionRefinement(inner.ExprRef)
		if !ok {
			t.Fatalf("missing inner any assertion refinement for source ref %d", inner.ExprRef)
		}
		if got := product.Get(reg, innerRefinement.Refinement(), assertion.Key); !assertion.Equal(got, assertion.Any()) {
			t.Fatalf("inner assertion = %s, want any", got)
		}
		if got := product.Get(reg, innerRefinement.Refinement(), evidence.Key); !evidence.Equal(got, evidence.ExplicitTop()) {
			t.Fatalf("inner evidence = %s, want explicit-top", got)
		}
	}

	factApply := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts: facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			KeySpace: keyspace.New(),
		}),
	})
	xSym := mustLocalAt(t, bindings, mustLocalStmt(t, stmts, 0), 0)
	input := state.State{}.WriteValue(reg, key.SymbolValue(xSym), product.NewWithPresence(reg, product.ShapeTop, presence.Present()))
	for _, point := range points {
		out := factApply(transfer.NodeContext{Registry: reg, Point: point}, input)
		fact, ok := facts.LocalAssignment(point)
		if !ok {
			t.Fatalf("missing local assignment at point %d", point)
		}
		assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
		base := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), assertion.Key, assertion.Any())
		base = product.Set(reg, base, evidence.Key, evidence.ExplicitTop())
		want := applyConcreteCastRefinement(reg, base, typ.Number)
		wantClaim := assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim)
		want = product.Set(reg, want, assertion.Key, wantClaim)
		want = product.Set(reg, want, evidence.Key, evidence.ExplicitTop())
		if !product.Equal(reg, assigned, want) {
			t.Fatalf("assigned claim/evidence = %s/%s, want %s/%s",
				product.Get(reg, assigned, assertion.Key), product.Get(reg, assigned, evidence.Key),
				product.Get(reg, want, assertion.Key), product.Get(reg, want, evidence.Key))
		}
		if got := product.Get(reg, assigned, evidence.Key); !evidence.Equal(got, evidence.ExplicitTop()) {
			t.Fatalf("assigned evidence = %v, want explicit-top", got)
		}
	}
}

func TestLowerColonCastRuntimeValidationClearsStaleAnyEvidence(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local x: any = 1
local a = x :: string
`)

	reg := standard.Registry()
	facts := lowerChunkFactsWithWIR(t, "colon-cast-clears-any", stmts, built, bindings, reg)
	local := mustLocalStmt(t, stmts, 1)
	point := requireStmtPoints(t, built, local, 1)[0]
	source := mustLocalSource(t, facts, point)
	refinement, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing cast refinement for source ref %d", source.ExprRef)
	}
	if got := refinement.Mode(); got != factflow.ExpressionRefinementRuntimeValidation {
		t.Fatalf("refinement mode = %v, want runtime validation", got)
	}
	input := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	input = product.Set(reg, input, evidence.Key, evidence.ExplicitTop())
	input = product.Set(reg, input, assertion.Key, assertion.Any())
	input = typevalue.WithWitness(reg, input, typ.Any)
	inner := refinement.Source()
	transferFn := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts: facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				inner.ExprRef: input,
			},
		}),
	})

	out := transferFn(transfer.NodeContext{Registry: reg, Point: point}, state.State{})
	fact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment at point %d", point)
	}
	assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
	if got := product.Get(reg, assigned, assertion.Key); !assertion.Equal(got, assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim)) {
		t.Fatalf("assigned assertion = %s, want runtime type claim", got)
	}
	if got := product.PresenceOf(assigned); !presence.Equal(got, presence.Present()) {
		t.Fatalf("assigned presence = %s, want present", got)
	}
	witness := product.Get(reg, assigned, typewitness.Key)
	gotType, ok := witness.Type()
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("assigned witness = %v/%v, want string", witness, ok)
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
	site, ok := facts.CallSite(localPoints[0])
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

func TestLowerExpandedClaimWrappedCallKeepsPerResultSlotRefinements(t *testing.T) {
	cases := []struct {
		name string
		wrap func(*ast.FuncCallExpr) ast.Expr
		want assertion.Value
	}{
		{
			name: "cast",
			wrap: func(call *ast.FuncCallExpr) ast.Expr {
				return &ast.CastExpr{Expr: call, Type: primitiveType("number"), Syntax: ast.CastSyntaxAs}
			},
			want: concreteCastAssertionForType(typ.Number),
		},
		{
			name: "non-nil",
			wrap: func(call *ast.FuncCallExpr) ast.Expr {
				return &ast.NonNilAssertExpr{Expr: call}
			},
			want: assertion.NonNil(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			makeCall := &ast.FuncCallExpr{Func: ident("make")}
			local := localAssign([]string{"a", "b"}, tc.wrap(makeCall))
			stmts := []ast.Stmt{local}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make"}})
			built := cfgbuild.BuildChunk(stmts, bindings)

			reg := standard.Registry()
			body := wirlower.Lower("expanded-claim-wrapped-call", stmts, bindings, built)
			facts := LowerDetailed(built.Graph, Config{Registry: reg, WIR: body}).Facts
			points := requireStmtPoints(t, built, local, 3)
			site, ok := facts.CallSite(points[0])
			if !ok {
				t.Fatal("missing wrapped call site")
			}
			innerRef, ok := site.Expr()
			if !ok || innerRef == 0 {
				t.Fatalf("call-site expr ref = %d/%v", innerRef, ok)
			}

			firstSource := mustLocalSource(t, facts, points[1])
			secondSource := mustLocalSource(t, facts, points[2])
			if firstSource.ExprRef == secondSource.ExprRef {
				t.Fatalf("expanded wrapped call reused one outer source ref for both result slots: %#v %#v", firstSource, secondSource)
			}

			assertSlotRefinement := func(source factflow.ValueSource, resultIndex int) {
				t.Helper()
				refinement, ok := facts.ExpressionRefinement(source.ExprRef)
				if !ok {
					t.Fatalf("missing refinement for source ref %d", source.ExprRef)
				}
				assertClaimRefinementProduct(t, refinement.Refinement(), tc.want)
				inner := refinement.Source()
				if inner.Kind != factflow.ValueSourceCall || inner.ResultIndex != resultIndex || inner.CallPoint != points[0] || !inner.HasCallPoint {
					t.Fatalf("refinement source = %#v, want call result %d at point %d", inner, resultIndex, points[0])
				}
			}
			assertSlotRefinement(firstSource, 0)
			assertSlotRefinement(secondSource, 1)

			firstValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
			secondValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
			transferFn := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
				Facts: facts,
				Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
					Registry: reg,
				}),
				CallOutcome: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
					if ctx.Point != points[0] {
						t.Fatalf("call result requested at point %d, want %d", ctx.Point, points[0])
					}
					return callpayload.CallOutcome{
						Results: []callpayload.CallResult{
							{Index: 0, Value: firstValue},
							{Index: 1, Value: secondValue},
						},
					}
				},
			})

			out := transferFn(transfer.NodeContext{Registry: reg, Point: points[1]}, state.State{})
			out = transferFn(transfer.NodeContext{Registry: reg, Point: points[2]}, out)
			firstFact, ok := facts.LocalAssignment(points[1])
			if !ok {
				t.Fatalf("missing first local assignment")
			}
			secondFact, ok := facts.LocalAssignment(points[2])
			if !ok {
				t.Fatalf("missing second local assignment")
			}
			firstAssigned := out.ReadValue(reg, key.SymbolValue(firstFact.TargetSymbol()))
			secondAssigned := out.ReadValue(reg, key.SymbolValue(secondFact.TargetSymbol()))
			wantFirst := product.Set(reg, firstValue, assertion.Key, tc.want)
			wantSecond := product.Set(reg, secondValue, assertion.Key, tc.want)
			if tc.want.Has(assertion.TypeClaim) {
				wantFirst = applyConcreteCastRefinement(reg, firstValue, typ.Number)
				wantSecond = applyConcreteCastRefinement(reg, secondValue, typ.Number)
			}
			if !product.Equal(reg, firstAssigned, wantFirst) {
				t.Fatalf("first assigned claim/witness/runtime = %s/%v/%s, want %s/%v/%s",
					product.Get(reg, firstAssigned, assertion.Key), product.Get(reg, firstAssigned, typewitness.Key), product.Get(reg, firstAssigned, runtimekind.Key),
					product.Get(reg, wantFirst, assertion.Key), product.Get(reg, wantFirst, typewitness.Key), product.Get(reg, wantFirst, runtimekind.Key))
			}
			if !product.Equal(reg, secondAssigned, wantSecond) {
				t.Fatalf("second assigned claim/witness/runtime = %s/%v/%s, want %s/%v/%s",
					product.Get(reg, secondAssigned, assertion.Key), product.Get(reg, secondAssigned, typewitness.Key), product.Get(reg, secondAssigned, runtimekind.Key),
					product.Get(reg, wantSecond, assertion.Key), product.Get(reg, wantSecond, typewitness.Key), product.Get(reg, wantSecond, runtimekind.Key))
			}
		})
	}
}

func TestLowerConcreteCastWrappedAnyCallPublishesRuntimeWitness(t *testing.T) {
	makeCall := &ast.FuncCallExpr{Func: ident("make")}
	local := localAssign([]string{"a"}, &ast.CastExpr{
		Expr:   makeCall,
		Type:   primitiveType("number"),
		Syntax: ast.CastSyntaxAs,
	})
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	reg := standard.Registry()
	facts := lowerChunkFactsWithWIR(t, "concrete-cast-wrapped-any-call", stmts, built, bindings, reg)
	points := requireStmtPoints(t, built, local, 2)
	source := mustLocalSource(t, facts, points[1])
	refinement, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing refinement for source ref %d", source.ExprRef)
	}
	assertConcreteCastRefinementProduct(t, refinement.Refinement(), typ.Number)

	anyResult := typevalueWithExplicitAny(reg, typ.Any)
	transferFn := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts: facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
		}),
		CallOutcome: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				Results: []callpayload.CallResult{{Index: 0, Value: anyResult}},
			}
		},
	})

	out := transferFn(transfer.NodeContext{Registry: reg, Point: points[1]}, state.State{})
	fact, ok := facts.LocalAssignment(points[1])
	if !ok {
		t.Fatalf("missing local assignment")
	}
	assigned := out.ReadValue(reg, key.SymbolValue(fact.TargetSymbol()))
	if got := product.Get(reg, assigned, assertion.Key); !got.Has(assertion.RuntimeClaim) || !got.Has(assertion.TypeClaim) {
		t.Fatalf("assigned assertion = %s, want runtime type claim", got)
	}
	witness := product.Get(reg, assigned, typewitness.Key)
	gotType, ok := witness.Type()
	if !ok || !typ.TypeEquals(gotType, typ.Number) {
		t.Fatalf("assigned witness = %v/%v, want number", witness, ok)
	}
}

func typevalueWithExplicitAny(reg *axis.Registry, t typ.Type) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Any())
	return typevalue.WithWitness(reg, value, t)
}
