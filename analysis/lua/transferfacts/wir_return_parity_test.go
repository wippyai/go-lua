package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerWithWIRReturnPointsMatchesSidecarReturnPresence(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(ok: boolean): (string?, string?)
    if ok then
        return "value", nil
    end
    return nil, "error"
end
`)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	sidecarPoints := semanticReturnFactPoints(built.Graph, result)
	wirPoints := wirReturnFactPoints(built.Graph, body)
	if !reflect.DeepEqual(wirPoints, sidecarPoints) {
		t.Fatalf("return points mismatch\n got: %#v\nwant: %#v", wirPoints, sidecarPoints)
	}
	for _, point := range built.Graph.RPO() {
		got := wirFacts.ReturnPresenceRelations(point)
		want := sidecarFacts.ReturnPresenceRelations(point)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("return presence at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
		}
	}
}

func TestLowerWithWIRReturnSourcesForNonExpressionOperands(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(...): (nil, any, any)
    return nil, produce(), ...
end
`, "produce")
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 2)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	returnFact, ok := facts.Return(points[1])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[1])
	}
	sources := returnFact.Sources()
	if len(sources) != 3 {
		t.Fatalf("return sources = %#v, want three", sources)
	}
	if sources[0].Kind != factflow.ValueSourceNil || sources[0].TargetIndex != 0 {
		t.Fatalf("nil return source = %#v", sources[0])
	}
	if sources[1].Kind != factflow.ValueSourceCall || sources[1].HasExpr || sources[1].CallPoint != points[0] ||
		!sources[1].HasCallPoint || sources[1].ResultIndex != 0 || !sources[1].Adjusted || sources[1].Expanded || sources[1].OpenTail {
		t.Fatalf("call return source = %#v, want adjusted direct call result at point %d", sources[1], points[0])
	}
	if sources[2].Kind != factflow.ValueSourceVararg || sources[2].HasExpr || !sources[2].Final ||
		!sources[2].Expanded || !sources[2].OpenTail {
		t.Fatalf("vararg return source = %#v, want open final vararg", sources[2])
	}
}

func TestLowerWithWIRReturnSourcesForScalarLiterals(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): (boolean, number, string)
    return false, 42, "ready"
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 1)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 3 {
		t.Fatalf("return sources = %#v, want three", sources)
	}
	if sources[0].Kind != factflow.ValueSourceLiteral || sources[0].LiteralKind != factflow.ValueSourceLiteralBool || sources[0].Bool {
		t.Fatalf("bool return source = %#v, want false literal", sources[0])
	}
	if sources[1].Kind != factflow.ValueSourceLiteral || sources[1].LiteralKind != factflow.ValueSourceLiteralInteger || sources[1].Int != 42 {
		t.Fatalf("number return source = %#v, want 42 literal", sources[1])
	}
	if sources[2].Kind != factflow.ValueSourceLiteral || sources[2].LiteralKind != factflow.ValueSourceLiteralString || sources[2].String != "ready" {
		t.Fatalf("string return source = %#v, want ready literal", sources[2])
	}
}

func TestLowerWithWIRReturnSourcesForRootPath(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): string
    return value
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 1)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourcePath || sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want WIR path source without expression ref", sources)
	}
}

func TestLowerWithWIRReturnSourcesForSegmentedPath(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): string
    local value = { name = "x" }
    local other = { name = "y" }
    return value.name
end
`)
	ret, ok := fn.Stmts[2].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, ret, 1)
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value").Field("name")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other").Field("name")
	body := wir.NewBody("synthetic-segmented-return")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpReturn,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))}}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want expression-backed WIR path", sources)
	}
	gotPath, ok := facts.ExpressionPath(sources[0].ExprRef)
	if !ok || !gotPath.Equal(otherPath) || gotPath.Equal(valuePath) {
		t.Fatalf("return source path = %v/%v, want WIR path %v not semantic path %v", gotPath, ok, otherPath, valuePath)
	}
}

func TestLowerWithWIRReturnSourcesUseLocalRootPath(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): any
    local builder = {}
    return builder
end
`)
	ret, ok := fn.Stmts[1].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[1])
	}
	points := requireStmtPoints(t, built, ret, 1)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourcePath || sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want WIR local path source without expression ref", sources)
	}
}

