package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestAnnotationAssignabilityReportsLiteralMismatch(t *testing.T) {
	diags := runDiagnostics(t, `local x: number = "no"`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot assign") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want source and annotation evidence", d.Explanation.Evidence())
	}
}

func TestAnnotationAssignabilityAcceptsSubtypeLiteral(t *testing.T) {
	diags := runDiagnostics(t, `local x: number = 42`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestExplicitNilFieldFreshAssignableSuppressesMismatch(t *testing.T) {
	got := typetable.NewRecord().
		Field("id", typ.String).
		Field("error", typ.Nil).
		Build()
	want := typetable.NewRecord().
		Field("id", typ.String).
		Field("error", typeexpr.Optional(typ.String)).
		Build()

	if !explicitNilFieldFreshAssignable(got, typeexpr.Optional(want)) {
		t.Fatal("explicit nil field should be fresh-assignable to nilable field contract")
	}
}

func TestAnnotationAssignabilityReportsNominalGenericArgumentMismatch(t *testing.T) {
	diags := runDiagnostics(t, `
function f(ch: Channel<string>)
    local bad: Channel<number> = ch
end
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", diags[0].Code, CodeAssignmentType)
	}
}

func TestAnnotationAssignabilityReportsRecursiveUnionArrayElementMismatch(t *testing.T) {
	diags := runDiagnostics(t, `
type TextNode = { kind: "text", value: string }
type GroupNode = { kind: "group", children: {TreeNode} }
type TreeNode = TextNode | GroupNode

function f(tree: TreeNode)
    if tree.kind == "group" then
        local first = tree.children[1]
        if first and first.kind == "text" then
            local value: string = first.value
            local bad_value: number = first.value
        end
    end
    if tree.kind == "text" then
        local children = tree.children
    end
end
`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	messages := make([]string, 0, len(diags))
	for _, diag := range diags {
		messages = append(messages, diag.Message)
	}
	if !containsDiagnosticMessage(messages, "cannot assign string to number") ||
		!containsDiagnosticMessage(messages, `has no member "children"`) {
		t.Fatalf("diagnostics = %#v, want first.value mismatch and text.children missing-member", messages)
	}
}

func TestAnnotationAssignabilityReportsArrayLiteralElementMismatch(t *testing.T) {
	diags := runDiagnostics(t, `local arr: {number} = {1, "two", 3}`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, `"two"`) || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestAnnotationAssignabilityAcceptsHomogeneousArrayLiteral(t *testing.T) {
	diags := runDiagnostics(t, `local arr: {number} = {1, 2, 3}`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestAnnotationAssignabilityDoesNotTrustCastEscape(t *testing.T) {
	diags := runDiagnostics(t, `local x: number = "no" as any`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if got := diags[0].Explanation.String(); !strings.Contains(got, "source expression") {
		t.Fatalf("explanation = %q, want source evidence", got)
	}
}

func TestAnnotationAssignabilityReportsScalarOperatorRHS(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "arithmetic", src: `local bad: string = 1 + 2`},
		{name: "relational", src: `local bad: string = 1 < 2`},
		{name: "concat", src: `local bad: number = "a" .. "b"`},
		{name: "logical", src: `local bad: number = true and false`},
		{name: "unary minus", src: `local bad: string = -1`},
		{name: "unary not", src: `local bad: number = not false`},
		{name: "unary len", src: `local bad: string = #"abc"`},
		{name: "unary bitnot", src: `local bad: string = ~1`},
		{name: "cast wrapper", src: `local bad: string = (1 + 2) as number`},
		{name: "non-nil wrapper", src: `local bad: string = (1 + 2)!`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
			}
			d := diags[0]
			if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
				t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
			}
			if !strings.Contains(d.Message, "cannot assign") {
				t.Fatalf("message = %q, want assignment mismatch", d.Message)
			}
		})
	}
}

