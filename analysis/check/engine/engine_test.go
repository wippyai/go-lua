package engine_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/lint"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCheckPublishesScalarAssignment(t *testing.T) {
	result, err := engine.Check(`local answer = 42`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["answer"]; got != "42" {
		t.Fatalf("published answer = %q, want 42; values = %#v", got, result.Values)
	}
	if result.Transactions != 2 {
		t.Fatalf("transactions = %d, want entry plus assignment", result.Transactions)
	}
}

func TestCheckPublishesNumberWitnessForSealedTableLength(t *testing.T) {
	result, err := engine.Check(`
local values: {string}? = { "alpha", "beta" }
local count: number = #values
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.HasPrefix(diagnostic.Key, "claim/unproven/") || strings.HasPrefix(diagnostic.Key, "type.assignment/") {
			t.Fatalf("sealed table length emitted diagnostic = %#v", result.Diagnostics)
		}
	}
}

func TestCheckKeepsConcatResultStringAfterOptionalOperandWarning(t *testing.T) {
	result, err := engine.Check(`
local value: string?
local output: string = "value:" .. value`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	concatWarning, assignmentFailure := false, false
	for _, diagnostic := range result.PublishedDiagnostics {
		concatWarning = concatWarning || diagnostic.Code == "type.operator.concat_operand"
		assignmentFailure = assignmentFailure || diagnostic.Code == "type.assignment" && diagnostic.Span.StartLine == 3
	}
	if !concatWarning || assignmentFailure {
		t.Fatalf("optional concat diagnostics = %#v, want warning without assignment failure", result.PublishedDiagnostics)
	}
}

func TestCheckPublishesProvenAnnotationAssignmentMismatch(t *testing.T) {
	result, err := engine.Check(`local value: string = 42`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Diagnostics)["type.assignment/op-00000002"]; got != "cannot assign value because it is number, not string" {
		t.Fatalf("assignment diagnostics = %#v, want proven mismatch", result.Diagnostics)
	}
	if len(result.PublishedDiagnostics) != 1 {
		t.Fatalf("published diagnostics = %#v, want one projected assignment", result.PublishedDiagnostics)
	}
	published := result.PublishedDiagnostics[0]
	if published.Code != "type.assignment" || published.Message != "cannot assign value because it is number, not string" || !published.Span.Valid() {
		t.Fatalf("published assignment = %#v, want code, message, and WIR span", published)
	}
	if len(published.Evidence) != 2 || published.Evidence[0].Kind != "abstract fact" || published.Evidence[0].Trust != "proven" || !strings.Contains(published.Evidence[0].Message, "value has literal value 42") || published.Evidence[1].Kind != "user assertion" || published.Evidence[1].Trust != "claimed" || !strings.Contains(published.Evidence[1].Message, "value is declared as string") {
		t.Fatalf("assignment evidence = %#v, want closed value and annotation claim", published.Evidence)
	}
	if len(published.Labels) != 2 || !strings.Contains(published.Help, "change the target type") {
		t.Fatalf("assignment labels/help = %#v / %q", published.Labels, published.Help)
	}
}

func TestCheckProjectProvesImportedSealedNestedRecordAssignment(t *testing.T) {
	result, err := lint.CheckProject(context.Background(), lint.ProjectInput{Entries: []lint.Entry{
		{Path: "types.lua", ModulePath: "types", Source: `
type Tool = string | { id: string, context: {[string]: any}?, description: string?, alias: string? }
type Entry = {
  id: string,
  meta: { type: string?, name: string?, comment: string? }?,
  data: { prompt: string?, tools: {Tool}?, context: {[string]: any}? }?,
}
local M = {}
M.Entry = Entry
M.KIND = "agent.trait"
return M
`},
		{Path: "main.lua", ModulePath: "main", Source: `
local types = require("types")
local entry: types.Entry = {
  id = "search-trait",
  meta = {type = types.KIND, name = "Search", comment = "Web search capability"},
  data = {
    prompt = "You can search the web using the search tool.",
    tools = {
      "tool:web-search",
      {id = "tool:scrape", description = "Scrape a URL", alias = "fetch"},
      {id = "tool:summarize", context = {max_length = 500}},
    },
    context = {api_key = "sk-123"},
  },
}
`},
	}, Targets: []string{"main"}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("imported sealed nested record assignment diagnostics = %#v", result.Diagnostics)
	}
}

func TestCheckPublishesKeyedIteratorWitnessFromTypedMap(t *testing.T) {
	result, err := engine.Check(`
local notes: {[string]: string} = { ready = "ok" }
for key, note in pairs(notes) do
  local stable_key: string = key
  local stable_note: string = note
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("keyed iterator diagnostics = %#v", result.Diagnostics)
	}
}

func TestCheckLiteralDiscriminantNarrowingStaysOnItsLoopArm(t *testing.T) {
	result, err := engine.Check(`
type Release = {kind: "release", reservation_token: string}
type Refund = {kind: "refund", payment_id: string}
type Compensation = Release | Refund
local compensations: {Compensation} = {}
for _, comp in ipairs(compensations) do
    local unsafe: string = comp.reservation_token
    if comp.kind == "release" then
        local token: string = comp.reservation_token
    else
        local payment: string = comp.payment_id
    end
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	unguardedMissing := false
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code != "type.member.missing" {
			continue
		}
		if diagnostic.Span.StartLine == 7 {
			unguardedMissing = true
			continue
		}
		t.Fatalf("literal branch leaked an arm fact: %#v", result.PublishedDiagnostics)
	}
	if !unguardedMissing {
		t.Fatalf("unguarded discriminated-union read was accepted: %#v", result.PublishedDiagnostics)
	}
}

func TestCheckRejectsUnguardedUnionMemberFromDeclaredCallResult(t *testing.T) {
	source := `
type Event = {kind: string}
type Timer = {elapsed: number}
type Result = Event | Timer
function get_result(use_timer: boolean): Result
    if use_timer then
        return {elapsed = 1}
    end
    return {kind = "exit"}
end
function f(use_timer: boolean)
    local result = get_result(use_timer)
    local kind: string = result.kind
end
`
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, "result.kind because it may be nil") {
			return
		}
	}
	t.Fatalf("unguarded declared union result member was accepted: %#v", result.PublishedDiagnostics)
}

func TestCheckRejectsConcreteReplacementOfTypedFunctionMember(t *testing.T) {
	result, err := engine.Check(`
local M = {}
function M.f(): string
    return "ok"
end
M.f = 42
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code != "type.assignment" || diagnostic.Span.StartLine != 6 || diagnostic.Span.StartCol != 7 {
			continue
		}
		if diagnostic.Message != "cannot assign M.f because assigned value is 42, not fun() -> string" {
			t.Fatalf("typed function member diagnostic = %#v", diagnostic)
		}
		if len(diagnostic.Evidence) != 2 || diagnostic.Evidence[0].Message != "assigned value has literal value 42" || diagnostic.Evidence[1].Message != "M.f is declared as fun() -> string" {
			t.Fatalf("typed function member evidence = %#v", diagnostic.Evidence)
		}
		if len(diagnostic.Labels) != 2 || diagnostic.Labels[0].Span.StartCol != 7 || diagnostic.Labels[1].Span.StartCol != 1 {
			t.Fatalf("typed function member labels = %#v", diagnostic.Labels)
		}
		return
	}
	t.Fatalf("typed function member replacement was not rejected: %#v", result.PublishedDiagnostics)
}

