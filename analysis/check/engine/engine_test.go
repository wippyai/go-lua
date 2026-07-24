package engine_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
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

func TestCheckDoesNotPublishAssignmentMismatchForUnknownValue(t *testing.T) {
	result, err := engine.Check(`local value: string = provider()`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := valuesByName(result.Diagnostics)["type.assignment/op-00000002"]; got != "" {
		t.Fatalf("unknown value emitted assignment mismatch: %#v", result.Diagnostics)
	}
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
	for _, diagnostic := range unknown.Diagnostics {
		if strings.HasPrefix(diagnostic.Key, "type.assignment/") {
			t.Fatalf("unknown import emitted assignment violation: %#v", unknown.Diagnostics)
		}
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
