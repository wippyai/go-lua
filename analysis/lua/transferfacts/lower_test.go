package transferfacts

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func lowerFacts(t *testing.T, result *semantics.Result, graph cfg.Graph, reg *axis.Registry) factflow.Facts {
	t.Helper()
	if reg == nil {
		t.Fatal("lowerFacts requires a registry")
	}
	return Lower(graph, Config{Registry: reg})
}

func TestLowerLiteralExpressionValues(t *testing.T) {
	nilLocal := localAssign([]string{"missing"}, &ast.NilExpr{})
	numberLocal := localAssign([]string{"count"}, number("7"))
	tableLocal := localAssign([]string{"box"}, &ast.TableExpr{})
	stmts := []ast.Stmt{nilLocal, numberLocal, tableLocal}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	_, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	body := wirlower.Lower("literal-expression-values", stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	nilAssign, ok := facts.LocalAssignment(requireStmtPoints(t, built, nilLocal, 1)[0])
	if !ok {
		t.Fatalf("missing nil local assignment")
	}
	nilSource := nilAssign.Source()
	numberAssign, ok := facts.LocalAssignment(requireStmtPoints(t, built, numberLocal, 1)[0])
	if !ok {
		t.Fatalf("missing number local assignment")
	}
	numberSource := numberAssign.Source()
	tableSource := mustLocalSource(t, facts, requireStmtPoints(t, built, tableLocal, 1)[0])

	if nilSource.Kind != factflow.ValueSourceNil {
		t.Fatalf("nil local source = %#v, want nil source", nilSource)
	}
	if numberSource.Kind != factflow.ValueSourceLiteral || numberSource.LiteralKind != factflow.ValueSourceLiteralInteger || numberSource.Int != 7 {
		t.Fatalf("number local source = %#v, want integer literal 7", numberSource)
	}
	assertExpressionValue(t, facts, tableSource.ExprRef, presence.Present(), runtimekind.Singleton(runtimekind.Table))
}

func TestLowerLogicalDefaultExpressionValueKeepsComputedWitness(t *testing.T) {
	stmts, bindings, built, _ := parseSemanticChunk(t, `
type Level = "debug" | "info"
local level: Level? = nil
local selected = level or "info"
`)
	reg := standard.Registry()
	body := wirlower.Lower("logical-default-expression", stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	selected, ok := stmts[2].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want selected local assignment", stmts[2])
	}
	var source factflow.ValueSource
	for _, point := range built.StmtPoints.PointsFor(selected) {
		if fact, ok := facts.LocalAssignment(point); ok {
			source = fact.Source()
			break
		}
	}
	if !source.HasExpr {
		t.Fatal("missing selected expression source")
	}
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		t.Fatalf("missing expression value for ref %d", source.ExprRef)
	}
	witness := product.Get(reg, value, typewitness.Key)
	got, ok := witness.Type()
	if !ok {
		t.Fatalf("logical default witness = %#v, want computed literal union", witness)
	}
	want := typeexpr.Union(typ.LiteralString("debug"), typ.LiteralString("info"))
	if !typ.TypeEquals(got, want) {
		t.Fatalf("logical default witness = %v, want %v", got, want)
	}
}

func TestLowerReturnExpressionOperationUsesNestedCallSource(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, _ := parseSemanticFunction(t, `
function f(user)
	return user.id .. ":" .. tostring(user.retries)
end`, "tostring")
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}

	body := wirlower.Lower("return-concat-operation", fn.Stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	returnPoints := requireStmtPoints(t, built, ret, 2)
	returnFact, ok := facts.Return(returnPoints[1])
	if !ok {
		t.Fatalf("missing return fact at point %d", returnPoints[1])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want one expression source", sources)
	}
	op, ok := facts.ExpressionOperation(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing expression operation for return expr ref %d", sources[0].ExprRef)
	}
	if op.Kind() != factflow.ExpressionOperationBinary || op.Op() != ".." {
		t.Fatalf("operation = %v %q, want binary concat", op.Kind(), op.Op())
	}
	right := op.Right()
	if right.Kind != factflow.ValueSourceCall || right.CallPoint != returnPoints[0] || !right.HasCallPoint {
		t.Fatalf("operation right source = %#v, want tostring call at point %d", right, returnPoints[0])
	}
}

func TestLowerReturnLengthComparisonUsesNestedUnarySource(t *testing.T) {
	reg := standard.Registry()
	fn, bindings, built, _ := parseSemanticFunction(t, `
function f(bindings)
	return #bindings > 0
end`)
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}

	body := wirlower.Lower("return-length-comparison", fn.Stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	returnPoints := requireStmtPoints(t, built, ret, 1)
	returnFact, ok := facts.Return(returnPoints[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", returnPoints[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want one expression source", sources)
	}
	op, ok := facts.ExpressionOperation(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing expression operation for return expr ref %d", sources[0].ExprRef)
	}
	if op.Kind() != factflow.ExpressionOperationBinary || op.Op() != ">" {
		t.Fatalf("operation = %v %q, want binary comparison", op.Kind(), op.Op())
	}
	left := op.Left()
	if left.Kind != factflow.ValueSourceExpression || !left.HasExpr {
		t.Fatalf("operation left source = %#v, want nested length expression", left)
	}
	nested, ok := facts.ExpressionOperation(left.ExprRef)
	if !ok {
		t.Fatalf("missing nested length operation for expr ref %d", left.ExprRef)
	}
	if nested.Kind() != factflow.ExpressionOperationUnary || nested.Op() != "#" {
		t.Fatalf("nested operation = %v %q, want unary length", nested.Kind(), nested.Op())
	}
	if nested.Left().Kind != factflow.ValueSourcePath || nested.Left().PathKey == "" {
		t.Fatalf("nested operand source = %#v, want direct path source", nested.Left())
	}
}

