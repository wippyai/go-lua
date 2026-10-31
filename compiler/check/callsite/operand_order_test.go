package callsite

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestCallsBeforeOperandReads(t *testing.T) {
	for _, tt := range []struct {
		name   string
		source string
		before []string
	}{
		{"statement call", `f(s, g(), s.n)`, []string{"g"}},
		{"last argument call", `f(s.n, g(s))`, nil},
		{"root call", `f(s, "x", s.n)`, nil},
		{"return list", `return f(s), s.n`, []string{"f"}},
		{"return list tail call", `return s.n, f(s)`, nil},
		{"assignment sources", `local a, b = f(s), g(s.n)`, []string{"f"}},
		{"target key", `s[f(s)] = s.n`, []string{"f"}},
		{"branch condition", `if f(s) == s.n then end`, []string{"f"}},
		{"callee call", `f(s)(s.n)`, []string{"f"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stmts, err := parse.ParseString("local f, g, s\n"+tt.source, "test.lua")
			if err != nil {
				t.Fatal(err)
			}
			graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts})
			var got []string
			for _, p := range graph.CFG().RPO() {
				for call := range CallsBeforeOperandReads(graph, p) {
					got = append(got, calleeName(call))
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.before) {
				t.Fatalf("calls before operand reads = %v, want %v", got, tt.before)
			}
		})
	}
}

func calleeName(call *ast.FuncCallExpr) string {
	switch fn := call.Func.(type) {
	case *ast.IdentExpr:
		return fn.Value
	case *ast.FuncCallExpr:
		return calleeName(fn)
	}
	return ""
}
