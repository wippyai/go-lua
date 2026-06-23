package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

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
