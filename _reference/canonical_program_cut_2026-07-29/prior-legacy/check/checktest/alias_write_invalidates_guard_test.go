package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

// A field-presence guard proven through one local reference must be
// invalidated by a write through any alias of the same underlying table.
// These three fixtures each guard box.value, then overwrite it through an
// alias before reading the guarded field again; the second read must not
// trust the stale guard. Distilled from the full-oracle fixtures
// semantic/alias-mutation-invalidates-field-guard,
// semantic/dynamic-index-mutation-invalidates-field-guard, and
// semantic/dynamic-key-variant-write-invalidates-alias.

func TestAliasStaticFieldWriteInvalidatesFieldPresenceGuard(t *testing.T) {
	result := Check(`
type Box = {
    value: string?,
}

local box: Box = {value = "ready"}
local alias = box

if box.value then
    alias.value = nil
    local after: string = box.value
end

return "ok"
`)
	for _, d := range result.Diagnostics {
		if d.Code == diagnostics.CodeAssignmentType {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want type.assignment: alias.value = nil must invalidate the box.value guard", result.Diagnostics)
}

func TestAliasDynamicIndexWriteInvalidatesFieldPresenceGuard(t *testing.T) {
	result := Check(`
type Box = {
    value: string?,
}

local box: Box = {value = "ready"}
local alias = box
local key = "value"

if box.value then
    alias[key] = nil
    local after: string = box.value
end

return "ok"
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "box.value") || !strings.Contains(diag.Message, "nil") {
		t.Fatalf("message = %q, want cannot-assign-because-nil for box.value", diag.Message)
	}
}

// A write through a reference that never aliases the guarded object must not
// invalidate the guard: box and other are distinct tables, so writing
// other.value does not touch box.value's proven presence.
func TestNonAliasingWritePreservesUnrelatedFieldPresenceGuard(t *testing.T) {
	result := Check(`
type Box = {
    value: string?,
}

local box: Box = {value = "ready"}
local other: Box = {value = "ready"}

if box.value then
    other.value = nil
    local after: string = box.value
end

return "ok"
`)
	requireNoDiagnostics(t, result.Diagnostics)
}

// A write to a different field of an aliased object must not clear the guard
// proven for another field: alias and box are the same table, but the write
// targets "other", not "value", so box.value's guard survives.
func TestAliasWriteToDifferentFieldPreservesFieldPresenceGuard(t *testing.T) {
	result := Check(`
type Box = {
    value: string?,
    other: string?,
}

local box: Box = {value = "ready", other = "x"}
local alias = box

if box.value then
    alias.other = nil
    local after: string = box.value
end

return "ok"
`)
	requireNoDiagnostics(t, result.Diagnostics)
}

func TestAliasDynamicKeyVariantWriteInvalidatesDiscriminantGuard(t *testing.T) {
	result := Check(`
type FileSlot = { kind: "file", path: string }
type TimerSlot = { kind: "timer", seconds: number }
type Slot = { value: FileSlot | TimerSlot }
type Slots = {[string]: Slot}

local slots: Slots = {
    active = {
        value = {kind = "file", path = "/tmp/active"},
    },
}
local active = slots.active
local key = "active"

if active.value.kind == "file" then
    slots[key].value = {kind = "timer", seconds = 20}
    local stale_path: string = active.value.path
end
`, WithStdlib())
	for _, d := range result.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want an error: writing a new variant through slots[key] must invalidate the kind==\"file\" guard read through the active alias", result.Diagnostics)
}
