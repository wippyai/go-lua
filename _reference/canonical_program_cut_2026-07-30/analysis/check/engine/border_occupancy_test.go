package engine_test

import (
	"strings"
	"testing"
)

// TestBorderSlotOfHoleyTableIsOccupied pins Lua's border contract on a table the
// analysis has a complete inventory for. Slots 1 and 3 are written and slot 2 is
// a hole, so the borders are 1 and 3; 0 is not one because slot 1 holds a value.
// Every border the operator may return therefore names a written slot, and the
// read at that border cannot be nil.
func TestBorderSlotOfHoleyTableIsOccupied(t *testing.T) {
	diagnostics := checkSource(t, `local function f(): string
    local t: {string} = {}
    t[1] = "a"
    t[3] = "c"
    local v: string = t[#t]
    return v
end
return f
`)
	if len(diagnostics) != 0 {
		t.Fatalf("border slot of a holey table was not proven occupied:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestBorderSlotWithoutFirstSlotStaysOptional pins the other half of the same
// contract: with slot 1 absent, 0 is a border of the table and t[0] is nil, so
// the border read keeps its optionality.
func TestBorderSlotWithoutFirstSlotStaysOptional(t *testing.T) {
	diagnostics := checkSource(t, `local function f(): string
    local t: {string} = {}
    t[3] = "c"
    local v: string = t[#t]
    return v
end
return f
`)
	if len(diagnostics) == 0 {
		t.Fatal("border read was proven occupied without a written first slot")
	}
}

// TestBorderSlotClearedFirstSlotStaysOptional pins that the proof reads the live
// inventory: clearing slot 1 restores the empty border.
func TestBorderSlotClearedFirstSlotStaysOptional(t *testing.T) {
	diagnostics := checkSource(t, `local function f(): string
    local t: {string} = {}
    t[1] = "a"
    t[3] = "c"
    t[1] = nil
    local v: string = t[#t]
    return v
end
return f
`)
	if len(diagnostics) == 0 {
		t.Fatal("border read survived the removal of the first slot")
	}
}

// TestBorderSlotKeepsOptionalElementType pins that the proof discharges only the
// border's own absence. An element contract that is itself optional keeps its
// nil, because that nil belongs to the declaration rather than to the border.
func TestBorderSlotKeepsOptionalElementType(t *testing.T) {
	diagnostics := checkSource(t, `local function f(): string
    local t: {string?} = {}
    t[1] = "a"
    t[3] = "c"
    local v: string = t[#t]
    return v
end
return f
`)
	if !strings.Contains(diagnosticSummaries(diagnostics), "string?") {
		t.Fatalf("optional element contract was discharged by the border proof:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestOpaqueArrayBorderStaysOptional pins that the proof needs an inventory: a
// declared array parameter may be empty, its border is then 0, and t[0] is nil.
func TestOpaqueArrayBorderStaysOptional(t *testing.T) {
	diagnostics := checkSource(t, `local function f(t: {string}): string
    local v: string = t[#t]
    return v
end
return f
`)
	if len(diagnostics) == 0 {
		t.Fatal("border read of an opaque array was proven occupied")
	}
}
