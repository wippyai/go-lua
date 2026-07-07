package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerWithWIRReturnPointsPublishReturnPresence(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(ok: boolean): (string?, string?)
    if ok then
        return "value", nil
    end
    return nil, "error"
end
`)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	wirPoints := wirReturnFactPoints(built.Graph, body)
	if len(wirPoints) != 2 {
		t.Fatalf("WIR return points = %#v, want two returns", wirPoints)
	}
	for _, point := range wirPoints {
		relations := wirFacts.ReturnPresenceRelations(point)
		assertReturnPresenceRelation(t, relations, 0, presence.Present(), 1, presence.Absent())
		assertReturnPresenceRelation(t, relations, 0, presence.Absent(), 1, presence.Present())
		assertReturnPresenceRelation(t, relations, 1, presence.Present(), 0, presence.Absent())
		assertReturnPresenceRelation(t, relations, 1, presence.Absent(), 0, presence.Present())
	}
}

func TestLowerWithWIRReturnPresenceArityComesFromWIRDeclaredReturns(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): (string?, string?)
    return nil
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 1)
	body := wirlower.LowerFunction("f", fn, bindings, built)
	if got := body.DeclaredReturnArity(); got != 2 {
		t.Fatalf("WIR declared return arity = %d, want 2", got)
	}
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	relations := facts.ReturnPresenceRelations(points[0])
	assertReturnPresenceRelation(t, relations, 0, presence.Absent(), 1, presence.Absent())
	assertReturnPresenceRelation(t, relations, 1, presence.Absent(), 0, presence.Absent())
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

func TestLowerWithWIRReturnLogicalDefaultCarriesComputedWitness(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(level: string?): string
    return level or "info"
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 1)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})

	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want one logical expression source", sources)
	}
	value, ok := facts.ExpressionValue(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing WIR logical return expression value for ref %d", sources[0].ExprRef)
	}
	got, ok := product.Get(reg, value, typewitness.Key).Type()
	if !ok {
		t.Fatalf("logical return witness = %#v, want computed union", product.Get(reg, value, typewitness.Key))
	}
	want := typ.String
	if !typ.TypeEquals(got, want) {
		t.Fatalf("logical return witness = %v, want %v", got, want)
	}
}

func TestLowerWithWIRNestedLogicalReturnCarriesComputedStringWitness(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function label(success: boolean, value: any): string
    return success and "ok" or (type(value) == "string" and ("value:" .. value) or "failed")
end
`, "type")
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 1)
	body := wirlower.Lower("nested-logical-return", fn.Stmts, bindings, built)
	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})

	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want one logical expression source\nWIR:\n%s", sources, wir.Print(body, built.Graph))
	}
	value, ok := facts.ExpressionValue(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing nested logical return expression value for ref %d\nWIR:\n%s", sources[0].ExprRef, wir.Print(body, built.Graph))
	}
	got, ok := product.Get(reg, value, typewitness.Key).Type()
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("nested logical return witness = %v/%v, want string; value=%#v\nWIR:\n%s", got, ok, value, wir.Print(body, built.Graph))
	}
}

func TestLowerWithWIRNegatedNilLogicalReturnCarriesComputedStringWitness(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function label(value: string?): string
    return not (value == nil) and ("value:" .. value) or ""
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 1)
	body := wirlower.Lower("negated-nil-logical-return", fn.Stmts, bindings, built)
	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})

	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want one logical expression source\nWIR:\n%s", sources, wir.Print(body, built.Graph))
	}
	value, ok := facts.ExpressionValue(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing negated nil logical return expression value for ref %d\nWIR:\n%s", sources[0].ExprRef, wir.Print(body, built.Graph))
	}
	got, ok := product.Get(reg, value, typewitness.Key).Type()
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("negated nil logical return witness = %v/%v, want string; value=%#v\nWIR:\n%s", got, ok, value, wir.Print(body, built.Graph))
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

