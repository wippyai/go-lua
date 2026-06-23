package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	lifecyclefx "github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestLifecycleResourceUnreleasedWarningUsesManifestEffects(t *testing.T) {
	src := `
local tx = {}
begin(tx)
`
	result := Check(src, lifecycleManifestOptions("begin")...)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeResourceUnreleased,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            3,
		Column:          1,
		MessageContains: []string{"resource", "`tx`", "transaction", "`active`", "expected", "`finished`"},
		EvidenceMin:     2,
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"this call acquires", "`tx`", "transaction:`active`", "requires `finished`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustRefuted,
				MessageContains: []string{"exit state still has", "`tx`", "transaction", "`active`", "no proof reaches `finished`"},
			},
		},
		LabelMin:      1,
		LabelContains: []string{"resource acquired"},
		HelpContains:  []string{"Transition `tx` to `finished`", "every return path"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[effect.lifecycle.unreleased]",
			"3 | begin(tx)",
			"↑ resource acquired",
			"missing proof: exit state still has `tx` in protocol transaction at `active`",
			"help: Transition `tx` to `finished` or escape ownership on every return path.",
		},
		RenderNotContains: []string{"^~", "\nwhere:\n"},
	})
}

func TestLifecycleResourceClosedOrEscapedThroughManifestEffectsDoesNotWarn(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "closed",
			src: `
local tx = {}
begin(tx)
finish(tx)
`,
		},
		{
			name: "escaped",
			src: `
local tx = {}
begin(tx)
transfer(tx)
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Check(tc.src, lifecycleManifestOptions("begin", "finish", "transfer")...)
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
			}
		})
	}
}

func TestLifecycleResourceEscapedThroughAliasDoesNotWarn(t *testing.T) {
	for _, src := range []string{
		`
local tx = {}
local alias = tx
begin(tx)
transfer(alias)
`,
		`
local tx = {}
local alias = tx
begin(alias)
transfer(tx)
`,
	} {
		result := Check(src, lifecycleManifestOptions("begin", "transfer")...)
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want no lifecycle warning when alias escapes the same resource", result.Diagnostics)
		}
	}
}

func TestLifecycleResourceClosedThroughAliasDoesNotWarn(t *testing.T) {
	for _, src := range []string{
		`
local tx = {}
local alias = tx
begin(tx)
finish(alias)
`,
		`
local tx = {}
local alias = tx
begin(alias)
finish(tx)
`,
	} {
		result := Check(src, lifecycleManifestOptions("begin", "finish")...)
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want no lifecycle warning when alias closes the same resource", result.Diagnostics)
		}
	}
}

func TestLifecycleResourceAliasReassignmentDoesNotCloseOriginal(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "close-reassigned-alias",
			src: `
local tx = {}
local alias = tx
begin(tx)
alias = {}
finish(alias)
`,
		},
		{
			name: "close-original-after-alias-reassigned",
			src: `
local alias
local tx = {}
alias = tx
begin(alias)
alias = {}
finish(tx)
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Check(tc.src, lifecycleManifestOptions("begin", "finish")...)
			if tc.name == "close-original-after-alias-reassigned" {
				if len(result.Diagnostics) != 0 {
					t.Fatalf("diagnostics = %#v, want original tx close to satisfy alias-acquired resource", result.Diagnostics)
				}
				return
			}
			if len(result.Diagnostics) != 1 {
				t.Fatalf("diagnostics = %#v, want one lifecycle warning for original tx", result.Diagnostics)
			}
			diag := result.Diagnostics[0]
			if diag.Code != diagnostics.CodeResourceUnreleased {
				t.Fatalf("diagnostic = %#v, want lifecycle warning", diag)
			}
			requireEvidenceMessage(t, diag, "this call acquires `tx` as transaction:`active` and requires `finished` before local ownership ends")
			if hasEvidenceMessage(diag, "this call transitions") {
				t.Fatalf("evidence = %#v, reassigned alias close must not count as a transition for original tx", diag.Explanation.Evidence())
			}
		})
	}
}