func TestLowerAnnotatedFunctionExpressionValueWitness(t *testing.T) {
	stmts, bindings, built, _ := parseSemanticChunk(t, `
local cb = function(item: string): number
    return 1
end
`)
	reg := standard.Registry()
	body := wirlower.Lower("annotated-function-expression", stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	point := requireStmtPoints(t, built, stmts[0], 1)[0]
	source := mustLocalSource(t, facts, point)
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		t.Fatalf("missing expression value for ref %d", source.ExprRef)
	}
	witness := product.Get(reg, value, typewitness.Key)
	got, ok := witness.Type()
	if !ok {
		t.Fatalf("function expression witness = %#v, want concrete function type", witness)
	}
	want := typ.Func().Param("item", typ.String).Returns(typ.Number).Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("function expression witness = %v, want %v", got, want)
	}
	fn, ok := stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("local source expr = %T, want function", stmts[0].(*ast.LocalAssignStmt).Exprs[0])
	}
	fnSymbol, ok := bindings.FunctionSymbol(fn)
	if !ok || fnSymbol == 0 {
		t.Fatalf("function symbol = %d/%v, want non-zero", fnSymbol, ok)
	}
	gotID, ok := product.Get(reg, value, identity.Key).ID()
	if !ok || gotID != identity.LuaFunction(uint64(fnSymbol)) {
		t.Fatalf("function expression identity = %v/%v, want %v", gotID, ok, identity.LuaFunction(uint64(fnSymbol)))
	}
}

func TestLowerReturnedFunctionExpressionUsesUniqueCallableReturnArm(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function make(): ((value: string) -> string) | false
    return function(value)
        return value
    end
end
`)
	reg := standard.Registry()
	body := wirlower.LowerFunction("make", fn, bindings, built)
	facts := Lower(built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, ret, 1)[0]
	returnFact, ok := facts.Return(point)
	if !ok {
		t.Fatalf("missing return fact at point %d", point)
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want one function expression source", sources)
	}
	value, ok := facts.ExpressionValue(sources[0].ExprRef)
	if !ok {
		t.Fatalf("missing expression value for ref %d", sources[0].ExprRef)
	}
	witness := product.Get(reg, value, typewitness.Key)
	got, ok := witness.Type()
	if !ok {
		t.Fatalf("function expression witness = %#v, want contextual return callable", witness)
	}
	want := typ.Func().Param("value", typ.String).Returns(typ.String).Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("function expression witness = %v, want %v", got, want)
	}
}

func TestLowerWIRClosureArgumentCarriesFunctionTypeWitness(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function f(): ()
    send(function(item: string): number
        return 1
    end)
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, stmt, 1)[0]
	reg := standard.Registry()
	wirBody := wirlower.Lower("closure-arg", fn.Stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: reg, Bindings: bindings, WIR: wirBody})
	site, ok := facts.CallSite(point)
	if !ok {
		t.Fatalf("missing call site at point %d", point)
	}
	arg, ok := site.ArgumentSourceAt(0)
	if !ok || arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		t.Fatalf("argument source = %#v/%v, want closure expression", arg, ok)
	}
	value, ok := facts.ExpressionValue(arg.ExprRef)
	if !ok {
		t.Fatalf("missing expression value for ref %d", arg.ExprRef)
	}
	witness := product.Get(reg, value, typewitness.Key)
	got, ok := witness.Type()
	if !ok {
		t.Fatalf("function argument witness = %#v, want concrete function type", witness)
	}
	want := typ.Func().Param("item", typ.String).Returns(typ.Number).Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("function argument witness = %v, want %v", got, want)
	}
}

func TestLowerDynamicIndexReadExpressionValueUsesRuntimeIndexType(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local users: {[string]: {id: string}} = {}
local id: string = "u1"
local user = users[id]
local maybe_users: {[string]: {id: string}}? = users
local maybe_user = maybe_users[id]
`)
	reg := standard.Registry()
	facts := lowerChunkFactsWithWIR(t, "dynamic-index-read-expression-value", stmts, result, built, bindings, reg)
	local := mustLocalStmt(t, stmts, 2)
	point := requireStmtPoints(t, built, local, 1)[0]
	source := mustLocalSource(t, facts, point)
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		t.Fatalf("missing expression value for dynamic index ref %d", source.ExprRef)
	}
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Maybe()) {
		t.Fatalf("dynamic index read presence = %s, want maybe", got)
	}
	got, ok := typevalue.TypeOf(reg, value)
	want := typeexpr.Optional(typetable.NewRecord().Field("id", typ.String).Build())
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("dynamic index read type = %v/%v, want %v", got, ok, want)
	}

	optionalLocal := mustLocalStmt(t, stmts, 4)
	optionalPoint := requireStmtPoints(t, built, optionalLocal, 1)[0]
	optionalSource := mustLocalSource(t, facts, optionalPoint)
	optionalValue, ok := facts.ExpressionValue(optionalSource.ExprRef)
	if !ok {
		t.Fatalf("missing expression value for optional dynamic index ref %d", optionalSource.ExprRef)
	}
	optionalGot, ok := typevalue.TypeOf(reg, optionalValue)
	if !ok || !typ.TypeEquals(optionalGot, want) {
		t.Fatalf("optional dynamic index read type = %v/%v, want %v", optionalGot, ok, want)
	}
}