func TestLowerWithWIRNaryConcatReturnCarriesProjectedStringValue(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(req: {method: string, path: string}): string
    return "Not found: " .. req.method .. " " .. req.path
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 1)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})

	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want WIR n-ary concat expression source", sources)
	}
	value, ok := facts.ExpressionValue(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing WIR n-ary concat expression value for ref %d", sources[0].ExprRef)
	}
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("WIR n-ary concat value type = %v/%v, want string", got, ok)
	}
}

func TestLowerWithWIRUnaryLengthReturnCarriesProjectedIntegerValue(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(req: {items: {string}}, other: {items: {number}}): integer
    return #req.items
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 1)
	otherPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "other").Field("items")
	body := wir.NewBody("synthetic-unary-length-return")
	temp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	operand := wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))}
	start := body.Emit(wir.Instruction{Op: wir.OpUnOp, Point: points[0], Dst: temp, A: operand, Operator: wir.UnLen})
	body.Emit(wir.Instruction{Op: wir.OpReturn, Point: points[0], List: body.AppendOperands([]wir.Operand{temp})})
	body.SetPointRange(points[0], start, body.Len())
	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})

	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want WIR unary expression source", sources)
	}
	op, ok := facts.ExpressionOperation(sources[0].ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationUnary || op.Op() != "#" {
		t.Fatalf("WIR unary operation = %#v/%v, want length", op, ok)
	}
	left := op.Left()
	if left.Kind != factflow.ValueSourceExpression || !left.HasExpr {
		t.Fatalf("WIR unary operand source = %#v, want expression-backed path", left)
	}
	gotPath, ok := facts.ExpressionPath(left.ExprRef)
	if !ok || !gotPath.Equal(otherPath) {
		t.Fatalf("WIR unary operand path = %v/%v, want %v", gotPath, ok, otherPath)
	}
	value, ok := facts.ExpressionValue(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing WIR unary length expression value for ref %d", sources[0].ExprRef)
	}
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("WIR unary length value type = %v/%v, want integer", got, ok)
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

func TestLowerWithWIRReturnMalformedOperandDoesNotFallbackToSemanticSource(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): string
    return value
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, ret, 1)[0]
	body := wir.NewBody("malformed-return-source")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpReturn,
		Point: point,
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandNone}}),
	})
	body.SetPointRange(point, start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	retFact, ok := facts.Return(point)
	if !ok {
		t.Fatalf("missing WIR-owned return at point %d", point)
	}
	sources := retFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceUnknown {
		t.Fatalf("return sources = %#v, want unknown instead of semantic fallback", sources)
	}
}

func TestLowerWithWIRReturnMissingTempDoesNotFallbackToSemanticSource(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): string
    return value
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, ret, 1)[0]
	body := wir.NewBody("missing-temp-return-source")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpReturn,
		Point: point,
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandTemp, Ref: 999}}),
	})
	body.SetPointRange(point, start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	retFact, ok := facts.Return(point)
	if !ok {
		t.Fatalf("missing WIR-owned return at point %d", point)
	}
	sources := retFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceUnknown {
		t.Fatalf("return sources = %#v, want unknown instead of semantic fallback", sources)
	}
}

func TestLowerWithWIRReturnedTypePredicateCarriesExpressionCondition(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value)
    return type(value) == "number"
