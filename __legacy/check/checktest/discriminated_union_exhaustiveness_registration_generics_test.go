package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDiscriminatedUnionExhaustivenessHandlesGenericEnvelopeRegistrationDispatch(t *testing.T) {
	src := `
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
`
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            13,
		Column:          13,
		Span: diagnostic.Span{
			StartLine: 13,
			StartCol:  13,
			EndLine:   13,
			EndCol:    31,
		},
		MessageContains: []string{"registered callbacks are not exhaustive", "router.tick"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `env.payload.kind`",
			"possible cases: `env.payload.kind == \"created\"`, `env.payload.kind == \"deleted\"`, `env.payload.kind == \"tick\"`",
			"registered cases: `router.created`, `router.deleted`",
			"missing registrations: `router.tick` for `env.payload.kind == \"tick\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`router`", "`env.payload.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "created", "deleted", "tick"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases", "`router.created`", "`router.deleted`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations", "`router.tick`", "`env.payload.kind == \"tick\"`"},
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
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: `router.tick`",
			"test.lua:13:13",
			"13 | local out = router:dispatch(env)",
			"↑ dispatch call",
			"because:",
			"proven: `router` is dispatched with discriminant `env.payload.kind`",
			"proven: possible cases: `env.payload.kind == \"created\"`, `env.payload.kind == \"deleted\"`, `env.payload.kind == \"tick\"`",
			"proven: registered cases: `router.created`, `router.deleted`",
			"test.lua:9:1",
			"9 | router:on(\"created\", function(env: Envelope<Payload>): string return env.payload.kind end)",
			"↑ registration call",
			"missing proof: missing registrations: `router.tick` for `env.payload.kind == \"tick\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
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
	src := `
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
`
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "router.tick"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `env.next.next.next.next.payload.kind`",
			"possible cases: `env.next.next.next.next.payload.kind == \"created\"`, `env.next.next.next.next.payload.kind == \"deleted\"`, `env.next.next.next.next.payload.kind == \"tick\"`",
			"registered cases: `router.created`, `router.deleted`",
			"missing registrations: `router.tick` for `env.next.next.next.next.payload.kind == \"tick\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`router`", "`env.next.next.next.next.payload.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "created", "deleted", "tick"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases", "`router.created`", "`router.deleted`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations", "`router.tick`", "`env.next.next.next.next.payload.kind == \"tick\"`"},
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
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: `router.tick`",
			"local out = router:dispatch(env)",
			"↑ dispatch call",
			"because:",
			"proven: `router` is dispatched with discriminant `env.next.next.next.next.payload.kind`",
			"proven: possible cases: `env.next.next.next.next.payload.kind == \"created\"`, `env.next.next.next.next.payload.kind == \"deleted\"`, `env.next.next.next.next.payload.kind == \"tick\"`",
			"proven: registered cases: `router.created`, `router.deleted`",
			"missing proof: missing registrations: `router.tick` for `env.next.next.next.next.payload.kind == \"tick\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessHandlesGenericEnvelopeFreeFunctionDispatch(t *testing.T) {
	src := `
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
`
	result := Check(src, WithGlobals("register", "dispatch"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"registered callbacks are not exhaustive", "router.tick"},
		EvidenceMin:     4,
		EvidenceOrdered: []string{
			"`router` is dispatched with discriminant `env.payload.kind`",
			"possible cases: `env.payload.kind == \"created\"`, `env.payload.kind == \"deleted\"`, `env.payload.kind == \"tick\"`",
			"registered cases: `router.created`, `router.deleted`",
			"missing registrations: `router.tick` for `env.payload.kind == \"tick\"`",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"`router`", "`env.payload.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "created", "deleted", "tick"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"registered cases", "`router.created`", "`router.deleted`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing registrations", "`router.tick`", "`env.payload.kind == \"tick\"`"},
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
			"warning[lint.union.exhaustiveness]: registered callbacks are not exhaustive; missing registration: `router.tick`",
			"local out = dispatch(router, env)",
			"↑ dispatch call",
			"because:",
			"proven: `router` is dispatched with discriminant `env.payload.kind`",
			"proven: possible cases: `env.payload.kind == \"created\"`, `env.payload.kind == \"deleted\"`, `env.payload.kind == \"tick\"`",
			"proven: registered cases: `router.created`, `router.deleted`",
			"missing proof: missing registrations: `router.tick` for `env.payload.kind == \"tick\"`",
			"help: Register each missing case",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
		},
	})
}