func TestAnnotationAssignabilityReportsChannelSelectBranchPayloadMismatch(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
type Event = { kind: "event", id: string, attempt: number }
type Timer = { kind: "timer", elapsed: number }
type Stop = { kind: "stop", reason: string }
type Source = { primary: Channel<Event>, timers: Channel<Timer>, stops: Channel<Stop> }
function consume(source: Source)
	local result = channel.select {
		source.primary:case_receive(),
		source.timers:case_receive(),
		source.stops:case_receive(),
	}
	if result.channel == source.primary then
		local event = result.value
		local wrong: number = event.id
	end
	if result.channel == source.timers then
		local timer = result.value
		local wrong: string = timer.elapsed
	end
end
`, []string{"channel"})
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	messages := diagnosticMessages(diags)
	if !containsDiagnosticMessage(messages, "cannot assign string to number") ||
		!containsDiagnosticMessage(messages, "cannot assign number to string") {
		t.Fatalf("diagnostics = %#v, want string->number and number->string channel payload mismatches", messages)
	}
}

func TestAnnotationAssignabilityChannelSelectDirectParameterBranches(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
type Event = { id: string }
type Timer = { elapsed: number }
function consume(primary: Channel<Event>, timers: Channel<Timer>): string
	local result = channel.select {
		primary:case_receive(),
		timers:case_receive(),
	}
	if result.channel == primary then
		local event = result.value
		local id: string = event.id
		local wrong: number = event.id
		return id
	end
	if result.channel == timers then
		local timer = result.value
		local elapsed: number = timer.elapsed
		local wrong: string = timer.elapsed
		return tostring(elapsed)
	end
	return ""
end
`, []string{"channel"})
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	messages := diagnosticMessages(diags)
	if !containsDiagnosticMessage(messages, "cannot assign string to number") ||
		!containsDiagnosticMessage(messages, "cannot assign number to string") {
		t.Fatalf("diagnostics = %#v, want direct-param channel payload mismatches only", messages)
	}
}

func TestAnnotationAssignabilityRejectsGradualUntypedDynamicMapWriteWithoutProof(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(raw, key: string)
			local map: {[string]: string} = {}
			map[key] = raw
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1 for unproven dynamic source: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want typed map assignment error", d)
	}
}

func TestAnnotationAssignabilityRejectsExplicitAnyAsProof(t *testing.T) {
	diags := runDiagnostics(t, `
		type Payload = {id: string, count: number}
		local raw: any = {id = "cfg", count = 2}
		local payload: Payload = raw
		if raw.id then
			local id: string = raw.id
		end
		local function consume(payload: Payload): number
			return payload.count + 1
		end
		local count = consume(raw)
	`)
	if len(diags) != 3 {
		t.Fatalf("diagnostics = %d, want 3: %#v", len(diags), diags)
	}
	var assignment, field, call bool
	for _, d := range diags {
		msg := d.Message
		assignment = assignment || strings.Contains(msg, "cannot assign any to") && strings.Contains(msg, "id: string")
		field = field || strings.Contains(msg, "cannot assign any to string")
		call = call || strings.Contains(msg, "argument 1 is any") && strings.Contains(msg, "id: string")
	}
	if !assignment || !field || !call {
		t.Fatalf("diagnostics = %#v, want explicit-any assignment, field, and call errors", diags)
	}
}

func TestAnnotationAssignabilityRejectsExplicitAnyFieldThroughIPairs(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
local raw: any = nil
local pages = {
	{ id = raw, route = "/ok" },
}
local accessible: {[string]: string} = {}
for _, page in ipairs(pages) do
	accessible[page.route] = page.id
end
`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want typed map assignment error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want string map assignment error", d)
	}
}

func TestAnnotationAssignabilityRejectsExplicitAnyFieldAfterEqualityGuard(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
local raw: any = nil
local pages = {
	{ id = raw, route = "/ok" },
}
local routes: {[string]: string} = { ["/ok"] = "page:ok" }
local accessible: {[string]: string} = {}
for _, page in ipairs(pages) do
	local route = page.route
	if route and routes[route] == page.id then
		accessible[route] = page.id
	end
end
`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want typed map assignment error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want string map assignment error", d)
	}
}

func TestAnnotationAssignabilityRejectsExplicitAnyFieldThroughFixtureGuardShape(t *testing.T) {
	diags := runDiagnosticsFull(t, `
local unknown_id: any = nil
local all_pages = {
	{ id = unknown_id, mount_route = "/ok/:part(.*)*", secure = false },
}
local routes_map: {[string]: string} = {
	["/ok/:part(.*)*"] = "page:ok",
}
local accessible: {[string]: string} = {}

for _, page in ipairs(all_pages) do
	local mr = page.mount_route
	if mr and routes_map[mr] == page.id and (not page.secure or can_access(page)) then
		accessible[mr] = page.id
	end
end
`, []string{"test", "type", "value", "can_access"}, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want typed map assignment error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want string map assignment error", d)
	}
}

