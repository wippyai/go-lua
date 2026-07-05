package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerLocalAssignmentUsesWIRLiteralSources(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string, make_value: () -> string)
    local from_param = value
    local from_literal = "ok"
    local from_call = make_value()
    local from_local = from_literal
end`)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	paramPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	paramAssign, ok := facts.RootAssignment(paramPoint)
	if !ok {
		t.Fatalf("missing param local assignment at point %d", paramPoint)
	}
	paramSource := paramAssign.Source()
	if paramSource.Kind != factflow.ValueSourceExpression || !paramSource.HasExpr {
		t.Fatalf("param assignment source = %#v, want expression-backed alias source until WIR path mutation proof migration", paramSource)
	}

	literalPoint := requireStmtPoints(t, built, fn.Stmts[1], 1)[0]
	literalAssign, ok := facts.RootAssignment(literalPoint)
	if !ok {
		t.Fatalf("missing literal local assignment at point %d", literalPoint)
	}
	literalSource := literalAssign.Source()
	if literalSource.Kind != factflow.ValueSourceLiteral || literalSource.LiteralKind != factflow.ValueSourceLiteralString ||
		literalSource.String != "ok" || literalSource.HasExpr {
		t.Fatalf("literal assignment source = %#v, want WIR string literal", literalSource)
	}

	callStmtPoints := requireStmtPoints(t, built, fn.Stmts[2], 2)
	var callAssign factflow.RootAssignment
	var assignPoint cfg.Point
	for _, point := range callStmtPoints {
		if got, ok := facts.RootAssignment(point); ok {
			callAssign = got
			assignPoint = point
			break
		}
	}
	if assignPoint == 0 {
		t.Fatalf("missing call local assignment in points %v", callStmtPoints)
	}
	callSource := callAssign.Source()
	if callSource.Kind != factflow.ValueSourceCall || !callSource.HasCallPoint || callSource.CallPoint == 0 ||
		callSource.ResultIndex != 0 || !callSource.HasExpr {
		t.Fatalf("call assignment source = %#v, want WIR call-result source with preserved expression ref", callSource)
	}

	localPoint := requireStmtPoints(t, built, fn.Stmts[3], 1)[0]
	localAssign, ok := facts.RootAssignment(localPoint)
	if !ok {
		t.Fatalf("missing local-source assignment at point %d", localPoint)
	}
	localSource := localAssign.Source()
	if localSource.Kind != factflow.ValueSourceExpression || !localSource.HasExpr || localSource.PathKey != "" {
		t.Fatalf("local assignment source = %#v, want WIR expression-backed path source", localSource)
	}
	wantPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "from_literal")
	gotPath, ok := facts.ExpressionPath(localSource.ExprRef)
	if !ok || !gotPath.Equal(wantPath) {
		t.Fatalf("local assignment expression path = %v/%v, want %v", gotPath, ok, wantPath)
	}
}

func TestLowerAssignmentLocalSourcePathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = "x"
    local other = "y"
    local out = value
end
`)
	assignStmt := fn.Stmts[2].(*ast.LocalAssignStmt)
	points := requireStmtPoints(t, built, assignStmt, 1)
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	outPath := path.NewPath(mustLocalAt(t, bindings, assignStmt, 0), "out")
	body := wir.NewBody("synthetic-assign")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpAssign,
		Point: points[0],
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(outPath))},
		A:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	assign, ok := facts.RootAssignment(points[0])
	if !ok {
		t.Fatalf("missing assignment at point %d", points[0])
	}
	source := assign.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		t.Fatalf("assignment source = %#v, want expression-backed WIR path", source)
	}
	gotPath, ok := facts.ExpressionPath(source.ExprRef)
	if !ok || !gotPath.Equal(otherPath) || gotPath.Equal(valuePath) {
		t.Fatalf("assignment expression path = %v/%v, want WIR path %v not semantic path %v", gotPath, ok, otherPath, valuePath)
	}
}

func TestLowerDynamicIndexKeySourcePathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(box: any): ()
    local value = "x"
    local other = "y"
    local payload = "z"
    box[value] = payload
end
`)
	assignStmt := fn.Stmts[3].(*ast.AssignStmt)
	points := requireStmtPoints(t, built, assignStmt, 1)
	boxPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "box")
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	payloadPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[2].(*ast.LocalAssignStmt), 0), "payload")
	body := wir.NewBody("synthetic-dynamic-key")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpDynamicIndexWrite,
		Point: points[0],
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(boxPath))},
		A:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
		B:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(payloadPath))},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	write, ok := facts.DynamicIndexWrite(points[0])
	if !ok {
		t.Fatalf("missing dynamic index write at point %d", points[0])
	}
	keySource := write.KeySource()
	if keySource.Kind != factflow.ValueSourceExpression || !keySource.HasExpr {
		t.Fatalf("dynamic key source = %#v, want expression-backed WIR path", keySource)
	}
	gotPath, ok := facts.ExpressionPath(keySource.ExprRef)
	if !ok || !gotPath.Equal(otherPath) || gotPath.Equal(valuePath) {
		t.Fatalf("dynamic key expression path = %v/%v, want WIR path %v not semantic path %v", gotPath, ok, otherPath, valuePath)
	}
}

func TestLowerPathAndDynamicAssignmentKeepExpressionSourcesDuringWIRMigration(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(box: any, key: string, value: string)
    box.name = value
    box[key] = value
end`)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	staticPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	pathAssign, ok := facts.PathAssignment(staticPoint)
	if !ok {
		t.Fatalf("missing static path assignment at point %d", staticPoint)
	}
	staticSource := pathAssign.Source()
	if staticSource.Kind != factflow.ValueSourceExpression || !staticSource.HasExpr {
		t.Fatalf("static path source = %#v, want expression-backed source until WIR path proof migration", staticSource)
	}
	staticWrite, ok := facts.PathStaticMemberWrite(staticPoint)
	if !ok {
		t.Fatalf("missing static member write at point %d", staticPoint)
	}
	if got := staticWrite.Source(); got.Kind != factflow.ValueSourceExpression || !got.HasExpr {
		t.Fatalf("static member write source = %#v, want expression-backed source until WIR path proof migration", got)
	}

	dynamicPoint := requireStmtPoints(t, built, fn.Stmts[1], 1)[0]
	dynamicWrite, ok := facts.DynamicIndexWrite(dynamicPoint)
	if !ok {
		t.Fatalf("missing dynamic index write at point %d", dynamicPoint)
	}
	dynamicSource := dynamicWrite.Source()
	if dynamicSource.Kind != factflow.ValueSourceExpression || !dynamicSource.HasExpr {
		t.Fatalf("dynamic write source = %#v, want expression-backed source until WIR path proof migration", dynamicSource)
	}
}

func TestLowerAssignmentDoesNotFallbackWhenWIRWriteInstructionMissing(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(box: any, key: string, value: string): ()
    local local_value = value
    box.name = value
    box[key] = value
end
`)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: wir.NewBody("empty")})

	localPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	if _, ok := facts.RootAssignment(localPoint); ok {
		t.Fatalf("WIR mode local assignment at point %d fell back to semantic sidecar", localPoint)
	}

	staticPoint := requireStmtPoints(t, built, fn.Stmts[1], 1)[0]
	if _, ok := facts.PathAssignment(staticPoint); ok {
		t.Fatalf("WIR mode path assignment at point %d fell back to semantic sidecar", staticPoint)
	}
	if _, ok := facts.PathStaticMemberWrite(staticPoint); ok {
		t.Fatalf("WIR mode static member write at point %d fell back to semantic sidecar", staticPoint)
	}

	dynamicPoint := requireStmtPoints(t, built, fn.Stmts[2], 1)[0]
	if _, ok := facts.DynamicIndexWrite(dynamicPoint); ok {
		t.Fatalf("WIR mode dynamic index write at point %d fell back to semantic sidecar", dynamicPoint)
	}
}