func TestCheckRejectsBroadWriteToSealedLiteralRecord(t *testing.T) {
	result, err := engine.Check(`
local item = { count = 1, name = "ready" }
for key, value in pairs(item) do
  item[key] = tostring(value)
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code != "type.assignment" || !strings.Contains(diagnostic.Message, "tostring(...)") || !strings.Contains(diagnostic.Message, `not 1 & "ready"`) {
			continue
		}
		for _, evidence := range diagnostic.Evidence {
			if strings.Contains(evidence.Message, "assignment target item[key] requires 1 & \"ready\"") {
				return
			}
		}
	}
	t.Fatalf("broad dynamic write did not reject sealed literal contract: diagnostics=%#v facts=%#v", result.PublishedDiagnostics, result.ValueFacts)
}

func TestCheckProjectsRecursiveRecordFieldMismatch(t *testing.T) {
	source := `
type Tree = { root: TreeNode? }
type TreeNode = { label: string, owner: Tree, children: {TreeNode}, parent: TreeNode? }
local tree: Tree = {root = nil}
local node: TreeNode = {label = 123, owner = tree, children = {}, parent = nil}
`
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code != "type.assignment" || !strings.Contains(diagnostic.Message, "node.label") {
			continue
		}
		if diagnostic.Span.StartLine != 5 || diagnostic.Span.StartCol != 33 {
			t.Fatalf("field mismatch span = %#v, want main source 5:33", diagnostic.Span)
		}
		if len(diagnostic.Evidence) != 2 || !strings.Contains(diagnostic.Evidence[0].Message, "node.label has literal value 123") || !strings.Contains(diagnostic.Evidence[1].Message, "node.label is declared as string") {
			t.Fatalf("field mismatch evidence = %#v", diagnostic.Evidence)
		}
		return
	}
	t.Fatalf("recursive field mismatch was not projected: %#v", result.PublishedDiagnostics)
}

func TestCheckCanonicalizesDirectCallDiagnosticsAtPublication(t *testing.T) {
	result, err := engine.Check(`
local function takes_string(s: string): string return s end
local value: number = 5
takes_string(value)
takes_string()
local target: number = 1
target()
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	got := make(map[string]engine.PublishedDiagnostic)
	for _, item := range result.PublishedDiagnostics {
		got[item.Code] = item
	}
	argument, ok := got["type.call.direct.argument_type"]
	if !ok {
		t.Fatalf("published diagnostics = %#v, want argument contract violation", result.PublishedDiagnostics)
	}
	if argument.Message != "argument 1 (value) is 5, not string" || !argument.Span.Valid() {
		t.Fatalf("argument diagnostic = %#v, want canonical message and argument span", argument)
	}
	if len(argument.Evidence) != 3 || argument.Evidence[0].Message != "argument 1 (value) has literal value 5" || argument.Evidence[1].Message != "takes_string parameter 1 expects string" || argument.Evidence[2].Trust != "refuted" || !strings.Contains(argument.Evidence[2].Message, "value satisfies the parameter type") {
		t.Fatalf("argument evidence = %#v, want closed value, contract, and missing-proof chain", argument.Evidence)
	}
	if !strings.Contains(argument.Help, "Pass `value`") || len(argument.Labels) != 1 {
		t.Fatalf("argument help/labels = %q / %#v", argument.Help, argument.Labels)
	}
	arity, ok := got["type.call.direct.too_few_args"]
	if !ok || len(arity.Evidence) != 2 || !strings.Contains(arity.Evidence[0].Message, "passes 0 arguments") || !strings.Contains(arity.Help, "missing required arguments") {
		t.Fatalf("arity diagnostic = %#v, want canonical call-contract explanation", arity)
	}
	nonCallable, ok := got["type.call.direct.not_callable"]
	if !ok || len(nonCallable.Evidence) != 1 || nonCallable.Evidence[0].Message != "target has literal value 1" || !strings.Contains(nonCallable.Help, "replace `target`") {
		t.Fatalf("non-callable diagnostic = %#v, want canonical callable explanation", nonCallable)
	}
}

func TestFixtureNilableDirectCallsPublishStructuredProjection(t *testing.T) {
	expected := map[string]int{
		"realworld/agent-workflow-engine-soundness":         24,
		"realworld/notification-delivery-runtime-soundness": 50,
	}
	suites, err := discoverFixtures(corpusRepositoryRoot(t) + "/testdata/fixtures")
	if err != nil {
		t.Fatalf("discover fixtures: %v", err)
	}
	for _, suite := range suites {
		line, selected := expected[suite.Name]
		if !selected {
			continue
		}
		diagnostics, _, _, err := fixtureDiagnostics(suite)
		if err != nil {
			t.Fatalf("%s: fixture diagnostics: %v", suite.Name, err)
		}
		var expectation fixtureDiagnosticExpectation
		for _, candidate := range suite.Suite.Check.Diagnostics {
			if candidate.Code == "type.call.direct.argument_type" && candidate.Line == line {
				expectation = candidate
				break
			}
		}
		for _, item := range diagnostics {
			if item.Code != "type.call.direct.argument_type" || item.Position.Line != line {
				continue
			}
			evidence := item.Explanation.Evidence()
			if !strings.Contains(item.Message, "cannot pass") || len(evidence) != 3 || !strings.Contains(evidence[0].Message, "can be time.Time or nil") || evidence[2].Reason.String() != "boundary validation missing" || evidence[2].Trust.String() != "refuted" || len(item.Labels) != 1 || item.Help == "" || !matchesDiagnosticExpectation(expectation, item, "main.lua", renderOptions(suite, "main.lua")) {
				t.Fatalf("%s:%d nilable call projection = %#v\n%s", suite.Name, line, item, diag.Render(item, renderOptions(suite, "main.lua")))
			}
			delete(expected, suite.Name)
			break
		}
	}
	if len(expected) != 0 {
		t.Fatalf("nilable direct-call projections missing for %#v", expected)
	}
}

func TestCheckPublishesInvokedTypedMemberCallArgumentContract(t *testing.T) {
	result, err := engine.Check(`
local function invoke(provider, payload)
  provider.send(payload)
end

local provider: { send: (number) -> () } = {
  send = function(value: number): () end,
}

invoke(provider, "bad")
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code != "type.call.direct.argument_type" || diagnostic.Message != `argument 2 is "bad", not number` {
			continue
		}
		if diagnostic.Span.StartLine != 10 || diagnostic.Span.StartCol != 18 {
			t.Fatalf("summary call span = %#v, want caller argument", diagnostic.Span)
		}
		if len(diagnostic.Evidence) != 3 || diagnostic.Evidence[0].Message != `argument 1 (payload) has literal value "bad"` || diagnostic.Evidence[1].Message != "inside invoke, argument 1 (payload) is passed to provider.send parameter 1, which requires number" || diagnostic.Evidence[2].Trust != "unknown" || diagnostic.Evidence[2].Message != "no proof on this path shows argument 1 (payload) is number" {
			t.Fatalf("summary call evidence = %#v", diagnostic.Evidence)
		}
		if len(diagnostic.Labels) != 1 || !strings.Contains(diagnostic.Labels[0].Message, "argument value") || !strings.Contains(diagnostic.Help, "argument 2") {
			t.Fatalf("summary call labels/help = %#v / %q", diagnostic.Labels, diagnostic.Help)
		}
		return
	}
	t.Fatalf("invoked member argument contract was not published: diagnostics=%#v facts=%#v", result.PublishedDiagnostics, result.ValueFacts)
}

func TestCheckRejectsRefutedTypedVariadicArgument(t *testing.T) {
	result, err := engine.Check(`
local function sum(...: number): number
    return 0
end
sum(1, 2, "three")
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, fact := range result.Diagnostics {
		if strings.HasPrefix(fact.Key, "type.call.direct.argument_type/") && strings.Contains(string(fact.Value), `argument 3 is "three", not number`) {
			return
		}
	}
	t.Fatalf("typed variadic argument mismatch was not proven: %#v", result.Diagnostics)
}

func TestCheckDoesNotPublishUncheckedCastTypeWitness(t *testing.T) {
	result, err := engine.Check(`
local value = 5 :: string
local values: {string} = {value}
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, fact := range result.Diagnostics {
		if strings.Contains(string(fact.Value), `claim "string[]" is not proven`) {
			return
		}
	}
	t.Fatalf("unchecked cast made aggregate annotation pass: %#v", result.Diagnostics)
}

func TestCheckRendersOptionalRecursiveAssignmentAsNilability(t *testing.T) {
	result, err := engine.Check(`
type Node = {
	name: string,
    child: Node?,
    set_child: (self: Node, child: Node) -> Node,
}
local function make_node(name: string): Node
    local node: Node = {
        name = name,
        child = nil,
        set_child = function(self: Node, child: Node): Node
            self.child = child
            return self
        end,
    }
    return node
end
local root = make_node("root")
local child = make_node("child")
root:set_child(child)
if root.child then
    local stable: string = root.child.name
end
local direct: Node = root.child
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, "cannot assign root.child because it may be nil") {
			return
		}
	}
	t.Fatalf("optional recursive assignment did not render nilability: %#v", result.PublishedDiagnostics)
}

func TestCheckKeepsGuardedRecursiveParentPresent(t *testing.T) {
	result, err := engine.Check(`
type Tree = {
	id: string,
	parent: Tree?,
	children: {Tree},
}
local root: Tree = { id = "root", parent = nil, children = {} }
local child: Tree = { id = "child", parent = root, children = {} }
table.insert(root.children, child)
local first = root.children[1]
if first then
	if first.parent then
		local parentID: string = first.parent.id
	end
end
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, "first.parent.id") {
			t.Fatalf("guarded recursive parent read was treated as nil: %#v", result.PublishedDiagnostics)
		}
	}
}

func TestCheckDoesNotPublishAssignmentMismatchForUnknownValue(t *testing.T) {
	result, err := engine.Check(`local value: string = provider()`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Diagnostics)["type.assignment/op-00000002"]; got != "" {
		t.Fatalf("unknown value emitted assignment mismatch: %#v", result.Diagnostics)
	}
}

func TestCheckCarriesCastIndexOptionalWitnessToAssignment(t *testing.T) {
	result, err := engine.Check(`
local value: any = {}
local item: number = (value :: {number})[1]
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, "item") && strings.Contains(diagnostic.Message, "number?") && strings.Contains(diagnostic.Message, "not number") {
			return
		}
	}
	t.Fatalf("cast index optional witness did not reach assignment: %#v", result.PublishedDiagnostics)
}

