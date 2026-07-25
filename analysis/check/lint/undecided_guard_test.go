package lint

import (
	"context"
	"strings"
	"testing"
)

func checkOneModule(t *testing.T, source string) ProjectResult {
	t.Helper()
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{{
		Path:       "main.lua",
		ModulePath: "main",
		Source:     source,
	}}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	return result
}

func renderedDiagnostics(result ProjectResult) []string {
	out := make([]string, 0, len(result.Diagnostics))
	for _, item := range result.Diagnostics {
		out = append(out, RenderDiagnostic(item))
	}
	return out
}

func requireDiagnosticAt(t *testing.T, result ProjectResult, line int, contains string) {
	t.Helper()
	for _, item := range result.Diagnostics {
		if item.Position.Line == line && strings.Contains(item.Message, contains) {
			return
		}
	}
	t.Fatalf("no diagnostic at line %d containing %q; got %q", line, contains, renderedDiagnostics(result))
}

func requireNoDiagnosticOnLine(t *testing.T, result ProjectResult, line int) {
	t.Helper()
	for _, item := range result.Diagnostics {
		if item.Position.Line == line {
			t.Fatalf("unexpected diagnostic at line %d: %s", line, RenderDiagnostic(item))
		}
	}
}

// A guard the engine cannot decide selects neither arm, so both arms are still
// checked. The claim inside is an obligation the arm owns whatever the guard
// turns out to be.
func TestUndecidedGuardStillChecksItsArm(t *testing.T) {
	result := checkOneModule(t, `local function pick(items: {number}, text: string): number
    local anchor: string = text
    if #items > 0 then
        local unsound: number = text
    end
    return 0
end
return pick
`)
	requireDiagnosticAt(t, result, 4, "not number")
}

// The false arm of the same undecided guard is checked on its own terms: two
// mutually exclusive arms are two separate obligations.
func TestUndecidedGuardChecksBothArms(t *testing.T) {
	result := checkOneModule(t, `local function pick(items: {number}, count: number): string
    local anchor: number = count
    if #items > 0 then
        local left: string = count
    else
        local right: string = count
    end
    return ""
end
return pick
`)
	requireDiagnosticAt(t, result, 4, "not string")
	requireDiagnosticAt(t, result, 6, "not string")
}

// A write inside an undecided arm stays private to that arm: it must not become
// the value read after the join, in either direction.
func TestUndecidedArmWriteDoesNotEscapeItsEdge(t *testing.T) {
	result := checkOneModule(t, `local function pick(flag: boolean): number
    local value = 1
    if flag then
        value = 2
    end
    local total: number = value
    return total
end
return pick
`)
	requireNoDiagnosticOnLine(t, result, 6)
}

// The true edge of a conjunction proves every conjunct, so an arm reached
// through `p and p.kind == "x"` reads p as the selected arm and no nil.
func TestUndecidedConjunctionNarrowsOnItsTrueEdge(t *testing.T) {
	result := checkOneModule(t, `type Task = {kind: "task", route_id: string}
type Timer = {kind: "timer", due_at: number}

local function route(p: Task | Timer | nil): string
    if p and p.kind == "task" then
        local id: string = p.route_id
        return id
    end
    return ""
end
return route
`)
	requireNoDiagnosticOnLine(t, result, 6)
}

// A runtime type test validates an explicit any boundary for the kind it
// certifies, and only for that kind.
func TestRuntimeTypeTestValidatesTheBoundaryItCertifies(t *testing.T) {
	result := checkOneModule(t, `local function certified(value: any): string
    if type(value) == "string" then
        local text: string = value
        return text
    end
    return ""
end

local function uncertified(value: any): string
    if type(value) == "number" then
        local unsound: string = value
        return unsound
    end
    return ""
end
return certified, uncertified
`)
	requireNoDiagnosticOnLine(t, result, 3)
	requireDiagnosticAt(t, result, 11, "any")
}

// A non-empty length guard proves the first element is present, whichever
// spelling the guard uses. Without the guard the same read stays optional.
func TestNonEmptyLengthGuardProvesTheFirstElement(t *testing.T) {
	result := checkOneModule(t, `local function head(xs: {number}): number
    if #xs ~= 0 then
        local first: number = xs[1]
        return first
    end
    local unguarded: number = xs[1]
    return unguarded
end
return head
`)
	requireNoDiagnosticOnLine(t, result, 3)
	requireDiagnosticAt(t, result, 6, "may be nil")
}

// A release that happens on one edge of an undecided guard does not discharge
// the resource obligation the other edge still carries.
func TestGuardedReleaseDoesNotDischargeTheObligation(t *testing.T) {
	result := checkOneModule(t, `local resource = require("resource")

local function maybe_close(flag: boolean)
    local conn = resource.connect()
    if flag then
        resource.close(conn)
    end
end
return maybe_close
`)
	requireDiagnosticAt(t, result, 4, "expected `closed`")
}

// A truthiness check on a member path is a discriminant over the receiver's
// arms: only an arm that declares the member can hold a truthy value there.
func TestMemberTruthinessSelectsTheArmDeclaringIt(t *testing.T) {
	result := checkOneModule(t, `type Event = {kind: string, error: string?}
type Timer = {elapsed: number}

function get_result(): Event | Timer
    return {kind = "exit", error = nil}
end

function selected()
    local result = get_result()
    if result.kind then
        local k: string = result.kind
    end
end
return selected
`)
	requireNoDiagnosticOnLine(t, result, 11)
}

// A parameter-free, capture-free body still calls the module bindings it reads.
// Its private entry carries them, so the callee's declared contract reaches the
// call inside that body.
func TestClosedBodyKeepsItsModuleCallableContract(t *testing.T) {
	result := checkOneModule(t, `function label(): string
    return "ok"
end

function closed()
    local unsound: number = label()
end
return closed
`)
	requireDiagnosticAt(t, result, 6, "not number")
}

// An optional receiver reached inside a static path keeps its nilability: the
// member is present only when that receiver is.
func TestIntermediateOptionalReceiverKeepsItsNilability(t *testing.T) {
	result := checkOneModule(t, `type Session = {user_id: string}
type Ctx = {session: Session?}

local function handle(ctx: Ctx): string
    local id: string = ctx.session.user_id
    return id
end
return handle
`)
	requireDiagnosticAt(t, result, 5, "may be nil")
}

// The guard that proves the receiver present restores the ordinary member read.
func TestGuardedOptionalReceiverReadsItsMember(t *testing.T) {
	result := checkOneModule(t, `type Session = {user_id: string}
type Ctx = {session: Session?}

local function handle(ctx: Ctx): string
    if ctx.session then
        local id: string = ctx.session.user_id
        return id
    end
    return ""
end
return handle
`)
	requireNoDiagnosticOnLine(t, result, 6)
}

// A declared map read can be absent whichever selector spells it, so `m.k` and
// `m["k"]` are the same optional element read.
func TestDeclaredMapFieldReadIsOptional(t *testing.T) {
	result := checkOneModule(t, `local function owner(all: {[string]: string}): string
    local name: string = all.owner
    return name
end
return owner
`)
	requireDiagnosticAt(t, result, 2, "may be nil")
}
