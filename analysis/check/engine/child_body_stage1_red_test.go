package engine_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

func TestStage1RedUncalledChildDiagnostics(t *testing.T) {
	r := checkChildAdmission(t, `local f = function() local bad: string = 1 end`)
	if len(r.Diagnostics) != 1 || !strings.HasPrefix(r.Diagnostics[0].Key, "child/") || string(r.Diagnostics[0].Value) != "cannot assign bad because it is number, not string" || !r.DiagnosticSpans[r.Diagnostics[0].Key].Valid() {
		t.Fatalf("uncalled child diagnostic/spans = %#v / %#v", r.Diagnostics, r.DiagnosticSpans)
	}
}

func TestStage1UncalledCapturedGenericReturnRejectsConcreteMismatch(t *testing.T) {
	r := checkChildAdmission(t, `
type Box<T> = {value: T}
type StringBox = {value: string}
local function make<T>(value: T): Box<T>
  return {value = value}
end
local function build(): StringBox
  return make(true)
end
return build`)
	for _, diagnostic := range r.PublishedDiagnostics {
		if strings.Contains(diagnostic.Fact.Key, "/type.return.contract/") && diagnostic.Span.StartLine == 8 {
			return
		}
	}
	t.Fatalf("captured generic return mismatch was not published: diagnostics=%#v published=%#v", r.Diagnostics, r.PublishedDiagnostics)
}

func TestTypedUncalledChildPublishesChannelSendPayloadViolation(t *testing.T) {
	r := checkChildAdmission(t, `
type Job = { id: string, meta: { attempt: number } }
local function dispatch(out: Channel<Job>)
    out:send({ id = 1, meta = { attempt = 1 } })
end
`)
	if len(r.Diagnostics) != 1 || !strings.HasPrefix(r.Diagnostics[0].Key, "child/") ||
		string(r.Diagnostics[0].Value) != "argument 1.id is 1, not string" {
		t.Fatalf("typed child channel-send diagnostics = %#v", r.Diagnostics)
	}
	if len(r.PublishedDiagnostics) != 1 || r.PublishedDiagnostics[0].Code != "type.call.direct.argument_type" ||
		r.PublishedDiagnostics[0].Span.StartLine != 4 || r.PublishedDiagnostics[0].Span.StartCol != 21 {
		t.Fatalf("typed child published diagnostics = %#v", r.PublishedDiagnostics)
	}
}

func TestStage1RedUncalledDeclaredUnionBoundaryPublishesMemberMismatch(t *testing.T) {
	r := checkChildAdmission(t, `
type Event = {kind: "event"}
type Timer = {kind: "timer", elapsed: number}
type Result = Event | Timer
local function inspect(result: Result)
  if result.kind == "event" then
    local elapsed: number = result.elapsed
  end
end`)
	if len(r.Diagnostics) != 1 || !strings.HasPrefix(r.Diagnostics[0].Key, "child/") || !strings.Contains(string(r.Diagnostics[0].Value), "has no member \"elapsed\"") {
		t.Fatalf("declared union child diagnostics = %#v", r.Diagnostics)
	}
	if len(r.PublishedDiagnostics) != 1 || r.PublishedDiagnostics[0].Code != "type.member.missing" {
		t.Fatalf("declared union child published diagnostics = %#v", r.PublishedDiagnostics)
	}
}

func TestStage1RedUncalledDeclaredOptionalFormalPublishesAssignmentMismatch(t *testing.T) {
	r := checkChildAdmission(t, `
local function require_name(name: string?)
  local exact: string = name
end`)
	if len(r.Diagnostics) != 1 || !strings.HasPrefix(r.Diagnostics[0].Key, "child/") || string(r.Diagnostics[0].Value) != "cannot assign name because it is string?, not string" {
		t.Fatalf("declared optional child diagnostics = %#v", r.Diagnostics)
	}
	if len(r.PublishedDiagnostics) != 1 || r.PublishedDiagnostics[0].Code != "type.assignment" || r.PublishedDiagnostics[0].Span.StartLine != 3 {
		t.Fatalf("declared optional child publication = %#v", r.PublishedDiagnostics)
	}
}

