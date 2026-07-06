package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
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
	wantPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "from_literal")
	if localSource.Kind != factflow.ValueSourceExpression || !localSource.HasExpr || localSource.PathKey != "" {
		t.Fatalf("local assignment source = %#v, want WIR expression-backed path source", localSource)
	}
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

func TestLowerAssignmentSegmentedSourcePathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = { name = "x" }
    local other = { name = "y" }
    local out = value.name
end
`)
	assignStmt := fn.Stmts[2].(*ast.LocalAssignStmt)
	points := requireStmtPoints(t, built, assignStmt, 1)
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value").Field("name")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other").Field("name")
	outPath := path.NewPath(mustLocalAt(t, bindings, assignStmt, 0), "out")
	body := wir.NewBody("synthetic-segmented-assign")
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

func TestLowerOrdinaryRootWriteSourcePathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local out = ""
    local value = "x"
    local other = "y"
    out = value
end
`)
	assignStmt := fn.Stmts[3].(*ast.AssignStmt)
	points := requireStmtPoints(t, built, assignStmt, 1)
	outPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "out")
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "value")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[2].(*ast.LocalAssignStmt), 0), "other")
	body := wir.NewBody("synthetic-root-write-source")
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
		t.Fatalf("missing root assignment at point %d", points[0])
	}
	source := assign.Source()
	assertWIRPathSource(t, source, otherPath)
	if source.PathKey == valuePath.Key() {
		t.Fatalf("root assignment source path = %s, want WIR path %s not semantic path %s", source.PathKey, otherPath.Key(), valuePath.Key())
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
	assertWIRPathSource(t, keySource, otherPath)
	if keySource.PathKey == valuePath.Key() {
		t.Fatalf("dynamic key source path = %s, want WIR path %s not semantic path %s", keySource.PathKey, otherPath.Key(), valuePath.Key())
	}
	gotKeyPath, ok := write.KeyPath()
	if !ok || !gotKeyPath.Equal(otherPath) || gotKeyPath.Equal(valuePath) {
		t.Fatalf("dynamic key path = %v/%v, want WIR path %v not semantic path %v", gotKeyPath, ok, otherPath, valuePath)
	}
}

func TestLowerDynamicIndexMissingWIRKeyDoesNotFallbackToASTKey(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(box: any, key: string, payload: string): ()
    box[key] = payload
end
`)
	assignStmt := fn.Stmts[0].(*ast.AssignStmt)
	points := requireStmtPoints(t, built, assignStmt, 1)
	boxPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "box")
	keyPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "key")
	payloadPath := path.NewPath(bindings.ParamSlots(fn)[2].Symbol, "payload")
	body := wir.NewBody("synthetic-missing-dynamic-key")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpDynamicIndexWrite,
		Point: points[0],
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(boxPath))},
		B:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(payloadPath))},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	write, ok := facts.DynamicIndexWrite(points[0])
	if !ok {
		t.Fatalf("missing dynamic index write at point %d", points[0])
	}
	keySource := write.KeySource()
	if keySource.Kind != factflow.ValueSourceUnknown || keySource.HasExpr || keySource.PathKey == keyPath.Key() {
		t.Fatalf("dynamic key source = %#v, want unknown WIR key without AST fallback to %s", keySource, keyPath.Key())
	}
	if gotKeyPath, ok := write.KeyPath(); ok || gotKeyPath.Equal(keyPath) {
		t.Fatalf("dynamic key path = %v/%v, want no AST fallback to %v", gotKeyPath, ok, keyPath)
	}
}

func TestLowerDynamicAppendKeySourceComesFromWIRExpression(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(binding: any): ()
    local normalized: {any} = {}
    normalized[#normalized + 1] = binding
end
`)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	assignStmt := fn.Stmts[1].(*ast.AssignStmt)
	points := requireStmtPoints(t, built, assignStmt, 1)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	write, ok := facts.DynamicIndexWrite(points[0])
	if !ok {
		t.Fatalf("missing dynamic index write at point %d", points[0])
	}
	keySource := write.KeySource()
	if keySource.Kind != factflow.ValueSourceExpression || !keySource.HasExpr {
		t.Fatalf("dynamic append key source = %#v, want WIR expression source", keySource)
	}
	op, ok := facts.ExpressionOperation(keySource.ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationBinary || op.Op() != "+" {
		t.Fatalf("dynamic append key operation = %#v/%v, want binary +", op, ok)
	}
	left := op.Left()
	if left.Kind != factflow.ValueSourceExpression || !left.HasExpr {
		t.Fatalf("dynamic append key left = %#v, want WIR expression source for #normalized", left)
	}
	leftOp, ok := facts.ExpressionOperation(left.ExprRef)
	if !ok || leftOp.Kind() != factflow.ExpressionOperationUnary || leftOp.Op() != "#" {
		t.Fatalf("dynamic append key left operation = %#v/%v, want unary #", leftOp, ok)
	}
}