func TestLowerChainedDynamicIndexReadExpressionValueProjectsMemberThenIndex(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local function meta(): {tags: {[string]: string}?}
    return { tags = { source = "fixture" } }
end
	local source = meta().tags["source"]
`)
	reg := standard.Registry()
	facts := lowerChunkFactsWithWIR(t, "chained-dynamic-index-read-expression-value", stmts, result, built, bindings, reg)
	local := mustLocalStmt(t, stmts, 1)
	var point cfg.Point
	for _, candidate := range requireStmtPoints(t, built, local, 2) {
		if _, ok := facts.RootAssignment(candidate); ok {
			point = candidate
			break
		}
	}
	if point == 0 {
		t.Fatalf("missing root assignment for chained dynamic index local")
	}
	source := mustLocalSource(t, facts, point)
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		t.Fatalf("missing expression value for chained dynamic index ref %d", source.ExprRef)
	}
	got, ok := typevalue.TypeOf(reg, value)
	want := typeexpr.Optional(typ.String)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("chained dynamic index read type = %v/%v, want %v", got, ok, want)
	}
}

func TestLowerLogicalExpressionValueUsesDeclaredMemberPathTypes(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Chunk = { type: string }

local chunk: Chunk = { type = "error" }
local target_pid: string = "pid"
local target_topic: string = "topic"
local ok = chunk.type == "error" and target_pid ~= "" and target_topic ~= ""
`)
	reg := standard.Registry()
	facts := lowerChunkFactsWithWIR(t, "logical-expression-declared-member-path", stmts, result, built, bindings, reg)
	local := mustLocalStmt(t, stmts, 4)
	point := requireStmtPoints(t, built, local, 1)[0]
	source := mustLocalSource(t, facts, point)
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		t.Fatalf("missing expression value for logical ref %d", source.ExprRef)
	}
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, typ.Boolean) {
		t.Fatalf("logical expression type = %v/%v, want boolean", got, ok)
	}
}

