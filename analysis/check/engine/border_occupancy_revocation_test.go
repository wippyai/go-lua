package engine_test

import (
	"testing"
)

// borderOccupancySource builds a container whose inventory is closed and whose
// first slot is occupied, but whose slots are not contiguous. The border proof
// that licenses a read at its own length is then the inventory's, not a folded
// constant length, so these cases exercise the occupancy discharge itself.
func borderOccupancySource(between string) string {
	return `local t: {string} = { "a" }
t[3] = "c"
local n = #t
` + between + `local v: string = t[n]
print(v)
`
}

// TestStoreRevokesInventoryBorderOccupancy pins the obligation the closed
// inventory owes. The length term names the border its container had when the
// term was taken; a store that can empty a slot may have retracted that border
// since, so the inventory no longer licenses a read at the remembered length.
// The key spelling never decides it: an unresolved key and a static integer
// address a slot alike.
func TestStoreRevokesInventoryBorderOccupancy(t *testing.T) {
	for _, item := range []struct{ name, between string }{
		{"static integer key", "t[3] = nil\n"},
		{"static integer key storing an optional value", "local function pick(): string? return nil end\nt[3] = pick()\n"},
	} {
		t.Run(item.name, func(t *testing.T) {
			if diagnostics := checkSource(t, borderOccupancySource(item.between)); len(diagnostics) == 0 {
				t.Fatal("a border read at a length taken before a store was accepted")
			}
		})
	}
	t.Run("unresolved key", func(t *testing.T) {
		diagnostics := checkSource(t, `local function slot(): number return 3 end
local t: {string} = { "a" }
t[3] = "c"
local n = #t
local k = slot()
t[k] = nil
local v: string = t[n]
print(v)
`)
		if len(diagnostics) == 0 {
			t.Fatal("a border read at a length taken before an unresolved-key store was accepted")
		}
	})
}

// TestInventoryBorderOccupancySurvivesUnrelatedOperations is the control the
// revocation cases are measured against. Slot one stays occupied, so every
// border the operator may return names a written slot as long as nothing
// between the length and the read could have retracted it.
func TestInventoryBorderOccupancySurvivesUnrelatedOperations(t *testing.T) {
	for _, item := range []struct{ name, between string }{
		{"no intervening store", ""},
		{"store lands on another container", "local other: {string} = { \"x\" }\nother[1] = nil\n"},
	} {
		t.Run(item.name, func(t *testing.T) {
			if diagnostics := checkSource(t, borderOccupancySource(item.between)); len(diagnostics) != 0 {
				t.Fatalf("a border read the inventory still proves occupied was refuted:\n%s", diagnosticSummaries(diagnostics))
			}
		})
	}
	t.Run("length taken after the store", func(t *testing.T) {
		diagnostics := checkSource(t, `local t: {string} = { "a" }
t[3] = "c"
t[3] = nil
local n = #t
local v: string = t[n]
print(v)
`)
		if len(diagnostics) != 0 {
			t.Fatalf("a length taken after the store was refused its own border:\n%s", diagnosticSummaries(diagnostics))
		}
	})
}