func TestLifecycleResourceAliasReassignmentDoesNotEscapeOriginal(t *testing.T) {
	result := Check(`
local tx = {}
local alias = tx
begin(tx)
alias = {}
transfer(alias)
`, lifecycleManifestOptions("begin", "transfer")...)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning for original tx", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeResourceUnreleased {
		t.Fatalf("diagnostic = %#v, want lifecycle warning", diag)
	}
	requireEvidenceMessage(t, diag, "this call acquires `tx` as transaction:`active` and requires `finished` before local ownership ends")
	if hasEvidenceMessage(diag, "escapes local ownership") {
		t.Fatalf("evidence = %#v, reassigned alias escape must not count for original tx", diag.Explanation.Evidence())
	}
}

func TestLifecycleResourcePartialAliasCloseKeepsObligationAndEvidence(t *testing.T) {
	src := `
local tx = {}
local alias = tx
begin(tx)
if flag then
    finish(alias)
end
`
	result := Check(src, lifecycleManifestOptions("begin", "finish", "flag")...)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning for partial alias close", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeResourceUnreleased {
		t.Fatalf("diagnostic = %#v, want lifecycle warning", diag)
	}
	requireEvidenceMessage(t, diag, "this call acquires `tx` as transaction:`active` and requires `finished` before local ownership ends")
	requireEvidenceMessage(t, diag, "this call transitions `alias` in protocol transaction from `active` to `finished` on a reachable path")
	requireEvidenceMessage(t, diag, "exit state still has `tx` in protocol transaction at a non-final state; no proof reaches `finished` or escapes ownership on every path")
	if !containsLifecycleLabel(diag, "lifecycle transition") {
		t.Fatalf("labels = %#v, want lifecycle transition label", diag.Labels)
	}
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	for _, want := range []string{
		"warning[effect.lifecycle.unreleased]",
		"4 | begin(tx)",
		"↑ resource acquired",
		"6 |     finish(alias)",
		"↑ lifecycle transition",
		"missing proof: exit state still has `tx` in protocol transaction at a non-final state",
		"help: Transition `tx` to `finished` or escape ownership on every return path.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered diagnostic missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"^~", "\nwhere:\n"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("rendered diagnostic contains forbidden noise %q:\n%s", forbidden, rendered)
		}
	}
}

func TestLifecycleResourcePartialAliasEscapeKeepsObligationAndEvidence(t *testing.T) {
	src := `
local tx = {}
local alias = tx
begin(tx)
if flag then
    transfer(alias)
end
`
	result := Check(src, lifecycleManifestOptions("begin", "transfer", "flag")...)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning for partial alias escape", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeResourceUnreleased {
		t.Fatalf("diagnostic = %#v, want lifecycle warning", diag)
	}
	requireEvidenceMessage(t, diag, "this call acquires `tx` as transaction:`active` and requires `finished` before local ownership ends")
	requireEvidenceMessage(t, diag, "this call escapes local ownership of `alias` in protocol transaction on a reachable path")
	requireEvidenceMessage(t, diag, "exit state still has `tx` in protocol transaction at `active`; no proof reaches `finished` or escapes ownership on every path")
	if !containsLifecycleLabel(diag, "ownership escaped") {
		t.Fatalf("labels = %#v, want ownership escaped label", diag.Labels)
	}
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	for _, want := range []string{
		"warning[effect.lifecycle.unreleased]",
		"4 | begin(tx)",
		"↑ resource acquired",
		"6 |     transfer(alias)",
		"↑ ownership escaped",
		"missing proof: exit state still has `tx` in protocol transaction at `active`",
		"help: Transition `tx` to `finished` or escape ownership on every return path.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered diagnostic missing %q:\n%s", want, rendered)
		}
	}
}