func TestLowerUnannotatedFunctionExpressionValueCarriesIdentity(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local cb = function(item)
    return item
end
`)
	reg := standard.Registry()
	facts := lowerChunkFactsWithWIR(t, "unannotated-function-expression", stmts, result, built, bindings, reg)
	point := requireStmtPoints(t, built, stmts[0], 1)[0]
	source := mustLocalSource(t, facts, point)
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		t.Fatalf("missing expression value for ref %d", source.ExprRef)
	}
	fn, ok := stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("local source expr = %T, want function", stmts[0].(*ast.LocalAssignStmt).Exprs[0])
	}
	fnSymbol, ok := bindings.FunctionSymbol(fn)
	if !ok || fnSymbol == 0 {
		t.Fatalf("function symbol = %d/%v, want non-zero", fnSymbol, ok)
	}
	gotID, ok := product.Get(reg, value, identity.Key).ID()
	if !ok || gotID != identity.LuaFunction(uint64(fnSymbol)) {
		t.Fatalf("function expression identity = %v/%v, want %v", gotID, ok, identity.LuaFunction(uint64(fnSymbol)))
	}
	if kind := product.Get(reg, value, runtimekind.Key); !kind.Contains(runtimekind.Function) {
		t.Fatalf("function expression runtime kind = %v, want function", kind)
	}
}

func TestLowerMemberFunctionDefinitionPublishesPathFunctionValue(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		rootName string
		field    string
	}{
		{
			name: "dotted",
			source: `
local M = {}
function M.run()
    return 1
end
`,
			rootName: "M",
			field:    "run",
		},
		{
			name: "method",
			source: `
local obj = {}
function obj:method(value)
    return value
end
`,
			rootName: "obj",
			field:    "method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts, bindings, built, result := parseSemanticChunk(t, tt.source)
			decl := mustLocalStmt(t, stmts, 0)
			def, ok := stmts[1].(*ast.FuncDefStmt)
			if !ok || def.Func == nil {
				t.Fatalf("statement = %T, want function definition", stmts[1])
			}
			reg := standard.Registry()
			facts := lowerChunkFactsWithWIR(t, "member-function-definition", stmts, result, built, bindings, reg)

			point := requireStmtPoints(t, built, def, 1)[0]
			wantPath := path.NewPath(mustLocalAt(t, bindings, decl, 0), tt.rootName).Field(tt.field)
			pathFact, ok := facts.PathAssignment(point)
			if !ok {
				t.Fatalf("missing path assignment at point %d", point)
			}
			if !pathFact.TargetPath().Equal(wantPath) {
				t.Fatalf("path assignment target = %v, want %v", pathFact.TargetPath(), wantPath)
			}
			staticWrite, ok := facts.PathStaticMemberWrite(point)
			if !ok {
				t.Fatalf("missing static member write at point %d", point)
			}
			if !staticWrite.TargetPath().Equal(wantPath) || staticWrite.Source() != pathFact.Source() {
				t.Fatalf("static member write = %v %#v, want %v same source", staticWrite.TargetPath(), staticWrite.Source(), wantPath)
			}
			if _, ok := facts.OrdinaryAssignment(point); ok {
				t.Fatalf("member function definition also lowered as root assignment")
			}

			source := pathFact.Source()
			if source.Kind != factflow.ValueSourceExpression || !source.HasExpr || source.ExprRef == 0 {
				t.Fatalf("path assignment source = %#v, want function expression source", source)
			}
			fnSymbol, ok := bindings.FunctionSymbol(def.Func)
			if !ok || fnSymbol == 0 {
				t.Fatalf("function symbol = %d/%v, want non-zero", fnSymbol, ok)
			}
			if got, ok := facts.ExpressionFunction(source.ExprRef); !ok || got != fnSymbol {
				t.Fatalf("expression function = %d/%v, want %d", got, ok, fnSymbol)
			}
			value, ok := facts.ExpressionValue(source.ExprRef)
			if !ok {
				t.Fatalf("missing expression value for ref %d", source.ExprRef)
			}
			if kind := product.Get(reg, value, runtimekind.Key); !kind.Contains(runtimekind.Function) {
				t.Fatalf("function definition value runtime kind = %v, want function", kind)
			}
			gotID, ok := product.Get(reg, value, identity.Key).ID()
			if !ok || gotID != identity.LuaFunction(uint64(fnSymbol)) {
				t.Fatalf("function definition identity = %v/%v, want %v", gotID, ok, identity.LuaFunction(uint64(fnSymbol)))
			}
		})
	}
}

func TestLowerRootFunctionDefinitionPublishesFunctionValue(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
function run()
    return 1
end
`)
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("statement = %T, want function definition", stmts[0])
	}
	reg := standard.Registry()
	facts := lowerChunkFactsWithWIR(t, "root-function-definition", stmts, result, built, bindings, reg)

	point := requireStmtPoints(t, built, def, 1)[0]
	rootFact, ok := facts.RootAssignment(point)
	if !ok {
		t.Fatalf("missing root assignment at point %d", point)
	}
	if rootFact.TargetSymbol() == 0 {
		t.Fatalf("root function definition target symbol missing")
	}
	source := rootFact.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr || source.ExprRef == 0 {
		t.Fatalf("root assignment source = %#v, want function expression source", source)
	}
	fnSymbol, ok := bindings.FunctionSymbol(def.Func)
	if !ok || fnSymbol == 0 {
		t.Fatalf("function symbol = %d/%v, want non-zero", fnSymbol, ok)
	}
	if got, ok := facts.ExpressionFunction(source.ExprRef); !ok || got != fnSymbol {
		t.Fatalf("expression function = %d/%v, want %d", got, ok, fnSymbol)
	}
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		t.Fatalf("missing expression value for ref %d", source.ExprRef)
	}
	if kind := product.Get(reg, value, runtimekind.Key); !kind.Contains(runtimekind.Function) {
		t.Fatalf("function definition value runtime kind = %v, want function", kind)
	}
	gotID, ok := product.Get(reg, value, identity.Key).ID()
	if !ok || gotID != identity.LuaFunction(uint64(fnSymbol)) {
		t.Fatalf("function definition identity = %v/%v, want %v", gotID, ok, identity.LuaFunction(uint64(fnSymbol)))
	}
}

func mustLocalSource(t *testing.T, facts factflow.Facts, point cfg.Point) factflow.ValueSource {
	t.Helper()
	fact, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing local assignment at point %d", point)
	}
	source := fact.Source()
	if !source.HasExpr || source.ExprRef == 0 {
		t.Fatalf("local source = %#v, want expr ref", source)
	}
	return source
}

func assertExpressionValue(t *testing.T, facts factflow.Facts, ref factflow.ExprRef, wantPresence presence.Value, wantRuntimeKind runtimekind.Value) {
	t.Helper()
	value, ok := facts.ExpressionValue(ref)
	if !ok {
		t.Fatalf("missing expression value for ref %d", ref)
	}
	if got := product.PresenceOf(value); !presence.Equal(got, wantPresence) {
		t.Fatalf("expression value presence = %s, want %s", got, wantPresence)
	}
	if got := product.Get(standard.Registry(), value, runtimekind.Key); !runtimekind.Equal(got, wantRuntimeKind) {
		t.Fatalf("expression value runtime kind = %s, want %s", got, wantRuntimeKind)
	}
}

func assertLoweredAssertion(t *testing.T, facts factflow.Facts, source factflow.ValueSource, want assertion.Value, wantInnerKind factflow.ValueSourceKind) {
	t.Helper()
	claim, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing assertion for source ref %d", source.ExprRef)
	}
	assertClaimRefinementProduct(t, claim.Refinement(), want)
	inner := claim.Source()
	if inner.ExprRef == 0 || inner.ExprRef == source.ExprRef || inner.Kind != wantInnerKind {
		t.Fatalf("assertion inner source = %#v, outer %#v", inner, source)
	}
}

