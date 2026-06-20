package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestGenericIdentityDoesNotLaunderExplicitAnyIntoRecordAssignment(t *testing.T) {
	result := Check(`
local function id<T>(x: T): T
    return x
end

local raw = ({ id = "ok" } :: any)
local req: { id: string } = id(raw)
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallResultAssignment)
	if got := diag.Explanation.String(); !strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "call result 1 comes from any/unknown") ||
		!strings.Contains(got, "no proof on this path shows call result 1 is {id: string}") {
		t.Fatalf("explanation = %q, want explicit-any claim and missing-proof evidence", got)
	}
}

func TestGenericIdentityDoesNotLaunderExplicitAnyIntoRecordCall(t *testing.T) {
	src := strings.TrimLeft(`
local function id<T>(x: T): T
    return x
end

local function accept(req: { id: string }): ()
end

local raw = ({ id = "ok" } :: any)
accept(id(raw))
`, "\n")
	result := Check(src)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	if got := diag.Explanation.String(); !strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "no proof on this path shows id(...) satisfies the parameter type") {
		t.Fatalf("explanation = %q, want explicit-any claim and missing-proof evidence", got)
	}
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.call.direct.argument_type]: argument 1 (id(...)) is any, not {id: string}
 --> test.lua:9:8
  |
9 | accept(id(raw))
  |        ↑ argument value

because:
  1. proven: argument 1 (id(...)) has type any
  2. claimed: accept parameter 1 expects {id: string}
 --> test.lua:5:28
  |
5 | local function accept(req: { id: string }): ()
  |                            ^
  3. claimed: user asserted any; not abstract-interpreter proof
  4. unvalidated value: argument 1 (id(...)) comes from any/unknown
  5. missing proof: no proof on this path shows id(...) satisfies the parameter type

help: Validate or narrow ` + "`id(...)`" + ` before passing it; any/unknown values do not prove parameter contracts.`
	assertRenderedEqual(t, rendered, want)
}

func TestGenericIdentityChainDoesNotLaunderExplicitAnyIntoRecordCall(t *testing.T) {
	result := Check(`
local function id<T>(x: T): T
    return x
end

local function again<T>(x: T): T
    return x
end

local function accept(req: { id: string }): ()
end

local raw = ({ id = "ok" } :: any)
accept(again(id(raw)))
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	if got := diag.Explanation.String(); !strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "no proof on this path shows again(...) satisfies the parameter type") {
		t.Fatalf("explanation = %q, want explicit-any claim and missing-proof evidence", got)
	}
}

func TestSummaryObligationDoesNotLaunderExplicitAnyIntoForwardedRecordCall(t *testing.T) {
	result := Check(`
local function accept(req: { id: string }): ()
end

local function forward(payload)
    accept(payload)
end

local raw = ({ id = "ok" } :: any)
	forward(raw)
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	requireEvidenceMessage(t, diag, "inside forward, argument 1 (raw) must satisfy {id: string}")
	if got := diag.Explanation.String(); !strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "no proof on this path shows raw satisfies the parameter type") {
		t.Fatalf("explanation = %q, want explicit-any callee obligation and missing-proof evidence", got)
	}
}

func TestCastClaimedValueDoesNotLaunderThroughGenericCall(t *testing.T) {
	// A cast adopts its target type for local inference, but the underlying any
	// value does not gain proof: flowing the cast result through a generic call
	// into a record parameter still leaves the contract unproven.
	result := Check(`
local function id<T>(x: T): T
    return x
end

local function accept(req: { id: string }): ()
end

local raw = ({ id = "ok" } :: any)
local trusted = raw :: { id: string }
accept(id(trusted))
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	if got := diag.Explanation.String(); !strings.Contains(got, "no proof on this path shows id(...) satisfies the parameter type") {
		t.Fatalf("explanation = %q, want missing-proof evidence for laundered cast", got)
	}
}

func TestGenericAnyFirstReturnDoesNotPoisonSecondReturnSlot(t *testing.T) {
	result := Check(`
local function pair<T>(x: T): (T, string)
    return x, "ok"
end

local raw = ({ id = "ok" } :: any)
local req: { id: string }, label: string = pair(raw)
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want exactly one diagnostic for first return slot", result.Diagnostics)
	}
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallResultAssignment)
	if !strings.Contains(diag.Message, "call result 1") || strings.Contains(diag.Message, "call result 2") {
		t.Fatalf("message = %q, want first result slot only", diag.Message)
	}
	if got := diag.Explanation.String(); !strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "call result 1 comes from any/unknown") ||
		!strings.Contains(got, "no proof on this path shows call result 1 is {id: string}") {
		t.Fatalf("explanation = %q, want explicit-any claim and missing-proof evidence", got)
	}
}