func TestLowerDynamicAppendInsideDefaultedIteratorComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(bindings: any): ()
    local normalized: {any} = {}
    for _, binding in ipairs(bindings or {}) do
        if type(binding) == "table" then
            normalized[#normalized + 1] = binding
        end
    end
end
`)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	genericFor, ok := fn.Stmts[1].(*ast.GenericForStmt)
	if !ok {
		t.Fatalf("statement 1 = %T, want generic for", fn.Stmts[1])
	}
	ifStmt, ok := genericFor.Stmts[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("loop body 0 = %T, want if", genericFor.Stmts[0])
	}
	assignStmt, ok := ifStmt.Then[0].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("if then 0 = %T, want assignment", ifStmt.Then[0])
	}
	points := requireStmtPoints(t, built, assignStmt, 1)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	write, ok := facts.DynamicIndexWrite(points[0])
	if !ok {
		t.Fatalf("missing dynamic index write at point %d", points[0])
	}
	source := write.Source()
	if source.Kind != factflow.ValueSourcePath || source.PathKey == "" {
		t.Fatalf("dynamic append source = %#v, want WIR path source for binding", source)
	}
}

func TestLowerDynamicIndexTablePathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(box: any, key: string, payload: string): ()
    local other_box = {}
    box[key] = payload
end
`)
	assignStmt := fn.Stmts[1].(*ast.AssignStmt)
	points := requireStmtPoints(t, built, assignStmt, 1)
	boxPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "box")
	keyPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "key")
	payloadPath := path.NewPath(bindings.ParamSlots(fn)[2].Symbol, "payload")
	otherBoxPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "other_box")
	body := wir.NewBody("synthetic-dynamic-table")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpDynamicIndexWrite,
		Point: points[0],
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherBoxPath))},
		A:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(keyPath))},
		B:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(payloadPath))},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	write, ok := facts.DynamicIndexWrite(points[0])
	if !ok {
		t.Fatalf("missing dynamic index write at point %d", points[0])
	}
	gotPath := write.TablePath()
	if !gotPath.Equal(otherBoxPath) || gotPath.Equal(boxPath) {
		t.Fatalf("dynamic table path = %v, want WIR path %v not semantic path %v", gotPath, otherBoxPath, boxPath)
	}
}

