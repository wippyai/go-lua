package hooks

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCheckIdents_AcceptsDeclaredOverlayGlobals(t *testing.T) {
	stmts, err := parse.ParseString(`up(function() end)`, "overlay.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
	graph := cfg.Build(fn)
	evidence := trace.GraphEvidence(graph, graph.Bindings())
	scopes := map[cfg.Point]*scope.State{}
	for _, p := range graph.RPO() {
		scopes[p] = scope.New()
	}

	withoutOverlay := CheckIdents(graph, evidence, scopes, nil, "overlay.lua")
	if len(withoutOverlay) == 0 {
		t.Fatal("expected implicit global without overlay to be reported")
	}

	upSym, ok := graph.GlobalSymbol("up")
	if !ok {
		t.Fatal("expected up to be a graph global")
	}
	withOverlay := CheckIdents(graph, evidence, scopes, map[cfg.SymbolID]typ.Type{
		upSym: typ.Func().Build(),
	}, "overlay.lua")
	if len(withOverlay) != 0 {
		t.Fatalf("expected overlay global to be accepted, got %v", withOverlay)
	}
}