func TestCheckPublishesOptionalReturnFromInvokedAnyFunction(t *testing.T) {
	result, err := engine.Check(`
local function f(v: any): number
		return (v :: {number})[1]
end
return f({10, 20})
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if strings.Contains(diagnostic.Code, "type.return.contract") && strings.Contains(diagnostic.Message, "may be nil") {
			return
		}
	}
	t.Fatalf("optional local return was not published: %#v", result.PublishedDiagnostics)
}

func TestCheckProjectsInferredClosedTableResultFromCapturedMember(t *testing.T) {
	result, err := lint.CheckProject(context.Background(), lint.ProjectInput{Entries: []lint.Entry{{
		Path:       "main.lua",
		ModulePath: "main",
		Source: `local M = {
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

local res = M.run()
local answer: string = res.answer`,
	}}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("inferred reassigned member result diagnostics = %#v", result.Diagnostics)
	}
}

func TestCheckProjectsReassignedNestedMemberClosureResult(t *testing.T) {
	result, err := engine.Check(`local M = {
	dep = {
		get = function()
			return nil
		end,
	},
}

M.dep = {
	get = function()
		return { answer = "ok" }
	end,
}

local res = M.dep.get()
local answer: string = res.answer`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("reassigned nested member result diagnostics = %#v", result.Diagnostics)
	}
	if got := valuesByName(result.Values)["answer"]; got != `"ok"` {
		t.Fatalf("reassigned nested member answer = %q, want \"ok\"; values=%#v", got, result.Values)
	}
}

func TestCheckPublishesReassignedMemberCallableAssignmentEvidence(t *testing.T) {
	result, err := engine.Check(`
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

local f: fun(): Res = M.run`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code != "type.assignment" || !strings.Contains(diagnostic.Message, "cannot assign M.run because it is fun() -> nil, not fun() -> Res") {
			continue
		}
		if len(diagnostic.Evidence) != 2 || diagnostic.Evidence[0].Kind != "abstract fact" || diagnostic.Evidence[0].Message != "M.run has type fun() -> nil" || diagnostic.Evidence[1].Kind != "user assertion" || diagnostic.Evidence[1].Message != "f is declared as fun() -> Res" {
			t.Fatalf("reassigned member assignment evidence = %#v", diagnostic.Evidence)
		}
		if len(diagnostic.Labels) != 2 || !strings.Contains(diagnostic.Help, "change the target type") {
			t.Fatalf("reassigned member assignment labels/help = %#v / %q", diagnostic.Labels, diagnostic.Help)
		}
		return
	}
	t.Fatalf("reassigned member callable assignment was not published: %#v", result.PublishedDiagnostics)
}

func TestCheckPublishesUncalledExplicitAnyBoundaryViolation(t *testing.T) {
	result, err := engine.Check(`
local function validate(data: any)
  local point: {x: number, y: number} = data
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(result.Diagnostics) != 1 || !strings.Contains(result.Diagnostics[0].Key, "/type.assignment/") || !strings.Contains(string(result.Diagnostics[0].Value), "comes from any/unknown") {
		t.Fatalf("uncalled explicit-any boundary diagnostics = %#v", result.Diagnostics)
	}
}

func TestCheckPublishesUncalledExplicitAnyBoundaryViolationThroughClosedCapture(t *testing.T) {
	result, err := engine.Check(`
local function consume(value: any)
  return value
end
local function validate(data: any)
  consume(data)
  local point: {x: number, y: number} = data
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(result.Diagnostics) != 1 || !strings.Contains(result.Diagnostics[0].Key, "/type.assignment/") || !strings.Contains(string(result.Diagnostics[0].Value), "data comes from any/unknown") {
		t.Fatalf("uncalled captured explicit-any boundary diagnostics = %#v", result.Diagnostics)
	}
}

func TestCheckPublishesUncalledStaticAssignmentThroughTypedClosedCapture(t *testing.T) {
	result, err := engine.Check(`
local function load(ok: boolean): string?
  return nil
end
local function process(ok: boolean)
  local x = load(ok)
  local s: string = x
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(result.Diagnostics) != 1 || !strings.Contains(result.Diagnostics[0].Key, "/type.assignment/") ||
		!strings.Contains(string(result.Diagnostics[0].Value), "cannot assign x") {
		t.Fatalf("uncalled typed-capture assignment diagnostics = %#v", result.Diagnostics)
	}
}

func TestCheckPublishesUncalledDeclaredIndexedOptionalContract(t *testing.T) {
	result, err := engine.Check(`
local function pick(values: {number}, index: number): number
  return values[index]
end
return pick
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.Contains(diagnostic.Key, "/type.return.contract/") && strings.Contains(string(diagnostic.Value), "may be nil") {
			return
		}
	}
	t.Fatalf("uncalled declared index diagnostic = %#v", result.Diagnostics)
}

func TestCheckPublishesUncalledDeclaredBranchAssignmentContract(t *testing.T) {
	result, err := engine.Check(`
type A = {tag: "a", value: string}
type B = {tag: "b", value: number}
local function check(r: A | B)
  if r.tag == "a" then
    return
  end
  local s: string = r.value
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && diagnostic.Span.StartLine == 8 && strings.Contains(diagnostic.Message, "cannot assign r.value") {
			return
		}
	}
	t.Fatalf("uncalled declared branch assignment diagnostic = %#v", result.PublishedDiagnostics)
}

func TestCheckClosedLiteralAbsenceKeepsDeclaredOptionalMemberWitness(t *testing.T) {
	result, err := engine.Check(`
type Message = { data: string }
type Timeout = { elapsed: number }
local function consume(messages: Channel<Message>, timeout: Channel<Timeout>): string?
  local result: { channel: any, value: Message | Timeout, ok: boolean } = channel.select {
    messages:case_receive(),
    timeout:case_receive(),
  }
  result = { channel = messages, value = { elapsed = 1 }, ok = true }
  if result.channel == messages then
    local data: string = result.value.data
    return data
  end
  return nil
end
return consume
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, "result.value.data") && strings.Contains(diagnostic.Message, "may be nil") {
			return
		}
	}
	t.Fatalf("declared optional member witness was not published: %#v", result.PublishedDiagnostics)
}

func TestCheckPublishesExplicitAnyBoundaryViolationThroughGuardedMemberRead(t *testing.T) {
	result, err := engine.Check(`
local raw: any = {kind = "task", route_id = "start"}
if raw.kind == "task" then
	local routeID: string = raw.route_id
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.HasPrefix(diagnostic.Key, "type.assignment/") && strings.Contains(string(diagnostic.Value), "raw.route_id") {
			return
		}
	}
	t.Fatalf("guarded explicit-any member read diagnostics = %#v", result.Diagnostics)
}

func TestCheckPublishesExplicitAnyBoundaryViolationThroughCompositeTypeGuard(t *testing.T) {
	result, err := engine.Check(`
local raw: any = {items = {"ok", 99}}
if type(raw.items) == "table" and type(raw.items[1]) == "string" then
	local labels: {string} = raw.items
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && diagnostic.Span.StartLine == 4 && strings.Contains(diagnostic.Message, "cannot assign raw.items because it is any, not string[]") {
			return
		}
	}
	t.Fatalf("composite guarded explicit-any member read diagnostics = %#v", result.PublishedDiagnostics)
}

func TestCheckPreservesExplicitAnyBoundaryThroughSealedTableIteration(t *testing.T) {
	result, err := engine.Check(`
local unknownID: any = nil
local pages = {{id = unknownID, route = "/ready"}}
local routes: {[string]: string} = { ["/ready"] = "ready" }
local accessible: {[string]: string} = {}
for _, page in ipairs(pages) do
	if routes[page.route] == page.id then
		accessible[page.route] = page.id
	end
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && diagnostic.Span.StartLine == 8 && strings.Contains(diagnostic.Message, "is any, not string") {
			return
		}
	}
	t.Fatalf("explicit-any field boundary was lost through sealed table iteration: %#v", result.PublishedDiagnostics)
}

func TestCheckProvesClosedAnyArrayAnnotation(t *testing.T) {
	result, err := engine.Check(`local values: {any} = {{kind = "task"}, {kind = "timer"}}`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.HasPrefix(diagnostic.Key, "claim/unproven/") {
			t.Fatalf("closed any-array annotation emitted unproven claim: %#v", result.Diagnostics)
		}
	}
}

func TestCheckInvalidatesMapEntryAliasAfterVariantWrite(t *testing.T) {
	result, err := engine.Check(`
type FileSlot = {kind: "file", path: string}
type TimerSlot = {kind: "timer", seconds: number}
type Slot = {value: FileSlot | TimerSlot}
type Slots = {[string]: Slot}
local slots: Slots = {active = {value = {kind = "file", path = "/tmp/active"}}}
local alias = slots.active
if alias.value.kind == "file" then
  alias.value = {kind = "timer", seconds = 5}
  local stale: string = slots.active.value.path
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.HasPrefix(diagnostic.Key, "type.assignment/") && strings.Contains(string(diagnostic.Value), "slots.active.value.path") {
			return
		}
	}
	t.Fatalf("stale map-entry alias diagnostics = %#v", result.Diagnostics)
}

func TestCheckWithImportsSeedsExactRequireExportAndOmitsAny(t *testing.T) {
	source := `local provider = require("provider")
local answer: string = provider.answer`
	typed, err := engine.CheckWithImports(source, map[string]typ.Type{
		"provider": typetable.NewRecord().Field("answer", typ.LiteralInt(42)).Build(),
	})
	if err != nil {
		t.Fatalf("CheckWithImports typed: %v", err)
	}
	if got := valuesByName(typed.Diagnostics)["type.assignment/op-00000006"]; !strings.Contains(got, "42") || !strings.Contains(got, "string") {
		t.Fatalf("typed require diagnostic = %#v", typed.Diagnostics)
	}
	unknown, err := engine.CheckWithImports(source, map[string]typ.Type{"provider": typ.Any})
	if err != nil {
		t.Fatalf("CheckWithImports unknown: %v", err)
	}
	// A sound checker rejects an any-typed value assigned to a declared type:
	// any proves nothing, so the annotation is an unproven claim. This mirrors
	// the oracle's annotation-lie family.
	rejected := false
	for _, diagnostic := range unknown.Diagnostics {
		if strings.HasPrefix(diagnostic.Key, "type.assignment/") && strings.Contains(string(diagnostic.Value), "any") {
			rejected = true
		}
	}
	if !rejected {
		t.Fatalf("any import assignment to declared string must be rejected, got %#v", unknown.Diagnostics)
	}
}

func TestCheckWithImportsProjectsResolvedCallableMemberResult(t *testing.T) {
	source := `local provider = require("provider")
local answer: string = provider.answer()`
	typed, err := engine.CheckWithImports(source, map[string]typ.Type{
		"provider": typetable.NewRecord().Field("answer", typ.Func().Returns(typ.LiteralInt(42)).Build()).Build(),
	})
	if err != nil {
		t.Fatalf("CheckWithImports typed callable: %v", err)
	}
	for _, diagnostic := range typed.Diagnostics {
		if strings.HasPrefix(diagnostic.Key, "type.assignment/") && strings.Contains(string(diagnostic.Value), "42") && strings.Contains(string(diagnostic.Value), "string") {
			return
		}
	}
	t.Fatalf("typed imported callable diagnostic = %#v", typed.Diagnostics)
}

func TestCheckWithImportsRejectsNonNumberAtInferredArithmeticBoundary(t *testing.T) {
	source := `local provider = require("provider")
local config = { rate = 4 }
local function scale(tokens)
  return tokens * config.rate
end
local function run()
  local item = provider.meta()
  return scale(item)
end
return run`
	provider := typetable.NewRecord().Field("meta", typ.Func().Returns(
		typetable.NewRecord().Field("name", typ.String).Build(),
	).Build()).Build()
	result, err := engine.CheckWithImports(source, map[string]typ.Type{"provider": provider})
	if err != nil {
		t.Fatalf("CheckWithImports: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.call.direct.argument_type" && diagnostic.Span.StartLine == 8 && strings.Contains(diagnostic.Message, "not number") {
			return
		}
	}
	t.Fatalf("inferred arithmetic boundary diagnostics = %#v", result.PublishedDiagnostics)
}

func TestCheckWithImportsRootTruthinessNarrowingUsesCurrentOptionalResultSummary(t *testing.T) {
	source := `local provider = require("provider")
local err = provider.fetch()
if err then
  return
end
local bad: number = "not a number"`
	errType := typetable.NewRecord().Field("message", typ.String).Build()
	provider := typetable.NewRecord().Field("fetch", typ.Func().Returns(typ.MaterializeOptional(errType)).Build()).Build()
	result, err := engine.CheckWithImports(source, map[string]typ.Type{"provider": provider})
	if err != nil {
		t.Fatalf("CheckWithImports: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && diagnostic.Span.StartLine == 6 && strings.Contains(diagnostic.Message, "not number") {
			return
		}
	}
	t.Fatalf("root truthiness guard did not retain its false-edge continuation: diagnostics=%#v", result.PublishedDiagnostics)
}

func TestCheckWithImportsProjectsTypedDirectCallableRecordResult(t *testing.T) {
	source := `local provider = require("provider")
local record: { id: string } = provider.make()`
	provider := typetable.NewRecord().Field("make", typ.Func().Returns(
		typetable.NewRecord().Field("id", typ.String).Build(),
	).Build()).Build()
	result, err := engine.CheckWithImports(source, map[string]typ.Type{"provider": provider})
	if err != nil {
		t.Fatalf("CheckWithImports typed direct callable: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("typed direct callable record diagnostics = %#v", result.Diagnostics)
	}
}

func TestCheckWithImportsCarriesIndexedIteratorElementWitness(t *testing.T) {
	source := `local provider = require("provider")
local rows = provider.list()
for _, row in ipairs(rows) do
  local id: string = row.id
  local ready: boolean = row.ready
end`
	row := typetable.NewRecord().Field("id", typ.String).Field("ready", typ.Boolean).Build()
	provider := typetable.NewRecord().Field("list", typ.Func().Returns(typ.NewArray(row)).Build()).Build()
	result, err := engine.CheckWithImports(source, map[string]typ.Type{"provider": provider})
	if err != nil {
		t.Fatalf("CheckWithImports: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("indexed iterator lost the imported array element witness: %#v", result.Diagnostics)
	}
}

func TestCheckWithImportsProjectsTypedReceiverMethodResult(t *testing.T) {
	source := `local provider = require("provider")
local object = provider.new()
local answer: number = object:answer()`
	widget := typetable.NewRecord().Field("answer", typ.Func().Returns(typ.LiteralInt(42)).Build()).Build()
	provider := typetable.NewRecord().Field("new", typ.Func().Returns(widget).Build()).Build()
	result, err := engine.CheckWithImports(source, map[string]typ.Type{"provider": provider})
	if err != nil {
		t.Fatalf("CheckWithImports typed receiver: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("typed imported receiver method diagnostics = %#v", result.Diagnostics)
	}
}

func TestCheckWithImportsProjectsPublishedInterfaceMethodResult(t *testing.T) {
	source := `local provider = require("provider")
local object = provider.new()
local answer: number = object:answer()`
	object := typ.NewInterface("provider.Object", []typ.Method{
		{Name: "answer", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	provider := typetable.NewRecord().Field("new", typ.Func().Returns(object).Build()).Build()
	result, err := engine.CheckWithImports(source, map[string]typ.Type{"provider": provider})
	if err != nil {
		t.Fatalf("CheckWithImports interface receiver: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("typed imported interface receiver diagnostics = %#v", result.Diagnostics)
	}
}

func TestCheckWithImportsRejectsAnyAtPublishedInterfaceMethodBoundary(t *testing.T) {
	duration := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "seconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	timeValue := typ.NewInterface("time.Time", []typ.Method{
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("other", typ.Self).Returns(duration).Build()},
	})
	host := manifest.New("time")
	host.SetExport(typetable.NewRecord().Field("now", typ.Func().Returns(timeValue).Build()).Build())
	result, err := lint.CheckProject(context.Background(), lint.ProjectInput{Entries: []lint.Entry{
		{Path: "session.lua", ModulePath: "session", Source: `
type ActiveSession = { created_at: any, last_activity: any }
local M = {}
function M.new(): ActiveSession end
return M`},
		{Path: "main.lua", ModulePath: "main", Source: `
local time = require("time")
local session = require("session")
local now = time.now()
local state = session.new()
local activity = state.last_activity or state.created_at
local elapsed = now:sub(activity)`},
	}, Targets: []string{"main"}, Manifests: []*manifest.Manifest{host}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	for _, entry := range result.Entries {
		for _, diagnostic := range entry.Engine.PublishedDiagnostics {
			if diagnostic.Code == "type.call.direct.argument_type" && strings.Contains(diagnostic.Message, "argument 1") && strings.Contains(diagnostic.Message, "time.Time") {
				return
			}
		}
	}
	t.Fatalf("published interface method did not reject any argument")
}

func TestCheckWithImportsJoinsInstantiatedGenericSummaryAcrossCalls(t *testing.T) {
	item := typ.NewTypeParam("T", nil)
	result := typ.NewTypeParam("U", nil)
	accumulator := typ.NewTypeParam("A", nil)
	filter := typ.Func().TypeParamRef(item).
		Param("arr", typ.NewArray(item)).
		Param("pred", typ.Func().Param("item", item).Returns(typ.Boolean).Build()).
		Returns(typ.NewArray(item)).Build()
	mapFn := typ.Func().TypeParamRef(item).TypeParamRef(result).
		Param("arr", typ.NewArray(item)).
		Param("fn", typ.Func().Param("item", item).Returns(result).Build()).
		Returns(typ.NewArray(result)).Build()
	reduce := typ.Func().TypeParamRef(item).TypeParamRef(accumulator).
		Param("arr", typ.NewArray(item)).
		Param("fn", typ.Func().Param("acc", accumulator).Param("item", item).Returns(accumulator).Build()).
		Param("initial", accumulator).
		Returns(accumulator).Build()
	provider := typetable.NewRecord().
		Field("filter", filter).
		Field("map", mapFn).
		Field("reduce", reduce).
		Build()
	resultCheck, err := engine.CheckWithImports(`local iter = require("iter")
type User = {name: string, age: number, active: boolean}
local users: {User} = {
  {name = "Ada", age = 31, active = true},
  {name = "Bob", age = 28, active = false},
}
local active = iter.filter(users, function(user: User): boolean return user.active end)
local names = iter.map(active, function(user: User): string return user.name end)
local lengths = iter.map(names, function(name: string): number return #name end)
local total: number = iter.reduce(lengths, function(acc: number, length: number): number return acc + length end, 0)`, map[string]typ.Type{"iter": provider})
	if err != nil {
		t.Fatalf("CheckWithImports: %v", err)
	}
	if len(resultCheck.Diagnostics) != 0 {
		t.Fatalf("instantiated generic summary was not joined at the final consumer: %#v", resultCheck.Diagnostics)
	}
}

func TestCheckRetainsCallableMemberCapabilityThroughIndexedReplacement(t *testing.T) {
	result, err := engine.Check(`
type Message = { _topic: string, topic: (self: Message) -> string }
local messages: {[string]: Message} = {}
if not messages["root"] then
  messages["root"] = {
    _topic = "installed",
    topic = function(self: Message): string return self._topic end,
  }
end
local installed: string = messages["root"]:topic()
local cached = messages["root"]
if cached then
  local cached_topic: string = cached:topic()
end
assert(messages["root"])
local asserted: string = messages["root"]:topic()
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.HasPrefix(diagnostic.Key, "claim/unproven/") || strings.HasPrefix(diagnostic.Key, "type.assignment/") {
			t.Fatalf("indexed replacement lost callable member capability: %#v", result.Diagnostics)
		}
	}
}

func TestCheckPublishesGenericReturnWhenItsDeclarationIsNotStructural(t *testing.T) {
	result, err := engine.Check(`
local function first<T>(items: {T}): T?
  return items[1]
end
local value = first({"ready"})
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.HasPrefix(diagnostic.Key, "lint.analysis.conservative/") {
			t.Fatalf("generic return declaration prevented publication: %#v", result.Diagnostics)
		}
	}
}

func TestCheckWithImportsRetainsChainedGenericArrayResult(t *testing.T) {
	item := typ.NewTypeParam("T", nil)
	u := typ.NewTypeParam("U", nil)
	a := typ.NewTypeParam("A", nil)
	mapper := typ.Func().Param("item", item).Returns(u).Build()
	reducer := typ.Func().Param("acc", a).Param("item", item).Returns(a).Build()
	predicate := typ.Func().Param("item", item).Returns(typ.Boolean).Build()
	iter := typetable.NewRecord().
		Field("filter", typ.Func().TypeParamRef(item).Param("arr", typ.NewArray(item)).Param("predicate", predicate).Returns(typ.NewArray(item)).Build()).
		Field("map", typ.Func().TypeParamRef(item).TypeParamRef(u).Param("arr", typ.NewArray(item)).Param("fn", mapper).Returns(typ.NewArray(u)).Build()).
		Field("reduce", typ.Func().TypeParamRef(item).TypeParamRef(a).Param("arr", typ.NewArray(item)).Param("fn", reducer).Param("initial", a).Returns(a).Build()).
		Build()
	result, err := engine.CheckWithImports(`
local iter = require("iter")
local words: {string} = {"Ada", "Bob"}
local filtered = iter.filter(words, function(name: string): boolean return #name > 0 end)
local names = iter.map(filtered, function(name: string): string return name end)
local named: {string} = names
local lengths = iter.map(names, function(name: string): number return #name end)
local measured: {number} = lengths
local total: number = iter.reduce(lengths, function(acc: number, length: number): number return acc + length end, 0)
`, map[string]typ.Type{"iter": iter})
	if err != nil {
		t.Fatalf("CheckWithImports: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("chained generic array result diagnostics = %#v published=%#v", result.Diagnostics, result.PublishedDiagnostics)
	}
}

func TestCheckWithImportsRetainsExplicitAnyResultAtEachAssignmentSite(t *testing.T) {
	source := `
local provider = require("provider")
local raw = provider.raw_config()
local config: {id: string} = raw
if raw.id then
  local id: string = raw.id
end
`
	result, err := engine.CheckWithImports(source, map[string]typ.Type{
		"provider": typetable.NewRecord().Field("raw_config", typ.Func().Returns(typ.Any).Build()).Build(),
	})
	if err != nil {
		t.Fatalf("CheckWithImports: %v", err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("imported explicit-any diagnostics = %#v, want exactly two assignment violations", result.Diagnostics)
	}
	seen := make(map[int]bool)
	for _, item := range result.PublishedDiagnostics {
		if item.Code == "type.assignment" {
			seen[item.Span.StartLine] = true
		}
	}
	if !seen[4] || !seen[6] {
		t.Fatalf("imported explicit-any diagnostics = %#v, want separate raw and raw.id assignment violations", result.PublishedDiagnostics)
	}
}

func TestCheckProvesArrayWithCastElementFromPublishedWitness(t *testing.T) {
	result, err := engine.Check(`
type Dispatch = {kind: "dispatch", envelope: {id: string}}
type Tick = {kind: "tick", at: number}
type Request = Dispatch | Tick

local dispatch: Dispatch = {kind = "dispatch", envelope = {id = "ready"}}
local tick: Tick = {kind = "tick", at = 1}
local requests: {Request} = {dispatch, dispatch :: Request, tick}
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "lint.claim.unproven" && diagnostic.Span.StartLine == 8 {
			t.Fatalf("cast element did not retain its published type witness: %#v", result.PublishedDiagnostics)
		}
	}
}

func TestCheckProjectsStdlibProviderReturnSlots(t *testing.T) {
	result, err := engine.Check(`
local text: string = tostring(42)
local kind: string = type(text)
local magnitude: number = math.abs(42)
local joined: string = table.concat({"a", "b"}, ",")
local ok: boolean, value = pcall(tostring, 42)

local function normalize(s: string): string
    local out: string = s:upper():sub(1):rep(2)
    return out
end

local function count(...: any): integer
    local n: integer = select("#", ...)
    return n
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, item := range result.Diagnostics {
		if strings.HasPrefix(item.Key, "lint.claim.unproven/") {
			t.Fatalf("stdlib result contract remained unproven: diagnostics=%#v", result.Diagnostics)
		}
	}
}

func TestCheckProjectsBoundaryStringMethodResultPrecisely(t *testing.T) {
	result := checkChildAdmission(t, `
local function precise(s: string): number
    return s:upper()
end
`)
	for _, item := range result.PublishedDiagnostics {
		if strings.Contains(item.Fact.Key, "/type.return.contract/") && item.Span.StartLine == 3 && strings.Contains(item.Message, "not number") {
			return
		}
	}
	t.Fatalf("boundary string method result remained unproven: %#v", result.PublishedDiagnostics)
}

func TestCheckRejectsMethodUnavailableOnDeclaredPrimitiveUnion(t *testing.T) {
	result := checkChildAdmission(t, `
local function invoke(value: string | number)
    value:upper()
end
`)
	for _, item := range result.PublishedDiagnostics {
		if item.Code == "type.member.missing" && item.Span.StartLine == 3 && strings.Contains(item.Message, `has no member "upper"`) {
			return
		}
	}
	t.Fatalf("declared primitive-union method remained unreported: %#v", result.PublishedDiagnostics)
}

func TestCheckInstantiatesGenericOptionalStdlibResult(t *testing.T) {
	result, err := engine.Check(`
local items: {string} = {"one", "two"}
local removed: string? = table.remove(items)
local required: string = table.remove(items)
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	var requiredRefuted bool
	for _, item := range result.Diagnostics {
		if strings.HasPrefix(item.Key, "claim/unproven/") && strings.Contains(string(item.Value), `"string?"`) {
			t.Fatalf("optional table.remove result remained unproven: %#v", result.Diagnostics)
		}
		if strings.HasPrefix(item.Key, "type.assignment/") && strings.Contains(string(item.Value), "required") && strings.Contains(string(item.Value), "not string") {
			requiredRefuted = true
		}
	}
	if !requiredRefuted {
		t.Fatalf("generic optional stdlib result = values %#v diagnostics %#v", result.Values, result.Diagnostics)
	}
}

func TestCheckPublishesFrozenTableMutation(t *testing.T) {
	result, err := engine.Check(`
local root = { value = 1 }
table.freeze(root)
root.value = 2
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, item := range result.Diagnostics {
		if strings.HasPrefix(item.Key, "effect.freeze.mutation/") && strings.Contains(string(item.Value), `cannot mutate frozen table "root"`) {
			return
		}
	}
	t.Fatalf("frozen table mutation was not proven: %#v", result.Diagnostics)
}

func TestCheckPublishesFrozenGuardedIndexMutation(t *testing.T) {
	result, err := engine.Check(`
local dyn = { x = 1 }
local key = "x"
if table.isfrozen(dyn) then
    dyn[key] = 1
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, item := range result.Diagnostics {
		if strings.HasPrefix(item.Key, "effect.freeze.mutation/") && strings.Contains(string(item.Value), `cannot mutate frozen table "dyn"`) {
			return
		}
	}
	t.Fatalf("guarded frozen index mutation was not proven: diagnostics=%#v outcomes=%#v artifact=%#v", result.Diagnostics, result.Outcomes, result.Artifact.Equations)
}

func TestCheckPublishesSendIsolationJudgmentsFromClosedCallFacts(t *testing.T) {
	result, err := engine.Check(`
local pid = "worker"
process.send(pid, "fresh", { id = "fresh" })
local alias = { id = "alias" }
process.send(pid, "alias", alias)
table.freeze(alias)
process.send(pid, "sealed", alias)
ownership.store(alias, {})
process.send(pid, "stored", alias)
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	want := map[string]bool{
		"send payload is proven isolated for zero-copy transfer":                   false,
		"send payload is not proven isolated or immutable; runtime will copy":      false,
		"send payload is proven immutable for zero-copy sharing":                   false,
		"send payload has a proven escaping alias; zero-copy transfer is rejected": false,
	}
	for _, item := range result.PublishedDiagnostics {
		if _, found := want[item.Message]; !found {
			continue
		}
		want[item.Message] = item.Code == "send.isolation" && item.Span.Valid() && len(item.Evidence) > 0
	}
	for message, found := range want {
		if !found {
			t.Fatalf("send-isolation diagnostic %q absent from %#v", message, result.PublishedDiagnostics)
		}
	}
}

func TestCheckProvesSealedRecordAndUnionLiteralAssignments(t *testing.T) {
	result, err := engine.Check(`
local record: {name: string, nested: {count: number}} = {name = "ok", nested = {count = 1}}
local member: {tag: string} | number = {tag = "member"}
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, fact := range result.Diagnostics {
		if strings.HasPrefix(fact.Key, "claim/unproven/") || strings.HasPrefix(fact.Key, "type.assignment/") {
			t.Fatalf("sealed record/union assignment was not proven: %#v", result.Diagnostics)
		}
	}
}

func TestCheckProjectsExplicitNilLiteralMember(t *testing.T) {
	result, err := engine.Check(`
local table = {value = nil}
local value: number = table.value
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, fact := range result.Diagnostics {
		if strings.HasPrefix(fact.Key, "type.assignment/") && strings.Contains(string(fact.Value), "nil, not number") {
			return
		}
	}
	t.Fatalf("explicit nil member did not refute number assignment: %#v", result.Diagnostics)
}

func TestCheckFailsClosedForTableValuedNewIndexRoute(t *testing.T) {
	result, err := engine.Check(`
local sink = {}
local object = setmetatable({}, { __newindex = sink })
object.answer = 42
local result = sink.answer
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["result"]; got != "unknown" {
		t.Fatalf("table-valued __newindex result = %q, want fail-closed unknown; values = %#v", got, result.Values)
	}
}

func TestCheckDynamicAliasWriteUpdatesSealedTableMember(t *testing.T) {
	result, err := engine.Check(`
type Box = { value: string? }
local box: Box = { value = "ready" }
local alias = box
local key = "value"
if box.value then
    alias[key] = nil
    local after: string = box.value
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, item := range result.Diagnostics {
		if strings.HasPrefix(item.Key, "type.assignment/") && strings.Contains(string(item.Value), "box.value because it is nil, not string") {
			return
		}
	}
	t.Fatalf("alias heap write did not update box.value: %#v", result.Diagnostics)
}

func TestCheckTableInsertPublishesExactHeapElement(t *testing.T) {
	result, err := engine.Check(`
local values = {}
table.insert(values, 7)
local first: number = values[1]
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, item := range result.Diagnostics {
		if strings.HasPrefix(item.Key, "type.assignment/") || strings.HasPrefix(item.Key, "claim/unproven/") {
			t.Fatalf("table.insert element was not retained: %#v", result.Diagnostics)
		}
	}
}

func TestCheckRoutesWhileThroughFrozenCyclicVM(t *testing.T) {
	result, err := engine.Check(`
local total = 0
while false do
    total = 1
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["total"]; got != "0" {
		t.Fatalf("published total = %q, want 0; values = %#v", got, result.Values)
	}
}

func TestCheckNumericForPreservesNumberWitness(t *testing.T) {
	result, err := engine.Check(`
for i = 1, 3 do
	local current: number = i
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, item := range result.Diagnostics {
		if strings.HasPrefix(item.Key, "claim/unproven/") {
			t.Fatalf("numeric loop counter lost its number witness: %#v", result.Diagnostics)
		}
	}
}

func TestCheckTreatsUnknownArrayLengthAsUnprovenBranch(t *testing.T) {
	result, err := engine.Check(`
local function first(values: {string}, index: integer): string
	if #values >= 1 and index >= 1 then
		return values[index]
	end
	return "fallback"
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.HasPrefix(diagnostic.Key, "lint.analysis.conservative/") {
			t.Fatalf("unknown table length aborted evaluation: %#v", result.Diagnostics)
		}
	}
}

func TestCheckExactDynamicWriteThroughAliasPublishesHeapMember(t *testing.T) {
	result, err := engine.Check(`
type Box = { value: string? }
local box: Box = { value = "ready" }
local alias = box
local key = "value"
alias[key] = nil
local after: string = box.value
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, item := range result.Diagnostics {
		if strings.Contains(string(item.Value), "cannot assign box.value because it is nil, not string") {
			for _, published := range result.PublishedDiagnostics {
				if published.Fact.Key == item.Key && len(published.Evidence) == 2 && strings.Contains(published.Evidence[0].Message, "box.value has type nil") {
					return
				}
			}
			t.Fatalf("exact dynamic write diagnostic lacks published nil evidence: %#v", result.PublishedDiagnostics)
		}
	}
	t.Fatalf("exact dynamic write through alias did not publish nil heap member: %#v", result.Diagnostics)
}

func TestCheckExactDynamicVariantWriteUpdatesAliasedMember(t *testing.T) {
	result, err := engine.Check(`
type FileSlot = { kind: "file", path: string }
type TimerSlot = { kind: "timer", seconds: number }
type Slot = { value: FileSlot | TimerSlot }
type Slots = {[string]: Slot}
local slots: Slots = { active = { value = {kind = "file", path = "/tmp/active"} } }
local active = slots.active
local key = "active"
if active.value.kind == "file" then
    slots[key].value = {kind = "timer", seconds = 20}
    local stale_path: string = active.value.path
end
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, "cannot assign active.value.path because it is nil, not string") {
			return
		}
	}
	t.Fatalf("exact dynamic variant write did not update the aliased member: %#v", result.PublishedDiagnostics)
}

func TestCheckExactBracketVariantWriteUpdatesDotMember(t *testing.T) {
	result, err := engine.Check(`
type FileSlot = { kind: "file", path: string }
type TimerSlot = { kind: "timer", seconds: number }
type Slot = { value: FileSlot | TimerSlot }
type Slots = {[string]: Slot}
local slots: Slots = { active = { value = {kind = "file", path = "/tmp/active"} } }
if slots.active.value.kind == "file" then
    local before: string = slots["active"].value.path
    slots["active"].value = {kind = "timer", seconds = 10}
    local stale_path: string = slots.active.value.path
    local stale_seconds: number = before
end
`)
	if err != nil {
		t.Fatal(err)
	}
	missingMember, staleBefore := false, false
	for _, diagnostic := range result.PublishedDiagnostics {
		missingMember = missingMember || diagnostic.Code == "type.member.missing" && strings.Contains(diagnostic.Message, `has no member "path"`)
		staleBefore = staleBefore || diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, `cannot assign before because it is "/tmp/active", not number`)
	}
	if !missingMember || !staleBefore {
		t.Fatalf("exact bracket write publications missing member=%v before=%v: %#v", missingMember, staleBefore, result.PublishedDiagnostics)
	}
}

func TestCheckElementGuardRetainsUntrustedArrayShape(t *testing.T) {
	result, err := engine.Check(`
local raw: any = {
    items = {"ok", 42},
}

if type(raw.items) == "table" and type(raw.items[1]) == "string" then
    local first: string = raw.items[1]
    local allItems: {string} = raw.items
end`)
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, "cannot assign raw.items because it is (\"ok\", 42), not string[]") {
			return
		}
	}
	t.Fatalf("element guard did not retain the sealed array counterexample: %#v", result.PublishedDiagnostics)
}

func TestCheckTriviallyTrueBranchPublishesTruthinessNarrowing(t *testing.T) {
	result, err := engine.Check(`
local value = 1
if true then
    local narrowed = value
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["narrowed"]; got != "1" {
		t.Fatalf("published narrowed value = %q, want 1; values = %#v", got, result.Values)
	}
	if !hasFact(result.Outcomes, "narrowing/", "truthy") {
		t.Fatalf("outcomes did not contain a truthy narrowing: %#v", result.Outcomes)
	}
}

func TestCheckLiteralBranchSelectsOnlyMatchingArm(t *testing.T) {
	result, err := engine.Check(`
local status = "ready"
local selected
if status == "ready" then
    selected = "then"
else
    selected = "else"
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["selected"]; got != `"then"` {
		t.Fatalf("published selected = %q, want Lua string spelling; values = %#v", got, result.Values)
	}
}

func TestCheckBinaryComparisons(t *testing.T) {
	for name, source := range map[string]string{
		"equal numbers":    `local result = 2 == 2`,
		"unequal strings":  `local result = "left" ~= "right"`,
		"numeric ordering": `local result = 2 < 3`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := engine.Check(source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if got := valuesByName(result.Values)["result"]; got != "true" {
				t.Fatalf("comparison = %q, want true; values = %#v", got, result.Values)
			}
		})
	}
}

func TestCheckUnknownBranchSelectsNoArm(t *testing.T) {
	for name, source := range map[string]string{
		"truthiness":       `local input = provider(); if input then local selected = true end`,
		"numeric relation": `local input = provider(); if input >= 1 then local selected = true end`,
		"index relation":   `local input = provider(); if input[1] then local selected = true end`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := engine.Check(source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if _, selected := valuesByName(result.Values)["selected"]; selected {
				t.Fatalf("unknown selector chose an arm: %#v", result.Values)
			}
		})
	}
}

func TestCheckPathAndNilPredicates(t *testing.T) {
	result, err := engine.Check(`
local left = 3
local right = 3
local absent
local selected
if left == right then
    selected = "path"
end
if absent == nil then
    selected = "nil"
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["selected"]; got != `"nil"` {
		t.Fatalf("published selected = %q, want Lua string spelling; values = %#v", got, result.Values)
	}
}

func TestCheckNumericBranchPredicate(t *testing.T) {
	result, err := engine.Check(`
local count = 3
local selected
if count >= 3 then
    selected = "then"
else
    selected = "else"
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["selected"]; got != `"then"` {
		t.Fatalf("published selected = %q, want Lua string spelling; values = %#v", got, result.Values)
	}
}

func TestCheckDoesNotTurnAnAbsentPathIntoFalse(t *testing.T) {
	result, err := engine.Check(`
if not_bound_here then
    local selected = true
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(result.Values) != 0 || valuesByName(result.Diagnostics)["analysis/conservative"] == "" {
		t.Fatalf("absent branch path was not published as a conservative diagnostic: %#v", result)
	}
}

func TestCheckEvaluatesClosedAllocationPairs(t *testing.T) {
	result, err := engine.Check(`
local object = { first = 1, child = { second = 2 } }
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	gotKinds := make([]string, len(result.Artifact.Equations))
	for index, operation := range result.Artifact.Equations {
		gotKinds[index] = operation.Occurrence.Kind
	}
	wantKinds := []string{
		"entry",
		"allocation-template", "object-materialization", "environment-write",
		"allocation-template", "object-materialization", "environment-write",
		"environment-write", "environment-write", "environment-write",
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("closed allocation topology = %#v, want %#v", gotKinds, wantKinds)
	}
	if !reflect.DeepEqual(result.Values, []equation.Fact{}) || !reflect.DeepEqual(result.Outcomes, []equation.Fact{}) || result.Diagnostics != nil {
		t.Fatalf("closed allocations published values=%#v outcomes=%#v diagnostics=%#v; want no public scalar, return, or diagnostic facts", result.Values, result.Outcomes, result.Diagnostics)
	}
}

func TestCheckUnknownCallPublishesExplicitUnknownResult(t *testing.T) {
	result, err := engine.Check(`local value = provider()`) // provider has no local outcome.
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["value"]; got != "unknown" {
		t.Fatalf("published value = %q, want explicit unknown; values = %#v", got, result.Values)
	}
	if result.Transactions != 5 { // entry, apply, external boundary, call-results, assignment
		t.Fatalf("transactions = %d, want entry plus complete provider call sequence and assignment", result.Transactions)
	}
}

func TestCheckMethodCallUsesSealedMemberCallable(t *testing.T) {
	result, err := engine.Check(`
type Object = {
    method: (self: Object, value: number) -> number
}

local object: Object = {
    method = function(self: Object, value: number): number
        return value
    end,
}
local result = object:method("wrong")
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, fact := range result.Diagnostics {
		if strings.HasPrefix(fact.Key, "claim/unproven/") {
			t.Fatalf("closed callable record emitted an unproven annotation: %#v", result.Diagnostics)
		}
		if strings.HasPrefix(fact.Key, "type.call.direct.argument_type/") && strings.Contains(string(fact.Value), `argument 1 is "wrong", not number`) {
			return
		}
	}
	t.Fatalf("method argument mismatch was not proven: %#v", result.Diagnostics)
}

func TestCheckSelfReturningMethodPreservesSealedReceiverForMethodChain(t *testing.T) {
	result, err := engine.Check(`
type Builder = {
    f: (self: Builder) -> Builder,
    g: (self: Builder) -> number,
}
local b: Builder = {
    f = function(self: Builder): Builder
        return self
    end,
    g = function(self: Builder): number
        return 1
    end,
}
local n: number = b:f():g()
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, fact := range result.Diagnostics {
		if strings.HasPrefix(fact.Key, "claim/unproven/") {
			t.Fatalf("self-returning method chain emitted an unproven annotation: %#v", result.Diagnostics)
		}
	}
}

func TestCheckMethodCallProvesNonCallableSealedMember(t *testing.T) {
	result, err := engine.Check(`
local object = { method = false }
object:method()
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, fact := range result.Diagnostics {
		if strings.HasPrefix(fact.Key, "type.call.direct.not_callable/") && strings.Contains(string(fact.Value), "false, not callable") {
			return
		}
	}
	t.Fatalf("non-callable method member was not proven: %#v", result.Diagnostics)
}

func TestCheckMethodCallLeavesUnknownReceiverUnreported(t *testing.T) {
	result, err := engine.Check(`
local object = provider()
object:method(1)
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, fact := range result.Diagnostics {
		if strings.HasPrefix(fact.Key, "type.call.direct.") {
			t.Fatalf("unknown method receiver produced a call diagnostic: %#v", result.Diagnostics)
		}
	}
}

func TestCheckMethodCallUsesLaterMemberWriteOverClosedAbsence(t *testing.T) {
	result, err := engine.Check(`
local object = {}
object.method = false
object:method()
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, fact := range result.Diagnostics {
		if strings.HasPrefix(fact.Key, "type.call.direct.not_callable/") && strings.Contains(string(fact.Value), "false, not callable") {
			return
		}
	}
	t.Fatalf("later member write did not replace closed absence: %#v", result.Diagnostics)
}

func TestCheckDynamicIndexReadsPublishConservativeUnknown(t *testing.T) {
	for name, source := range map[string]string{
		"path destination":      `local key = "missing"; local result = record[key]; local observed = result`,
		"temporary destination": `local key = "missing"; local result = record[key].field; local observed = result`,
		"nested dynamic key":    `local first = "one"; local second = "two"; local result = record[first][second]; local observed = result`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := engine.Check(source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if got := valuesByName(result.Values)["observed"]; got != "unknown" {
				t.Fatalf("dynamic result = %q, want conservative unknown; values = %#v", got, result.Values)
			}
		})
	}
}

func TestCheckPublishesOrderedReturnTuple(t *testing.T) {
	result, err := engine.Check(`
local answer = 42
return answer, nil, false
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	got := valuesByName(result.Outcomes)
	for key, want := range map[string]string{
		"return/arity": "3",
		"return/0":     "42",
		"return/1":     "nil",
		"return/2":     "false",
	} {
		if got[key] != want {
			t.Errorf("published %s = %q, want %q; outcomes = %#v", key, got[key], want, result.Outcomes)
		}
	}
}

func TestCheckPublishesEmptyReturnTuple(t *testing.T) {
	result, err := engine.Check("return")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	got := valuesByName(result.Outcomes)
	if got["return/arity"] != "0" {
		t.Fatalf("published return arity = %q, want 0; outcomes = %#v", got["return/arity"], result.Outcomes)
	}
	if _, found := got["return/0"]; found {
		t.Fatalf("empty return published a first value: %#v", result.Outcomes)
	}
}

func TestCheckRetainsDistinctProvenAssignmentDiagnostics(t *testing.T) {
	result, err := engine.Check(`
local text: string = 1
local count: number = "one"
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	got := valuesByName(result.Diagnostics)
	if len(got) != 2 {
		t.Fatalf("assignment diagnostics = %#v, want two distinct facts", result.Diagnostics)
	}
	for _, want := range []string{
		"cannot assign text because it is number, not string",
		"cannot assign count because it is string, not number",
	} {
		found := false
		for key, detail := range got {
			if strings.HasPrefix(key, "type.assignment/op-") && detail == want {
				found = true
			}
		}
		if !found {
			t.Errorf("assignment diagnostics = %#v, missing %q", result.Diagnostics, want)
		}
	}
}

func TestCheckUsesTopForUnmaterializedMemberRead(t *testing.T) {
	result, err := engine.Check(`
local record = provider()
local name = record.name
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["name"]; got != "unknown" {
		t.Fatalf("member read = %q, want unknown; values = %#v", got, result.Values)
	}
}

func TestCheckUnknownClaimDoesNotChooseABranch(t *testing.T) {
	result, err := engine.Check(`
local raw = provider()
local value = raw :: string
local result
if value then
    result = "then"
else
    result = "else"
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["result"]; got != "nil" {
		t.Fatalf("unknown claim selected a branch: result=%q values=%#v", got, result.Values)
	}
}

func TestCheckUsesTopForUnmaterializedCurrentMemberRead(t *testing.T) {
	result, err := engine.Check(`
local record = provider()
local count = record.count + 1
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Values)["count"]; got != "unknown" {
		t.Fatalf("member arithmetic = %q, want unknown; values = %#v", got, result.Values)
	}
}

func TestCheckRejectsAsyncCallbackMemberAsUnvalidated(t *testing.T) {
	result, err := engine.Check(`local function make_async()
    local obj = {}
    coroutine.spawn(function()
        obj.get_value = function(self): number
            return 42
        end
    end)
    return obj
end

local async_obj = make_async()
local v: number = async_obj:get_value()`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.call.direct.not_callable" {
			t.Fatalf("async callback member was treated as a closed nil: %#v", result.PublishedDiagnostics)
		}
		if diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, "async_obj:get_value(...)") && strings.Contains(diagnostic.Message, "any, not number") {
			return
		}
	}
	t.Fatalf("async callback assignment diagnostic = %#v", result.PublishedDiagnostics)
}

func TestCheckProjectsCalledChildConcatEvidence(t *testing.T) {
	result, err := engine.Check(`
local function label(maybe: string?): string
    return "prefix:" .. maybe
end

return label(nil)
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, item := range result.PublishedDiagnostics {
		if item.Code != "type.operator.concat_operand" {
			continue
		}
		if len(item.Evidence) != 2 || item.Evidence[0].Message != "right operand `maybe` has type nil" || item.Evidence[1].Message != "no guard on this path proves maybe is non-nil" {
			t.Fatalf("concat diagnostic did not retain child evidence: %#v", item)
		}
		return
	}
	t.Fatalf("concat diagnostic absent: %#v", result.PublishedDiagnostics)
}

func TestCheckPublishesAdjustedOpenReturnTailSlots(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   map[string]string
	}{
		{name: "open tail", source: `return provider()`, want: map[string]string{"return/arity": "1", "return/0": "unknown"}},
		{name: "prefix and open tail", source: `return "prefix", provider()`, want: map[string]string{"return/arity": "2", "return/0": `"prefix"`, "return/1": "unknown"}},
		{name: "parenthesized tail is adjusted", source: `return (provider())`, want: map[string]string{"return/arity": "1", "return/0": "unknown"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := engine.Check(test.source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			got := valuesByName(result.Outcomes)
			for key, want := range test.want {
				if got[key] != want {
					t.Errorf("published %s = %q, want %q; outcomes = %#v", key, got[key], want, result.Outcomes)
				}
			}
			if len(got) != len(test.want) {
				t.Errorf("published outcomes = %#v, want exactly %#v", got, test.want)
			}
		})
	}
}

// This is the slot-retention regression: the false-arm transaction contributes
// nothing, while the selected arm's same-named slot must survive VM closure
// merging intact.
func TestCheckGuardedReturnRetainsSelectedSlotAtMerge(t *testing.T) {
	result, err := engine.Check(`
if false then
    return "then"
else
    return "else"
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	got := valuesByName(result.Outcomes)
	if got["return/arity"] != "1" || got["return/0"] != `"else"` {
		t.Fatalf("guarded return outcomes = %#v, want the selected else slot", result.Outcomes)
	}
}

func TestCheckMultipleReachableReturnsJoinConservatively(t *testing.T) {
	result, err := engine.Check(`
for i = 1, 1 do
    return "loop"
end
return "fallthrough"
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	got := valuesByName(result.Outcomes)
	if got["return/arity"] != "1" || got["return/0"] != "unknown" {
		t.Fatalf("multiple return outcomes = %#v, want one conservative return slot", result.Outcomes)
	}
}

func TestCheckPublishesFrontAndConservativeFailuresAsDiagnostics(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		code       string
		wantAbsent bool
	}{
		{name: "admitted open table tail", source: `local values = { provider() }`, wantAbsent: true},
		{name: "conservative", source: `local missing = absent_name + 1`, code: "analysis/conservative"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := engine.Check(test.source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if test.wantAbsent {
				if len(result.Diagnostics) != 0 {
					t.Fatalf("diagnostics = %#v, want none for admitted open table tail", result.Diagnostics)
				}
				return
			}
			if got := valuesByName(result.Diagnostics)[test.code]; got == "" {
				t.Fatalf("diagnostics = %#v, missing %q", result.Diagnostics, test.code)
			}
		})
	}
}

func TestCheckUnknownCallConditionDoesNotAuthorizeEitherGuardedArm(t *testing.T) {
	result, err := engine.Check(`
if provider() then
    local value = 1
end
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(result.Values) != 0 || len(result.Outcomes) != 0 {
		t.Fatalf("unknown condition published guarded facts: values=%#v outcomes=%#v", result.Values, result.Outcomes)
	}
}

func TestCheckWholeModuleShapedFile(t *testing.T) {
	result, err := engine.Check(`
local dependency = require("dependency")

local function first(value)
    return value
end

local function second()
    local local_value = first(42)
    return local_value
end

local answer = second()
return answer
`)
	if err != nil {
		t.Fatalf("Check whole file: %v", err)
	}
	wantValues := []equation.Fact{
		{Key: "answer", Value: []byte("unknown")},
		{Key: "dependency", Value: []byte("unknown")},
	}
	wantOutcomes := []equation.Fact{
		{Key: "return/0", Value: []byte("unknown")},
		{Key: "return/arity", Value: []byte("1")},
	}
	if !reflect.DeepEqual(result.Values, wantValues) || !reflect.DeepEqual(result.Outcomes, wantOutcomes) || result.Diagnostics != nil {
		t.Fatalf("whole-module result values=%#v outcomes=%#v diagnostics=%#v; want values=%#v outcomes=%#v and no diagnostics", result.Values, result.Outcomes, result.Diagnostics, wantValues, wantOutcomes)
	}
}

func valuesByName(values []equation.Fact) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[value.Key] = string(value.Value)
	}
	return result
}

func hasFact(facts []equation.Fact, prefix, value string) bool {
	for _, fact := range facts {
		if len(fact.Key) >= len(prefix) && fact.Key[:len(prefix)] == prefix && string(fact.Value) == value {
			return true
		}
	}
	return false
}