func TestLowerDynamicIndexWriteDoesNotFallbackToASTTargetInWIRMode(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(box: any, key: string, payload: string): ()
    box[key] = payload
end
`)
	assignStmt := fn.Stmts[0].(*ast.AssignStmt)
	points := requireStmtPoints(t, built, assignStmt, 1)
	staticPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "box").Field("name")
	payloadPath := path.NewPath(bindings.ParamSlots(fn)[2].Symbol, "payload")
	body := wir.NewBody("synthetic-non-dynamic-write")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpStaticMemberWrite,
		Point: points[0],
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(staticPath))},
		A:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(payloadPath))},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	if _, ok := facts.DynamicIndexWrite(points[0]); ok {
		t.Fatalf("WIR mode dynamic index write at point %d fell back to AST target", points[0])
	}
}

func TestLowerStaticMemberWriteSourcePathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(box: any): ()
    local value = "x"
    local other = "y"
    box.name = value
end
`)
	assignStmt := fn.Stmts[2].(*ast.AssignStmt)
	points := requireStmtPoints(t, built, assignStmt, 1)
	boxPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "box").Field("name")
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	body := wir.NewBody("synthetic-static-write-source")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpStaticMemberWrite,
		Point: points[0],
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(boxPath))},
		A:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	write, ok := facts.PathStaticMemberWrite(points[0])
	if !ok {
		t.Fatalf("missing static member write at point %d", points[0])
	}
	source := write.Source()
	assertWIRPathSource(t, source, otherPath)
	if source.PathKey == valuePath.Key() {
		t.Fatalf("static member source path = %s, want WIR path %s not semantic path %s", source.PathKey, otherPath.Key(), valuePath.Key())
	}
}

func TestLowerDynamicIndexValueSourcePathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(box: any, key: string): ()
    local value = "x"
    local other = "y"
    box[key] = value
end
`)
	assignStmt := fn.Stmts[2].(*ast.AssignStmt)
	points := requireStmtPoints(t, built, assignStmt, 1)
	boxPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "box")
	keyPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "key")
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	body := wir.NewBody("synthetic-dynamic-write-source")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpDynamicIndexWrite,
		Point: points[0],
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(boxPath))},
		A:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(keyPath))},
		B:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	write, ok := facts.DynamicIndexWrite(points[0])
	if !ok {
		t.Fatalf("missing dynamic index write at point %d", points[0])
	}
	source := write.Source()
	assertWIRPathSource(t, source, otherPath)
	if source.PathKey == valuePath.Key() {
		t.Fatalf("dynamic value source path = %s, want WIR path %s not semantic path %s", source.PathKey, otherPath.Key(), valuePath.Key())
	}
	gotValuePath, ok := write.ValuePath()
	if !ok || !gotValuePath.Equal(otherPath) || gotValuePath.Equal(valuePath) {
		t.Fatalf("dynamic value path = %v/%v, want WIR path %v not semantic path %v", gotValuePath, ok, otherPath, valuePath)
	}
}

func TestLowerDynamicIndexNonPathWIRValueDoesNotFallbackToASTValuePath(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(box: any, key: string): ()
    local payload = "x"
    box[key] = payload
end
`)
	assignStmt := fn.Stmts[1].(*ast.AssignStmt)
	points := requireStmtPoints(t, built, assignStmt, 1)
	boxPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "box")
	keyPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "key")
	payloadPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "payload")
	body := wir.NewBody("synthetic-nonpath-dynamic-value")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpDynamicIndexWrite,
		Point: points[0],
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(boxPath))},
		A:     wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(keyPath))},
		B:     wir.Operand{Kind: wir.OperandConst, Ref: uint32(body.InternConst(wir.Const{Kind: wir.ConstString, Str: "literal"}))},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	write, ok := facts.DynamicIndexWrite(points[0])
	if !ok {
		t.Fatalf("missing dynamic index write at point %d", points[0])
	}
	if gotValuePath, ok := write.ValuePath(); ok || gotValuePath.Equal(payloadPath) {
		t.Fatalf("dynamic value path = %v/%v, want no AST fallback to %v", gotValuePath, ok, payloadPath)
	}
	if source := write.Source(); source.Kind != factflow.ValueSourceLiteral || source.String != "literal" || source.HasExpr {
		t.Fatalf("dynamic value source = %#v, want WIR literal source without AST fallback", source)
	}
}