func assertLoweredConcreteCastAssertion(t *testing.T, facts factflow.Facts, source factflow.ValueSource, want typ.Type, wantInnerKind factflow.ValueSourceKind) {
	t.Helper()
	claim, ok := facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		t.Fatalf("missing assertion for source ref %d", source.ExprRef)
	}
	assertConcreteCastRefinementProduct(t, claim.Refinement(), want)
	inner := claim.Source()
	if inner.ExprRef == 0 || inner.ExprRef == source.ExprRef || inner.Kind != wantInnerKind {
		t.Fatalf("assertion inner source = %#v, outer %#v", inner, source)
	}
}

func refinementAssertion(t *testing.T, refinement factflow.ExpressionRefinement) assertion.Value {
	t.Helper()
	return product.Get(standard.Registry(), refinement.Refinement(), assertion.Key)
}

func assertConcreteCastRefinementProduct(t *testing.T, value product.Value, want typ.Type) {
	t.Helper()
	reg := standard.Registry()
	wantAssertion := concreteCastAssertionForType(want)
	if got := product.Get(reg, value, assertion.Key); !assertion.Equal(got, wantAssertion) {
		t.Fatalf("assertion value = %s, want %s", got, wantAssertion)
	}
	witness := product.Get(reg, value, typewitness.Key)
	gotType, ok := witness.Type()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("type witness = %v/%v, want %s", witness, ok, want)
	}
	if got := product.Get(reg, value, evidence.Key); !evidence.Equal(got, evidence.Top()) {
		t.Fatalf("cast refinement evidence = %s, want top", got)
	}
}

func concreteCastRefinementValue(reg *axis.Registry, t typ.Type) product.Value {
	return product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, t), t), assertion.Key, concreteCastAssertionForType(t))
}

func concreteCastAssertionForType(t typ.Type) assertion.Value {
	t = unwrap.Alias(t)
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return assertion.Type()
	}
	return assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim)
}

func applyConcreteCastRefinement(reg *axis.Registry, value product.Value, t typ.Type) product.Value {
	declared := concreteCastRefinementValue(reg, t)
	merged := valueref.MergeDeclaredContract(reg, value, declared)
	currentClaim := product.Get(reg, merged, assertion.Key)
	return product.Set(reg, merged, assertion.Key, assertion.Combine(currentClaim, concreteCastAssertionForType(t)))
}

func assertClaimRefinementProduct(t *testing.T, value product.Value, want assertion.Value) {
	t.Helper()
	reg := standard.Registry()
	if got := product.Get(reg, value, assertion.Key); !assertion.Equal(got, want) {
		t.Fatalf("assertion value = %s, want %s", got, want)
	}
	wantProduct := product.Set(reg, product.Top(), assertion.Key, want)
	wantEvidence := evidence.Top()
	if want.Has(assertion.AnyClaim) {
		wantEvidence = evidence.ExplicitTop()
		wantProduct = product.Set(reg, wantProduct, evidence.Key, wantEvidence)
	}
	if want.Has(assertion.TypeClaim) {
		if got := product.Get(reg, value, evidence.Key); !evidence.Equal(got, wantEvidence) {
			t.Fatalf("assertion refinement evidence = %s, want %s", got, wantEvidence)
		}
		return
	}
	if got := product.ShapeOf(value); got != product.ShapeTop {
		t.Fatalf("assertion refinement shape = %s, want top", got)
	}
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Top()) {
		t.Fatalf("assertion refinement presence = %s, want top", got)
	}
	if got := product.Get(reg, value, runtimekind.Key); !runtimekind.Equal(got, runtimekind.Top()) {
		t.Fatalf("assertion refinement runtime kind = %s, want top", got)
	}
	if got := product.Get(reg, value, evidence.Key); !evidence.Equal(got, wantEvidence) {
		t.Fatalf("assertion refinement evidence = %s, want %s", got, wantEvidence)
	}
	if !product.Equal(reg, value, wantProduct) {
		t.Fatalf("claim refinement carried unexpected axes")
	}
}

func assertLoweredBranchValuePresence(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantPath path.Path,
	wantTrue presence.Value,
	hasTrue bool,
	wantFalse presence.Value,
	hasFalse bool,
) {
	t.Helper()
	refinement, ok := branchRefinementAt(facts.BranchRefinements(point), wantPath)
	if !ok {
		t.Fatalf("missing branch refinement at point %d", point)
	}
	assertOptionalValuePresence(t, "true edge", refinement.TrueValue, wantTrue, hasTrue)
	assertOptionalValuePresence(t, "false edge", refinement.FalseValue, wantFalse, hasFalse)
}

func assertLoweredBranchPresenceProof(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantPath path.Path,
	wantPresence presence.Value,
	wantTrue bool,
	wantFalse bool,
) {
	t.Helper()
	for _, proof := range facts.BranchPathEvidence(point) {
		if proof.Kind() != factflow.BranchPathEvidencePresence || !proof.Path().Equal(wantPath) {
			continue
		}
		gotPresence, ok := proof.Presence()
		if !ok || !presence.Equal(gotPresence, wantPresence) {
			continue
		}
		if proof.ActiveOnEdge(true) != wantTrue || proof.ActiveOnEdge(false) != wantFalse {
			t.Fatalf(
				"branch presence proof active true/false = %v/%v, want %v/%v",
				proof.ActiveOnEdge(true),
				proof.ActiveOnEdge(false),
				wantTrue,
				wantFalse,
			)
		}
		return
	}
	t.Fatalf("missing branch presence proof at point %d for %s presence %s", point, wantPath, wantPresence)
}