func TestDirectCallRejectsGradualTopThroughOrDefault(t *testing.T) {
	diags := runDiagnostics(t, `
local http = {
	get = function(url: string, options: table)
		return { url = url, options = options }, nil
	end,
}

local function main(args)
	local url = (args and args.url) or "http://localhost:8085/hello"
	return http.get(url, { timeout = "2s" })
end
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want gradual-top string argument error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeDirectCallArgType || !strings.Contains(d.Message, "string") {
		t.Fatalf("diagnostic = %#v, want direct call string argument error", d)
	}
}

func TestAnnotationAssignabilitySkipsUnannotatedIdentifierSources(t *testing.T) {
	diags := runDiagnostics(t, `
		local y = value
		local x: number = y
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unannotated identifier source", diags)
	}
}

func TestAnnotationAssignabilitySkipsAnnotatedIdentifierWithoutPointProof(t *testing.T) {
	diags := runDiagnostics(t, `
		local x: string? = value
		local s: string = x
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none without point-local source proof", diags)
	}
}

func TestAnnotationAssignabilityReportsMaybeParameterWithoutNarrowing(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string?)
			local y: string = x
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string?") {
		t.Fatalf("diagnostic = %#v, want optional parameter assignment error", d)
	}
}

func TestAnnotationAssignabilityReportsMissingUnionMapEntry(t *testing.T) {
	diags := runDiagnostics(t, `
		type Allow = {kind: "allow", reason: string}
		type Deny = {kind: "deny", reason: string}
		type Decision = Allow | Deny
		local cache: {[string]: Decision} = {}
		local missing: Decision = cache["missing"]
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want missing-key optionality error: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "nil |") {
		t.Fatalf("diagnostic = %#v, want nilable union assignment error", d)
	}
}

func TestAnnotationAssignabilityReportsMissingUnionMapEntryUnderInferredReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		type Task = {kind: "task", id: string}
		type Timer = {kind: "timer", id: string}
		type Envelope = Task | Timer
		type State = {processed: {[string]: Envelope}, counters: {[string]: number}}
		type Actor = {state: State}
		local function new_actor(): Actor
			return {state = {processed = {}, counters = {}}}
		end
		local actor = new_actor()
		actor.state.processed["m1"] = {kind = "task", id = "m1"}
		actor.state.counters["task"] = 1
		local missing_processed: Envelope = actor.state.processed["missing"]
		local missing_counter: number = actor.state.counters["missing"]
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want processed and counter missing-key errors: %#v", len(diags), diags)
	}
	messages := diagnosticMessages(diags)
	if !containsDiagnosticMessage(messages, "nil |") ||
		!containsDiagnosticMessage(messages, "number?") {
		t.Fatalf("diagnostics = %#v, want both missing-key assignment errors", messages)
	}
}

func TestAnnotationAssignabilityUsesSolvedTypeTestState(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string | number)
			if type(x) == "string" then
				local n: number = x
			else
				local s: string = x
			end
		end
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	for _, d := range diags {
		if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign") {
			t.Fatalf("diagnostic = %#v, want assignment mismatch", d)
		}
	}
}

func TestAnnotationAssignabilityUsesTypeIsWrapperErrorBranchState(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {x: number, y: number}
		local function isPoint(x)
			return Point:is(x)
		end
		function validate(data: any)
			local val, err = isPoint(data)
			if err ~= nil then
				local p: Point = val
			end
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "nil") {
		t.Fatalf("diagnostic = %#v, want nil-to-Point assignment error", d)
	}
}

func TestAnnotationAssignabilityUsesDeclaredLocalValueForTypeTestState(t *testing.T) {
	diags := runDiagnostics(t, `
		local y: string | number = 42
		if type(y) == "string" then
			local n: number = y
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string") || !strings.Contains(d.Message, "number") {
		t.Fatalf("diagnostic = %#v, want string-to-number assignment mismatch", d)
	}
}

func TestAnnotationAssignabilityUsesSolvedTypeNotState(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string | number)
			if type(x) ~= "string" then
				local s: string = x
			else
				local n: number = x
			end
		end
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	for _, d := range diags {
		if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign") {
			t.Fatalf("diagnostic = %#v, want assignment mismatch", d)
		}
	}
}

func TestAnnotationAssignabilityAcceptsAssertedMaybeParameter(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
		function f(x: string?)
			assert(x)
			local y: string = x
		end
	`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after assert", diags)
	}
}

