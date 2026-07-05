package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
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
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIRScalars: true, WIR: body})

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

func TestLowerWithWIRReturnSourcesForRootPathWhenKeySpaceProvided(t *testing.T) {
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
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, KeySpace: keyspace.New(), WIR: body})

	returnFact, ok := facts.Return(points[0])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[0])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourcePath || sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want WIR path source without expression ref", sources)
	}
}

func TestLowerWithWIRReturnSourcesFallsBackForExpressionOperands(t *testing.T) {
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
	if len(sources) != 1 || sources[0].Kind != factflow.ValueSourceExpression || !sources[0].HasExpr {
		t.Fatalf("return sources = %#v, want semantic expression fallback", sources)
	}
}
