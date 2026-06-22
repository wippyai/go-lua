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

func TestDiscriminatedUnionExhaustivenessHandlesGenericEnvelopeRegistrationDispatch(t *testing.T) {
	result := Check(`
type Created = { kind: "created", id: string }
type Deleted = { kind: "deleted", id: string }
type Tick = { kind: "tick", elapsed: number }
type Payload = Created | Deleted | Tick
type Envelope<T> = { payload: T }

local router: any = {}
router:on("created", function(env: Envelope<Payload>): string return env.payload.kind end)
router:on("deleted", function(env: Envelope<Payload>): string return env.payload.kind end)

local env: Envelope<Payload> = { payload = { kind = "created", id = "evt-1" } }
local out = router:dispatch(env)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "router.tick"},
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `env.payload.kind`",
			"registered cases: `router.created`, `router.deleted`",
			"missing registrations: `router.tick` for `env.payload.kind == \"tick\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessAcceptsCompleteGenericEnvelopeRegistrationDispatch(t *testing.T) {
	result := Check(`
type Created = { kind: "created", id: string }
type Deleted = { kind: "deleted", id: string }
type Tick = { kind: "tick", elapsed: number }
type Payload = Created | Deleted | Tick
type Envelope<T> = { payload: T }

local router: any = {}
router:on("created", function(env: Envelope<Payload>): string return env.payload.kind end)
router:on("deleted", function(env: Envelope<Payload>): string return env.payload.kind end)
router:on("tick", function(env: Envelope<Payload>): string return env.payload.kind end)

local env: Envelope<Payload> = { payload = { kind = "created", id = "evt-1" } }
local out = router:dispatch(env)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning for complete generic envelope registrations", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessHandlesDeepGenericEnvelopeRegistrationDispatch(t *testing.T) {
	result := Check(`
type Created = { kind: "created", id: string }
type Deleted = { kind: "deleted", id: string }
type Tick = { kind: "tick", elapsed: number }
type Payload = Created | Deleted | Tick
type Layer<T> = { next: T }
type Envelope<T> = Layer<Layer<Layer<Layer<{ payload: T }>>>>

local router: any = {}
router:on("created", function(env: Envelope<Payload>): string return env.next.next.next.next.payload.kind end)
router:on("deleted", function(env: Envelope<Payload>): string return env.next.next.next.next.payload.kind end)

local env: Envelope<Payload> = {
    next = { next = { next = { next = { payload = { kind = "created", id = "evt-1" } } } } }
}
local out = router:dispatch(env)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "router.tick"},
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `env.next.next.next.next.payload.kind`",
			"registered cases: `router.created`, `router.deleted`",
			"missing registrations: `router.tick` for `env.next.next.next.next.payload.kind == \"tick\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessHandlesGenericEnvelopeFreeFunctionDispatch(t *testing.T) {
	result := Check(`
type Created = { kind: "created", id: string }
type Deleted = { kind: "deleted", id: string }
type Tick = { kind: "tick", elapsed: number }
type Payload = Created | Deleted | Tick
type Envelope<T> = { payload: T }

local router: any = {}
register(router, "created", function(env: Envelope<Payload>): string return env.payload.kind end)
register(router, "deleted", function(env: Envelope<Payload>): string return env.payload.kind end)

local env: Envelope<Payload> = { payload = { kind = "created", id = "evt-1" } }
local out = dispatch(router, env)
`, WithGlobals("register", "dispatch"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "router.tick"},
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `env.payload.kind`",
			"registered cases: `router.created`, `router.deleted`",
			"missing registrations: `router.tick` for `env.payload.kind == \"tick\"`",
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

func TestDiscriminatedUnionExhaustivenessReportsResultDiscriminantMutationBeforeRead(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
    if result.ok then
        result.ok = false
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
		Code:     diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity: diagnostic.SeverityWarning,
		MessageContains: []string{
			"result field read is not exhaustive",
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

func TestDiscriminatedUnionExhaustivenessKeepsRegistrationsAfterLocalReadOnlyCall(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function inspect_router(router): ()
end

local router: any = {}
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
inspect_router(router)

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, WithDiagnosticRule(
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

func TestDiscriminatedUnionExhaustivenessKeepsRegistrationsAfterStaticNonCaseAssignment(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local router: any = {}
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
router.metadata = "ready"

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, WithDiagnosticRule(
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
			"registered cases: `router.begin`, `router.commit`",
			"missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessReportsNestedRegistrationCase(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local app: any = { router = {} }
app.router:on("begin", function(action: Action): string return action.kind end)
app.router:on("commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = app.router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "app.router.cancel"},
		EvidenceOrdered: []string{
			"`app.router` is dispatched with discriminant `action.kind`",
			"registered cases: `app.router.begin`, `app.router.commit`",
			"missing registrations: `app.router.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessSkipsNestedRegistrationsAfterDynamicAssignment(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "cancel"
local app: any = { router = {} }
app.router:on("begin", function(action: Action): string return action.kind end)
app.router:on("commit", function(action: Action): string return action.kind end)
app.router[key] = function(action: Action): string return action.kind end

local action: Action = { kind = "begin", id = "evt-1" }
local out = app.router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after nested dynamic registration assignment", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsNestedRegistrationsAfterStaticAssignment(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local app: any = { router = {} }
app.router:on("begin", function(action: Action): string return action.kind end)
app.router:on("commit", function(action: Action): string return action.kind end)
app.router.cancel = function(action: Action): string return action.kind end

local action: Action = { kind = "begin", id = "evt-1" }
local out = app.router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after nested static registration assignment", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessHandlesNestedFreeFunctionRegistrations(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local app: any = { router = {} }
register(app.router, "begin", function(action: Action): string return action.kind end)
register(app.router, "commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = dispatch(app.router, action)
`, WithGlobals("register", "dispatch"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "app.router.cancel"},
		EvidenceOrdered: []string{
			"`app.router` is dispatched with discriminant `action.kind`",
			"registered cases: `app.router.begin`, `app.router.commit`",
			"missing registrations: `app.router.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessMatchesRegistrationAliasAtRegistrationPoint(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local app: any = { router = {} }
local router = app.router
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = app.router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "app.router.cancel"},
		EvidenceOrdered: []string{
			"`app.router` is dispatched with discriminant `action.kind`",
			"registered cases: `app.router.begin`, `app.router.commit`",
			"missing registrations: `app.router.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessAcceptsCompleteRegistrationAlias(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local app: any = { router = {} }
local router = app.router
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
router:on("cancel", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = app.router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning for complete alias registration", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessKeepsAliasRegistrationsAfterAliasReassignment(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local app: any = { router = {} }
local router = app.router
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
router = {}

local action: Action = { kind = "begin", id = "evt-1" }
local out = app.router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "app.router.cancel"},
		EvidenceOrdered: []string{
			"`app.router` is dispatched with discriminant `action.kind`",
			"registered cases: `app.router.begin`, `app.router.commit`",
			"missing registrations: `app.router.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessDoesNotCountRegistrationBeforeAliasPointsAtRegistry(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local app: any = { router = {} }
local router: any = {}
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
router = app.router

local action: Action = { kind = "begin", id = "evt-1" }
local out = app.router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning when registrations were on a previous alias target", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsRegistrationAliasStaticCaseFill(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local app: any = { router = {} }
local router = app.router
app.router:on("begin", function(action: Action): string return action.kind end)
app.router:on("commit", function(action: Action): string return action.kind end)
router.cancel = function(action: Action): string return action.kind end

local action: Action = { kind = "begin", id = "evt-1" }
local out = app.router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after alias writes missing case registration", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsRegistrationAliasDynamicMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local app: any = { router = {} }
local router = app.router
local key = "cancel"
app.router:on("begin", function(action: Action): string return action.kind end)
app.router:on("commit", function(action: Action): string return action.kind end)
router[key] = function(action: Action): string return action.kind end

local action: Action = { kind = "begin", id = "evt-1" }
local out = app.router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after alias dynamic registration mutation", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsNestedFreeFunctionRegistrationsAfterDynamicKey(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "cancel"
local app: any = { router = {} }
register(app.router, "begin", function(action: Action): string return action.kind end)
register(app.router, "commit", function(action: Action): string return action.kind end)
register(app.router, key, function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = dispatch(app.router, action)
`, WithGlobals("register", "dispatch"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after nested dynamic-key free registration", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessCountsNamedLocalCallbacks(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function on_begin(action: Action): string
    return action.kind
end

local on_commit = function(action: Action): string
    return action.kind
end

local router: any = {}
router:on("begin", on_begin)
router:on("commit", on_commit)

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, WithDiagnosticRule(
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
			"registered cases: `router.begin`, `router.commit`",
			"missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessDoesNotCountReassignedNamedCallback(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local on_begin: any = function(action: Action): string
    return action.kind
end
on_begin = nil

local router: any = {}
router:on("begin", on_begin)
router:on("commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning when a registration callback is no longer known callable", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessCountsAliasedRegistrationCallback(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local on_begin: any = function(action: Action): string
    return action.kind
end
local begin_callback = on_begin

local router: any = {}
router:on("begin", begin_callback)
router:on("commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "router.cancel"},
		EvidenceOrdered: []string{
			"registered cases: `router.begin`, `router.commit`",
			"missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessDoesNotCountReassignedCallbackAlias(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local on_begin: any = function(action: Action): string
    return action.kind
end
local begin_callback = on_begin
begin_callback = nil

local router: any = {}
router:on("begin", begin_callback)
router:on("commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning when callback alias is no longer known callable", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsRegistrationsAfterLocalDynamicMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function reset_router(router, key: string): ()
    router[key] = nil
end

local router: any = {}
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
reset_router(router, "cancel")

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after local dynamic mutation invalidates registration proof", result.Diagnostics)
	}
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

func TestOptionalExhaustivenessDoesNotUseInvalidatedValueProof(t *testing.T) {
	result := Check(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    if maybe ~= nil then
        maybe = nil
        sink.seen = maybe
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after reassignment invalidates value proof", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessReportsAliasConsumedBeforeOriginalInvalidated(t *testing.T) {
	result := Check(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    if maybe ~= nil then
        local alias = maybe
        maybe = nil
        sink.seen = alias
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"optional handling is not exhaustive", "maybe == nil"},
		EvidenceOrdered: []string{
			"branch checks optional `maybe`",
			"consumed case: `maybe ~= nil`",
			"missing cases: `maybe == nil`",
		},
	})
}

func TestOptionalExhaustivenessDoesNotUseInvalidatedFieldProof(t *testing.T) {
	result := Check(`
type Box = { value: string? }
type Sink = { seen: string }

local function remember(box: Box, sink: Sink): string
    if box.value ~= nil then
        box.value = nil
        sink.seen = box.value
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after field reassignment invalidates value proof", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessDoesNotUseDynamicIndexInvalidatedFieldProof(t *testing.T) {
	result := Check(`
type Box = { value: string? }
type Sink = { seen: string }

local function remember(box: Box, key: string, sink: Sink): string
    if box.value ~= nil then
        box[key] = nil
        sink.seen = box.value
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after dynamic index write invalidates field proof", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessDoesNotUseCallInvalidatedFieldProof(t *testing.T) {
	result := Check(`
type Box = { value: string? }
type Sink = { seen: string }

local function clear(box: Box, key: string): ()
    box[key] = nil
end

local function remember(box: Box, sink: Sink): string
    if box.value ~= nil then
        clear(box, "value")
        sink.seen = box.value
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after call invalidates field proof", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessDoesNotUseAllBranchesInvalidatedValueProof(t *testing.T) {
	result := Check(`
type Sink = { seen: string }

local function remember(maybe: string?, flag: boolean, sink: Sink): string
    if maybe ~= nil then
        if flag then
            maybe = nil
        else
            maybe = nil
        end
        sink.seen = maybe
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after every branch invalidates value proof", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessReportsReachableBranchConsumptionBeforeInvalidation(t *testing.T) {
	result := Check(`
type Sink = { seen: string }

local function remember(maybe: string?, flag: boolean, sink: Sink): string
    if maybe ~= nil then
        if flag then
            sink.seen = maybe
        else
            maybe = nil
        end
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"optional handling is not exhaustive", "maybe == nil"},
		EvidenceOrdered: []string{
			"branch checks optional `maybe`",
			"consumed case: `maybe ~= nil`",
			"missing cases: `maybe == nil`",
		},
	})
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

func TestOptionalExhaustivenessAcceptsErrorTerminatedValueBranch(t *testing.T) {
	result := Check(`
local function value_or_default(maybe: string?): string
    if maybe ~= nil then
        error(maybe)
    end
    return "fallback"
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning when value branch raises", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessDoesNotTreatShadowedErrorAsTerminating(t *testing.T) {
	result := Check(`
local function value_or_default(maybe: string?): string
    local function error(message: string): () end
    if maybe ~= nil then
        error(maybe)
    end
    return "fallback"
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"optional handling is not exhaustive", "maybe == nil"},
		EvidenceOrdered: []string{
			"branch checks optional `maybe`",
			"consumed case: `maybe ~= nil`",
			"missing cases: `maybe == nil`",
		},
	})
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
