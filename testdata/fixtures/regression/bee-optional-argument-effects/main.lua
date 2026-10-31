-- Bee runtime-pin checker regression: bee.hub.modules:contents:124
-- Expected vs actual: Guarded offset is read before the call mutates state; actual argument expected integer, got integer?.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.0 fails.
-- Run with pinlint-driver/v1524/checker or pinlint-driver/v160/checker.
type State = {pending: boolean, mode: string, next_offset: integer?}
local function request(state: State, operation: string, offset: integer): integer
    state.next_offset = nil
    return offset
end
local function next(state: State): integer?
    if state.pending or not state.next_offset then return nil end
    return request(state, state.mode == "file" and "read_file" or "files", state.next_offset)
end
return next
