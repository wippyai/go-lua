package flowbuild

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	fbcore "github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBorrowOnlyCallDoesNotEscapeArgument(t *testing.T) {
	for _, tt := range []struct {
		name    string
		source  string
		escapes bool
	}{
		{"builtin", `local value = {}; type(value)`, false},
		{"shadowed", `local type = function(x) end; local value = {}; type(value)`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stmts, err := parse.ParseString(tt.source, "test.lua")
			if err != nil {
				t.Fatal(err)
			}
			graph := cfg.Build(&ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: stmts}, "type")
			fnType := typ.Func().Param("value", typ.Any)
			if !tt.escapes {
				fnType.Effects(effect.BorrowsOnly())
			}
			fc := &fbcore.FlowContext{Graph: graph, Derived: &fbcore.Derived{Synth: func(ast.Expr, cfg.Point) typ.Type { return fnType.Build() }}}
			roots := collectCallAliasRoots(fc)
			found := false
			graph.EachCallSite(func(p cfg.Point, call *cfg.CallInfo) {
				if call == nil || len(call.ArgSymbols) == 0 || call.ArgSymbols[0] == 0 {
					return
				}
				for _, sym := range roots[p] {
					if sym == call.ArgSymbols[0] {
						found = true
					}
				}
			})
			if found != tt.escapes {
				t.Fatalf("alias escape = %v, want %v", found, tt.escapes)
			}
		})
	}
}
