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
	src := strings.TrimLeft(`
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
`, "\n")
	result := Check(src, WithGlobals("observe_unrelated"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            12,
		Column:          13,
		MessageContains: []string{"registered callbacks are not exhaustive", "router.cancel"},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`router` is dispatched with discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases: `router.begin`, `router.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations: `router.cancel` for `action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"registration call", "dispatch call"},
		HelpContains:  []string{"Register each missing case", "explicit fallback"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: ` + "`router.cancel`" + `
 --> test.lua:12:13
   |
12 | local out = router:dispatch(action)
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
	src := strings.TrimLeft(`
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
`, "\n")
	requireDiagnostic(t, Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	)), diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            12,
		Column:          13,
		Span:            diagnostic.Span{StartLine: 12, StartCol: 13, EndLine: 12, EndCol: 34},
		MessageContains: []string{"registered callbacks are not exhaustive", "router.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"registered cases: `router.begin`, `router.commit`",
			"missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`router` is dispatched with discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases", "`router.begin`", "`router.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations", "`router.cancel`", "`action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"registration call", "dispatch call"},
		HelpContains:  []string{"Register each missing case", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: `router.cancel`",
			"--> test.lua:12:13",
			"12 | local out = router:dispatch(action)",
			"↑ dispatch call",
			"because:",
			"1. proven: `router` is dispatched with discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: registered cases: `router.begin`, `router.commit`",
			"--> test.lua:7:1",
			"7 | router:on(\"begin\", function(action: Action): string return action.kind end)",
			"↑ registration call",
			"4. missing proof: missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"router.metadata",
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessKeepsRegistrationsAfterKnownNonCaseDynamicAssignment(t *testing.T) {
	src := strings.TrimLeft(`
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
`, "\n")
	requireDiagnostic(t, Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	)), diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            13,
		Column:          13,
		Span:            diagnostic.Span{StartLine: 13, StartCol: 13, EndLine: 13, EndCol: 34},
		MessageContains: []string{"registered callbacks are not exhaustive", "router.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"registered cases: `router.begin`, `router.commit`",
			"missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`router` is dispatched with discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases", "`router.begin`", "`router.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations", "`router.cancel`", "`action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"registration call", "dispatch call"},
		HelpContains:  []string{"Register each missing case", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: `router.cancel`",
			"--> test.lua:13:13",
			"13 | local out = router:dispatch(action)",
			"↑ dispatch call",
			"because:",
			"1. proven: `router` is dispatched with discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: registered cases: `router.begin`, `router.commit`",
			"--> test.lua:8:1",
			"8 | router:on(\"begin\", function(action: Action): string return action.kind end)",
			"↑ registration call",
			"4. missing proof: missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"router[key]",
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessCountsStaticFieldCallbackRegistration(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local router: any = {}
router.begin = function(action: Action): string return action.kind end
router:on("commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = router:dispatch(action)
`, "\n")
	requireDiagnostic(t, Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	)), diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            11,
		Column:          13,
		Span:            diagnostic.Span{StartLine: 11, StartCol: 13, EndLine: 11, EndCol: 34},
		MessageContains: []string{"registered callbacks are not exhaustive", "router.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"registered cases: `router.begin`, `router.commit`",
			"missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`router` is dispatched with discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases", "`router.begin`", "`router.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations", "`router.cancel`", "`action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"registration call", "dispatch call"},
		HelpContains:  []string{"Register each missing case", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: `router.cancel`",
			"--> test.lua:11:13",
			"11 | local out = router:dispatch(action)",
			"↑ dispatch call",
			"because:",
			"1. proven: `router` is dispatched with discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: registered cases: `router.begin`, `router.commit`",
			"--> test.lua:7:1",
			"7 | router.begin = function(action: Action): string return action.kind end",
			"↑ registration call",
			"4. missing proof: missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"router.cancel, router.cancel",
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessInvalidatesOnlyWrittenRegistrationKey(t *testing.T) {
	src := strings.TrimLeft(`
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
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            12,
		Column:          13,
		MessageContains: []string{"registered callbacks are not exhaustive", "router.begin", "router.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"registered cases: `router.commit`",
			"missing registrations: `router.begin` for `action.kind == \"begin\"`, `router.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`router` is dispatched with discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases", "`router.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations", "`router.begin`", "`router.cancel`"},
			},
		},
		LabelContains: []string{"registration call", "dispatch call"},
		HelpContains:  []string{"Register each missing case", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive",
			"--> test.lua:12:13",
			"12 | local out = router:dispatch(action)",
			"↑ dispatch call",
			"because:",
			"1. proven: `router` is dispatched with discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: registered cases: `router.commit`",
			"--> test.lua:8:1",
			"8 | router:on(\"commit\", function(action: Action): string return action.kind end)",
			"↑ registration call",
			"4. missing proof: missing registrations: `router.begin` for `action.kind == \"begin\"`, `router.cancel` for `action.kind == \"cancel\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"registered cases: `router.begin`",
			"want string",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessReportsNestedRegistrationCase(t *testing.T) {
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local app: any = { router = {} }
app.router:on("begin", function(action: Action): string return action.kind end)
app.router:on("commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = app.router:dispatch(action)
`, "\n")
	requireDiagnostic(t, Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	)), diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            11,
		Column:          13,
		Span:            diagnostic.Span{StartLine: 11, StartCol: 13, EndLine: 11, EndCol: 38},
		MessageContains: []string{"registered callbacks are not exhaustive", "app.router.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`app.router` is dispatched with discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"registered cases: `app.router.begin`, `app.router.commit`",
			"missing registrations: `app.router.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`app.router` is dispatched with discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases", "`app.router.begin`", "`app.router.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations", "`app.router.cancel`", "`action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"registration call", "dispatch call"},
		HelpContains:  []string{"Register each missing case", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: `app.router.cancel`",
			"--> test.lua:11:13",
			"11 | local out = app.router:dispatch(action)",
			"↑ dispatch call",
			"because:",
			"1. proven: `app.router` is dispatched with discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: registered cases: `app.router.begin`, `app.router.commit`",
			"--> test.lua:7:1",
			"7 | app.router:on(\"begin\", function(action: Action): string return action.kind end)",
			"↑ registration call",
			"4. missing proof: missing registrations: `app.router.cancel` for `action.kind == \"cancel\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"`router.cancel`",
			"want string",
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
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local app: any = { router = {} }
register(app.router, "begin", function(action: Action): string return action.kind end)
register(app.router, "commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = dispatch(app.router, action)
`, "\n")
	requireDiagnostic(t, Check(src, WithGlobals("register", "dispatch"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	)), diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            11,
		Column:          13,
		Span:            diagnostic.Span{StartLine: 11, StartCol: 13, EndLine: 11, EndCol: 39},
		MessageContains: []string{"registered callbacks are not exhaustive", "app.router.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`app.router` is dispatched with discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"registered cases: `app.router.begin`, `app.router.commit`",
			"missing registrations: `app.router.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`app.router` is dispatched with discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases", "`app.router.begin`", "`app.router.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations", "`app.router.cancel`", "`action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"registration call", "dispatch call"},
		HelpContains:  []string{"Register each missing case", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: `app.router.cancel`",
			"--> test.lua:11:13",
			"11 | local out = dispatch(app.router, action)",
			"↑ dispatch call",
			"because:",
			"1. proven: `app.router` is dispatched with discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: registered cases: `app.router.begin`, `app.router.commit`",
			"--> test.lua:7:1",
			"7 | register(app.router, \"begin\", function(action: Action): string return action.kind end)",
			"↑ registration call",
			"4. missing proof: missing registrations: `app.router.cancel` for `action.kind == \"cancel\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"`router.cancel`",
			"want string",
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
	src := strings.TrimLeft(`
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
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            12,
		Column:          13,
		MessageContains: []string{"registered callbacks are not exhaustive", "app.router.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`app.router` is dispatched with discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"registered cases: `app.router.begin`, `app.router.commit`",
			"missing registrations: `app.router.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`app.router` is dispatched with discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases", "`app.router.begin`", "`app.router.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations", "`app.router.cancel`", "`action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"registration call", "dispatch call"},
		HelpContains:  []string{"Register each missing case", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: `app.router.cancel`",
			"--> test.lua:12:13",
			"12 | local out = app.router:dispatch(action)",
			"↑ dispatch call",
			"because:",
			"1. proven: `app.router` is dispatched with discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: registered cases: `app.router.begin`, `app.router.commit`",
			"--> test.lua:8:1",
			"8 | router:on(\"begin\", function(action: Action): string return action.kind end)",
			"↑ registration call",
			"4. missing proof: missing registrations: `app.router.cancel` for `action.kind == \"cancel\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"`router.cancel`",
			"want string",
			"^~",
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
	src := strings.TrimLeft(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local router: any = {}
register(router, "begin", function(action: Action): string return action.kind end)
register(router, "commit", function(action: Action): string return action.kind end)

local action: Action = { kind = "begin", id = "evt-1" }
local out = dispatch(router, action)
`, "\n")
	result := Check(src, WithGlobals("register", "dispatch"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            11,
		Column:          13,
		MessageContains: []string{"registered callbacks are not exhaustive", "router.cancel"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `action.kind`",
			"possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"registered cases: `router.begin`, `router.commit`",
			"missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`router` is dispatched with discriminant `action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`action.kind == \"begin\"`", "`action.kind == \"cancel\"`", "`action.kind == \"commit\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases", "`router.begin`", "`router.commit`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations", "`router.cancel`", "`action.kind == \"cancel\"`"},
			},
		},
		LabelContains: []string{"registration call", "dispatch call"},
		HelpContains:  []string{"Register each missing case", "explicit fallback"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: `router.cancel`",
			"--> test.lua:11:13",
			"11 | local out = dispatch(router, action)",
			"↑ dispatch call",
			"because:",
			"1. proven: `router` is dispatched with discriminant `action.kind`",
			"2. proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"3. proven: registered cases: `router.begin`, `router.commit`",
			"--> test.lua:7:1",
			"7 | register(router, \"begin\", function(action: Action): string return action.kind end)",
			"↑ registration call",
			"4. missing proof: missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"router.cancel, router.cancel",
			"want string",
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
