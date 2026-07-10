package precheck

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestPrecheckReportsStructuralBreakAndGoto(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		code     diagnostic.Code
		label    string
		evidence string
		help     string
	}{
		{
			name:     "break outside loop",
			src:      `break`,
			code:     CodeBreakOutsideLoop,
			label:    "break statement",
			evidence: "this break is not inside a while, repeat, or for loop",
			help:     "Move this break inside a loop",
		},
		{name: "break in nested function", src: `
			while true do
				local f = function() break end
			end
		`, code: CodeBreakOutsideLoop, label: "break statement", evidence: "this break is not inside a while, repeat, or for loop", help: "Move this break inside a loop"},
		{name: "goto missing label", src: `goto missing`, code: CodeGotoUndefinedLabel, label: "unresolved goto", evidence: `no label named "missing" is declared in this scope`, help: "Add ::missing:: in this scope"},
		{name: "duplicate label", src: "::dup::\n::dup::", code: CodeDuplicateLabel, label: "duplicate label", evidence: `this label reuses "dup" in the same scope`, help: "Rename one label"},
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
			requireLabel(t, diags[0], tc.label)
			requireEvidence(t, diags[0], tc.evidence)
			if !strings.Contains(diags[0].Help, tc.help) {
				t.Fatalf("help = %q, want %q", diags[0].Help, tc.help)
			}
		})
	}
}

func TestPrecheckDuplicateLabelRendersEvidenceChain(t *testing.T) {
	src := "::dup::\n::dup::"
	stmts := mustStmts(t, src)
	diags := Precheck(stmts)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	rendered := diagnostic.Render(diags[0], diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"precheck_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[syntax.label.duplicate]: duplicate label "dup"
 --> precheck_test.lua:2:1
  |
2 | ::dup::
  | ↑ duplicate label

because:
  1. proven: label "dup" is first defined here
 --> precheck_test.lua:1:1
  |
  | ↓ first label
1 | ::dup::
  2. proven: this label reuses "dup" in the same scope

help: Rename one label, or remove the second ::dup:: label.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch\nwant:\n%s\n\ngot:\n%s", want, rendered)
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

func requireLabel(t *testing.T, d diagnostic.Diagnostic, want string) {
	t.Helper()
	for _, label := range d.Labels {
		if label.Message == want {
			return
		}
	}
	t.Fatalf("labels = %#v, want %q", d.Labels, want)
}

func requireEvidence(t *testing.T, d diagnostic.Diagnostic, want string) {
	t.Helper()
	for _, item := range d.Explanation.Evidence() {
		if strings.Contains(item.Message, want) {
			return
		}
	}
	t.Fatalf("evidence = %#v, want %q", d.Explanation.Evidence(), want)
}

func mustStmts(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "precheck_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts
}
