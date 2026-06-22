package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDiscriminatedUnionExhaustivenessReportsMissingCase(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function route(action: Action): string
    if action.kind == "begin" then
        return action.id
    elseif action.kind == "commit" then
        return action.payment_id
    end
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
		Line:            7,
		Column:          8,
		MessageContains: []string{
			"discriminated union handling is not exhaustive",
			"action.kind == \"cancel\"",
		},
		EvidenceOrdered: []string{
			"branch chain checks discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"handled cases: `action.kind == \"begin\"`, `action.kind == \"commit\"`",
			"missing cases: `action.kind == \"cancel\"`",
			"no default branch handles the remaining union cases",
		},
		LabelContains: []string{"union case check"},
		HelpContains:  []string{"Handle each missing case"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: discriminated union handling is not exhaustive; missing case: ` + "`action.kind == \"cancel\"`" + `
 --> test.lua:7:8
  |
7 |     if action.kind == "begin" then
  |        ↑ union case check

because:
  1. proven: branch chain checks discriminant ` + "`action.kind`" + `
  2. proven: possible cases: ` + "`action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`" + `
  3. proven: handled cases: ` + "`action.kind == \"begin\"`, `action.kind == \"commit\"`" + `
  4. missing proof: missing cases: ` + "`action.kind == \"cancel\"`" + `
  5. missing proof: no default branch handles the remaining union cases

help: Handle each missing case, or add an else branch when a fallback is valid.`
	assertRenderedEqual(t, rendered, want)
}

func TestDiscriminatedUnionExhaustivenessAcceptsExhaustiveChainAndElseFallback(t *testing.T) {
	exhaustive := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function route(action: Action): string
    if action.kind == "begin" then
        return action.id
    elseif action.kind == "commit" then
        return action.payment_id
    elseif action.kind == "cancel" then
        return action.reason
    end
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(exhaustive.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning", exhaustive.Diagnostics)
	}

	withDefault := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function route(action: Action): string
    if action.kind == "begin" then
        return action.id
    elseif action.kind == "commit" then
        return action.payment_id
    else
        return "fallback"
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(withDefault.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning with else fallback", withDefault.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsMixedGuardChain(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function route(action: Action, retry: boolean): string
    if action.kind == "begin" then
        return action.id
    elseif retry then
        return "retry"
    end
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning for mixed guard chain", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessHandlesNegatedLiteralCase(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function route(action: Action): string
    if action.kind ~= "cancel" then
        return "active"
    end
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
		MessageContains: []string{"action.kind == \"cancel\""},
		EvidenceOrdered: []string{
			"handled cases: `action.kind == \"begin\"`, `action.kind == \"commit\"`",
			"missing cases: `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessHandlesGenericEnvelope(t *testing.T) {
	result := Check(`
type Created = { kind: "created", id: string }
type Deleted = { kind: "deleted", id: string }
type Tick = { kind: "tick", elapsed: number }
type Payload = Created | Deleted | Tick
type Envelope<T> = { payload: T }

local function route(env: Envelope<Payload>): string
    if env.payload.kind == "created" then
        return env.payload.id
    elseif env.payload.kind == "deleted" then
        return env.payload.id
    end
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
		MessageContains: []string{"env.payload.kind == \"tick\""},
		EvidenceOrdered: []string{
			"branch chain checks discriminant `env.payload.kind`",
			"handled cases: `env.payload.kind == \"created\"`, `env.payload.kind == \"deleted\"`",
			"missing cases: `env.payload.kind == \"tick\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessHandlesResultShape(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    if result.ok then
        return result.value
    end
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
		MessageContains: []string{"result.ok == false"},
		EvidenceOrdered: []string{
			"branch chain checks discriminant `result.ok`",
			"handled cases: `result.ok == true`",
			"missing cases: `result.ok == false`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessReportsUnguardedResultValueRead(t *testing.T) {
	src := strings.TrimLeft(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    return result.value
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
		Line:            4,
		Column:          12,
		MessageContains: []string{
			"result field read is not exhaustive",
			"result.value",
			"result.ok == true",
		},
		EvidenceOrdered: []string{
			"`result` is result-shaped and discriminated by `result.ok`",
			"`result.value` exists only for `result.ok == true`",
			"no stable guard proves `result.ok == true` before this read",
		},
		LabelContains: []string{"result field read"},
		HelpContains:  []string{"Check the result case before reading this field"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: result field read is not exhaustive; ` + "`result.value`" + ` requires ` + "`result.ok == true`" + `
 --> test.lua:4:12
  |
4 |     return result.value
  |            ↑ result field read

because:
  1. proven: ` + "`result`" + ` is result-shaped and discriminated by ` + "`result.ok`" + `
  2. proven: ` + "`result.value`" + ` exists only for ` + "`result.ok == true`" + `
  3. missing proof: no stable guard proves ` + "`result.ok == true`" + ` before this read

help: Check the result case before reading this field, or return from the opposite case before continuing.`
	assertRenderedEqual(t, rendered, want)
}

func TestDiscriminatedUnionExhaustivenessReportsUnguardedResultErrorRead(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    return result.error
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{
			"result.error",
			"result.ok == false",
		},
		EvidenceOrdered: []string{
			"`result.error` exists only for `result.ok == false`",
			"no stable guard proves `result.ok == false` before this read",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessAcceptsGuardedResultReads(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    if result.ok then
        return result.value
    else
        return result.error
    end
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no result-shape exhaustiveness warning for guarded reads", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsConcreteResultCaseRead(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    result = { ok = true, value = "fresh" }
    return result.value
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no result-shape exhaustiveness warning for concrete success case", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessReportsResultGuardInvalidatedBeforeRead(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>, replacement: Result<string>): string
    if result.ok then
        result = replacement
        return result.value
    else
        return ""
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
		MessageContains: []string{
			"result.value",
			"result.ok == true",
		},
		EvidenceOrdered: []string{
			"`result.value` exists only for `result.ok == true`",
			"no stable guard proves `result.ok == true` before this read",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessReportsStructuralOkEnvelopeRead(t *testing.T) {
	result := Check(`
type Decode<T> = { ok: true, payload: T } | { ok: false, reason: string }

local function use(decoded: Decode<string>): string
    return decoded.payload
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{
			"decoded.payload",
			"decoded.ok == true",
		},
		EvidenceOrdered: []string{
			"`decoded` is result-shaped and discriminated by `decoded.ok`",
			"`decoded.payload` exists only for `decoded.ok == true`",
			"no stable guard proves `decoded.ok == true` before this read",
		},
	})
}

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
	result := Check(`
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
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
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

func TestDiscriminatedUnionExhaustivenessReportsMissingRegistrationCase(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local router: any = {}
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{
			"registered callbacks are not exhaustive",
			"router.cancel",
		},
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"registered cases: `router.begin`, `router.commit`",
			"missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
		},
		LabelContains: []string{"registration call", "dispatch call"},
		HelpContains:  []string{"Register each missing case"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: ` + "`router.cancel`" + `
 --> test.lua:11:13
   |
11 | local out = router:dispatch(action)
   |             ↑ dispatch call

because:
  1. proven: ` + "`router`" + ` is dispatched with discriminant ` + "`action.kind`" + `
  2. proven: possible cases: ` + "`action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`" + `
  3. proven: registered cases: ` + "`router.begin`, `router.commit`" + `
 --> test.lua:7:1
  |
7 | router:on("begin", function(action: Action): string return action.kind end)
  | ↑ registration call
  4. missing proof: missing registrations: ` + "`router.cancel`" + ` for ` + "`action.kind == \"cancel\"`" + `

help: Register each missing case, or dispatch through an explicit fallback when missing registrations are intentional.`
	assertRenderedEqual(t, rendered, want)
}

func TestDiscriminatedUnionExhaustivenessAcceptsCompleteRegistrations(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local router: any = {}
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
router:on("cancel", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning for complete registrations", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsDynamicRegistrationKey(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local router: any = {}
local key = "cancel"
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
router:on(key, function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning for dynamic registration key", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsRegistrationsAfterUnknownReceiverMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local router: any = {}
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
router:reset()

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after unknown receiver call can mutate registry", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessKeepsRegistrationsAfterUnrelatedUnknownCall(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local router: any = {}
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
observe_unrelated("ready")

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, WithGlobals("observe_unrelated"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "router.cancel"},
	})
}

func TestDiscriminatedUnionExhaustivenessHandlesFreeFunctionRegistrations(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local router: any = {}
register(router, "begin", function(action: Action): string return action.kind end)
register(router, "commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = dispatch(router, action)
`, WithGlobals("register", "dispatch"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "router.cancel"},
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `action.kind`",
			"missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
		},
	})
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

func TestOptionalExhaustivenessReportsConsumedValueWithoutNilCase(t *testing.T) {
	src := strings.TrimLeft(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    if maybe ~= nil then
        sink.seen = maybe
    end
    return sink.seen
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
		Line:            4,
		Column:          8,
		MessageContains: []string{
			"optional handling is not exhaustive",
			"maybe == nil",
		},
		EvidenceOrdered: []string{
			"branch checks optional `maybe`",
			"possible cases: `maybe ~= nil`, `maybe == nil`",
			"consumed case: `maybe ~= nil`",
			"missing cases: `maybe == nil`",
			"no else branch handles the remaining optional case",
		},
		LabelContains: []string{"optional case check"},
		HelpContains:  []string{"Handle the nil case"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: optional handling is not exhaustive; missing case: ` + "`maybe == nil`" + `
 --> test.lua:4:8
  |
4 |     if maybe ~= nil then
  |        ↑ optional case check

because:
  1. proven: branch checks optional ` + "`maybe`" + `
  2. proven: possible cases: ` + "`maybe ~= nil`, `maybe == nil`" + `
  3. proven: consumed case: ` + "`maybe ~= nil`" + `
  4. missing proof: missing cases: ` + "`maybe == nil`" + `
  5. missing proof: no else branch handles the remaining optional case

help: Handle the nil case with an else branch, or return before continuing when nil is intentionally ignored.`
	assertRenderedEqual(t, rendered, want)
}

func TestOptionalExhaustivenessAcceptsExplicitNilHandlingAndGuardReturn(t *testing.T) {
	withElse := Check(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    if maybe ~= nil then
        sink.seen = maybe
    else
        sink.seen = "missing"
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(withElse.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning with else fallback", withElse.Diagnostics)
	}

	guardReturn := Check(`
local function value_or_default(maybe: string?): string
    if maybe ~= nil then
        return maybe
    end
    return "fallback"
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(guardReturn.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning for guard-return fallback", guardReturn.Diagnostics)
	}

	nilGuard := Check(`
local function value_or_default(maybe: string?): string
    if maybe == nil then
        return "fallback"
    end
    return maybe
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(nilGuard.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after nil guard", nilGuard.Diagnostics)
	}
}

func TestOptionalExhaustivenessHandlesTruthyOptionalWithoutBooleanFalsePositive(t *testing.T) {
	stringOptional := Check(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    if maybe then
        sink.seen = maybe
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, stringOptional, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{
			"optional handling is not exhaustive",
			"maybe == nil",
		},
		EvidenceOrdered: []string{
			"branch checks optional `maybe`",
			"consumed case: `maybe ~= nil`",
			"missing cases: `maybe == nil`",
		},
	})

	booleanOptional := Check(`
type Sink = { seen: string }

local function remember(flag: boolean?, sink: Sink): string
    if flag then
        sink.seen = "true"
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(booleanOptional.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning when false is a non-nil value", booleanOptional.Diagnostics)
	}
}

func hasDiagnosticCode(diags []diagnostic.Diagnostic, code diagnostic.Code) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}
