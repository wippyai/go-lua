package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

func TestFrozenTableMutationWarningIsOptInAndEvidenceBacked(t *testing.T) {
	result := runDiagnosticsResultFull(t, `
type Config = { name: string, child: { tag: string } }
local cfg: Config = { name = "prod", child = { tag = "old" } }
table.freeze(cfg)
cfg.name = "staging"
`, []string{"table"}, signaturelookup.Source{IncludeStdlib: true})
	if diags := ProduceWithConfig(result, Config{}); len(diags) != 0 {
		t.Fatalf("default diagnostics = %#v, want frozen-table mutation disabled by default", diags)
	}

	diags := ProduceWithConfig(result, Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeFrozenTableMutation: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one frozen-table mutation warning", diags)
	}
	d := diags[0]
	if d.Code != CodeFrozenTableMutation || d.Severity != diagnostic.SeverityWarning {
		t.Fatalf("diagnostic code/severity = %s/%s, want %s/warning", d.Code, d.Severity, CodeFrozenTableMutation)
	}
	if !strings.Contains(d.Message, "cannot mutate frozen table") || !strings.Contains(d.Message, "cfg") {
		t.Fatalf("message = %q", d.Message)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) < 2 {
		t.Fatalf("evidence = %#v, want mutation and freeze proof", evidence)
	}
	if !diagnosticEvidenceContains(evidence, "this assignment mutates table") ||
		!diagnosticEvidenceContains(evidence, "table \"cfg\" was frozen by this call before the assignment") {
		t.Fatalf("evidence = %#v, want mutation and freeze proof chain", evidence)
	}
	if len(d.Labels) < 2 {
		t.Fatalf("labels = %#v, want mutation and freeze labels", d.Labels)
	}
	if !strings.Contains(d.Help, "mutable copy") {
		t.Fatalf("help = %q", d.Help)
	}
}

func TestFrozenTableMutationAcceptsReplacingFrozenChildThroughMutableParent(t *testing.T) {
	result := runDiagnosticsResultFull(t, `
type Child = { tag: string }
type Config = { child: Child }
local child: Child = { tag = "old" }
local cfg: Config = { child = child }
table.freeze(child)
cfg.child = { tag = "new" }
`, []string{"table"}, signaturelookup.Source{IncludeStdlib: true})
	diags := ProduceWithConfig(result, Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeFrozenTableMutation: diagnostic.Enable(),
	}}})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want no warning for replacing a frozen child reference through a mutable parent", diags)
	}
}

func TestFrozenTableMutationUsesIsFrozenBranchProof(t *testing.T) {
	result := runDiagnosticsResultFull(t, `
type Config = { name: string }
local cfg: Config = { name = "prod" }
if table.isfrozen(cfg) then
    cfg.name = "staging"
end
`, []string{"table"}, signaturelookup.Source{IncludeStdlib: true})
	diags := ProduceWithConfig(result, Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeFrozenTableMutation: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one branch-proof frozen-table mutation warning", diags)
	}
	if d := diags[0]; d.Code != CodeFrozenTableMutation ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "table \"cfg\" is already frozen here") {
		t.Fatalf("diagnostic = %#v, want incoming-state freeze evidence", d)
	}
}

func TestFrozenTableMutationReportsMutatingCall(t *testing.T) {
	result := runDiagnosticsResultFull(t, `
local items = { "a" }
table.freeze(items)
table.insert(items, "b")
`, []string{"table"}, signaturelookup.Source{IncludeStdlib: true})
	diags := ProduceWithConfig(result, Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeFrozenTableMutation: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one mutating-call frozen-table warning", diags)
	}
	d := diags[0]
	if d.Code != CodeFrozenTableMutation ||
		!strings.Contains(d.Message, "cannot call mutator on frozen table") ||
		!strings.Contains(d.Message, "items") {
		t.Fatalf("diagnostic = %#v, want mutating-call frozen-table warning", d)
	}
	evidence := d.Explanation.Evidence()
	if !diagnosticEvidenceContains(evidence, "this call mutates table \"items\"") ||
		!diagnosticEvidenceContains(evidence, "table \"items\" was frozen by this call before the mutating call") {
		t.Fatalf("evidence = %#v, want call mutation and freeze proof chain", evidence)
	}
	if len(d.Labels) < 2 {
		t.Fatalf("labels = %#v, want call mutation and freeze labels", d.Labels)
	}
	if !strings.Contains(d.Help, "mutable copy") {
		t.Fatalf("help = %q", d.Help)
	}
}