func TestLifecycleResourceBranchAcquireKeepsBothAliasEvidence(t *testing.T) {
	src := `
local tx = {}
local alias = tx
if flag then
    begin(tx)
else
    begin(alias)
end
`
	result := Check(src, lifecycleManifestOptions("begin", "flag")...)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning for branch acquire", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeResourceUnreleased {
		t.Fatalf("diagnostic = %#v, want lifecycle warning", diag)
	}
	requireEvidenceMessage(t, diag, "this call acquires `tx` as transaction:`active` and requires `finished` before local ownership ends")
	requireEvidenceMessage(t, diag, "this call acquires `alias` as transaction:`active` and requires `finished` before local ownership ends")
	if got := countEvidenceMessages(diag, "this call acquires"); got != 2 {
		t.Fatalf("acquire evidence count = %d, want both branch acquire sites: %#v", got, diag.Explanation.Evidence())
	}
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	for _, want := range []string{
		"5 |     begin(tx)",
		"↑ resource acquired",
		"7 |     begin(alias)",
		"missing proof: exit state still has",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered diagnostic missing %q:\n%s", want, rendered)
		}
	}
}

func TestLifecycleResourceSequentialReacquireDropsStaleAcquireEvidence(t *testing.T) {
	src := `
local tx = {}
begin(tx)
finish(tx)
begin(tx)
`
	result := Check(src, lifecycleManifestOptions("begin", "finish")...)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning for second acquire", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeResourceUnreleased {
		t.Fatalf("diagnostic = %#v, want lifecycle warning", diag)
	}
	if got := countEvidenceMessages(diag, "this call acquires"); got != 1 {
		t.Fatalf("acquire evidence count = %d, want only latest open acquire: %#v", got, diag.Explanation.Evidence())
	}
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	for _, want := range []string{
		"5 | begin(tx)",
		"↑ resource acquired",
		"missing proof: exit state still has `tx` in protocol transaction at `active`",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered diagnostic missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "3 | begin(tx)") {
		t.Fatalf("rendered diagnostic included stale first acquire:\n%s", rendered)
	}
}

func TestLifecycleResourceClosedThroughTableFieldAliasDoesNotWarn(t *testing.T) {
	for _, src := range []string{
		`
local tx = {}
local holder = { tx = tx }
begin(tx)
finish(holder.tx)
`,
		`
local tx = {}
local holder = { tx = tx }
begin(holder.tx)
finish(tx)
`,
	} {
		result := Check(src, lifecycleManifestOptions("begin", "finish")...)
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want no lifecycle warning when table field alias closes the same resource", result.Diagnostics)
		}
	}
}

func TestLifecycleResourceEscapedThroughTableFieldAliasDoesNotWarn(t *testing.T) {
	for _, src := range []string{
		`
local tx = {}
local holder = { tx = tx }
begin(tx)
transfer(holder.tx)
`,
		`
local tx = {}
local holder = { tx = tx }
begin(holder.tx)
transfer(tx)
`,
	} {
		result := Check(src, lifecycleManifestOptions("begin", "transfer")...)
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want no lifecycle warning when table field alias escapes the same resource", result.Diagnostics)
		}
	}
}

func TestLifecycleResourceClosedThroughKnownDynamicFieldAliasDoesNotWarn(t *testing.T) {
	for _, src := range []string{
		`
local tx = {}
local holder = {}
local key = "tx"
begin(tx)
holder[key] = tx
finish(holder.tx)
`,
		`
local holder = { tx = {} }
local key = "tx"
begin(holder.tx)
local tx = holder[key]
finish(tx)
`,
	} {
		result := Check(src, lifecycleManifestOptions("begin", "finish")...)
		if len(result.Diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v, want no lifecycle warning when known dynamic field aliases the same resource", result.Diagnostics)
		}
	}
}

func TestLifecycleResourceKnownDynamicFieldAliasRespectsKeyReassignment(t *testing.T) {
	result := Check(`
local holder = { tx = {} }
local key = "tx"
begin(holder.tx)
key = "other"
local tx = holder[key]
finish(tx)
`, lifecycleManifestOptions("begin", "finish")...)
	if !hasDiagnosticCode(result.Diagnostics, diagnostics.CodeResourceUnreleased) {
		t.Fatalf("diagnostics = %#v, want lifecycle warning when dynamic key no longer names holder.tx", result.Diagnostics)
	}
}