func assertLoweredBranchTruthyProof(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantPath path.Path,
	wantTrue bool,
	wantFalse bool,
) {
	t.Helper()
	for _, proof := range facts.BranchPathEvidence(point) {
		if proof.Kind() != factflow.BranchPathEvidenceTruthy || !proof.Path().Equal(wantPath) {
			continue
		}
		if proof.ActiveOnEdge(true) != wantTrue || proof.ActiveOnEdge(false) != wantFalse {
			t.Fatalf(
				"branch truthy proof active true/false = %v/%v, want %v/%v",
				proof.ActiveOnEdge(true),
				proof.ActiveOnEdge(false),
				wantTrue,
				wantFalse,
			)
		}
		return
	}
	t.Fatalf("missing branch truthy proof at point %d for %s", point, wantPath)
}

func assertOptionalValuePresence(
	t *testing.T,
	label string,
	gotFn func() (factflow.ValueRefinement, bool),
	want presence.Value,
	wantOK bool,
) {
	t.Helper()
	got, ok := gotFn()
	if ok != wantOK {
		t.Fatalf("%s value refinement ok = %v, want %v", label, ok, wantOK)
	}
	if !ok {
		return
	}
	constraint, hasConstraint := got.Constraint()
	if !hasConstraint {
		t.Fatalf("%s constraint missing", label)
	}
	gotPresence := product.PresenceOf(constraint)
	if !presence.Equal(gotPresence, want) {
		t.Fatalf("%s presence = %s, want %s", label, gotPresence, want)
	}
}

func assertLoweredBranchFalsyAbsent(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantPath path.Path,
	edge bool,
) {
	t.Helper()
	refinement, ok := branchRefinementAt(facts.BranchRefinements(point), wantPath)
	if !ok {
		t.Fatalf("missing branch refinement at point %d", point)
	}
	value, ok := refinement.ValueForEdge(edge)
	if !ok {
		t.Fatalf("missing value refinement on edge %v", edge)
	}
	if !value.FalsyAbsent() {
		t.Fatalf("value refinement on edge %v is not marked falsy-absent", edge)
	}
}

func assertLoweredBranchValueRefinement(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantPath path.Path,
	wantTrue valueRefinementExpectation,
	wantFalse valueRefinementExpectation,
) {
	t.Helper()
	refinement, ok := branchRefinementAt(facts.BranchRefinements(point), wantPath)
	if !ok {
		t.Fatalf("missing branch refinement at point %d", point)
	}
	trueValue, ok := refinement.TrueValue()
	if !ok {
		t.Fatalf("missing true-edge value refinement")
	}
	falseValue, ok := refinement.FalseValue()
	if !ok {
		t.Fatalf("missing false-edge value refinement")
	}
	assertValueRefinement(t, "true edge", trueValue, wantTrue)
	assertValueRefinement(t, "false edge", falseValue, wantFalse)
}

func assertBranchLiteralType(t *testing.T, facts factflow.Facts, point cfg.Point, wantPath path.Path, edge bool, want typ.Type) {
	t.Helper()
	for _, refinement := range facts.BranchRefinements(point) {
		if !refinement.TargetPath().Equal(wantPath) {
			continue
		}
		value, ok := refinement.ValueForEdge(edge)
		if !ok {
			continue
		}
		constraint, ok := value.Constraint()
		if !ok {
			continue
		}
		got, ok := typevalue.TypeOf(standard.Registry(), constraint)
		if ok && typ.TypeEquals(got, want) {
			return
		}
	}
	t.Fatalf("missing branch literal type %s on edge %v at point %d for %s", want, edge, point, wantPath.String())
}

func assertLoweredBranchPathEquality(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantLeft path.Path,
	wantRight path.Path,
	wantTrue bool,
	wantFalse bool,
) {
	t.Helper()
	if wantRight.Less(wantLeft) {
		wantLeft, wantRight = wantRight, wantLeft
	}
	relations := facts.BranchPathRelations(point)
	if len(relations) != 2 {
		t.Fatalf("branch path relations at point %d = %d, want 2", point, len(relations))
	}
	var equality, inequality factflow.BranchPathRelation
	for _, relation := range relations {
		switch relation.Kind() {
		case factflow.BranchPathRelationEqual:
			equality = relation
		case factflow.BranchPathRelationNotEqual:
			inequality = relation
		default:
			t.Fatalf("branch path relation kind = %v, want equality or inequality", relation.Kind())
		}
	}
	if equality.Kind() != factflow.BranchPathRelationEqual {
		t.Fatal("missing equality branch path relation")
	}
	if inequality.Kind() != factflow.BranchPathRelationNotEqual {
		t.Fatal("missing inequality branch path relation")
	}
	for _, relation := range []factflow.BranchPathRelation{equality, inequality} {
		if !relation.LeftPath().Equal(wantLeft) {
			t.Fatalf("branch path relation left = %#v, want %#v", relation.LeftPath(), wantLeft)
		}
		if !relation.RightPath().Equal(wantRight) {
			t.Fatalf("branch path relation right = %#v, want %#v", relation.RightPath(), wantRight)
		}
	}
	if equality.ActiveOnEdge(true) != wantTrue || equality.ActiveOnEdge(false) != wantFalse {
		t.Fatalf("equality relation active true/false = %v/%v, want %v/%v", equality.ActiveOnEdge(true), equality.ActiveOnEdge(false), wantTrue, wantFalse)
	}
	if inequality.ActiveOnEdge(true) != !wantTrue || inequality.ActiveOnEdge(false) != !wantFalse {
		t.Fatalf("inequality relation active true/false = %v/%v, want %v/%v", inequality.ActiveOnEdge(true), inequality.ActiveOnEdge(false), !wantTrue, !wantFalse)
	}
	if len(facts.BranchRefinements(point)) != 0 {
		t.Fatalf("path equality relation at point %d also lowered as branch refinement", point)
	}
}

