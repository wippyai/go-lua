package regression

import "testing"

const callArgumentEffectPrelude = `
type State = {pending: boolean, mode: string, next_offset: integer?}
local function request(state: State, operation: string, offset: integer): integer
    state.next_offset = nil
    return offset
end
local function clear(state: State): string
    state.next_offset = nil
    return "files"
end
`

// A call's field writes happen when the callee runs, after its arguments are
// evaluated, so a narrowing that holds on entry to the call holds for them.
func TestCallArgumentReadsStateBeforeCalleeEffects(t *testing.T) {
	checkBothModes(t, callArgumentEffectPrelude+`
local function next(state: State): integer?
    if state.pending or not state.next_offset then return nil end
    return request(state, state.mode == "file" and "read_file" or "files", state.next_offset)
end
return next
`, "")
}

func TestCallArgumentReadsStateBeforeCalleeEffectsSimple(t *testing.T) {
	checkBothModes(t, callArgumentEffectPrelude+`
local function next(state: State): integer?
    if not state.next_offset then return nil end
    return request(state, "files", state.next_offset)
end
return next
`, "")
}

func TestCallEffectsInvalidateNarrowingAfterCall(t *testing.T) {
	checkBothModes(t, callArgumentEffectPrelude+`
local function next(state: State): integer?
    if not state.next_offset then return nil end
    request(state, "files", state.next_offset)
    return request(state, "files", state.next_offset)
end
return next
`, "argument 3: expected integer, got integer?")
}

func TestEarlierArgumentCallEffectsInvalidateLaterArgument(t *testing.T) {
	checkBothModes(t, callArgumentEffectPrelude+`
local function next(state: State): integer?
    if not state.next_offset then return nil end
    return request(state, clear(state), state.next_offset)
end
return next
`, "argument 3: expected integer, got integer?")
}

func TestNestedCallWithoutEffectsKeepsArgumentNarrowing(t *testing.T) {
	checkBothModes(t, callArgumentEffectPrelude+`
local function next(state: State): integer?
    if not state.next_offset then return nil end
    return request(state, tostring(state.mode), state.next_offset)
end
return next
`, "")
}

func TestEarlierSourceCallEffectsInvalidateLaterSource(t *testing.T) {
	checkBothModes(t, callArgumentEffectPrelude+`
local function next(state: State): integer?
    if not state.next_offset then return nil end
    local _, offset = clear(state), request(state, "files", state.next_offset)
    return offset
end
return next
`, "argument 3: expected integer, got integer?")
}

// Lua evaluates every source before it assigns any target.
func TestAssignmentTargetWriteFollowsSourceReads(t *testing.T) {
	checkBothModes(t, callArgumentEffectPrelude+`
local function take(value: integer): integer return value end
local function next(state: State, count: integer?): integer?
    if state.next_offset then state.next_offset, count = nil, take(state.next_offset) end
    return count
end
return next
`, "")
}

func TestAssignmentTargetWriteInvalidatesNextStatement(t *testing.T) {
	checkBothModes(t, callArgumentEffectPrelude+`
local function take(value: integer): integer return value end
local function next(state: State): integer?
    if not state.next_offset then return nil end
    state.next_offset = nil
    return take(state.next_offset)
end
return next
`, "argument 1: expected integer, got integer?")
}
