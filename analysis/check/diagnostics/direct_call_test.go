package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestDirectCallReportsNonCallableTarget(t *testing.T) {
	diags := runDiagnostics(t, `
		local x: number = 42
		x()
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallNotCallable || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "not callable") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "x has type number") {
		t.Fatalf("explanation evidence = %#v, want callee type evidence", d.Explanation.Evidence())
	}
	if !diagnosticHasLabel(d, labelCallTarget) {
		t.Fatalf("labels = %#v, want call-target focus label", d.Labels)
	}
	if !strings.Contains(d.Help, "replace `x` with a callable expression") {
		t.Fatalf("help = %q, want actionable non-callable target help", d.Help)
	}
}

func TestDirectCallReportsPossiblyNilTargetWithDirectCallCode(t *testing.T) {
	diags := runDiagnostics(t, `
		local maybe: (() -> string)? = nil
		maybe()
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallNotCallable || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot call maybe") || !strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "maybe has a callable type, but may also be nil") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "no guard on this path proves maybe is non-nil before this call") {
		t.Fatalf("explanation evidence = %#v, want optional callable and missing guard proof", d.Explanation.Evidence())
	}
	if !diagnosticHasLabel(d, labelCallTarget) {
		t.Fatalf("labels = %#v, want call-target focus label", d.Labels)
	}
	if !strings.Contains(d.Help, "Guard `maybe` with a nil check") {
		t.Fatalf("help = %q, want actionable possibly-nil target help", d.Help)
	}
}

func TestDirectCallRendersPossiblyNilTargetTrace(t *testing.T) {
	src := strings.TrimLeft(`
local maybe: (() -> string)? = nil
maybe()
`, "\n")
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	rendered := diagnostic.Render(diags[0], diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.call.direct.not_callable]: cannot call maybe because it may be nil
 --> diagnostics_test.lua:2:1
  |
2 | maybe()
  | ↑ call target

because:
  1. proven: maybe has a callable type, but may also be nil
  2. missing proof: no guard on this path proves maybe is non-nil before this call

help: Guard ` + "`maybe`" + ` with a nil check before calling it.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestDirectCallReportsTooFewArgs(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a
		end
		add(1)
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallTooFewArgs || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "expects 2 arguments") || !strings.Contains(d.Message, "got 1") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want call and declaration evidence", d.Explanation.Evidence())
	}
	if !diagnosticHasLabel(d, labelCallExpression) {
		t.Fatalf("labels = %#v, want call-expression focus label", d.Labels)
	}
}

func TestDirectCallReportsTooManyArgs(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a
		end
		add(1, 2, 3)
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallTooManyArgs || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "expects 2 arguments") || !strings.Contains(d.Message, "got 3") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Labels) != 1 || d.Labels[0].Message != "extra argument" {
		t.Fatalf("labels = %#v, want extra argument label", d.Labels)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want call and declaration evidence", d.Explanation.Evidence())
	}
}

func TestDirectCallTooManyArgsSuppressesResultAssignmentDiagnostic(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a
		end
		local x: string = add(1, 2, 3)
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want only too-many-args: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeDirectCallTooManyArgs || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
}

func TestDirectCallReportsWrongArgumentType(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a
		end
		add(1, "wrong")
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 2") || !strings.Contains(d.Message, `"wrong"`) || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want call and parameter evidence", d.Explanation.Evidence())
	}
}

func TestCallParamObligationReportsStricterThanDirectUnionContract(t *testing.T) {
	diags := runDiagnostics(t, `
		local function scale(x: number | string): number
			return x * 2
		end
		scale("not-number")
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want stricter callee obligation only: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 1") || !strings.Contains(d.Message, `"not-number"`) || !strings.Contains(d.Message, "not number") {
		t.Fatalf("message = %q", d.Message)
	}
	evidence := d.Explanation.Evidence()
	if !diagnosticEvidenceContains(evidence, "inside scale, argument 1 must satisfy number") {
		t.Fatalf("explanation = %q, want callee-use obligation evidence", d.Explanation.String())
	}
	if len(evidence) < 2 || !spanEqual(evidence[1].Span, d.Labels[0].Span) {
		t.Fatalf("obligation evidence span = %#v, label span = %#v; want argument-focused evidence", evidence, d.Labels)
	}
}

func TestDirectCallUsesGenericResultFalseEdgeBoundaryProof(t *testing.T) {
	diags := runDiagnostics(t, `
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }

		local function err<T>(message: string): Result<T>
			return { ok = false, error = message }
		end

		local function map_result<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return { ok = true, value = fn(result.value) }
			end
			return err(result.error)
		end

		local r = map_result({ ok = true, value = "x" }, function(value: string): number
			return #value
		end)
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want generic false-edge result.error accepted", diags)
	}
}