func TestLifecycleResourceClosedThroughDirectHelperParamDoesNotWarn(t *testing.T) {
	result := Check(`
local function close(resource)
    finish(resource)
end

local tx = {}
begin(tx)
close(tx)
`, lifecycleManifestOptions("begin", "finish")...)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want no lifecycle warning when helper parameter closes resource", result.Diagnostics)
	}
}

func TestLifecycleResourceEarlyReturnKeepsOpenObligation(t *testing.T) {
	src := `
local tx = {}
begin(tx)
if flag then
    return
end
finish(tx)
`
	result := Check(src, lifecycleManifestOptions("begin", "finish", "flag")...)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning for early return before close", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeResourceUnreleased {
		t.Fatalf("diagnostic = %#v, want lifecycle warning", diag)
	}
	requireEvidenceMessage(t, diag, "this call acquires `tx` as transaction:`active` and requires `finished` before local ownership ends")
	requireEvidenceMessage(t, diag, "this call transitions `tx` in protocol transaction from `active` to `finished` on a reachable path")
	requireEvidenceMessage(t, diag, "exit state still has `tx` in protocol transaction at a non-final state; no proof reaches `finished` or escapes ownership on every path")
}

func TestLifecycleResourceAcceptsAnyDeclaredObligationFinalState(t *testing.T) {
	bothFinals := Check(`
local tx = {}
begin(tx)
if flag then
    commit(tx)
else
    rollback(tx)
end
`, lifecycleMultiFinalManifestOptions("begin", "commit", "rollback", "flag")...)
	if len(bothFinals.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want no lifecycle warning when every path reaches a declared final state", bothFinals.Diagnostics)
	}

	partialSrc := `
local tx = {}
begin(tx)
if flag then
    commit(tx)
end
`
	partial := Check(partialSrc, lifecycleMultiFinalManifestOptions("begin", "commit", "flag")...)
	if len(partial.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning for missing rollback/default final path", partial.Diagnostics)
	}
	diag := requireDiagnostic(t, partial, diagnosticExpectation{
		Code:            diagnostics.CodeResourceUnreleased,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            3,
		Column:          1,
		Span:            diagnostic.Span{StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 8},
		MessageContains: []string{
			"resource `tx` remains in transaction state",
			"expected `committed` or `rolled_back`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				Span:            diagnostic.Span{StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 8},
				MessageContains: []string{"acquires `tx`", "transaction:`active`", "`committed` or `rolled_back`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				Span:            diagnostic.Span{StartLine: 5, StartCol: 5, EndLine: 5, EndCol: 13},
				MessageContains: []string{"transitions `tx`", "from `active` to `committed`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustRefuted,
				MessageContains: []string{"exit state still has `tx`", "no proof reaches `committed` or `rolled_back`"},
			},
		},
		LabelMin:      2,
		LabelContains: []string{"resource acquired", "lifecycle transition"},
		HelpContains:  []string{"Transition `tx`", "`committed` or `rolled_back`", "every return path"},
		Sources:       diagnostic.SourceMap{"test.lua": partialSrc},
		RenderOrderedContains: []string{
			"warning[effect.lifecycle.unreleased]",
			"3 | begin(tx)",
			"↑ resource acquired",
			"5 |     commit(tx)",
			"↑ lifecycle transition",
			"missing proof: exit state still has `tx` in protocol transaction at a non-final state",
			"help: Transition `tx` to `committed` or `rolled_back` or escape ownership on every return path.",
		},
		RenderNotContains: []string{"^~", "\nwhere:\n"},
	})
	requireEvidenceMessage(t, diag, "this call acquires `tx` as transaction:`active` and requires `committed` or `rolled_back` before local ownership ends")
	requireEvidenceMessage(t, diag, "this call transitions `tx` in protocol transaction from `active` to `committed` on a reachable path")
	requireEvidenceMessage(t, diag, "exit state still has `tx` in protocol transaction at a non-final state; no proof reaches `committed` or `rolled_back` or escapes ownership on every path")
}

func TestLifecycleResourceNoReturnBranchDoesNotNeedFinalState(t *testing.T) {
	clean := Check(`
local tx = {}
begin(tx)
if flag then
    commit(tx)
else
    error("abort")
end
`, lifecycleMultiFinalManifestOptions("begin", "commit", "flag", "error")...)
	if len(clean.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want no lifecycle warning when every normal-return path reaches a final state", clean.Diagnostics)
	}

	shadowedSrc := `
local error = function(msg) end
local tx = {}
begin(tx)
if flag then
    commit(tx)
else
    error("abort")
end
`
	shadowed := Check(shadowedSrc, lifecycleMultiFinalManifestOptions("begin", "commit", "flag")...)
	if len(shadowed.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want warning when shadowed error returns normally", shadowed.Diagnostics)
	}
	diag := requireDiagnostic(t, shadowed, diagnosticExpectation{
		Code:            diagnostics.CodeResourceUnreleased,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            4,
		Column:          1,
		Span:            diagnostic.Span{StartLine: 4, StartCol: 1, EndLine: 4, EndCol: 8},
		MessageContains: []string{
			"resource `tx` remains in transaction state",
			"expected `committed` or `rolled_back`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				Span:            diagnostic.Span{StartLine: 4, StartCol: 1, EndLine: 4, EndCol: 8},
				MessageContains: []string{"acquires `tx`", "transaction:`active`", "`committed` or `rolled_back`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				Span:            diagnostic.Span{StartLine: 6, StartCol: 5, EndLine: 6, EndCol: 13},
				MessageContains: []string{"transitions `tx`", "from `active` to `committed`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustRefuted,
				MessageContains: []string{"exit state still has `tx`", "no proof reaches `committed` or `rolled_back`"},
			},
		},
		LabelMin:      2,
		LabelContains: []string{"resource acquired", "lifecycle transition"},
		HelpContains:  []string{"Transition `tx`", "`committed` or `rolled_back`", "every return path"},
		Sources:       diagnostic.SourceMap{"test.lua": shadowedSrc},
		RenderOrderedContains: []string{
			"warning[effect.lifecycle.unreleased]",
			"4 | begin(tx)",
			"↑ resource acquired",
			"6 |     commit(tx)",
			"↑ lifecycle transition",
			"missing proof: exit state still has `tx` in protocol transaction at a non-final state",
			"help: Transition `tx` to `committed` or `rolled_back` or escape ownership on every return path.",
		},
		RenderNotContains: []string{"^~", "\nwhere:\n", "no-return"},
	})
	requireEvidenceMessage(t, diag, "this call transitions `tx` in protocol transaction from `active` to `committed` on a reachable path")
	requireEvidenceMessage(t, diag, "exit state still has `tx` in protocol transaction at a non-final state; no proof reaches `committed` or `rolled_back` or escapes ownership on every path")
}

func TestLifecycleResourceIntermediateStateEvidenceShowsWhyFinalIsMissing(t *testing.T) {
	src := `
local tx = {}
begin(tx)
prepare(tx)
`
	result := Check(src, lifecycleMultiFinalManifestOptions("begin", "prepare")...)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning for non-final intermediate state", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeResourceUnreleased {
		t.Fatalf("diagnostic = %#v, want lifecycle warning", diag)
	}
	requireEvidenceMessage(t, diag, "this call acquires `tx` as transaction:`active` and requires `committed` or `rolled_back` before local ownership ends")
	requireEvidenceMessage(t, diag, "this call transitions `tx` in protocol transaction from `active` to `prepared` on a reachable path")
	requireEvidenceMessage(t, diag, "exit state still has `tx` in protocol transaction at `prepared`; no proof reaches `committed` or `rolled_back` or escapes ownership on every path")
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	for _, want := range []string{
		"3 | begin(tx)",
		"↑ resource acquired",
		"4 | prepare(tx)",
		"↑ lifecycle transition",
		"missing proof: exit state still has `tx` in protocol transaction at `prepared`",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered diagnostic missing %q:\n%s", want, rendered)
		}
	}
}