func TestAnnotationAssignabilitySkipsRootLiteralIndexProjection(t *testing.T) {
	diags := runDiagnostics(t, `
		local xs: {number} = {1, 2}
		local x: number = xs[1]
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestAnnotationAssignabilityAcceptsWhileIndexReadProvenInRange(t *testing.T) {
	diags := runDiagnostics(t, `
		function first(xs: {number}): number
			local i: number = 1
			while i <= #xs do
				local v: number = xs[i]
				if v > 0 then
					return v
				end
				i = i + 1
			end
			return 0
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for proven in-range positive index", diags)
	}
}

func TestAnnotationAssignabilityAcceptsInferredFunctionFieldAliasReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		type Res = { answer: string }
		local M = {
			dep = {
				get = function()
					return nil
				end,
			},
		}
		function M.run()
			return M.dep.get()
		end
		M.dep = {
			get = function()
				return { answer = "ok" }
			end,
		}
		local f: fun(): Res = M.run
		local res = f()
		local answer: string = res.answer
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for current function-field alias return", diags)
	}
}

func TestAnnotationAssignabilityRejectsReassignedFunctionFieldAliasReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		type Res = { answer: string }
		local M = {
			dep = {
				get = function()
					return nil
				end,
			},
		}
		function M.run()
			return M.dep.get()
		end
		M.run = function()
			return nil
		end
		local f: fun(): Res = M.run
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType {
		t.Fatalf("diagnostic = %#v, want assignment mismatch for reassigned wrapper", d)
	}
}

func TestAnnotationAssignabilityReportsNestedOptionalIndexProjection(t *testing.T) {
	diags := runDiagnostics(t, `
		type Response = {
			result: {
				data: {
					departments: {string}?,
				},
			},
		}
		local response: Response = {
			result = {
				data = {
					departments = {"engineering"},
				},
			},
		}
		local first: string = response.result.data.departments[1]
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string?") {
		t.Fatalf("diagnostic = %#v, want optional nested index assignment error", d)
	}
}

func TestAnnotationAssignabilityReportsNestedOptionalIndexAfterGuardCalls(t *testing.T) {
	diags := runDiagnostics(t, `
		type Response = {
			result: {
				data: {
					departments: {string}?,
				},
			},
		}
		local response: Response = {
			result = {
				data = {
					departments = {"engineering"},
				},
			},
		}
		test.not_nil(response.result.data.departments, "departments required")
		test.eq(type(response.result.data.departments), "table", "departments should be a table")
		local count: number = #response.result.data.departments
		local first: string = response.result.data.departments[1]
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string?") {
		t.Fatalf("diagnostic = %#v, want optional nested index assignment error", d)
	}
}

func TestAnnotationAssignabilityReportsMissingRequiredField(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {x: number, y: number}
		local p: Point = {x = 10}
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "y") {
		t.Fatalf("diagnostic = %#v, want missing required field y", d)
	}
}

func TestLexicalTypeShadowingResolvesNearestVisibleAlias(t *testing.T) {
	diags := runDiagnostics(t, `
		type Value = number
		local a: Value = 10
		if true then
			type Value = string
			local b: Value = "hello"
		end
		local c: Value = 20
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestUnresolvedLexicalTypeReferences(t *testing.T) {
	cases := []struct {
		name string
		src  string
		line int
	}{
		{
			name: "not visible outside block",
			src: `
if true then
	type LocalPoint = {x: number, y: number}
end
local p: LocalPoint = {x = 1, y = 2}
`,
			line: 5,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
			}
			d := diags[0]
			if d.Code != CodeUnresolvedTypeReference || d.Severity != diagnostic.SeverityError {
				t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
			}
			if d.Position.Line != tc.line {
				t.Fatalf("diagnostic line = %d, want %d", d.Position.Line, tc.line)
			}
			if !strings.Contains(d.Message, "unknown type") {
				t.Fatalf("message = %q", d.Message)
			}
		})
	}
}

func TestForwardTypeReferenceResolves(t *testing.T) {
	// Type declarations are not order-dependent within a scope: a sibling
	// alias may reference one declared later, including through a recursive
	// cycle. The forward reference must resolve without an unresolved-type
	// diagnostic.
	diags := runDiagnostics(t, `
type Group = {kind: "group", children: {Node}}
type Node = {kind: "leaf"} | Group
local p: Node = {kind = "leaf"}
`)
	for _, d := range diags {
		if d.Code == CodeUnresolvedTypeReference {
			t.Fatalf("forward type reference reported unresolved: %#v", d)
		}
	}
}

func TestUnresolvedValueReferencesReportsImplicitGlobalReads(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		local x = missing + known
		missing = 42
		print(known)
	`, []string{"known", "print"})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeUnresolvedValueReference || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if d.Position.Line != 2 || !strings.Contains(d.Message, "missing") {
		t.Fatalf("diagnostic = %#v, want unresolved read of missing on line 2", d)
	}
}

