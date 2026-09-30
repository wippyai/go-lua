-- Bee runtime-pin checker regression: bee.launch:desktop_lifecycle:268/286/288/312/315
-- Expected vs actual: Assignments of declared phase members preserve Child; actual Child-to-Child arguments rejected after valid field writes.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.0 fails.
-- Run with pinlint-driver/v1524/checker or pinlint-driver/v160/checker.
type Phase = "boot" | "admit" | "running" | "save" | "exit" | "stopping"
type Child = {phase: Phase, pending: string, deadline: integer?, activation: string?, ready: boolean}
local function fail(child: Child): () child.phase = "stopping" end
local function render(child: Child): boolean return child.ready end
local function control(child: Child): boolean child.deadline = 10; return false end
local function receive(selected: Child?): ()
    local child = selected
    if not child then return end
    if child.phase == "stopping" then return end
    if child.phase == "boot" then
        child.phase, child.pending, child.deadline = "admit", "request", 10
        fail(child)
    elseif child.phase == "admit" then
        child.phase, child.pending, child.deadline = "running", "", nil
        if child.ready then fail(child) end
        render(child)
    elseif child.phase == "running" then
        child.phase = "save"
        if not control(child) then fail(child) end
    elseif child.phase == "save" then
        child.phase = "exit"
        if not control(child) then fail(child) end
    end
end
return receive