func TestStage1RedUncalledGuardedIndexedFormalKeepsOnlyUnprovenReadOptional(t *testing.T) {
	r := checkChildAdmission(t, `
local function inspect(values: {number}, index: number)
  if index >= 1 and index <= #values then
    local proven: number = values[index]
  end
  if index >= 1 then
    local unproven: number = values[index]
  end
	if index >= 1 and index <= #values then
		values[#values] = nil
		local invalidated: number = values[index]
	end
end`)
	proven, unproven, invalidated := false, false, false
	for _, diagnostic := range r.PublishedDiagnostics {
		if diagnostic.Code != "type.assignment" {
			continue
		}
		switch diagnostic.Span.StartLine {
		case 4:
			proven = true
		case 7:
			unproven = diagnostic.Message == "cannot assign unproven because it may be nil" &&
				len(diagnostic.Evidence) == 3 && strings.Contains(diagnostic.Evidence[0].Message, "unproven can be number or nil")
		case 11:
			invalidated = diagnostic.Message == "cannot assign invalidated because it may be nil" &&
				len(diagnostic.Evidence) == 3 && strings.Contains(diagnostic.Evidence[0].Message, "invalidated can be number or nil")
		}
	}
	if proven || !unproven || !invalidated {
		t.Fatalf("guarded indexed-formal diagnostics = %#v, want failures after missing or invalidated bounds", r.PublishedDiagnostics)
	}
}

func TestStage1RedUncalledTypePredicateRejectsUnvalidatedAny(t *testing.T) {
	source := `
type Point = {x: number, y: number}
local function validate(data: any)
    if not Point:is(data) then
        local p: Point = data
    end
	end`
	r := checkChildAdmission(t, source)
	for _, diagnostic := range r.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && diagnostic.Span.StartLine == 5 {
			return
		}
	}
	t.Fatalf("type predicate false edge did not retain any boundary: diagnostics=%#v published=%#v", r.Diagnostics, r.PublishedDiagnostics)
}

func TestStage1RedUncalledDeclaredStdlibOptionalConcatPublishesWarning(t *testing.T) {
	r := checkChildAdmission(t, `
local function unguarded(s: string): string
  local raw = s:match("p")
  return "value:" .. raw
end`)
	for _, diagnostic := range r.PublishedDiagnostics {
		if diagnostic.Code == "type.operator.concat_operand" && diagnostic.Span.StartLine == 4 {
			return
		}
	}
	t.Fatalf("declared stdlib optional concat warning = %#v", r.PublishedDiagnostics)
}

func TestStage1RedUncalledDeclaredUnionOrderedComparisonDiagnosesNonNumber(t *testing.T) {
	r := checkChildAdmission(t, `
local function before_zero(x: number | string): boolean
  return x < 0
end`)
	for _, diagnostic := range r.PublishedDiagnostics {
		if diagnostic.Code == "type.operator.comparison_operand" && diagnostic.Message == "operator < cannot compare number | string with 0" {
			return
		}
	}
	t.Fatalf("declared union ordered comparison diagnostic = %#v", r.PublishedDiagnostics)
}

func TestStage1RedUncalledDeclaredNumericUnionOrderedComparisonStaysSilent(t *testing.T) {
	r := checkChildAdmission(t, `
local function before_zero(x: integer | number): boolean
  return x < 0
end`)
	for _, diagnostic := range r.PublishedDiagnostics {
		if diagnostic.Code == "type.operator.comparison_operand" {
			t.Fatalf("numeric union ordered comparison diagnosed: %#v", r.PublishedDiagnostics)
		}
	}
}

func TestStage1RedUncalledDeclaredUnknownUnionOrderedComparisonStaysSilent(t *testing.T) {
	r := checkChildAdmission(t, `
local function before_zero(x: number | unknown): boolean
  return x < 0
end`)
	for _, diagnostic := range r.PublishedDiagnostics {
		if diagnostic.Code == "type.operator.comparison_operand" {
			t.Fatalf("unknown union ordered comparison diagnosed: %#v", r.PublishedDiagnostics)
		}
	}
}

func TestStage1RedUncalledDeclaredUnionBoundaryPublishesMissingMethod(t *testing.T) {
	r := checkChildAdmission(t, `
type Dog = {kind: "dog", bark: () -> ()}
type Cat = {kind: "cat", meow: () -> ()}
type Animal = Dog | Cat
local function speak(a: Animal)
  if a.kind == "dog" then
    a.meow()
  end
end`)
	if len(r.Diagnostics) != 1 || !strings.HasPrefix(r.Diagnostics[0].Key, "child/") || !strings.Contains(string(r.Diagnostics[0].Value), "has no member \"meow\"") {
		t.Fatalf("declared union method diagnostics = %#v", r.Diagnostics)
	}
	if len(r.PublishedDiagnostics) != 1 || r.PublishedDiagnostics[0].Code != "type.member.missing" || r.PublishedDiagnostics[0].Span.StartLine != 7 {
		t.Fatalf("declared union method publication = %#v", r.PublishedDiagnostics)
	}
}