func TestDirectCallUsesLoopLocalMethodReturnBoundaryProof(t *testing.T) {
	diags := runDiagnostics(t, `
		type Message = {
			from: fun(self: Message): string,
			payload: fun(self: Message): any,
		}

		type Channel = {
			receive: fun(self: Channel): (Message, boolean),
		}

		local process = {}
		function process.listen(): Channel
			error("stub")
		end
		function process.send(pid: string, topic: string): boolean
			return true
		end

		local done = false
		coroutine.spawn(function()
			local ch = process.listen()
			while not done do
				local msg, ok = ch:receive()
				if not ok then
					break
				end
				local p = msg:payload()
				local data = p and p:data() or nil
				local reply_to = msg:from()
				if type(data) ~= "table" or type(data.amount) ~= "number" then
					process.send(reply_to, "nak")
				else
					process.send(reply_to, "ack")
				end
			end
		end)
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want loop-local method return accepted", diags)
	}
}

func TestDirectCallReportsWrongArgumentTypeInNestedReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a + b
		end
		local function f(): number
			return add("bad", 2)
		end
		local x = f()
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 1") || !strings.Contains(d.Message, `"bad"`) || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestDirectCallSkipsGenericIdentityArgumentClaims(t *testing.T) {
	diags := runDiagnostics(t, `
		local function identity<T>(x: T): T
			return x
		end
		local n: number = identity(42)
		local s: string = identity("hello")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for generic identity call", diags)
	}
}

func TestDirectCallAcceptsTypedOptionalParam(t *testing.T) {
	diags := runDiagnostics(t, `
		local function log(msg: string, level: string?)
		end
		log("hello")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for typed optional param", diags)
	}
}

func TestDirectCallAcceptsUntypedDefaultOptional(t *testing.T) {
	diags := runDiagnostics(t, `
		local function greet(name, greeting)
			local message = greeting or "Hello"
			return message
		end
		greet("World")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for untyped default optional", diags)
	}
}

func TestDirectCallAcceptsMultipleOrDefaults(t *testing.T) {
	diags := runDiagnostics(t, `
		local function pick(a, b, c)
			local left = b or "left"
			local right = c or "right"
			return left, right
		end
		pick("head")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for trailing defaults", diags)
	}
}

func TestDirectCallAcceptsExplicitNilCheckOptional(t *testing.T) {
	diags := runDiagnostics(t, `
		local function maybe(value: string?)
			if value == nil then
				return
			end
			return value
		end
		maybe()
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for optional nil-checked param", diags)
	}
}

