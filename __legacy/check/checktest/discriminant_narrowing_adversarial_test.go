package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

func TestBooleanDiscriminantNarrowsAssignedField(t *testing.T) {
	result := Check(`
type OK = { ok: true, value: string }
type ERR = { ok: false, value: number }
local r: OK | ERR = { ok = true, value = "x" }

if r.ok then
    local s: string = r.value
else
    local n: number = r.value
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("boolean discriminant emitted diagnostics: %#v", result.Diagnostics)
	}
}

func TestLiteralDiscriminantNarrowsAssignedField(t *testing.T) {
	result := Check(`
type A = { tag: "a", value: string }
type B = { tag: "b", value: number }
local r: A | B = { tag = "a", value = "x" }

if r.tag == "a" then
    local s: string = r.value
else
    local n: number = r.value
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("literal discriminant emitted diagnostics: %#v", result.Diagnostics)
	}
}

func TestNestedDiscriminantGuardNarrowsAliasPayload(t *testing.T) {
	result := Check(`
type FileSlot = { kind: "file", path: string }
type TimerSlot = { kind: "timer", seconds: number }
type Slot = { value: FileSlot | TimerSlot }

local slot: Slot = { value = { kind = "file", path = "/tmp/active" } }
local alias = slot

if alias.value.kind == "file" then
    local path: string = alias.value.path
    alias.value = { kind = "timer", seconds = 5 }
    local stale_path: string = slot.value.path
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeMissingMember,
		DiagnosticCount: 1,
		Line:            12,
		MessageContains: []string{"has no member \"path\""},
	})
}

func TestStaticMapEntryAliasKeepsNestedDiscriminantGuard(t *testing.T) {
	result := Check(`
type FileSlot = { kind: "file", path: string }
type TimerSlot = { kind: "timer", seconds: number }
type Slot = { value: FileSlot | TimerSlot }
type Slots = {[string]: Slot}

local slots: Slots = {
    active = {
        value = { kind = "file", path = "/tmp/active" },
    },
}
local alias = slots.active

if alias.value.kind == "file" then
    local before: string = alias.value.path
    alias.value = { kind = "timer", seconds = 5 }
    local stale_path: string = slots.active.value.path
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeMissingMember,
		DiagnosticCount: 1,
		Line:            17,
		MessageContains: []string{"has no member \"path\""},
	})
}

func TestTypeGuardElseNarrowsCallArgument(t *testing.T) {
	result := Check(`
local function need_string(s: string): string
    return s
end

local function f(v: number | string): string
    if type(v) == "number" then
        return ""
    else
        return need_string(v)
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("type guard else emitted diagnostics: %#v", result.Diagnostics)
	}
}

func TestTypeGuardElseNarrowsCallArgumentDiagnosticType(t *testing.T) {
	result := Check(`
local function need_number(n: number): number
    return n
end

local function f(v: number | string): number
    if type(v) == "number" then
        return 0
    else
        return need_number(v)
    end
end
`, WithStdlib())
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallArgType,
		DiagnosticCount: 1,
		Line:            10,
		Column:          28,
		MessageContains: []string{"argument 1 (v) is string", "not number"},
		EvidenceMin:     2,
		EvidenceContains: []string{
			"argument 1 (v) has type string",
			"need_number parameter 1 expects number",
		},
		RenderContains:    []string{"argument 1 (v) is string, not number"},
		RenderNotContains: []string{"number | string"},
		LabelMin:          1,
		LabelContains:     []string{"argument value"},
	})
}

func TestNegativeDiscriminantGuardDoesNotProveSiblingFieldWhenOtherVariantLacksIt(t *testing.T) {
	result := Check(`
type Auth = { kind: "auth", scope: string }
type Query = { kind: "query", resource: string }
type Tick = { kind: "tick" }
type Request = Auth | Query | Tick

local function f(request: Request): string
    if request.kind ~= "auth" then
        local resource: string = request.resource
        return resource
    end
    return request.scope
end
`)
	if !hasDiagnosticCode(result.Diagnostics, diagnostics.CodeAssignmentType) {
		t.Fatalf("diagnostics = %#v, want request.kind ~= auth not to prove resource because tick remains", result.Diagnostics)
	}
}