func TestLifecycleResourceLoopAcquireKeepsSingleEvidenceChain(t *testing.T) {
	src := `
local tx = {}
while flag do
    begin(tx)
    break
end
`
	result := Check(src, lifecycleManifestOptions("begin", "flag")...)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one bounded lifecycle warning for loop acquire", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeResourceUnreleased {
		t.Fatalf("diagnostic = %#v, want lifecycle warning", diag)
	}
	if got := countEvidenceMessages(diag, "this call acquires"); got != 1 {
		t.Fatalf("acquire evidence count = %d, want one loop acquire site: %#v", got, diag.Explanation.Evidence())
	}
	requireEvidenceMessage(t, diag, "exit state still has `tx` in protocol transaction at `active`; no proof reaches `finished` or escapes ownership on every path")
}

func TestLifecycleResourceLoopAcquireAndCloseDoesNotWarn(t *testing.T) {
	result := Check(`
local tx = {}
while flag do
    begin(tx)
    finish(tx)
    break
end
`, lifecycleManifestOptions("begin", "finish", "flag")...)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want no lifecycle warning when loop body closes acquired resource", result.Diagnostics)
	}
}

func TestLifecycleResourceFieldReassignmentDoesNotCloseOriginal(t *testing.T) {
	result := Check(`
local tx = {}
local holder = { tx = tx }
begin(tx)
holder.tx = {}
finish(holder.tx)
`, lifecycleManifestOptions("begin", "finish")...)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning for original tx", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeResourceUnreleased {
		t.Fatalf("diagnostic = %#v, want lifecycle warning", diag)
	}
	requireEvidenceMessage(t, diag, "this call acquires `tx` as transaction:`active` and requires `finished` before local ownership ends")
	if hasEvidenceMessage(diag, "this call transitions") {
		t.Fatalf("evidence = %#v, reassigned field close must not count as a transition for original tx", diag.Explanation.Evidence())
	}
}