func TestLowerPathAndDynamicAssignmentUseWIRPathSources(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(box: any, key: string, value: string)
    local local_value = value
    box.name = value
    box[key] = value
end`)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	localPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	localAssign, ok := facts.RootAssignment(localPoint)
	if !ok {
		t.Fatalf("missing local assignment at point %d", localPoint)
	}
	if source := localAssign.Source(); source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		t.Fatalf("local assignment source = %#v, want expression-backed source while local expression APIs remain live", source)
	}

	valuePath := path.NewPath(bindings.ParamSlots(fn)[2].Symbol, "value")
	staticPoint := requireStmtPoints(t, built, fn.Stmts[1], 1)[0]
	pathAssign, ok := facts.PathAssignment(staticPoint)
	if !ok {
		t.Fatalf("missing static path assignment at point %d", staticPoint)
	}
	boxPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "box")
	wantTarget := boxPath.Field("name")
	if got := pathAssign.TargetPathRef(); !got.Equal(wantTarget) {
		t.Fatalf("path assignment target = %s, want WIR target %s", got.String(), wantTarget.String())
	}
	assertWIRPathSource(t, pathAssign.Source(), valuePath)
	staticWrite, ok := facts.PathStaticMemberWrite(staticPoint)
	if !ok {
		t.Fatalf("missing static member write at point %d", staticPoint)
	}
	if got := staticWrite.TargetPathRef(); !got.Equal(wantTarget) {
		t.Fatalf("static member write target = %s, want WIR target %s", got.String(), wantTarget.String())
	}
	assertWIRPathSource(t, staticWrite.Source(), valuePath)

	dynamicPoint := requireStmtPoints(t, built, fn.Stmts[2], 1)[0]
	dynamicWrite, ok := facts.DynamicIndexWrite(dynamicPoint)
	if !ok {
		t.Fatalf("missing dynamic index write at point %d", dynamicPoint)
	}
	assertWIRPathSource(t, dynamicWrite.Source(), valuePath)
}

func TestLowerWIRPathAndDynamicWritesPublishWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function f(box: any, key: string, value: string)
    box.name = value
    box[key] = value
end`)
	body := wirlower.Lower("assignment-no-sidecars", fn.Stmts, bindings, built)
	facts := Lower(nil, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	boxPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "box")
	valuePath := path.NewPath(bindings.ParamSlots(fn)[2].Symbol, "value")
	staticPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	wantStaticTarget := boxPath.Field("name")
	pathAssign, ok := facts.PathAssignment(staticPoint)
	if !ok {
		t.Fatalf("missing WIR no-sidecar static path assignment at point %d", staticPoint)
	}
	if got := pathAssign.TargetPathRef(); !got.Equal(wantStaticTarget) {
		t.Fatalf("path assignment target = %s, want %s", got.String(), wantStaticTarget.String())
	}
	assertWIRPathSource(t, pathAssign.Source(), valuePath)
	staticWrite, ok := facts.PathStaticMemberWrite(staticPoint)
	if !ok {
		t.Fatalf("missing WIR no-sidecar static member write at point %d", staticPoint)
	}
	if got := staticWrite.TargetPathRef(); !got.Equal(wantStaticTarget) {
		t.Fatalf("static member write target = %s, want %s", got.String(), wantStaticTarget.String())
	}

	dynamicPoint := requireStmtPoints(t, built, fn.Stmts[1], 1)[0]
	dynamicWrite, ok := facts.DynamicIndexWrite(dynamicPoint)
	if !ok {
		t.Fatalf("missing WIR no-sidecar dynamic index write at point %d", dynamicPoint)
	}
	assertWIRPathSource(t, dynamicWrite.Source(), valuePath)
	invalidation, ok := facts.PathDescendantInvalidation(dynamicPoint)
	if !ok {
		t.Fatalf("missing WIR no-sidecar dynamic index invalidation at point %d", dynamicPoint)
	}
	if got := invalidation.ContainerPathRef(); !got.Equal(boxPath) {
		t.Fatalf("dynamic invalidation container = %s, want %s", got.String(), boxPath.String())
	}
}

