package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

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

func TestDiscriminatedUnionExhaustivenessKeepsRegistrationsAfterKnownNonCaseDynamicAssignment(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "audit"
local router: any = {}
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
router[key] = nil

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

func TestDiscriminatedUnionExhaustivenessCountsStaticFieldCallbackRegistration(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local router: any = {}
router.begin = function(action: Action): string return action.kind end
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
			"`router` is dispatched with discriminant `action.kind`",
			"registered cases: `router.begin`, `router.commit`",
			"missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessInvalidatesOnlyWrittenRegistrationKey(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local router: any = {}
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
router.begin = nil

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
		MessageContains: []string{"registered callbacks are not exhaustive", "router.begin", "router.cancel"},
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `action.kind`",
			"registered cases: `router.commit`",
			"missing registrations: `router.begin` for `action.kind == \"begin\"`, `router.cancel` for `action.kind == \"cancel\"`",
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

func TestDiscriminatedUnionExhaustivenessDoesNotTypeNarrowCallbackFromRegistrationShape(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local tracker: any = {}
tracker:on("begin", function(action: Action): string return action.id end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = tracker:remember(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:     diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity: diagnostic.SeverityWarning,
		MessageContains: []string{
			"case-specific field read is not exhaustive",
			"action.id",
			"action.kind == \"begin\"",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessTypedCallbackSignatureNarrowsGenericEnvelope(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel
type Envelope<T> = { payload: T }

local function visit_begin(cb: fun(env: Envelope<Begin>): string): string
	return cb({ payload = { kind = "begin", id = "evt-1" } })
end

local out = visit_begin(function(env)
	return env.payload.id
end)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want typed callback signature to seed Envelope<Begin>", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessBroadTypedRegistrationDoesNotNarrowCallbackCase(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel
type Router = {
	on: fun(self: Router, key: string, cb: fun(action: Action): string): (),
	dispatch: fun(self: Router, action: Action): (),
}

local function make_router(): Router
	error("stub")
end

local router: Router = make_router()
router:on("begin", function(action)
	return action.id
end)

local action: Action = { kind = "begin", id = "evt-1" }
router:dispatch(action)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:     diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity: diagnostic.SeverityWarning,
		MessageContains: []string{
			"case-specific field read is not exhaustive",
			"action.id",
			"action.kind == \"begin\"",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessBroadDirectTypedCallbackWarnsInsideCallback(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local visit: fun(cb: fun(action: Action): string): string = function(cb)
	return cb({ kind = "begin", id = "evt-1" })
end

local out = visit(function(action)
	return action.id
end)
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:     diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity: diagnostic.SeverityWarning,
		MessageContains: []string{
			"case-specific field read is not exhaustive",
			"action.id",
			"action.kind == \"begin\"",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessSkipsRedundantCaseFieldWarningWhenOtherCaseProven(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Action = Begin | Commit

local action: Action = { kind = "commit", payment_id = "pay-1" }
if action.kind == "commit" then
	local id = action.id
else
	local id = action.id
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeMissingMember,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		MessageContains: []string{"id"},
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

func TestDiscriminatedUnionExhaustivenessKeepsRegistrationsAfterKnownNonCaseDynamicKey(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "audit"
local router: any = {}
router:on("begin", function(action: Action): string return action.kind end)
router:on("commit", function(action: Action): string return action.kind end)
router:on(key, function(action: Action): string return action.kind end)

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

func TestDiscriminatedUnionExhaustivenessAcceptsKnownCaseDynamicRegistration(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "cancel"
local router: any = {}
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
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after known dynamic registration", result.Diagnostics)
	}
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