func assertValueRefinement(t *testing.T, label string, got factflow.ValueRefinement, want valueRefinementExpectation) {
	t.Helper()
	constraint, hasConstraint := got.Constraint()
	if !hasConstraint {
		t.Fatalf("%s constraint missing", label)
	}
	gotPresence := product.PresenceOf(constraint)
	hasPresence := !presence.Equal(gotPresence, presence.Top())
	if hasPresence != want.hasPresence {
		t.Fatalf("%s presence ok = %v, want %v", label, hasPresence, want.hasPresence)
	}
	if want.hasPresence && !presence.Equal(gotPresence, want.presence) {
		t.Fatalf("%s presence = %s, want %s", label, gotPresence, want.presence)
	}
	gotRuntimeKind := product.Get(standard.Registry(), constraint, runtimekind.Key)
	hasRuntimeKind := !runtimekind.Equal(gotRuntimeKind, runtimekind.Top())
	if hasRuntimeKind != want.hasRuntimeKind {
		t.Fatalf("%s runtime kind ok = %v, want %v", label, hasRuntimeKind, want.hasRuntimeKind)
	}
	if want.hasRuntimeKind && !runtimekind.Equal(gotRuntimeKind, want.runtimeKind) {
		t.Fatalf("%s runtime kind = %s, want %s", label, gotRuntimeKind, want.runtimeKind)
	}
}

func assertLoweredObjectEntry(t *testing.T, entry factflow.ObjectEntry, wantSuffix path.Path, wantKind factflow.ValueSourceKind) {
	t.Helper()
	if !entry.Suffix().Equal(wantSuffix) {
		t.Fatalf("entry suffix = %#v, want %#v", entry.Suffix(), wantSuffix)
	}
	source := entry.Source()
	if source.Kind != wantKind {
		t.Fatalf("entry source = %#v, want kind %v", source, wantKind)
	}
	if wantKind == factflow.ValueSourceExpression && (!source.HasExpr || source.ExprRef == 0) {
		t.Fatalf("entry source = %#v, want expression with expr ref", source)
	}
}

type valueRefinementExpectation struct {
	presence    presence.Value
	hasPresence bool

	runtimeKind    runtimekind.Value
	hasRuntimeKind bool
}

type testLowerSparseAxis uint8

const (
	testLowerSparseAxisBottom testLowerSparseAxis = iota
	testLowerSparseAxisLow
	testLowerSparseAxisTop
)

var testLowerSparseAxisKey = axis.NewKey[testLowerSparseAxis]("transferfacts.test.sparse")

func testLowerSparseAxisSpec() axis.Spec[testLowerSparseAxis] {
	return axis.Spec[testLowerSparseAxis]{
		Key:    testLowerSparseAxisKey,
		Bottom: func() testLowerSparseAxis { return testLowerSparseAxisBottom },
		Top:    func() testLowerSparseAxis { return testLowerSparseAxisTop },
		Equal:  func(a, b testLowerSparseAxis) bool { return a == b },
		LessOrEq: func(a, b testLowerSparseAxis) bool {
			return a <= b
		},
		Join: func(a, b testLowerSparseAxis) testLowerSparseAxis {
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b testLowerSparseAxis) testLowerSparseAxis {
			if a < b {
				return a
			}
			return b
		},
		Widen: func(prev, next testLowerSparseAxis) testLowerSparseAxis {
			if prev > next {
				return prev
			}
			return next
		},
		Hash: func(v testLowerSparseAxis) uint64 {
			return uint64(v) + 1
		},
	}
}

func fieldSuffix(name string) path.Path {
	return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: name}}}
}

func fieldChainSuffix(names ...string) path.Path {
	segments := make([]segment.Segment, len(names))
	for i, name := range names {
		segments[i] = segment.Segment{Kind: segment.SegmentField, Name: name}
	}
	return path.Path{Segments: segments}
}

func stringSuffix(name string) path.Path {
	return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentIndexString, Name: name}}}
}

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func number(value string) *ast.NumberExpr {
	return &ast.NumberExpr{Value: value}
}

func stringLit(value string) *ast.StringExpr {
	return &ast.StringExpr{Value: value}
}

func primitiveType(name string) *ast.PrimitiveTypeExpr {
	return &ast.PrimitiveTypeExpr{Name: name}
}

func typeCall(arg ast.Expr) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Func: ident("type"), Args: []ast.Expr{arg}}
}

func dot(obj ast.Expr, name string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       stringLit(name),
		KeySyntax: ast.AttrKeyDot,
	}
}

func stringIndex(obj ast.Expr, key string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       stringLit(key),
		KeySyntax: ast.AttrKeyIndex,
	}
}

func dynamicIndex(obj ast.Expr, key ast.Expr) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       key,
		KeySyntax: ast.AttrKeyIndex,
	}
}

