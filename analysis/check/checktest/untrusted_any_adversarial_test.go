package checktest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestGenericIdentityDoesNotLaunderExplicitAnyIntoRecordAssignment(t *testing.T) {
	src := strings.TrimLeft(`
local function id<T>(x: T): T
    return x
end

local raw = ({ id = "ok" } :: any)
local req: { id: string } = id(raw)
`, "\n")
	result := Check(src)
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallResultAssignment,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            6,
		Column:          29,
		MessageContains: []string{
			"call result 1",
			"any",
			"not {id: string}",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"id returns any"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"assignment target req requires {id: string}"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"user asserted any", "not abstract-interpreter proof"},
			},
			{
				Kind:            diagnostic.EvidencePrecisionBoundary,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"call result 1 comes from any/unknown"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no proof on this path", "call result 1", "{id: string}"},
			},
		},
		LabelContains: []string{"declared type", "call result"},
		HelpContains:  []string{"Assign the call result", "compatible target type", "change the callee return type"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			`error[type.call.direct.result_assignment]: call result 1 is any, not {id: string}`,
			`  |            ↓ declared type`,
			`6 | local req: { id: string } = id(raw)`,
			`  |                             ↑ call result`,
			`1. proven: id returns any`,
			`2. claimed: assignment target req requires {id: string}`,
			`3. claimed: user asserted any; not abstract-interpreter proof`,
			`4. unvalidated value: call result 1 comes from any/unknown`,
			`5. missing proof: no proof on this path shows call result 1 is {id: string}`,
			`help: Assign the call result to a compatible target type, or change the callee return type if this result is valid.`,
		},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.call.direct.result_assignment]: call result 1 is any, not {id: string}
 --> test.lua:6:29
  |
  |            ↓ declared type
6 | local req: { id: string } = id(raw)
  |                             ↑ call result

because:
  1. proven: id returns any
  2. claimed: assignment target req requires {id: string}
  3. claimed: user asserted any; not abstract-interpreter proof
  4. unvalidated value: call result 1 comes from any/unknown
  5. missing proof: no proof on this path shows call result 1 is {id: string}

help: Assign the call result to a compatible target type, or change the callee return type if this result is valid.`
	assertRenderedEqual(t, rendered, want)
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
	want := `error[type.call.direct.argument_type]: argument 1 (id(...)) comes from any/unknown; no proof shows it is {id: string}
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

func TestUntrustedOptionalAnyArgumentReportsOneDiagnostic(t *testing.T) {
	result := Check(`
local function need(id: string): () end

local function f(raw: any?): ()
    need(raw)
end
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one diagnostic for the same untrusted optional argument", result.Diagnostics)
	}
	msg := result.Diagnostics[0].Message
	if !strings.Contains(msg, "raw") || (!strings.Contains(msg, "may be nil") && !strings.Contains(msg, "any/unknown")) {
		t.Fatalf("diagnostic message = %q, want actionable nil or any/unknown cause", msg)
	}
}

func TestTruthyChildGuardDoesNotLaunderExplicitAnyMember(t *testing.T) {
	result := Check(`
local raw = ({ id = 1 } :: any)
if raw.id then
    local id: string = raw.id
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            4,
		MessageContains: []string{
			"raw.id",
			"any",
			"not string",
		},
	})
}

func TestLiteralChildGuardDoesNotLaunderExplicitAnySibling(t *testing.T) {
	result := Check(`
local raw = ({ kind = "task", route_id = "start" } :: any)
if raw.kind == "task" then
    local route_id: string = raw.route_id
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            4,
		MessageContains: []string{
			"raw.route_id",
			"any",
			"not string",
		},
	})
}

func TestLiteralChildGuardDoesNotLaunderAnnotatedAnySibling(t *testing.T) {
	result := Check(`
local raw: any = { kind = "task", route_id = "start" }
if raw.kind == "task" then
    local route_id: string = raw.route_id
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            4,
		MessageContains: []string{
			"raw.route_id",
			"any",
			"not string",
		},
	})
}

func TestScalarTypeGuardSurvivesLiteralExclusionOnAnyChild(t *testing.T) {
	result := Check(`
local function need(kind: string): () end

local function decode(raw: any): ()
    if type(raw.kind) ~= "string" then
        return
    end
    if raw.kind == "task" then
        return
    end
    if raw.kind == "timer" then
        return
    end
    need(raw.kind)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want scalar type guard to validate raw.kind", result.Diagnostics)
	}
}

func TestScalarTypeGuardSurvivesLiteralExclusionInsideReturnedObjectCall(t *testing.T) {
	result := Check(`
local function err(code: string, message: string): {code: string, message: string}
    return {code = code, message = message}
end

local function decode(raw: any): {ok: false, error: {code: string, message: string}}?
    if type(raw.kind) ~= "string" then
        return nil
    end
    if raw.kind == "task" then
        return nil
    end
    if raw.kind == "timer" then
        return nil
    end
    return {ok = false, error = err("unknown_kind", raw.kind)}
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want scalar type guard to validate raw.kind in returned object call", result.Diagnostics)
	}
}

func TestScalarTypeGuardSurvivesLiteralExclusionInsideImportedReturnedObjectCall(t *testing.T) {
	protocol := CheckFileAndExport(`
local M = {}
function M.err(code: string, message: string): {code: string, message: string}
    return {code = code, message = message}
end
return M
`, "protocol", "protocol.lua")
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want none", protocol.Errors)
	}
	result := CheckFile(`
local protocol = require("protocol")

local function decode(raw: any): {ok: false, error: {code: string, message: string}}?
    if type(raw.kind) ~= "string" then
        return nil
    end
    if raw.kind == "task" then
        return nil
    end
    if raw.kind == "timer" then
        return nil
    end
    return {ok = false, error = protocol.err("unknown_kind", raw.kind)}
end
`, "main.lua", WithModule("protocol", protocol))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want scalar type guard to validate raw.kind for imported call", result.Diagnostics)
	}
}

