package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestMemberCallPreservesRootPresenceGuardAcrossUnrelatedDynamicIndexWrite(t *testing.T) {
	diags := runDiagnostics(t, `
		type Obj = {m: () -> ()}
		function f(x: Obj?, t: {[string]: number}, key: string)
			if x then
				t[key] = 1
				x.m()
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want unrelated dynamic index write to preserve root presence guard", diags)
	}
}

func TestMemberCallReportsMissingMethodAfterDiscriminantNarrowing(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: () -> ()}
		type Cat = {kind: "cat", meow: () -> ()}
		type Animal = Dog | Cat

		local function speak(a: Animal)
			if a.kind == "dog" then
				a.meow()
			end
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeMissingMember || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "meow") || !strings.Contains(d.Message, "dog") {
		t.Fatalf("message = %q, want missing meow on narrowed dog variant", d.Message)
	}
	if len(d.Explanation.Evidence()) == 0 || d.Explanation.String() == "" {
		t.Fatalf("explanation = %#v, want non-empty evidence", d.Explanation)
	}
}

func TestMemberCallAcceptsMatchingDiscriminantMethod(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: () -> ()}
		type Cat = {kind: "cat", meow: () -> ()}
		type Animal = Dog | Cat

		local function speak(a: Animal)
			if a.kind == "dog" then
				a.bark()
			else
				a.meow()
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestMemberReadReportsStaticBracketMissingFieldAfterDiscriminantNarrowing(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: string}
		type Cat = {kind: "cat", meow: string}
		type Animal = Dog | Cat

		local function speak(a: Animal)
			if a.kind == "dog" then
				local bad = a["meow"]
			end
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want static bracket missing-member read: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeMissingMember || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, `"meow"`) || !strings.Contains(d.Message, "dog") {
		t.Fatalf("message = %q, want missing meow on narrowed dog variant", d.Message)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) != 1 ||
		!diagnosticEvidenceContains(evidence, `a["meow"] reads member "meow" from receiver type`) ||
		d.Explanation.String() == "" {
		t.Fatalf("explanation evidence = %#v, want path-specific member-read receiver evidence", evidence)
	}
	if !diagnosticHasLabel(d, labelMemberRead) {
		t.Fatalf("labels = %#v, want member-read focus label", d.Labels)
	}
	if !strings.Contains(d.Help, "Narrow the receiver before reading `meow`") {
		t.Fatalf("help = %q, want actionable missing-member help", d.Help)
	}
}

func TestMemberReadSkipsDynamicBracketKeyAfterDiscriminantNarrowing(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: string}
		type Cat = {kind: "cat", meow: string}
		type Animal = Dog | Cat

		local function speak(a: Animal, key: string)
			if a.kind == "dog" then
				local unknown = a[key]
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want no static member-read diagnostic for dynamic key", diags)
	}
}

func TestMemberReadReportsAliasVariantWriteInvalidatedGuardWithEvidence(t *testing.T) {
	result := runDiagnosticsResult(t, `
		type FileSlot = {
			kind: "file",
			path: string,
		}
		type TimerSlot = {
			kind: "timer",
			seconds: number,
		}
		type Slot = {
			value: FileSlot | TimerSlot,
		}
		type Slots = {[string]: Slot}

		local slots: Slots = {
			active = {
				value = {kind = "file", path = "/tmp/active"},
			},
		}

		local alias = slots.active

		if alias.value.kind == "file" then
			local before: string = alias.value.path
			alias.value = {kind = "timer", seconds = 5}
			local stale_path: string = slots.active.value.path
			local stale_seconds: number = before
		end
	`)
	assertStalePathMissingMemberEvidence(t, result)
}

func TestMemberReadReportsBracketVariantWriteInvalidatedGuardWithEvidence(t *testing.T) {
	result := runDiagnosticsResult(t, `
		type FileSlot = {
			kind: "file",
			path: string,
		}
		type TimerSlot = {
			kind: "timer",
			seconds: number,
		}
		type Slot = {
			value: FileSlot | TimerSlot,
		}
		type Slots = {[string]: Slot}

		local slots: Slots = {
			active = {
				value = {kind = "file", path = "/tmp/active"},
			},
		}

		if slots.active.value.kind == "file" then
			local before: string = slots["active"].value.path
			slots["active"].value = {kind = "timer", seconds = 10}
			local stale_path: string = slots.active.value.path
			local stale_seconds: number = before
		end
	`)
	assertStalePathMissingMemberEvidence(t, result)
}

func assertStalePathMissingMemberEvidence(t *testing.T, result *body.Result) {
	t.Helper()
	point, expr := requireLocalAssignmentExprByName(t, result, "stale_path")
	read, ok := expr.(*ast.AttrGetExpr)
	if !ok {
		t.Fatalf("stale_path expr = %T, want AttrGetExpr", expr)
	}
	envs := guardEnvironments(result)
	context := producerContext{resolver: newResultResolver(result, nil)}
	typers := memberReadTypers{
		narrowed: newStructuralFlowExpressionTyper(result, context.resolver, point, envs[point]),
		base:     newStructuralFlowExpressionTyper(result, context.resolver, point, guardEnv{}),
		result:   result,
		point:    point,
	}
	receiver, ok := typers.receiverType(read.Object)
	if !ok {
		t.Fatal("member-read receiver type unavailable")
	}
	if !fieldProvablyAbsent(receiver, "path") {
		t.Fatalf("receiver type = %s, want path provably absent", formatType(receiver))
	}
	broad, broadOK := typers.base.broadType(read.Object)
	if !broadOK || !isMultiArmUnion(broad) {
		t.Fatalf("broad receiver type = %s/%v, want original union", formatType(broad), ok)
	}
	fieldBroad := broad
	if withoutNil := projectionWithoutNil(broad); withoutNil != nil && !typ.IsNever(withoutNil) {
		fieldBroad = withoutNil
	}
	if field, ok := access.Field(fieldBroad, "path"); !ok {
		t.Fatalf("broad receiver type = %s, does not admit path after nil stripping; field=%s/%v", formatType(broad), formatType(field), ok)
	}
	produced, ok := memberRead(context).read(read, typers)
	if !ok || produced.Code != CodeMissingMember {
		t.Fatalf("memberRead.read = %#v/%v, want missing-member diagnostic", produced, ok)
	}

	diags := Produce(result)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d %#v, want stale member read and stale literal assignment", len(diags), diags)
	}
	var missing diagnostic.Diagnostic
	for _, d := range diags {
		if d.Code == CodeMissingMember {
			missing = d
			break
		}
	}
	if missing.Code != CodeMissingMember || missing.Severity != diagnostic.SeverityError {
		t.Fatalf("missing-member diagnostic = %#v, want error; all diagnostics = %#v", missing, diags)
	}
	if !strings.Contains(missing.Message, `"path"`) || !strings.Contains(missing.Message, "timer") {
		t.Fatalf("message = %q, want missing path on timer variant", missing.Message)
	}
	evidence := missing.Explanation.Evidence()
	if len(evidence) != 1 ||
		!diagnosticEvidenceContains(evidence, `slots.active.value.path reads member "path" from receiver type`) ||
		missing.Explanation.String() == "" {
		t.Fatalf("explanation evidence = %#v, want path-specific stale member-read evidence", evidence)
	}
	if !diagnosticHasLabel(missing, labelMemberRead) {
		t.Fatalf("labels = %#v, want member-read focus label", missing.Labels)
	}
}

func TestMemberCallReportsUnionReceiverMissingMethod(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string | number)
			x:upper()
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeMissingMember || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "upper") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q, want missing upper on string|number receiver", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "x.upper has receiver type") {
		t.Fatalf("explanation evidence = %#v, want member receiver evidence", d.Explanation.Evidence())
	}
	if !diagnosticHasLabel(d, labelMemberCall) {
		t.Fatalf("labels = %#v, want member-call focus label", d.Labels)
	}
	if !strings.Contains(d.Help, "Narrow the receiver before reading `upper`") {
		t.Fatalf("help = %q, want actionable missing-member help", d.Help)
	}
}

func TestMemberCallEvidenceKeepsNestedCalleePath(t *testing.T) {
	cases := []struct {
		name         string
		src          string
		code         diagnostic.Code
		wantEvidence string
	}{
		{
			name: "missing nested member",
			src: `
type ReadyClient = {kind: "ready", ready: () -> ()}
type IdleClient = {kind: "idle", wait: () -> ()}
type Client = ReadyClient | IdleClient
type Box = {client: Client}
function f(box: Box)
    if box.client.kind == "ready" then
        box.client:run()
    end
end
`,
			code:         CodeMissingMember,
			wantEvidence: "box.client.run has receiver type",
		},
		{
			name: "non-callable nested member",
			src: `
type BadClient = {kind: "bad", run: number}
type GoodClient = {kind: "good", run: () -> ()}
type Client = BadClient | GoodClient
type Box = {client: Client}
function f(box: Box)
    if box.client.kind == "bad" then
        box.client:run()
    end
end
`,
			code:         CodeNotCallable,
			wantEvidence: "box.client.run has type number at call",
		},
		{
			name: "nested member call contract",
			src: `
type Client = {invoke: (id: string) -> ()}
type Box = {client: Client}
function f(box: Box)
    box.client.invoke(42)
end
`,
			code:         CodeDirectCallArgType,
			wantEvidence: "box.client.invoke parameter 1 expects string",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want one nested member-call diagnostic: %#v", len(diags), diags)
			}
			d := diags[0]
			if d.Code != tc.code || d.Severity != diagnostic.SeverityError {
				t.Fatalf("diagnostic = %#v, want %s error", d, tc.code)
			}
			if !diagnosticEvidenceContains(d.Explanation.Evidence(), tc.wantEvidence) {
				t.Fatalf("evidence = %#v, want %q", d.Explanation.Evidence(), tc.wantEvidence)
			}
		})
	}
}

func TestMemberCallAcceptsUnionReceiverWhenAllAlternativesCallable(t *testing.T) {
	diags := runDiagnostics(t, `
		type Left = {run: () -> string}
		type Right = {run: () -> number}

		function f(x: Left | Right)
			x:run()
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestMemberCallReportsOptionalSymbolReceiver(t *testing.T) {
	diags := runDiagnostics(t, `
		type Message = {topic: (self: Message) -> string}
		function f(m: Message?)
			m:topic()
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeOptionalMethodCall || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot call method on an optional value without a nil check") {
		t.Fatalf("message = %q", d.Message)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "receiver m is optional at call to m.topic") {
		t.Fatalf("evidence = %#v, want path-specific optional receiver evidence", d.Explanation.Evidence())
	}
}

func TestMemberCallReportsOptionalExpressionReceiver(t *testing.T) {
	diags := runDiagnostics(t, `
		type Message = {topic: (self: Message) -> string}
		local function make(): {Message}
			return {}
		end
		local _: string = make()[1]:topic()
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeOptionalMethodCall || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot call method on an optional value without a nil check") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestMemberCallAcceptsNarrowedOptionalReceiver(t *testing.T) {
	diags := runDiagnostics(t, `
		type Message = {topic: (self: Message) -> string}
		function f(m: Message?)
			if m then
				m:topic()
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after nil check", diags)
	}
}

func TestMemberCallReportsWrongArgumentType(t *testing.T) {
	diags := runDiagnostics(t, `
		type Client = {invoke: (model_id: string, payload: any) -> ()}
		function f(c: Client)
			c.invoke(42, {})
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 1") || !strings.Contains(d.Message, "string") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestMemberCallReportsTooFewArgs(t *testing.T) {
	diags := runDiagnostics(t, `
		type Client = {invoke: (model_id: string, payload: number) -> ()}
		function f(c: Client)
			c.invoke("model")
		end
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
}

func TestMemberCallReportsTooManyArgs(t *testing.T) {
	diags := runDiagnostics(t, `
		type Client = {invoke: (model_id: string, payload: number) -> ()}
		function f(c: Client)
			c.invoke("model", 1, true)
		end
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

func TestColonMemberCallConsumesReceiverParameter(t *testing.T) {
	diags := runDiagnostics(t, `
		type ClientSelf = {id: string}
		type Client = {id: string, invoke: (self: ClientSelf, model_id: string) -> ()}
		function f(c: Client)
			c:invoke(42)
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 1") || !strings.Contains(d.Message, "string") {
		t.Fatalf("message = %q", d.Message)
	}

	ok := runDiagnostics(t, `
		type ClientSelf = {id: string}
		type Client = {id: string, invoke: (self: ClientSelf, model_id: string) -> ()}
		function f(c: Client)
			c:invoke("model")
		end
	`)
	if len(ok) != 0 {
		t.Fatalf("diagnostics = %#v, want none for matching colon call", ok)
	}
}

func TestMemberCallSkipsUnreachableDiscriminantBranch(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: () -> ()}

		local function speak(a: Dog)
			if a.kind == "cat" then
				a.meow()
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unreachable branch body", diags)
	}
}

func TestMemberCallAcceptsUnnarrowedPrimitiveMethod(t *testing.T) {
	diags := runDiagnostics(t, `
		local value: string = "abc"
		value:upper()
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}
