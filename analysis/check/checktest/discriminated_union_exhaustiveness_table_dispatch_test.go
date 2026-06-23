package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDiscriminatedUnionExhaustivenessReportsMissingDispatchTableKey(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

type Handler = (Action) -> string
local handlers = {
    begin = function(action: Action): string return "begin:" .. action.kind end,
    commit = function(action: Action): string return "commit:" .. action.kind end,
}

local action: Action = { kind = "begin", id = "evt-1" }
local handler: Handler? = handlers[action.kind]
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            13,
		Column:          27,
		MessageContains: []string{
			"dispatch table is not exhaustive",
			"handlers.cancel",
		},
		EvidenceOrdered: []string{
			"`handlers` is indexed by discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
		LabelContains: []string{"dispatch table", "dispatch lookup"},
		HelpContains:  []string{"Add each missing dispatch key"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: dispatch table is not exhaustive; missing key: ` + "`handlers.cancel`" + `
 --> test.lua:13:27
   |
13 | local handler: Handler? = handlers[action.kind]
   |                           ↑ dispatch lookup

because:
  1. proven: ` + "`handlers`" + ` is indexed by discriminant ` + "`action.kind`" + `
  2. proven: possible cases: ` + "`action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`" + `
  3. proven: dispatch table provides keys: ` + "`handlers.begin`, `handlers.commit`" + `
 --> test.lua:7:18
  |
  |                  ↓ dispatch table
7 | local handlers = {
  4. missing proof: missing dispatch keys: ` + "`handlers.cancel`" + ` for ` + "`action.kind == \"cancel\"`" + `

help: Add each missing dispatch key, or route through an explicit fallback when missing keys are intentional.`
	assertRenderedEqual(t, rendered, want)
}

func TestDiscriminatedUnionExhaustivenessAcceptsCompleteDispatchTable(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
    cancel = function(action: Action): string return action.kind end,
}

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning for complete dispatch table", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessReportsDirectDispatchCall(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}

local action: Action = { kind = "begin", id = "evt-1" }
local output = handlers[action.kind](action)
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            12,
		Column:          16,
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
		EvidenceOrdered: []string{
			"`handlers` is indexed by discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`handlers` is indexed by discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"dispatch table provides keys: `handlers.begin`, `handlers.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"dispatch table", "dispatch lookup"},
		HelpContains:  []string{"Add each missing dispatch key"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: dispatch table is not exhaustive; missing key: ` + "`handlers.cancel`" + `
 --> test.lua:12:16
   |
12 | local output = handlers[action.kind](action)
   |                ↑ dispatch lookup

because:
  1. proven: ` + "`handlers`" + ` is indexed by discriminant ` + "`action.kind`" + `
  2. proven: possible cases: ` + "`action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`" + `
  3. proven: dispatch table provides keys: ` + "`handlers.begin`, `handlers.commit`" + `
 --> test.lua:6:18
  |
  |                  ↓ dispatch table
6 | local handlers = {
  4. missing proof: missing dispatch keys: ` + "`handlers.cancel`" + ` for ` + "`action.kind == \"cancel\"`" + `

help: Add each missing dispatch key, or route through an explicit fallback when missing keys are intentional.`
	assertRenderedEqual(t, rendered, want)
}

func TestDiscriminatedUnionExhaustivenessReportsCapturedDispatchTableKey(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel
type Handler = (Action) -> string

local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}

local function route(action: Action): string
    local handler: Handler? = handlers[action.kind]
    return ""
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
		EvidenceOrdered: []string{
			"`handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
		LabelContains: []string{"dispatch table", "dispatch lookup"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: dispatch table is not exhaustive; missing key: ` + "`handlers.cancel`" + `
 --> test.lua:13:31
   |
13 |     local handler: Handler? = handlers[action.kind]
   |                               ↑ dispatch lookup

because:
  1. proven: ` + "`handlers`" + ` is indexed by discriminant ` + "`action.kind`" + `
  2. proven: possible cases: ` + "`action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`" + `
  3. proven: dispatch table provides keys: ` + "`handlers.begin`, `handlers.commit`" + `
 --> test.lua:7:18
  |
  |                  ↓ dispatch table
7 | local handlers = {
  4. missing proof: missing dispatch keys: ` + "`handlers.cancel`" + ` for ` + "`action.kind == \"cancel\"`" + `

help: Add each missing dispatch key, or route through an explicit fallback when missing keys are intentional.`
	assertRenderedEqual(t, rendered, want)
}

func TestDiscriminatedUnionExhaustivenessAcceptsCapturedDispatchTableWithChildStaticFill(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}

local function route(action: Action): string
    handlers.cancel = function(item: Action): string return item.kind end
    local handler = handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after captured static fill", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsCapturedDispatchTableWithParentStaticFill(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local handlers = {
    begin = function(action: Action): string return action.kind end,
}
handlers.commit = function(action: Action): string return action.kind end
handlers.cancel = function(action: Action): string return action.kind end

local function route(action: Action): string
    local handler = handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after parent static fills", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsCapturedDispatchTableAfterMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}

local function route(action: Action, key: string): string
    handlers[key] = function(item: Action): string return item.kind end
    local handler = handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after captured dynamic mutation", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsCapturedDispatchTableAfterParentDynamicMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "cancel"
local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}
handlers[key] = function(action: Action): string return action.kind end

local function route(action: Action): string
    local handler = handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after parent dynamic mutation", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsCapturedDispatchTableAfterParentUnknownCallMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}
patch_handlers(handlers)

local function route(action: Action): string
    local handler = handlers[action.kind]
    return ""
end
`, WithGlobals("patch_handlers"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after parent unknown call can mutate captured dispatch table", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsDispatchTableAfterUnknownCallMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}

patch_handlers(handlers)

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, WithGlobals("patch_handlers"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after unknown call can mutate dispatch table", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessKeepsDispatchTableAfterUnrelatedUnknownCall(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}

observe_unrelated("ready")

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, "\n")
	result := Check(src, WithGlobals("observe_unrelated"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            14,
		Column:          17,
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`handlers` is indexed by discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"dispatch table provides keys: `handlers.begin`, `handlers.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"dispatch table", "dispatch lookup"},
		HelpContains:  []string{"Add each missing dispatch key", "explicit fallback"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: dispatch table is not exhaustive; missing key: ` + "`handlers.cancel`" + `
 --> test.lua:14:17
   |
14 | local handler = handlers[action.kind]
   |                 ↑ dispatch lookup

because:
  1. proven: ` + "`handlers`" + ` is indexed by discriminant ` + "`action.kind`" + `
  2. proven: possible cases: ` + "`action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`" + `
  3. proven: dispatch table provides keys: ` + "`handlers.begin`, `handlers.commit`" + `
 --> test.lua:6:18
  |
  |                  ↓ dispatch table
6 | local handlers = {
  4. missing proof: missing dispatch keys: ` + "`handlers.cancel`" + ` for ` + "`action.kind == \"cancel\"`" + `

help: Add each missing dispatch key, or route through an explicit fallback when missing keys are intentional.`
	assertRenderedEqual(t, rendered, want)
}

func TestDiscriminatedUnionExhaustivenessKeepsDispatchTableAfterLocalReadOnlyCall(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function inspect_handlers(handlers): ()
end

local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}

inspect_handlers(handlers)

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, "\n")
	requireDiagnostic(t, Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	)), diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            17,
		Column:          17,
		Span:            diagnostic.Span{StartLine: 17, StartCol: 17, EndLine: 17, EndCol: 38},
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`handlers` is indexed by discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`handlers` is indexed by discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"dispatch table provides keys", "`handlers.begin`", "`handlers.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing dispatch keys", "`handlers.cancel`", "`action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"dispatch table", "dispatch lookup"},
		HelpContains:  []string{"Add each missing dispatch key", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: dispatch table is not exhaustive; missing key: `handlers.cancel`",
			"--> test.lua:17:17",
			"17 | local handler = handlers[action.kind]",
			"↑ dispatch lookup",
			"because:",
			"1. proven: `handlers` is indexed by discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"--> test.lua:9:18",
			"↓ dispatch table",
			"9 | local handlers = {",
			"4. missing proof: missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
			"help: Add each missing dispatch key",
		},
		RenderNotContains: []string{
			"inspect_handlers",
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessSkipsDispatchTableAfterLocalDynamicMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function patch_handlers(handlers, key: string): ()
    handlers[key] = function(action: Action): string return action.kind end
end

local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}

patch_handlers(handlers, "cancel")

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after local dynamic mutation invalidates dispatch proof", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsDominatingStaticDispatchFill(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local handlers = {
    begin = function(action: Action): string return action.kind end,
}
handlers.commit = function(action: Action): string return action.kind end
handlers.cancel = function(action: Action): string return action.kind end

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after static table fills", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessInvalidatesDispatchTableAfterBranchReassignment(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function route(action: Action, replace: boolean): ()
    local handlers = {
        begin = function(item: Action): string return item.kind end,
        commit = function(item: Action): string return item.kind end,
        cancel = function(item: Action): string return item.kind end,
    }
    if replace then
        handlers = {
            begin = function(item: Action): string return item.kind end,
            commit = function(item: Action): string return item.kind end,
        }
    end
    local handler = handlers[action.kind]
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            18,
		Column:          21,
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`handlers` is indexed by discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`handlers` is indexed by discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"dispatch table provides keys", "`handlers.begin`", "`handlers.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing dispatch keys", "`handlers.cancel`", "`action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"dispatch table", "dispatch lookup"},
		HelpContains:  []string{"Add each missing dispatch key", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: dispatch table is not exhaustive; missing key: `handlers.cancel`",
			"--> test.lua:18:21",
			"18 |     local handler = handlers[action.kind]",
			"↑ dispatch lookup",
			"because:",
			"1. proven: `handlers` is indexed by discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"--> test.lua:13:20",
			"↓ dispatch table",
			"13 |         handlers = {",
			"4. missing proof: missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
			"help: Add each missing dispatch key",
		},
		RenderNotContains: []string{
			"handlers.cancel, handlers.cancel",
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessUsesDominatingDispatchTableReassignmentAsBase(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
    cancel = function(action: Action): string return action.kind end,
}
handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            17,
		Column:          17,
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`handlers` is indexed by discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`handlers` is indexed by discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"dispatch table provides keys", "`handlers.begin`", "`handlers.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing dispatch keys", "`handlers.cancel`", "`action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"dispatch table", "dispatch lookup"},
		HelpContains:  []string{"Add each missing dispatch key", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: dispatch table is not exhaustive; missing key: `handlers.cancel`",
			"--> test.lua:17:17",
			"17 | local handler = handlers[action.kind]",
			"↑ dispatch lookup",
			"because:",
			"1. proven: `handlers` is indexed by discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"--> test.lua:11:12",
			"↓ dispatch table",
			"11 | handlers = {",
			"4. missing proof: missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
			"help: Add each missing dispatch key",
		},
		RenderNotContains: []string{
			"9 |     cancel = function",
			"handlers.cancel, handlers.cancel",
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessAcceptsBranchReassignmentToCompleteDispatchTable(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function route(action: Action, replace: boolean): ()
    local handlers = {
        begin = function(item: Action): string return item.kind end,
        commit = function(item: Action): string return item.kind end,
        cancel = function(item: Action): string return item.kind end,
    }
    if replace then
        handlers = {
            begin = function(item: Action): string return item.kind end,
            commit = function(item: Action): string return item.kind end,
            cancel = function(item: Action): string return item.kind end,
        }
    end
    local handler = handlers[action.kind]
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning when every replacement keeps all keys", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessInvalidatesDispatchTableAcrossLoopBackedge(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function route(action: Action, keep_running: boolean): ()
    local handlers = {
        begin = function(item: Action): string return item.kind end,
        commit = function(item: Action): string return item.kind end,
        cancel = function(item: Action): string return item.kind end,
    }
    while keep_running do
        local handler = handlers[action.kind]
        handlers = {
            begin = function(item: Action): string return item.kind end,
            commit = function(item: Action): string return item.kind end,
        }
    end
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            13,
		Column:          25,
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`handlers` is indexed by discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`handlers` is indexed by discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"dispatch table provides keys", "`handlers.begin`", "`handlers.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing dispatch keys", "`handlers.cancel`", "`action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"dispatch table", "dispatch lookup"},
		HelpContains:  []string{"Add each missing dispatch key", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: dispatch table is not exhaustive; missing key: `handlers.cancel`",
			"--> test.lua:13:25",
			"13 |         local handler = handlers[action.kind]",
			"↑ dispatch lookup",
			"because:",
			"1. proven: `handlers` is indexed by discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"--> test.lua:14:20",
			"↓ dispatch table",
			"14 |         handlers = {",
			"4. missing proof: missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
			"help: Add each missing dispatch key",
		},
		RenderNotContains: []string{
			"handlers.cancel, handlers.cancel",
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessSkipsOpenDispatchTableConstruction(t *testing.T) {
	dynamicKey := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "cancel"
local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
    [key] = function(action: Action): string return action.kind end,
}

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(dynamicKey.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning for dynamic table key", dynamicKey.Diagnostics)
	}

	explicitFallback := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local fallback = function(action: Action): string return action.kind end
local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind] or fallback
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(explicitFallback.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning for explicit fallback expression", explicitFallback.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessIgnoresNonStringDispatchTableEntries(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local handlers = {
    function(action: Action): string return action.kind end,
    begin = function(action: Action): string return action.kind end,
    [2] = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
		EvidenceOrdered: []string{
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
	})
}