func TestScalarTypeGuardSurvivesLiteralExclusionInsideExportedFunctionObjectCall(t *testing.T) {
	protocol := CheckFileAndExport(`
type AppError = {code: string, message: string}
type Result = {ok: false, error: AppError}
local M = {}
M.AppError = AppError
M.Result = Result
function M.err(code: string, message: string): AppError
    return {code = code, message = message}
end
return M
`, "protocol", "protocol.lua")
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want none", protocol.Errors)
	}
	result := CheckFileAndExport(`
local protocol = require("protocol")
local M = {}

function M.decode(raw: any): protocol.Result
    if type(raw.kind) ~= "string" then
        return {ok = false, error = protocol.err("bad", "kind")}
    end
    if raw.kind == "task" then
        return {ok = false, error = protocol.err("task", "task")}
    end
    if raw.kind == "timer" then
        return {ok = false, error = protocol.err("timer", "timer")}
    end
    return {ok = false, error = protocol.err("unknown_kind", raw.kind)}
end

return M
`, "validator", "validator.lua", WithModule("protocol", protocol))
	if len(result.Errors) != 0 {
		t.Fatalf("diagnostics = %#v, want scalar type guard to validate raw.kind in exported function", result.Errors)
	}
}

func TestScalarChildTypeGuardSurvivesPriorTableRootGuard(t *testing.T) {
	result := Check(`
local function need(kind: string): () end

local function decode(raw: any): ()
    if type(raw) ~= "table" then
        return
    end
    if type(raw.kind) ~= "string" then
        return
    end
    if raw.kind == "task" then
        return
    end
    if raw.kind == "timer" then
        return
    end
    need(raw.kind)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want child scalar type guard to survive prior table root guard", result.Diagnostics)
	}
}

func TestScalarChildTypeGuardSurvivesPriorTableRootGuardForAssignment(t *testing.T) {
	result := Check(`
local function decode(raw: any): ()
    if type(raw) ~= "table" then
        return
    end
    if type(raw.kind) ~= "string" then
        return
    end
    if raw.kind == "task" then
        return
    end
    if raw.kind == "timer" then
        return
    end
    local kind: string = raw.kind
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want child scalar type guard to validate raw.kind for assignment", result.Diagnostics)
	}
}

func TestScalarChildTypeGuardSurvivesFailedOrGuardAndCastAlias(t *testing.T) {
	result := Check(`
local function need(id: string): () end

local function run(step: any): ()
    if type(step) ~= "table" then
        return
    end
    if type(step.func_id) ~= "string" or step.func_id == "" then
        return
    end
    local func_id = step.func_id :: string
    need(func_id)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want scalar guard on step.func_id to survive failed OR guard and local cast alias", result.Diagnostics)
	}
}

func TestUntrustedAnyArgumentUsedByStringLibraryReportsOneDiagnosticPerUse(t *testing.T) {
	result := Check(`
local function schema(id: string): () end

local function f(tool_id: any): ()
    schema(tool_id)
    for part in string.gmatch(tool_id, "[^:]+") do
    end
end
`, WithStdlib())
	bySpan := map[string]int{}
	for _, diag := range result.Diagnostics {
		key := fmt.Sprintf("%d:%d", diag.Span.StartLine, diag.Span.StartCol)
		bySpan[key]++
	}
	for key, count := range bySpan {
		if count > 1 {
			t.Fatalf("diagnostics = %#v, duplicate diagnostics at %s", result.Diagnostics, key)
		}
	}
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

func TestConcreteCastValidatesValueThroughGenericCall(t *testing.T) {
	// A concrete cast is runtime validation. Once the cast result flows through a
	// generic identity call, the returned value keeps that validated target type.
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
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want concrete cast to validate through generic call", result.Diagnostics)
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

func TestTruthyGuardOnAnyFieldDoesNotValidateConcreteAssignment(t *testing.T) {
	result := Check(`
local function run(block: any): string?
    if block.text then
        local s: string = block.text
        return s
    end
    return nil
end

local function rows(block: any): {string}?
    if type(block.items) == "table" then
        local labels: {string} = block.items
        return labels
    end
    return nil
end
`)
	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want two untrusted-any assignment diagnostics", result.Diagnostics)
	}
	for _, diag := range result.Diagnostics {
		if diag.Code != diagnostics.CodeAssignmentType {
			t.Fatalf("diagnostic = %#v, want assignment diagnostic", diag)
		}
		if got := diag.Explanation.String(); !strings.Contains(got, "comes from any/unknown") ||
			!strings.Contains(got, "no proof on this path") {
			t.Fatalf("explanation = %q, want any/unknown origin and missing proof", got)
		}
	}
}

func TestConcreteCallContextDoesNotEraseGenericAnyFieldObligation(t *testing.T) {
	result := Check(`
local function run(block: any)
    if block.text then
        local s: string = block.text
        return s
    end
    return nil
end

local a = run({text = "hi"})
return a
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if got := diag.Explanation.String(); !strings.Contains(got, "block.text comes from any/unknown") ||
		!strings.Contains(got, "no proof on this path") {
		t.Fatalf("explanation = %q, want generic any-field obligation despite concrete call context", got)
	}
}
