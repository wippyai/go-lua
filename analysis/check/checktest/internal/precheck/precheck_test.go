package precheck

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestPrecheckReportsStructuralBreakAndGoto(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code diagnostic.Code
	}{
		{name: "break outside loop", src: `break`, code: CodeBreakOutsideLoop},
		{name: "break in nested function", src: `
			while true do
				local f = function() break end
			end
		`, code: CodeBreakOutsideLoop},
		{name: "goto missing label", src: `goto missing`, code: CodeGotoUndefinedLabel},
		{name: "duplicate label", src: "::dup::\n::dup::", code: CodeDuplicateLabel},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stmts := mustStmts(t, tc.src)
			diags := Precheck(stmts)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
			}
			if diags[0].Code != tc.code {
				t.Fatalf("diagnostic code = %s, want %s", diags[0].Code, tc.code)
			}
		})
	}
}

func TestPrecheckAllowsForwardGotoAcrossNestedBlocks(t *testing.T) {
	cases := []string{
		"goto target\n do\n  local x = 1\n end\n::target::",
		"if true then\n  local x = 1\n end\n goto target\n::target::",
	}
	for _, src := range cases {
		stmts := mustStmts(t, src)
		diags := Precheck(stmts)
		if len(diags) != 0 {
			t.Fatalf("diagnostics = %#v, want none", diags)
		}
	}
}

func mustStmts(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "precheck_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts
}
