package transfer_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestExtractExpressionEvidence_IncludesNestedCalls(t *testing.T) {
	chunk, err := parse.ParseString(`
local x = outer(inner(a), other())
if guard(check(x)) then
	print(x)
end
return wrap(done())
`, "calls.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	graph := cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}, "outer", "inner", "other", "guard", "check", "print", "wrap", "done", "a")

	evidence := trace.ExpressionEvidence(graph, graph.Bindings()).Calls
	got := make(map[string]int)
	for _, ev := range evidence {
		if ev.Info != nil {
			got[ev.Info.CalleeName]++
		}
	}
	for _, name := range []string{"outer", "inner", "other", "guard", "check", "print", "wrap", "done"} {
		if got[name] != 1 {
			t.Fatalf("call evidence for %q = %d, want 1; all=%v", name, got[name], got)
		}
	}
}

func TestExtractExpressionEvidence_FieldDefaults(t *testing.T) {
	chunk, err := parse.ParseString(`
local value = opts.name or "default"
`, "defaults.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	graph := cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}, "opts")

	sym, ok := graph.SymbolAt(graph.Exit(), "opts")
	if !ok || sym == 0 {
		t.Fatal("expected opts symbol")
	}
	evidence := trace.ExpressionEvidence(graph, graph.Bindings())
	if len(evidence.FieldDefaults) != 1 {
		t.Fatalf("field defaults = %#v, want one", evidence.FieldDefaults)
	}
	got := evidence.FieldDefaults[0]
	if got.Target != sym || got.Field != "name" {
		t.Fatalf("field default target = (%d,%q), want (%d,name)", got.Target, got.Field, sym)
	}
}