func TestDirectCallAcceptsVariadicExtraArgs(t *testing.T) {
	diags := runDiagnostics(t, `
		local function log(prefix: string, ...: number)
		end
		log("n", 1, 2, 3)
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for typed variadic extra args", diags)
	}
}

func TestDirectCallResultAssignmentReportsAnnotatedLocalMismatch(t *testing.T) {
	src := `
local function add(a: number, b: number): number
	return a + b
end
local x: string = add(1, 2)
`
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallResultAssignment || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "call result") || !strings.Contains(d.Message, "string") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	stmts := mustStmts(t, src)
	fn := stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	assign := stmts[1].(*ast.LocalAssignStmt)
	if got := d.Explanation.Evidence(); len(got) != 2 {
		t.Fatalf("explanation evidence = %#v, want 2 items", got)
	} else {
		if !strings.Contains(got[0].Message, "add declares call result 1 as number") {
			t.Fatalf("return evidence message = %q", got[0].Message)
		}
		if got[0].Span != ast.SpanOf(fn.ReturnTypes[0]) {
			t.Fatalf("return evidence span = %#v, want %#v", got[0].Span, ast.SpanOf(fn.ReturnTypes[0]))
		}
		if got[1].Span != ast.SpanOf(assign.Types[0]) {
			t.Fatalf("declared type evidence span = %#v, want %#v", got[1].Span, ast.SpanOf(assign.Types[0]))
		}
	}
}

func TestDirectCallResultAssignmentReportsTypedMemberCalleeWithoutManifestSignature(t *testing.T) {
	src := strings.TrimLeft(`
type API = { make: () -> number }
local api: API = {
    make = function(): number
        return 1
    end,
}
local x: string = api.make()
`, "\n")
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one direct-call result assignment diagnostic: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallResultAssignment || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want direct-call result assignment", d)
	}
	if !strings.Contains(d.Message, "call result") || !strings.Contains(d.Message, "number") ||
		!strings.Contains(d.Message, "string") {
		t.Fatalf("message = %q, want number result to string target mismatch", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "returns number") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "assignment target x requires string") {
		t.Fatalf("evidence = %#v, want member-call result and target annotation evidence", d.Explanation.Evidence())
	}
	if len(d.Labels) < 2 || d.Labels[0].Message != "call result" || d.Labels[1].Message != "declared type" ||
		d.Labels[0].Span != d.Span {
		t.Fatalf("labels/span = %#v/%#v, want call-result span and declared type labels", d.Labels, d.Span)
	}
	rendered := diagnostic.Render(d, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.call.direct.result_assignment]: call result 1 is number, not string
 --> diagnostics_test.lua:7:19
  |
  |          ↓ declared type
7 | local x: string = api.make()
  |                   ↑ call result

because:
  1. proven: api.make returns number
  2. claimed: assignment target x requires string

help: Assign the call result to a compatible target type, or change the callee return type if this result is valid.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestDirectCallMemberArgumentProofFailureTakesPrecedenceOverResultAssignment(t *testing.T) {
	diags := runDiagnostics(t, `
type API = { make: (name: string) -> number }
local api: API = {
	make = function(name: string): number
		return 1
	end,
}
local x: string = api.make(42)
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one direct-call argument diagnostic: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType {
		t.Fatalf("diagnostic = %#v, want direct-call argument diagnostic", d)
	}
	if !diagnosticHasLabel(d, "argument value") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "argument 1 has literal value 42") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "parameter 1 expects string") {
		t.Fatalf("diagnostic = %#v, want member-call argument value and parameter evidence", d)
	}
	for _, diag := range diags {
		if diag.Code == CodeDirectCallResultAssignment {
			t.Fatalf("diagnostics include result-assignment diagnostic despite member-call argument proof failure: %#v", diags)
		}
	}
}

func TestDirectCallArgumentProofFailureTakesPrecedenceOverResultAssignment(t *testing.T) {
	diags := runDiagnostics(t, `
local function f(x: { id: string }): number
	return 1
end

local raw = ({ id = "ok" } :: any)
local y: string = f(raw)
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want one direct-call argument diagnostic: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType {
		t.Fatalf("diagnostic = %#v, want direct-call argument diagnostic", d)
	}
	if got := d.Explanation.String(); !strings.Contains(got, "f parameter 1 expects") ||
		!strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "no proof on this path shows raw satisfies the parameter type") {
		t.Fatalf("explanation = %q, want parameter declaration and explicit-any missing-proof evidence", got)
	}
	for _, diag := range diags {
		if diag.Code == CodeDirectCallResultAssignment {
			t.Fatalf("diagnostics include result-assignment diagnostic despite argument proof failure: %#v", diags)
		}
	}
}

func TestDirectCallResultAssignmentSkipsGenericReturnContracts(t *testing.T) {
	diags := runDiagnostics(t, `
		local function id<T>(x: T): T
			return x
		end
		local s: string = id("hello")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for generic return contract", diags)
	}
}