func TestLowerWithWIRReturnSourcesUsesTempExpressionOperands(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): string
    return value .. "!"
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 1)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want WIR temp expression source", sources)
	}
	op, ok := facts.ExpressionOperation(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing WIR temp expression operation for ref %d", sources[0].ExprRef)
	}
	if op.Kind() != factflow.ExpressionOperationBinary || op.Op() != ".." {
		t.Fatalf("WIR temp expression operation = %#v, want concat", op)
	}
}

func TestLowerWithWIRReturnSourcesDerivesTempExpressionFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): string
    return value .. "!"
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 1)
	body := wir.NewBody("synthetic-return")
	temp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	left := wir.Operand{Kind: wir.OperandConst, Ref: uint32(body.InternConst(wir.Const{Kind: wir.ConstString, Str: "left"}))}
	right := wir.Operand{Kind: wir.OperandConst, Ref: uint32(body.InternConst(wir.Const{Kind: wir.ConstString, Str: "right"}))}
	start := body.Emit(wir.Instruction{Op: wir.OpConcat, Point: points[0], Dst: temp, A: left, B: right})
	body.Emit(wir.Instruction{Op: wir.OpReturn, Point: points[0], List: body.AppendOperands([]wir.Operand{temp})})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want WIR temp expression source", sources)
	}
	op, ok := facts.ExpressionOperation(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing WIR temp expression operation for ref %d", sources[0].ExprRef)
	}
	if op.Kind() != factflow.ExpressionOperationBinary || op.Op() != ".." {
		t.Fatalf("WIR temp expression operation = %#v, want concat", op)
	}
	if left := op.Left(); left.Kind != factflow.ValueSourceLiteral || left.LiteralKind != factflow.ValueSourceLiteralString || left.String != "left" {
		t.Fatalf("WIR temp concat left source = %#v, want literal left", left)
	}
}

func TestLowerWithWIRReturnTempExpressionUsesSegmentedPathOperand(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): string
    local value = { name = "x" }
    local other = { name = "y" }
    return value.name .. "!"
end
`)
	ret, ok := fn.Stmts[2].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, ret, 1)
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value").Field("name")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other").Field("name")
	body := wir.NewBody("synthetic-segmented-return-temp")
	temp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	left := wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))}
	right := wir.Operand{Kind: wir.OperandConst, Ref: uint32(body.InternConst(wir.Const{Kind: wir.ConstString, Str: "!"}))}
	start := body.Emit(wir.Instruction{Op: wir.OpConcat, Point: points[0], Dst: temp, List: body.AppendOperands([]wir.Operand{left, right})})
	body.Emit(wir.Instruction{Op: wir.OpReturn, Point: points[0], List: body.AppendOperands([]wir.Operand{temp})})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want WIR temp expression source", sources)
	}
	op, ok := facts.ExpressionOperation(sources[0].ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationBinary || op.Op() != ".." {
		t.Fatalf("WIR temp expression operation = %#v/%v, want concat", op, ok)
	}
	leftSource := op.Left()
	if leftSource.Kind != factflow.ValueSourceExpression || !leftSource.HasExpr {
		t.Fatalf("WIR concat left source = %#v, want expression-backed WIR path", leftSource)
	}
	gotPath, ok := facts.ExpressionPath(leftSource.ExprRef)
	if !ok || !gotPath.Equal(otherPath) || gotPath.Equal(valuePath) {
		t.Fatalf("WIR concat left source path = %v/%v, want WIR path %v not semantic path %v", gotPath, ok, otherPath, valuePath)
	}
}

func TestLowerWithWIRReturnSourcesDoesNotFallbackWhenReturnInstructionMissing(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): string
    return value
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 1)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: wir.NewBody("empty")})

	if _, ok := facts.Return(points[0]); ok {
		t.Fatalf("WIR mode return at point %d fell back to semantic sidecar", points[0])
	}
}