func TestLowerWIRRootAssignmentsPublishKindWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function f(value: string): ()
    local out = value
    out = "updated"
end`)
	body := wirlower.Lower("root-assignment-no-sidecars", fn.Stmts, bindings, built)
	facts := Lower(nil, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	outPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "out")
	localPoint := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	localAssign, ok := facts.RootAssignment(localPoint)
	if !ok {
		t.Fatalf("missing WIR no-sidecar local root assignment at point %d", localPoint)
	}
	if got := localAssign.Kind(); got != factflow.RootAssignmentLocalDeclaration {
		t.Fatalf("local root assignment kind = %v, want local declaration", got)
	}
	if got := localAssign.TargetPath(); !got.Equal(outPath) {
		t.Fatalf("local root assignment target = %s, want %s", got.String(), outPath.String())
	}

	ordinaryPoint := requireStmtPoints(t, built, fn.Stmts[1], 1)[0]
	ordinaryAssign, ok := facts.RootAssignment(ordinaryPoint)
	if !ok {
		t.Fatalf("missing WIR no-sidecar ordinary root assignment at point %d", ordinaryPoint)
	}
	if got := ordinaryAssign.Kind(); got != factflow.RootAssignmentOrdinaryRootWrite {
		t.Fatalf("ordinary root assignment kind = %v, want ordinary root write", got)
	}
	if got := ordinaryAssign.TargetPath(); !got.Equal(outPath) {
		t.Fatalf("ordinary root assignment target = %s, want %s", got.String(), outPath.String())
	}
}

func TestLowerWIRTableRootAssignmentPublishesExpressionSourceWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function f(value: string): ()
    local out = { name = value }
end`)
	body := wirlower.Lower("table-root-assignment-no-sidecars", fn.Stmts, bindings, built)
	facts := Lower(nil, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	outPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "out")
	point := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	assign, ok := facts.RootAssignment(point)
	if !ok {
		t.Fatalf("missing WIR no-sidecar table root assignment at point %d", point)
	}
	if got := assign.Kind(); got != factflow.RootAssignmentLocalDeclaration {
		t.Fatalf("table root assignment kind = %v, want local declaration", got)
	}
	if got := assign.TargetPath(); !got.Equal(outPath) {
		t.Fatalf("table root assignment target = %s, want %s", got.String(), outPath.String())
	}
	source := assign.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		t.Fatalf("table root assignment source = %#v, want expression source", source)
	}
	assertExpressionValue(t, facts, source.ExprRef, presence.Present(), runtimekind.Singleton(runtimekind.Table))
}

func TestLowerWIRDynamicIndexRootAssignmentPublishesExpressionSourceWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function f(box: {[string]: string}, key: string): ()
    local out = box[key]
