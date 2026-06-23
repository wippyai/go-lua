package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDiscriminatedUnionExhaustivenessCountsNamedLocalCallbacks(t *testing.T) {
	src := `
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
`
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            20,
		Column:          13,
		Span: diagnostic.Span{
			StartLine: 20,
			StartCol:  13,
			EndLine:   20,
			EndCol:    34,
		},
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
				MessageContains: []string{"`router`", "`action.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "begin", "cancel", "commit"},
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
		LabelMin:      2,
		LabelContains: []string{"registration call", "dispatch call"},
		HelpContains: []string{
			"Register each missing case",
			"explicit fallback",
			"missing registrations are intentional",
		},
		Sources: diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: `router.cancel`",
			"test.lua:20:13",
			"20 | local out = router:dispatch(action)",
			"↑ dispatch call",
			"because:",
			"proven: `router` is dispatched with discriminant `action.kind`",
			"proven: possible cases: `action.kind == \"begin\"`, `action.kind == \"cancel\"`, `action.kind == \"commit\"`",
			"proven: registered cases: `router.begin`, `router.commit`",
			"test.lua:16:1",
			"16 | router:on(\"begin\", on_begin)",
			"↑ registration call",
			"missing proof: missing registrations: `router.cancel` for `action.kind == \"cancel\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
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