func TestLifecycleResourceEscapedThroughCapturedAliasDoesNotWarn(t *testing.T) {
	result := Check(`
local tx = {}
local alias = tx
begin(tx)
local release = function()
    transfer(alias)
end
release()
`, lifecycleManifestOptions("begin", "transfer")...)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want no lifecycle warning when direct closure escapes captured alias", result.Diagnostics)
	}
}

func TestLifecycleResourceClosedThroughCapturedAliasDoesNotWarn(t *testing.T) {
	result := Check(`
local tx = {}
local alias = tx
begin(tx)
local close = function()
    finish(alias)
end
close()
`, lifecycleManifestOptions("begin", "finish")...)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want no lifecycle warning when direct closure closes captured alias", result.Diagnostics)
	}
}

func lifecycleManifestOptions(globals ...string) []Option {
	opts := []Option{
		WithManifest("lifecycle", lifecycleAdversarialManifest()),
		WithDiagnosticRule(diagnostics.CodeResourceUnreleased, diagnostic.Enable()),
	}
	if len(globals) > 0 {
		opts = append(opts, WithGlobals(globals...))
	}
	return opts
}

func lifecycleAdversarialManifest() *manifest.Manifest {
	m := manifest.New("lifecycle")
	if err := m.DefineTypestateProtocol(typestate.Definition{
		Protocol:    typestate.Protocol("transaction"),
		States:      []typestate.State{typestate.State("active"), typestate.State("finished")},
		FinalStates: []typestate.State{typestate.State("finished")},
		Transitions: []typestate.TransitionDecl{{
			From: typestate.State("active"),
			To:   typestate.State("finished"),
		}},
	}); err != nil {
		panic(err)
	}
	m.DefineFunctionSignature("begin", signature.Function{
		Type: typ.Func().
			Param("tx", typ.Any).
			Build(),
		Effect: effect.Empty.With(lifecyclefx.Acquire{
			Target:   effect.ParamRef{Index: 0},
			Protocol: typestate.Protocol("transaction"),
			State:    typestate.State("active"),
			Obligation: typestate.Obligation{
				Final: typestate.State("finished"),
			},
		}),
	})
	m.DefineFunctionSignature("finish", signature.Function{
		Type: typ.Func().
			Param("tx", typ.Any).
			Build(),
		Effect: effect.Empty.With(lifecyclefx.Transition{
			Target:   effect.ParamRef{Index: 0},
			Protocol: typestate.Protocol("transaction"),
			From:     typestate.State("active"),
			To:       typestate.State("finished"),
		}),
	})
	m.DefineFunctionSignature("transfer", signature.Function{
		Type: typ.Func().
			Param("tx", typ.Any).
			Build(),
		Effect: effect.Empty.With(lifecyclefx.Escape{
			Target:   effect.ParamRef{Index: 0},
			Protocol: typestate.Protocol("transaction"),
		}),
	})
	return m
}

