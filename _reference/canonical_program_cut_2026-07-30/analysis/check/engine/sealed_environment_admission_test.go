package engine_test

import "testing"

// TestCapturedCalleeMemberWriteRevokesAGuardNarrowing pins the interprocedural
// revocation. The guard proves box.value non-nil, the captured callee writes nil
// through the same cell, and the read after it must therefore see the declared
// optional again. Without a heap subject for the declared record formal the
// callee's write cannot reach that read, and without admission for a body whose
// free environment is a sealed callable the body is not evaluated at all.
func TestCapturedCalleeMemberWriteRevokesAGuardNarrowing(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
local function clear(b: {value: string?}) b.value = nil end
local function interproc(box: {value: string?}): string
    if box.value then
        clear(box)
        local n: string = box.value
        return n
    end
    return "x"
end
return interproc
`)
	if !containsMessage(messages, "cannot assign box.value") {
		t.Fatalf("published = %#v, want the post-call read of box.value refuted", messages)
	}
}

// TestSealedEnvironmentSurfaceIsScopedToTheWrittenCell pins the surface of that
// admission. The same body also reads a cell no callee writes; that read still
// depends on a caller-owned refinement, so it stays dormant while the written
// cell publishes.
func TestSealedEnvironmentSurfaceIsScopedToTheWrittenCell(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
local function clear(b: {value: string?}) b.value = nil end
local function interproc(box: {value: string?}, other: {value: string?}): string
    if box.value and other.value then
        clear(box)
        local n: string = box.value
        local m: string = other.value
        return n .. m
    end
    return "x"
end
return interproc
`)
	if !containsMessage(messages, "cannot assign box.value") {
		t.Fatalf("published = %#v, want the written cell refuted", messages)
	}
	if containsMessage(messages, "cannot assign other.value") {
		t.Fatalf("published = %#v, want the unwritten cell left to its caller", messages)
	}
}

// TestSealedEnvironmentStaysDormantWithoutAMemberWrite pins the admission
// reason. A captured callee that writes nothing gives this entry no authority
// the caller lacks, so the body publishes nothing: the wrapper's postcondition
// is not part of what a declaration-owned entry decides.
func TestSealedEnvironmentStaysDormantWithoutAMemberWrite(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
local function assertNotNil(val: any)
    assert(val, "not nil")
end
local function process(a: string?)
    assertNotNil(a)
    local s: string = a
    return s
end
return process
`)
	if len(messages) != 0 {
		t.Fatalf("published = %#v, want a dormant body", messages)
	}
}

// TestMutableCaptureLeavesTheBodyDormant pins the sealing condition itself: a
// capture the enclosing scope can rebind carries no authority a later call
// observes, so no member-write obligation is admitted through it.
func TestMutableCaptureLeavesTheBodyDormant(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
local clear = function(b: {value: string?}) b.value = nil end
local function interproc(box: {value: string?}): string
    if box.value then
        clear(box)
        local n: string = box.value
        return n
    end
    return "x"
end
clear = function(b: {value: string?}) end
return interproc
`)
	if containsMessage(messages, "cannot assign box.value") {
		t.Fatalf("published = %#v, want a rebindable capture to leave the body dormant", messages)
	}
}

// TestDeclaredContainerCaptureIsSeededAsItsDeclaration pins the capture axiom.
// The captured sink's member surface is mutated from another body, so no
// allocation-time snapshot of it is valid; its declaration is, and it carries
// the heap subject the escape through it needs. narrow reaches the sink at the
// wider slot type, so the write through the sink corrupts it.
func TestDeclaredContainerCaptureIsSeededAsItsDeclaration(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
local isink: {ref: {x: number | string}} = {ref = {x = 0}}
local function istash(o: {x: number | string}) isink.ref = o end
local function escape(): number
    local narrow: {x: number} = {x = 1}
    istash(narrow)
    isink.ref.x = "boom"
    local n: number = narrow.x
    return n
end
return istash, escape
`)
	if !containsMessage(messages, "cannot assign narrow.x") {
		t.Fatalf("published = %#v, want the write through the captured sink to refute narrow.x", messages)
	}
}

// TestSealedCallableMayCloseOverASealedCell pins the environment relation. The
// captured helper closes over the same declared sink this body seals, so the
// entry reconstructs the whole environment and the body is evaluated: the write
// through the sink is observed at the read after it. A local the sink never
// reaches keeps its own value.
func TestSealedCallableMayCloseOverASealedCell(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
local isink: {ref: {x: number | string}} = {ref = {x = 0}}
local function istash(o: {x: number | string}) isink.ref = o end
local function escape(): number
    local narrow: {x: number} = {x = 1}
    isink.ref.x = "boom"
    local n: number = isink.ref.x
    return n + narrow.x
end
return istash, escape
`)
	if !containsMessage(messages, "cannot assign isink.ref.x") {
		t.Fatalf("published = %#v, want the write through the sealed sink observed", messages)
	}
	if containsMessage(messages, "cannot assign narrow.x") {
		t.Fatalf("published = %#v, want narrow untouched when it never reaches the sink", messages)
	}
}