func TestStage1RedCalledChildDiagnosticUsesChildSpan(t *testing.T) {
	r := checkChildAdmission(t, `
local retained = 0
local f = function(value)
  local keep = retained
  local bad: string = value
end
f(1)`)
	if len(r.Diagnostics) != 1 || !strings.HasPrefix(r.Diagnostics[0].Key, "child/") || string(r.Diagnostics[0].Value) != "cannot assign value because it is number, not string" {
		t.Fatalf("called child diagnostic = %#v", r.Diagnostics)
	}
	span, ok := r.DiagnosticSpans[r.Diagnostics[0].Key]
	if !ok || !span.Valid() || span.StartLine != 5 {
		t.Fatalf("called child span = %#v, want child assignment line", span)
	}
}

func TestStage1RedThreeLevelCaptures(t *testing.T) {
	r := checkChildAdmission(t, `local x = 1; return function() return function() return function() return x end end end`)
	if got := valuesByName(r.Diagnostics); len(got) != 0 || valuesByName(r.Outcomes)["return/arity"] != "1" || !strings.HasPrefix(valuesByName(r.Outcomes)["return/0"], "scalar/function/") {
		t.Fatalf("three-level closure outcome = diagnostics %#v outcomes %#v", r.Diagnostics, r.Outcomes)
	}
}

func TestStage1RedSiblingSharedMutation(t *testing.T) {
	r := checkChildAdmission(t, `local x = 0; local inc = function() x = x + 1 end; local read = function() return x end; inc(); return read()`)
	if len(r.Diagnostics) != 0 || valuesByName(r.Outcomes)["return/0"] != "1" {
		t.Fatalf("sibling cell did not converge to one: diagnostics=%#v outcomes=%#v", r.Diagnostics, r.Outcomes)
	}
}

func TestStage1RedReturnedClosures(t *testing.T) {
	r := checkChildAdmission(t, `local function make(x) return function() return x end end; local f = make(1); return f()`)
	if len(r.Diagnostics) != 0 || valuesByName(r.Outcomes)["return/0"] != "1" {
		t.Fatalf("returned closure lost its captured cell: diagnostics=%#v outcomes=%#v", r.Diagnostics, r.Outcomes)
	}
}

func TestStage1RedClosedCallableContractFillsUnavailableChildResult(t *testing.T) {
	r := checkChildAdmission(t, `
local function make(): number
  local witness: number = 1
  return witness
end
local value: number = make()`)
	if len(r.Diagnostics) != 0 {
		t.Fatalf("closed callable contract did not fill result: diagnostics=%#v values=%#v", r.Diagnostics, r.Values)
	}
}

func TestStage1RedClosedCallableContractRetainsOptionalTupleSlot(t *testing.T) {
	r := checkChildAdmission(t, `
type Config = { host: string }
local function parse(ok: boolean): (Config?, string?)
  if ok then
    return { host = "localhost" }, nil
  end
  return nil, "missing"
end
local cfg, err = parse(false)
local host: string = cfg.host`)
	for _, diagnostic := range r.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, "cfg.host") && strings.Contains(diagnostic.Message, "may be nil") {
			return
		}
	}
	t.Fatalf("optional tuple slot did not reach the local assignment: diagnostics=%#v values=%#v facts=%#v", r.PublishedDiagnostics, r.Values, r.ValueFacts)
}

func TestStage1RedCapturedOptionalResultMethodWithoutGuardIsRejected(t *testing.T) {
	r := checkChildAdmission(t, `
type DB = {release: fun(self)}
local real_db: DB = {release = function(self) end}
local function fetch(ok: boolean): (DB?, string?)
  if not ok then return nil, "failed" end
  return real_db
end
local function use(ok: boolean)
  local db, err = fetch(ok)
  db:release()
end`)
	for _, diagnostic := range r.PublishedDiagnostics {
		if diagnostic.Code == "type.call.direct.not_callable" && strings.Contains(diagnostic.Message, "db.release may be nil") {
			return
		}
	}
	t.Fatalf("unguarded captured optional result call was not rejected: diagnostics=%#v", r.PublishedDiagnostics)
}