func lifecycleMultiFinalManifestOptions(globals ...string) []Option {
	opts := []Option{
		WithManifest("lifecycle", lifecycleMultiFinalManifest()),
		WithDiagnosticRule(diagnostics.CodeResourceUnreleased, diagnostic.Enable()),
	}
	if len(globals) > 0 {
		opts = append(opts, WithGlobals(globals...))
	}
	return opts
}

func lifecycleMultiFinalManifest() *manifest.Manifest {
	m := manifest.New("lifecycle")
	if err := m.DefineTypestateProtocol(typestate.Definition{
		Protocol:    typestate.Protocol("transaction"),
		States:      []typestate.State{typestate.State("active"), typestate.State("prepared"), typestate.State("committed"), typestate.State("rolled_back")},
		FinalStates: []typestate.State{typestate.State("committed"), typestate.State("rolled_back")},
		Transitions: []typestate.TransitionDecl{
			{From: typestate.State("active"), To: typestate.State("prepared")},
			{From: typestate.State("active"), To: typestate.State("committed")},
			{From: typestate.State("active"), To: typestate.State("rolled_back")},
		},
	}); err != nil {
		panic(err)
	}
	m.DefineFunctionSignature("begin", signature.Function{
		Type: typ.Func().
			Param("tx", typ.Any).
			Build(),
		Effect: effect.Empty.With(lifecyclefx.Acquire{
			Target:   effect.ParamRef{Index: 0},
			Protocol: typestate.Protocol("transaction"),
			State:    typestate.State("active"),
			Obligation: typestate.Obligation{
				Finals: typestate.NewFinalStates(typestate.State("committed"), typestate.State("rolled_back")),
			},
		}),
	})
	m.DefineFunctionSignature("commit", signature.Function{
		Type: typ.Func().
			Param("tx", typ.Any).
			Build(),
		Effect: effect.Empty.With(lifecyclefx.Transition{
			Target:   effect.ParamRef{Index: 0},
			Protocol: typestate.Protocol("transaction"),
			From:     typestate.State("active"),
			To:       typestate.State("committed"),
		}),
	})
	m.DefineFunctionSignature("prepare", signature.Function{
		Type: typ.Func().
			Param("tx", typ.Any).
			Build(),
		Effect: effect.Empty.With(lifecyclefx.Transition{
			Target:   effect.ParamRef{Index: 0},
			Protocol: typestate.Protocol("transaction"),
			From:     typestate.State("active"),
			To:       typestate.State("prepared"),
		}),
	})
	m.DefineFunctionSignature("rollback", signature.Function{
		Type: typ.Func().
			Param("tx", typ.Any).
			Build(),
		Effect: effect.Empty.With(lifecyclefx.Transition{
			Target:   effect.ParamRef{Index: 0},
			Protocol: typestate.Protocol("transaction"),
			From:     typestate.State("active"),
			To:       typestate.State("rolled_back"),
		}),
	})
	return m
}

func containsLifecycleLabel(diag diagnostic.Diagnostic, want string) bool {
	for _, label := range diag.Labels {
		if strings.Contains(label.Message, want) {
			return true
		}
	}
	return false
}

func hasEvidenceMessage(diag diagnostic.Diagnostic, want string) bool {
	for _, evidence := range diag.Explanation.Evidence() {
		if strings.Contains(evidence.Message, want) {
			return true
		}
	}
	return false
}

func countEvidenceMessages(diag diagnostic.Diagnostic, want string) int {
	count := 0
	for _, evidence := range diag.Explanation.Evidence() {
		if strings.Contains(evidence.Message, want) {
			count++
		}
	}
	return count
}
