package callbackenv

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestInferDetectsDominatedCallbackEnvOverlay(t *testing.T) {
	graph := callbackEnvTestGraph(t, `
_G.ctx = 1
cb()
_G.ctx = nil
`)
	result := Infer(graph, callbackEnvTestEvidence(graph), graph.ParamSlots(), func(expr ast.Expr, _ cfg.Point) typ.Type {
		if _, ok := expr.(*ast.NumberExpr); ok {
			return typ.Integer
		}
		return typ.Unknown
	}, nil)
	overlay, ok := result.ForParam(0)
	if result == nil || !ok {
		t.Fatalf("Infer returned no overlay: %+v", result)
	}
	if got, ok := overlay.Type("ctx"); !ok || got == nil || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("ctx overlay = %v, want integer", got)
	}
}

func TestMergeOverlayJoinsDuplicateNames(t *testing.T) {
	got := MergeOverlay(
		OverlayFromContractMap(map[string]typ.Type{"ctx": typ.String, "skip": nil}),
		OverlayFromContractMap(map[string]typ.Type{"ctx": typ.Number, "": typ.Boolean}),
	)
	want := typ.JoinReturnSlot(typ.String, typ.Number)
	if merged, ok := got.Type("ctx"); len(got) != 1 || !ok || !typ.TypeEquals(merged, want) {
		t.Fatalf("MergeOverlay = %+v, want ctx=%v", got, want)
	}
}

func callbackEnvTestGraph(t *testing.T, body string) *cfg.Graph {
	t.Helper()
	stmts, err := parse.Parse(strings.NewReader(body), "callbackenv_test")
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"cb"}},
		Stmts:   stmts,
	}, "_G")
}

func callbackEnvTestEvidence(graph *cfg.Graph) api.FlowEvidence {
	if graph == nil {
		return api.FlowEvidence{}
	}
	return trace.GraphEvidence(graph, graph.Bindings())
}