func TestStage1RedClosedMethodContractFillsUnavailableChildResult(t *testing.T) {
	r := checkChildAdmission(t, `
local Counter = {count = 0}
function Counter:get(): number
  return self.count
end
local value: number = Counter:get()`)
	if len(r.Diagnostics) != 0 {
		t.Fatalf("closed method contract did not fill result: diagnostics=%#v values=%#v", r.Diagnostics, r.Values)
	}
}

func TestStage1RedGenericConstructorPublishesEvaluatedReturnShape(t *testing.T) {
	r := checkChildAdmission(t, `
local function zip<A, B>(a: A, b: B): { first: A, second: B }
  return { first = a, second = b }
end
local pair = zip("x", 5)
local first: string = pair.first
local second: number = pair.second`)
	if len(r.Diagnostics) != 0 {
		t.Fatalf("evaluated generic constructor result was not published: diagnostics=%#v values=%#v", r.Diagnostics, r.Values)
	}
}

func TestStage1RedMutableCaptureWriteback(t *testing.T) {
	r := checkChildAdmission(t, `local x = 0; local set = function() x = 2 end; set(); local y: number = x`)
	if len(r.Diagnostics) != 0 || valuesByName(r.Values)["y"] != "2" {
		t.Fatalf("capture writeback was not visible to caller: diagnostics=%#v values=%#v", r.Diagnostics, r.Values)
	}
}

func TestStage1RedArgumentCaptureAliasing(t *testing.T) {
	r := checkChildAdmission(t, `local x = {n = 0}; local f = function(a) x.n = 1; a.n = 2 end; f(x); local y: number = x.n`)
	if len(r.Diagnostics) != 0 {
		t.Fatalf("aliasing generated a speculative diagnostic: %#v", r.Diagnostics)
	}
}

func TestStage1RedArgumentCaptureAliasWriteback(t *testing.T) {
	r := checkChildAdmission(t, `local x = 0; local f = function(a) local keep = x; a = 2 end; f(x); local y: number = x`)
	if len(r.Diagnostics) != 0 || valuesByName(r.Values)["y"] != "2" {
		t.Fatalf("argument/capture alias writeback = diagnostics %#v values %#v", r.Diagnostics, r.Values)
	}
}

func TestStage1RedSelfAndMutualRecursion(t *testing.T) {
	r := checkChildAdmission(t, `local f; local g; f = function() return g() end; g = function() return f() end; return f()`)
	if len(r.Diagnostics) != 0 || valuesByName(r.Outcomes)["return/arity"] != "1" {
		t.Fatalf("recursive lexical SCC did not close cleanly: diagnostics=%#v outcomes=%#v", r.Diagnostics, r.Outcomes)
	}
}

func TestStage1RedDirectAndCapturedRecursion(t *testing.T) {
	for name, source := range map[string]string{
		"direct":  `local f; f = function() return f() end; return f()`,
		"capture": `local n = 1; local f; f = function() local keep = n; return f() end; return f()`,
	} {
		t.Run(name, func(t *testing.T) {
			r := checkChildAdmission(t, source)
			if len(r.Diagnostics) != 0 || valuesByName(r.Outcomes)["return/arity"] != "1" {
				t.Fatalf("recursive lexical body did not close: diagnostics=%#v outcomes=%#v", r.Diagnostics, r.Outcomes)
			}
		})
	}
}

func TestStage1RedMixedKnownUnknownTargets(t *testing.T) {
	r := checkChildAdmission(t, `local f = function() return 1 end; local g = unknown and f or provider; return g()`)
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Key != "analysis/conservative" || len(r.Values) != 0 || len(r.Outcomes) != 0 {
		t.Fatalf("mixed target must fail closed atomically: %#v", r)
	}
}

func TestStage1RedIncompleteEntryAtomicity(t *testing.T) {
	r := checkChildAdmission(t, `local x = 0; local f = function(a) x = a end; f(provider())`)
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Key != "analysis/conservative" || len(r.Values) != 0 || len(r.Outcomes) != 0 {
		t.Fatalf("failed projection leaked partial facts: %#v", r)
	}
}

func checkChildAdmission(t *testing.T, source string) engine.Result {
	t.Helper()
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return result
}