func TestUnresolvedValueReferencesReportsNestedReads(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		local t = {[key] = {value = source}}
		sink[t[other]] = value
	`, []string{"sink", "value"})
	if len(diags) != 3 {
		t.Fatalf("diagnostics = %d, want 3: %#v", len(diags), diags)
	}
	for _, d := range diags {
		if d.Code != CodeUnresolvedValueReference {
			t.Fatalf("diagnostic code = %s, want %s: %#v", d.Code, CodeUnresolvedValueReference, diags)
		}
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
	if len(d.Explanation.Evidence()) == 0 {
		t.Fatalf("explanation evidence = %#v, want non-empty", d.Explanation.Evidence())
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
	if !strings.Contains(d.Message, "cannot call method on optional value without nil check") {
		t.Fatalf("message = %q", d.Message)
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
	if !strings.Contains(d.Message, "cannot call method on optional value without nil check") {
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

func TestNumericForReportsStringInit(t *testing.T) {
	diags := runDiagnostics(t, `
		for i = "one", 10 do
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeNumericForOperand || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "initial value") || !strings.Contains(d.Message, `"one"`) {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestNumericForReportsStringLimitAndStep(t *testing.T) {
	diags := runDiagnostics(t, `
		for i = 1, "ten", "one" do
		end
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeNumericForOperand || !strings.Contains(diags[0].Message, "limit") {
		t.Fatalf("first diagnostic = %#v, want limit numeric-for operand", diags[0])
	}
	if diags[1].Code != CodeNumericForOperand || !strings.Contains(diags[1].Message, "step") {
		t.Fatalf("second diagnostic = %#v, want step numeric-for operand", diags[1])
	}
}

func TestNumericForAcceptsNumbersAndDefaultStep(t *testing.T) {
	diags := runDiagnostics(t, `
		for i = 1, 10 do
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestNumericForSkipsUnknownAndPartlyNumericUnion(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(value, mixed: number | string)
			for i = value, mixed do
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unknown and partly numeric union", diags)
	}
}

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
	if len(d.Explanation.Evidence()) == 0 {
		t.Fatalf("explanation evidence = %#v, want non-empty", d.Explanation.Evidence())
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

func TestAnnotationAssignabilityUsesBoundaryProofAfterAssignedTypeCast(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		type Point = {x: number, y: number}
		local function validate(data: any)
			Point(data)
			local p: {x: number, y: number} = data
			return p
		end
		local function validate_assign(data: any)
			local v = Point(data)
			local p: {x: number, y: number} = data
			return p
		end
		local function expect_point(x)
			return Point(x)
		end
		local function validate_wrapped(data: any)
			expect_point(data)
			local p: {x: number, y: number} = data
			return p
		end
		return validate, validate_assign, validate_wrapped
	`, nil)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after type-cast postcondition", diags)
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

func TestReturnContractReportsLiteralMismatch(t *testing.T) {
	fn := mustFunctionExpr(t, `function f(): number return "hello" end`)
	result, err := program.RunFunction(fn, program.Config{
		Check: body.Config{
			Registry: standard.Registry(),
		},
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	diags := Produce(result.RootResult())
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeReturnContractType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "returned value") || !strings.Contains(d.Message, "hello") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	wantReturn := fn.Stmts[0].(*ast.ReturnStmt)
	if got := d.Explanation.Evidence(); len(got) != 2 {
		t.Fatalf("explanation evidence = %#v, want 2 items", got)
	} else {
		if got[0].Span != ast.SpanOf(wantReturn.Exprs[0]) {
			t.Fatalf("returned value evidence span = %#v, want %#v", got[0].Span, ast.SpanOf(wantReturn.Exprs[0]))
		}
		if got[1].Span != ast.SpanOf(fn.ReturnTypes[0]) {
			t.Fatalf("declared return evidence span = %#v, want %#v", got[1].Span, ast.SpanOf(fn.ReturnTypes[0]))
		}
	}
}

func TestReturnContractReportsProjectedIndexOptional(t *testing.T) {
	diags := runDiagnostics(t, `
		local function pick(xs: {number}, i: integer): number
			return xs[i]
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeReturnContractType || !strings.Contains(d.Message, "number?") {
		t.Fatalf("diagnostic = %#v, want return contract optional index error", d)
	}
}

func TestReturnContractSkipsOptionalUnknownAndGenericReturns(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "optional nil", src: `local function f(): number? return nil end`},
		{name: "unknown", src: `local function f(): unknown return "hello" end`},
		{name: "any", src: `local function f(): any return "hello" end`},
		{name: "generic", src: `local function id<T>(x: T): T return "hello" end`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 0 {
				t.Fatalf("diagnostics = %#v, want none", diags)
			}
		})
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
	assign := stmts[1].(*ast.LocalAssignStmt)
	call := assign.Exprs[0].(*ast.FuncCallExpr)
	if got := d.Explanation.Evidence(); len(got) != 2 {
		t.Fatalf("explanation evidence = %#v, want 2 items", got)
	} else {
		if got[0].Span != ast.SpanOf(call) {
			t.Fatalf("call evidence span = %#v, want %#v", got[0].Span, ast.SpanOf(call))
		}
		if got[1].Span != ast.SpanOf(assign.Types[0]) {
			t.Fatalf("declared type evidence span = %#v, want %#v", got[1].Span, ast.SpanOf(assign.Types[0]))
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

func TestReturnContractReportsGenericDirectCallConcreteMismatch(t *testing.T) {
	src := `
		type Box<T> = {value: T}
		type StringBox = {value: string}
		local function make<T>(value: T): Box<T>
			return {value = value}
		end
		local function build(): StringBox
			return make(true)
		end
	`
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeReturnContractType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want return contract error", d)
	}
}

func TestReturnContractSkipsUninferredGenericDirectCallReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		type User = {id: string}
		type Result<T> = {ok: true, value: T} | {ok: false, error: string}
		local function invalid<T>(message: string): Result<T>
			return {ok = false, error = message}
		end
		local function decode(): Result<User>
			return invalid("id")
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for uninferred generic return", diags)
	}
}

func runDiagnostics(t *testing.T, src string) []diagnostic.Diagnostic {
	t.Helper()
	return runDiagnosticsWithGlobals(t, src, []string{"test", "type", "value"})
}

func runDiagnosticsWithGlobals(t *testing.T, src string, globals []string) []diagnostic.Diagnostic {
	t.Helper()
	return runDiagnosticsFull(t, src, globals, signaturelookup.Source{})
}

func runDiagnosticsWithSignatures(t *testing.T, src string, signatures signaturelookup.Source) []diagnostic.Diagnostic {
	t.Helper()
	return runDiagnosticsFull(t, src, []string{"test", "type", "value"}, signatures)
}

func runDiagnosticsFull(t *testing.T, src string, globals []string, signatures signaturelookup.Source) []diagnostic.Diagnostic {
	t.Helper()
	stmts, err := parse.ParseString(src, "diagnostics_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	reg := standard.Registry()
	result, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{
			Registry:   reg,
			Globals:    globals,
			Signatures: signatures,
		},
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	return Produce(result.RootResult())
}

func diagnosticMessages(diags []diagnostic.Diagnostic) []string {
	out := make([]string, len(diags))
	for i, diag := range diags {
		out[i] = diag.Message
	}
	return out
}

func containsDiagnosticMessage(messages []string, want string) bool {
	for _, message := range messages {
		if strings.Contains(message, want) {
			return true
		}
	}
	return false
}

func mustStmts(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "diagnostics_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts
}

func mustFunctionExpr(t *testing.T, src string) *ast.FunctionExpr {
	t.Helper()
	stmts := mustStmts(t, src)
	if len(stmts) != 1 {
		t.Fatalf("stmts = %d, want 1", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want *ast.FuncDefStmt with function", stmts[0])
	}
	return def.Func
}
