package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerParenthesizedReturnCallSourceMatchesAdjustedCallSite(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		globals []string
	}{
		{
			name: "direct",
			source: `
function f(): string
    return (make())
end
`,
			globals: []string{"make"},
		},
		{
			name: "method",
			source: `
function f(value: string): string
    return (value:gsub("x", "y"))
end
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, bindings, built := parseSemanticFunction(t, tt.source, tt.globals...)
			ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
			if !ok {
				t.Fatalf("stmt = %T, want return", fn.Stmts[0])
			}
			points := requireStmtPoints(t, built, ret, 2)
			facts := LowerDetailed(built.Graph, Config{
				Registry: standard.Registry(),
				WIR:      wirlower.LowerFunction("adjusted-return-"+tt.name, fn, bindings, built),
			}).Facts

			site, ok := facts.CallSiteView(points[0])
			if !ok {
				t.Fatalf("missing call site at point %d", points[0])
			}
			if !site.Final() || !site.Adjusted() || site.Expanded() || site.OpenTail() {
				t.Fatalf("call site shape = final:%v adjusted:%v expanded:%v open:%v, want adjusted final", site.Final(), site.Adjusted(), site.Expanded(), site.OpenTail())
			}

			returnFact, ok := facts.Return(points[1])
			if !ok {
				t.Fatalf("missing return fact at point %d", points[1])
			}
			sources := returnFact.Sources()
			if len(sources) != 1 {
				t.Fatalf("return sources = %#v, want one", sources)
			}
			assertCallSourceShapeMatchesSite(t, sources[0], points[0], site)
		})
	}
}

func TestLowerOrdinaryReturnCallKeepsOpenTailShape(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(): (string, number)
    return make()
end
`, "make")
	ret := fn.Stmts[0].(*ast.ReturnStmt)
	points := requireStmtPoints(t, built, ret, 2)
	facts := LowerDetailed(built.Graph, Config{
		Registry: standard.Registry(),
		WIR:      wirlower.LowerFunction("open-tail-return", fn, bindings, built),
	}).Facts

	site, ok := facts.CallSiteView(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	if !site.Final() || site.Adjusted() || !site.Expanded() || !site.OpenTail() {
		t.Fatalf("call site shape = final:%v adjusted:%v expanded:%v open:%v, want open final", site.Final(), site.Adjusted(), site.Expanded(), site.OpenTail())
	}
	returnFact, ok := facts.Return(points[1])
	if !ok {
		t.Fatalf("missing return fact at point %d", points[1])
	}
	sources := returnFact.Sources()
	if len(sources) != 1 {
		t.Fatalf("return sources = %#v, want one", sources)
	}
	assertCallSourceShapeMatchesSite(t, sources[0], points[0], site)
}

func assertCallSourceShapeMatchesSite(t *testing.T, source factflow.ValueSource, point cfg.Point, site factflow.CallSiteView) {
	t.Helper()
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.CallPoint != point {
		t.Fatalf("return source = %#v, want call source from point %d", source, point)
	}
	if source.Final != site.Final() || source.Adjusted != site.Adjusted() || source.Expanded != site.Expanded() || source.OpenTail != site.OpenTail() {
		t.Fatalf(
			"return source shape = final:%v adjusted:%v expanded:%v open:%v, call site = final:%v adjusted:%v expanded:%v open:%v",
			source.Final, source.Adjusted, source.Expanded, source.OpenTail,
			site.Final(), site.Adjusted(), site.Expanded(), site.OpenTail(),
		)
	}
}
