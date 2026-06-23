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
	src := strings.TrimLeft(`
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
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            8,
		Column:          8,
		MessageContains: []string{"env.payload.kind == \"tick\""},
		EvidenceMin:     5,
		EvidenceOrdered: []string{
			"branch chain checks discriminant `env.payload.kind`",
			"possible cases: `env.payload.kind == \"created\"`, `env.payload.kind == \"deleted\"`, `env.payload.kind == \"tick\"`",
			"handled cases: `env.payload.kind == \"created\"`, `env.payload.kind == \"deleted\"`",
			"missing cases: `env.payload.kind == \"tick\"`",
			"no default branch handles the remaining union cases",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"branch chain checks discriminant `env.payload.kind`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`env.payload.kind == \"created\"`", "`env.payload.kind == \"deleted\"`", "`env.payload.kind == \"tick\"`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"handled cases", "`env.payload.kind == \"created\"`, `env.payload.kind == \"deleted\"`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing cases", "`env.payload.kind == \"tick\"`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no default branch handles the remaining union cases"},
			},
		},
		LabelContains: []string{"union case check"},
		HelpContains:  []string{"Handle each missing case", "else branch"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: discriminated union handling is not exhaustive; missing case: `env.payload.kind == \"tick\"`",
			"test.lua:8:8",
			"8 |     if env.payload.kind == \"created\" then",
			"↑ union case check",
			"because:",
			"proven: branch chain checks discriminant `env.payload.kind`",
			"proven: possible cases: `env.payload.kind == \"created\"`, `env.payload.kind == \"deleted\"`, `env.payload.kind == \"tick\"`",
			"proven: handled cases: `env.payload.kind == \"created\"`, `env.payload.kind == \"deleted\"`",
			"missing proof: missing cases: `env.payload.kind == \"tick\"`",
			"missing proof: no default branch handles the remaining union cases",
			"help: Handle each missing case",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
		},
	})
}

func hasDiagnosticCode(diags []diagnostic.Diagnostic, code diagnostic.Code) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}