end
`, "type")
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, ret, 1)[0]
	body := wirlower.Lower("returned-type-predicate", fn.Stmts, bindings, built)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	retFact, ok := facts.Return(point)
	if !ok {
		t.Fatalf("missing WIR-owned return at point %d", point)
	}
	sources := retFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want expression source\nWIR:\n%s", sources, wir.Print(body, built.Graph))
	}
	condition, ok := facts.ExpressionCondition(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing expression condition for returned predicate ref %d", sources[0].ExprRef)
	}
	valuePath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "value")
	trueFacts := condition.FactsForValue(true)
	for _, refinement := range trueFacts.Refinements() {
		if !refinement.TargetPath().Equal(valuePath) {
			continue
		}
		assertValueRefinement(t, "returned type predicate true refinement", refinement.Value(), valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Number),
			hasRuntimeKind: true,
		})
		return
	}
	t.Fatalf("missing true-value type refinement for %s; got %#v", valuePath, trueFacts.Refinements())
}

func TestLowerWithWIRReturnedConjunctionCarriesLeftExpressionCondition(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value)
    return type(value) == "number" and value > 0
end
`, "type")
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, ret, 1)[0]
	body := wirlower.Lower("returned-conjunction-predicate", fn.Stmts, bindings, built)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	retFact, ok := facts.Return(point)
	if !ok {
		t.Fatalf("missing WIR-owned return at point %d", point)
	}
	sources := retFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want expression source\nWIR:\n%s", sources, wir.Print(body, built.Graph))
	}
	condition, ok := facts.ExpressionCondition(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing expression condition for returned conjunction ref %d\nWIR:\n%s", sources[0].ExprRef, wir.Print(body, built.Graph))
	}
	valuePath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "value")
	trueFacts := condition.FactsForValue(true)
	for _, refinement := range trueFacts.Refinements() {
		if !refinement.TargetPath().Equal(valuePath) {
			continue
		}
		assertValueRefinement(t, "returned conjunction true refinement", refinement.Value(), valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Number),
			hasRuntimeKind: true,
		})
		return
	}
	t.Fatalf("missing true-value type refinement for %s; got %#v\nWIR:\n%s", valuePath, trueFacts.Refinements(), wir.Print(body, built.Graph))
}

func TestLowerWithWIRReturnedNestedPathConjunctionCarriesRootPresence(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(entry: {id: string, meta: {type: string?}?}?): boolean?
    return entry and entry.meta and entry.meta.type == "agent.gen1"
end
`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, ret, 1)[0]
	body := wirlower.Lower("returned-nested-path-conjunction", fn.Stmts, bindings, built)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	retFact, ok := facts.Return(point)
	if !ok {
		t.Fatalf("missing WIR-owned return at point %d", point)
	}
	sources := retFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want expression source\nWIR:\n%s", sources, wir.Print(body, built.Graph))
	}
	condition, ok := facts.ExpressionCondition(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing expression condition for returned nested path conjunction ref %d\nWIR:\n%s", sources[0].ExprRef, wir.Print(body, built.Graph))
	}
	entryPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "entry")
	trueFacts := condition.FactsForValue(true)
	for _, refinement := range trueFacts.Refinements() {
		if !refinement.TargetPath().Equal(entryPath) {
			continue
		}
		assertValueRefinement(t, "returned nested path conjunction root presence", refinement.Value(), valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Table),
			hasRuntimeKind: true,
		})
		return
	}
	t.Fatalf("missing true-value presence refinement for %s; got %#v\nWIR:\n%s", entryPath, trueFacts.Refinements(), wir.Print(body, built.Graph))
}

func TestLowerWithWIRReturnPublishesWithoutSemanticReturnView(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): string
    local value = "semantic assignment point"
    return value
end
`)
	local := fn.Stmts[0].(*ast.LocalAssignStmt)
	points := requireStmtPoints(t, built, local, 1)
	point := points[0]
	if _, ok := result.ReturnView(point); ok {
		t.Fatalf("point %d unexpectedly has semantic return view", point)
	}

	body := wir.NewBody("synthetic-return-on-non-return-point")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpReturn,
		Point: point,
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandConst, Ref: uint32(body.InternConst(wir.Const{Kind: wir.ConstString, Str: "wir"}))}}),
	})
	body.SetPointRange(point, start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	returnFact, ok := facts.Return(point)
	if !ok {
		t.Fatalf("missing WIR-owned return at point %d without semantic ReturnView", point)
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceLiteral ||
		sources[0].LiteralKind != factflow.ValueSourceLiteralString || sources[0].String != "wir" {
		t.Fatalf("WIR return sources = %#v, want literal wir", sources)
	}
}