func localAssign(names []string, exprs ...ast.Expr) *ast.LocalAssignStmt {
	return &ast.LocalAssignStmt{Names: names, Exprs: exprs}
}

func assign(lhs []ast.Expr, rhs ...ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: lhs, Rhs: rhs}
}

func parseSemanticChunk(t *testing.T, source string, globals ...string) ([]ast.Stmt, *bind.Result, *cfgbuild.Result, *semantics.Result) {
	t.Helper()
	stmts, err := parse.ParseString(source, "transferfacts_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: globals})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	return stmts, bindings, built, result
}

func parseSemanticFunction(t *testing.T, source string, globals ...string) (*ast.FunctionExpr, *bind.Result, *cfgbuild.Result, *semantics.Result) {
	t.Helper()
	stmts, err := parse.ParseString(source, "transferfacts_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("ParseString returned %d statements, want 1", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("statement = %T, want function definition", stmts[0])
	}
	bindings := bind.BindFunction(def.Func, bind.Options{Globals: globals})
	built := cfgbuild.BuildFunction(def.Func, bindings)
	if built == nil {
		t.Fatalf("BuildFunction returned nil")
	}
	result, err := semantics.ExtractFunction(def.Func, bindings, built)
	if err != nil {
		t.Fatalf("ExtractFunction: %v", err)
	}
	return def.Func, bindings, built, result
}

func mustLocalStmt(t *testing.T, stmts []ast.Stmt, index int) *ast.LocalAssignStmt {
	t.Helper()
	if index < 0 || index >= len(stmts) {
		t.Fatalf("statement index %d out of range for %d statements", index, len(stmts))
	}
	stmt, ok := stmts[index].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("statement %d = %T, want *ast.LocalAssignStmt", index, stmts[index])
	}
	return stmt
}

func mustIfStmt(t *testing.T, stmts []ast.Stmt, index int) *ast.IfStmt {
	t.Helper()
	if index < 0 || index >= len(stmts) {
		t.Fatalf("statement index %d out of range for %d statements", index, len(stmts))
	}
	stmt, ok := stmts[index].(*ast.IfStmt)
	if !ok {
		t.Fatalf("statement %d = %T, want *ast.IfStmt", index, stmts[index])
	}
	return stmt
}

func mustLocalAt(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	id, ok := bindings.LocalSymbolAt(stmt, index)
	if !ok {
		t.Fatalf("missing local symbol %d for %v", index, stmt.Names)
	}
	return id
}

func mustIdentSymbol(t *testing.T, bindings *bind.Result, ident *ast.IdentExpr) symbol.ID {
	t.Helper()
	id, ok := bindings.SymbolOf(ident)
	if !ok {
		t.Fatalf("missing symbol for %q", ident.Value)
	}
	return id
}

func requireStmtPoints(t *testing.T, built *cfgbuild.Result, stmt ast.Stmt, want int) []cfg.Point {
	t.Helper()
	points := built.StmtPoints.PointsFor(stmt)
	if len(points) != want {
		t.Fatalf("points for %T = %v, want %d", stmt, points, want)
	}
	return points
}

func assertNoPointFact(t *testing.T, facts factflow.Facts, point cfg.Point) {
	t.Helper()
	if _, ok := facts.LocalAssignment(point); ok {
		t.Fatalf("point %d lowered as local assignment", point)
	}
	if _, ok := facts.OrdinaryAssignment(point); ok {
		t.Fatalf("point %d lowered as ordinary assignment", point)
	}
	if _, ok := facts.PathAssignment(point); ok {
		t.Fatalf("point %d lowered as path assignment", point)
	}
	if _, ok := facts.PathDescendantInvalidation(point); ok {
		t.Fatalf("point %d lowered as path descendant invalidation", point)
	}
	if len(facts.BranchRefinements(point)) != 0 {
		t.Fatalf("point %d lowered as branch refinement", point)
	}
	if relations := facts.BranchPathRelations(point); len(relations) != 0 {
		t.Fatalf("point %d lowered as branch path relation", point)
	}
	if _, ok := facts.Return(point); ok {
		t.Fatalf("point %d lowered as return", point)
	}
	if _, ok := callproducer.FromFacts(facts, point); ok {
		t.Fatalf("point %d lowered as call producer", point)
	}
	if _, ok := facts.CallSite(point); ok {
		t.Fatalf("point %d lowered as call site", point)
	}
}

func assertNoCompilerASTTypes(t *testing.T, typ reflect.Type) {
	t.Helper()
	seen := make(map[reflect.Type]struct{})
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		if typ == nil {
			return
		}
		if _, ok := seen[typ]; ok {
			return
		}
		seen[typ] = struct{}{}
		if strings.Contains(typ.PkgPath(), "/compiler/ast") {
			t.Fatalf("transfer fact type includes compiler AST type: %v", typ)
		}
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(typ.Elem())
		case reflect.Map:
			walk(typ.Key())
			walk(typ.Elem())
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				walk(typ.Field(i).Type)
			}
		}
	}
	walk(typ)
}

func branchRefinementAt(refinements []factflow.BranchRefinement, wantPath path.Path) (factflow.BranchRefinement, bool) {
	for _, refinement := range refinements {
		if refinement.TargetPath().Equal(wantPath) {
			return refinement, true
		}
	}
	return factflow.BranchRefinement{}, false
}
