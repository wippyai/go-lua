package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerTailArgumentCallSourceMatchesExpandedProducerSite(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(): ()
    consume(produce())
end
`, "consume", "produce")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 2)
	facts := LowerDetailed(built.Graph, Config{
		Registry: standard.Registry(),
		WIR:      wirlower.LowerFunction("tail-argument-shape", fn, bindings, built),
	}).Facts

	producer, ok := facts.CallSiteView(points[0])
	if !ok {
		t.Fatalf("missing producer call site at point %d", points[0])
	}
	if !producer.Final() || !producer.Expanded() || producer.Adjusted() || producer.OpenTail() {
		t.Fatalf(
			"producer site = final:%v adjusted:%v expanded:%v open:%v, want expanded non-open tail",
			producer.Final(), producer.Adjusted(), producer.Expanded(), producer.OpenTail(),
		)
	}
	consumer, ok := facts.CallSiteView(points[1])
	if !ok {
		t.Fatalf("missing consumer call site at point %d", points[1])
	}
	source, ok := consumer.ArgumentSourceAt(0)
	if !ok || source.Kind != factflow.ValueSourceCall || source.CallPoint != points[0] {
		t.Fatalf("consumer argument source = %#v/%v, want producer call result", source, ok)
	}
	if source.Final != producer.Final() || source.Expanded != producer.Expanded() || source.Adjusted != producer.Adjusted() || source.OpenTail != producer.OpenTail() {
		t.Fatalf(
			"argument source = final:%v adjusted:%v expanded:%v open:%v, producer site = final:%v adjusted:%v expanded:%v open:%v",
			source.Final, source.Adjusted, source.Expanded, source.OpenTail,
			producer.Final(), producer.Adjusted(), producer.Expanded(), producer.OpenTail(),
		)
	}
}

func TestLowerSpreadVarargArgumentIsOpenTail(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(...): ()
    consume(...)
end
`, "consume")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, stmt, 1)[0]
	facts := LowerDetailed(built.Graph, Config{
		Registry: standard.Registry(),
		WIR:      wirlower.LowerFunction("spread-vararg-shape", fn, bindings, built),
	}).Facts

	site, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatalf("missing consumer call site at point %d", point)
	}
	source, ok := site.ArgumentSourceAt(0)
	if !ok || source.Kind != factflow.ValueSourceVararg {
		t.Fatalf("consumer argument source = %#v/%v, want vararg", source, ok)
	}
	if !source.Final || !source.Expanded || !source.OpenTail || source.Adjusted {
		t.Fatalf("consumer vararg shape = final:%v expanded:%v open:%v adjusted:%v, want open expanded tail", source.Final, source.Expanded, source.OpenTail, source.Adjusted)
	}
}
