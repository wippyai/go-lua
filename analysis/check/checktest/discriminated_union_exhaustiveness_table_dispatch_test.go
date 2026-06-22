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
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
		EvidenceOrdered: []string{
			"`handlers` is indexed by discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
		LabelContains: []string{"dispatch table", "dispatch lookup"},
		HelpContains:  []string{"Add each missing dispatch key"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: dispatch table is not exhaustive",
			"test.lua:12:",
			"local output = handlers[action.kind](action)",
			"dispatch lookup",
			"because:",
			"`handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
			"help:",
			"Add each missing dispatch key",
		},
	})
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
	result := Check(`
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
`, WithGlobals("observe_unrelated"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
	})
}

func TestDiscriminatedUnionExhaustivenessKeepsDispatchTableAfterLocalReadOnlyCall(t *testing.T) {
	result := Check(`
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
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
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
        }
    end
    local handler = handlers[action.kind]
end
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
			"`handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessUsesDominatingDispatchTableReassignmentAsBase(t *testing.T) {
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
handlers = {
    begin = function(action: Action): string return action.kind end,
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
	result := Check(`
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
			"`handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
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

func TestDiscriminatedUnionExhaustivenessReportsNestedDispatchTableKey(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}

local action: Action = { kind = "begin", id = "evt-1" }
local handler = registry.handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "registry.handlers.cancel"},
		EvidenceOrdered: []string{
			"`registry.handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `registry.handlers.begin`, `registry.handlers.commit`",
			"missing dispatch keys: `registry.handlers.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessSkipsNestedDispatchTableWithDynamicKey(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "cancel"
local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
        [key] = function(action: Action): string return action.kind end,
    },
}

local action: Action = { kind = "begin", id = "evt-1" }
local handler = registry.handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning for nested dispatch table with dynamic key", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsNestedDispatchTableStaticFill(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}
registry.handlers.cancel = function(action: Action): string return action.kind end

local action: Action = { kind = "begin", id = "evt-1" }
local handler = registry.handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after nested static dispatch fill", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsNestedDispatchTableAfterDynamicMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}
local key = "cancel"
registry.handlers[key] = function(action: Action): string return action.kind end

local action: Action = { kind = "begin", id = "evt-1" }
local handler = registry.handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after nested dynamic dispatch mutation", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessKeepsDispatchTableAfterKnownNonCaseDynamicMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "audit"
local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}
handlers[key] = function(action: Action): string return action.kind end

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
			"`handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessAcceptsKnownCaseDynamicDispatchFill(t *testing.T) {
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

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after known dynamic dispatch fill", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessReportsCapturedNestedDispatchTableKey(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}

local function route(action: Action): string
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "registry.handlers.cancel"},
		EvidenceOrdered: []string{
			"`registry.handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `registry.handlers.begin`, `registry.handlers.commit`",
			"missing dispatch keys: `registry.handlers.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessAcceptsCapturedNestedDispatchTableWithParentStaticFill(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}
registry.handlers.cancel = function(action: Action): string return action.kind end

local function route(action: Action): string
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after captured nested parent static fill", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsCapturedNestedDispatchTableWithChildStaticFill(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}

local function route(action: Action): string
    registry.handlers.cancel = function(item: Action): string return item.kind end
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after captured nested child static fill", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsCapturedNestedDispatchTableAfterChildDynamicMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}

local function route(action: Action, key: string): string
    registry.handlers[key] = function(item: Action): string return item.kind end
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after captured nested dynamic mutation", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsCapturedNestedDispatchTableAfterParentUnknownCallMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}
patch_handlers(registry.handlers)

local function route(action: Action): string
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithGlobals("patch_handlers"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after parent unknown call can mutate captured nested dispatch table", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessKeepsCapturedNestedDispatchTableAfterParentReadOnlyCall(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function inspect_handlers(handlers): ()
end

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}
inspect_handlers(registry.handlers)

local function route(action: Action): string
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "registry.handlers.cancel"},
		EvidenceOrdered: []string{
			"`registry.handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `registry.handlers.begin`, `registry.handlers.commit`",
			"missing dispatch keys: `registry.handlers.cancel` for `action.kind == \"cancel\"`",
		},
	})
}