end`)
	body := wirlower.Lower("dynamic-index-root-assignment-no-sidecars", fn.Stmts, bindings, built)
	facts := Lower(nil, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	outPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "out")
	boxPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "box")
	keyPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "key")
	point := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	assign, ok := facts.RootAssignment(point)
	if !ok {
		t.Fatalf("missing WIR no-sidecar dynamic-index root assignment at point %d", point)
	}
	if got := assign.TargetPath(); !got.Equal(outPath) {
		t.Fatalf("dynamic-index root assignment target = %s, want %s", got.String(), outPath.String())
	}
	source := assign.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		t.Fatalf("dynamic-index root assignment source = %#v, want expression source", source)
	}
	if _, ok := facts.ExpressionPath(source.ExprRef); ok {
		t.Fatalf("dynamic-index root assignment source ref %d unexpectedly has a static path", source.ExprRef)
	}
	dynamicExpr, ok := facts.DynamicIndexExpression(source.ExprRef)
	if !ok {
		t.Fatalf("missing dynamic-index expression for ref %d", source.ExprRef)
	}
	if got := dynamicExpr.TablePath(); !got.Equal(boxPath) {
		t.Fatalf("dynamic-index table path = %s, want %s", got.String(), boxPath.String())
	}
	keySource := dynamicExpr.KeySource()
	if keySource.Kind != factflow.ValueSourceExpression || !keySource.HasExpr {
		t.Fatalf("dynamic-index key source = %#v, want expression source", keySource)
	}
	gotKeyPath, ok := facts.ExpressionPath(keySource.ExprRef)
	if !ok || !gotKeyPath.Equal(keyPath) {
		t.Fatalf("dynamic-index key path = %s/%v, want %s", gotKeyPath.String(), ok, keyPath.String())
	}
}

func TestLowerWIRClosureRootAssignmentPublishesExpressionSourceWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function f(): ()
    local out = function(): string
        return "ok"
    end
end`)
	body := wirlower.Lower("closure-root-assignment-no-sidecars", fn.Stmts, bindings, built)
	facts := Lower(nil, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	stmt := fn.Stmts[0].(*ast.LocalAssignStmt)
	outPath := path.NewPath(mustLocalAt(t, bindings, stmt, 0), "out")
	point := requireStmtPoints(t, built, stmt, 1)[0]
	assign, ok := facts.RootAssignment(point)
	if !ok {
		t.Fatalf("missing WIR no-sidecar closure root assignment at point %d", point)
	}
	if got := assign.TargetPath(); !got.Equal(outPath) {
		t.Fatalf("closure root assignment target = %s, want %s", got.String(), outPath.String())
	}
	source := assign.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		t.Fatalf("closure root assignment source = %#v, want expression source", source)
	}
	wantFn, ok := bindings.FunctionSymbol(stmt.Exprs[0].(*ast.FunctionExpr))
	if !ok {
		t.Fatal("missing function symbol for closure expression")
	}
	gotFn, ok := facts.ExpressionFunction(source.ExprRef)
	if !ok || gotFn != wantFn {
		t.Fatalf("closure expression function = %v/%v, want %v", gotFn, ok, wantFn)
	}
	assertExpressionValue(t, facts, source.ExprRef, presence.Present(), runtimekind.Singleton(runtimekind.Function))
}

func assertWIRPathSource(t *testing.T, source factflow.ValueSource, want path.Path) {
	t.Helper()
	if source.Kind != factflow.ValueSourcePath || source.PathKey != want.Key() || source.HasExpr || source.ExprRef != 0 {
		t.Fatalf("source = %#v, want WIR path source %s without expression shim", source, want.Key())
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

func TestAssignmentCallResultExprRefComesFromWIRCallIdentity(t *testing.T) {
	body := wir.NewBody("call-result-expr")
	callPoint := cfg.Point(10)
	assignPoint := cfg.Point(11)
	callResult := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	callStart := body.Emit(wir.Instruction{
		Op:      wir.OpCall,
		Point:   callPoint,
		Results: body.AppendOperands([]wir.Operand{callResult}),
		ExprID:  wir.ExpressionID(77),
	})
	body.SetPointRange(callPoint, callStart, body.Len())
	assignStart := body.Emit(wir.Instruction{
		Op:    wir.OpAssign,
		Point: assignPoint,
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(path.NewPath(1, "target")))},
		A:     callResult,
	})
	body.SetPointRange(assignPoint, assignStart, body.Len())

	l := lowerer{
		wir:   body,
		exprs: make(map[any]factflow.ExprRef),
	}
	source, ok := l.assignmentSourceFromWIR(assignPoint, sourceprovenance.ASTSource{Final: true})
	if !ok {
		t.Fatal("assignmentSourceFromWIR returned false")
	}
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.CallPoint != callPoint ||
		source.ResultIndex != 0 || !source.HasExpr || source.ExprRef == 0 {
		t.Fatalf("assignment source = %#v, want WIR call result with WIR-derived expression ref", source)
	}
}
