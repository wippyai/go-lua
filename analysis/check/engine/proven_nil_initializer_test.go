package engine_test

import (
	"strings"
	"testing"
)

// TestProvenNilInitializerRefutesDeclaration pins the obligation an initializer
// owes. The front lowers an indexed read into the declared cell and annotates it
// in place, so the claim's source is its own target; the value is nonetheless
// the initializer's result and refutes a declaration that admits no nil.
func TestProvenNilInitializerRefutesDeclaration(t *testing.T) {
	for _, item := range []struct{ name, source string }{
		{"static key", `local t: {string} = { "a", "b" }
local n = #t
t[2] = nil
local v: string = t[n]
print(v)
`},
		{"unresolved key", `local t: {string} = { "a", "b" }
local n = #t
local j = 2
t[j] = nil
local v: string = t[n]
print(v)
`},
		{"container behind a member lens", `local box: {items: {string}} = { items = { "a", "b" } }
local n = #box.items
box.items[2] = nil
local v: string = box.items[n]
print(v)
`},
		{"proven-absent member of a closed table", `local sealed = { present = 1 }
local absent = sealed
local v: string = absent.missing
print(v)
`},
	} {
		t.Run(item.name, func(t *testing.T) {
			if diagnostics := checkSource(t, item.source); len(diagnostics) == 0 {
				t.Fatal("a declaration whose initializer was proven nil was accepted")
			}
		})
	}
}

// TestDeclarationWithoutInitializerStaysSilent is the carve-out this fix must
// preserve. The cell holds Lua's default nil, published by the write the front
// emits for the declaration itself, and that slot is the local's downstream
// contract rather than an assignment of nil to it.
func TestDeclarationWithoutInitializerStaysSilent(t *testing.T) {
	for _, item := range []struct{ name, source string }{
		{"bare declaration", `local w: string
print(w)
`},
		{"bare declaration assigned later", `local w: string
w = "x"
print(w)
`},
		{"bare table declaration", `local w: {string}
print(w)
`},
		{"bare record declaration", `local w: {name: string}
print(w)
`},
	} {
		t.Run(item.name, func(t *testing.T) {
			if diagnostics := checkSource(t, item.source); len(diagnostics) != 0 {
				t.Fatalf("a declaration with no initializer was refuted:\n%s", diagnosticSummaries(diagnostics))
			}
		})
	}
}

// TestUnrefutedAndNilAdmittingDeclarationsStaySilent covers the other side: an
// initializer the analysis never refuted, and a declaration whose own type
// admits the nil the initializer produced.
func TestUnrefutedAndNilAdmittingDeclarationsStaySilent(t *testing.T) {
	for _, item := range []struct{ name, source string }{
		{"proven string initializer", `local function text(): string return "x" end
local w: string = text()
print(w)
`},
		{"declaration admits nil", `local t: {string} = { "a", "b" }
local n = #t
t[2] = nil
local v: string? = t[n]
print(v)
`},
		{"element read with no store", `local t: {string} = { "a", "b" }
local n = #t
local v: string = t[n]
print(v)
`},
	} {
		t.Run(item.name, func(t *testing.T) {
			if diagnostics := checkSource(t, item.source); len(diagnostics) != 0 {
				t.Fatalf("a declaration the analysis never refuted was reported:\n%s", diagnosticSummaries(diagnostics))
			}
		})
	}
}

// TestProvenNilInitializerNamesTheSource pins the message. The refutation is
// about the initializer's value, so it names the declared type it failed.
func TestProvenNilInitializerNamesTheSource(t *testing.T) {
	summary := diagnosticSummaries(checkSource(t, `local t: {string} = { "a", "b" }
local n = #t
t[2] = nil
local v: string = t[n]
print(v)
`))
	if !strings.Contains(summary, "not string") {
		t.Fatalf("the refutation did not name the declared type it failed:\n%s", summary)
	}
}
