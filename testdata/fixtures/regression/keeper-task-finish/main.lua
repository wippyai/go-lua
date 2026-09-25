-- keeper.task.tools:finish
--
-- The one orchestrator exit tool. Validates the emitted signal against the
-- phase's allowed set (derived from state_machine.TRANSITIONS), verifies
-- success-like signals against authoritative state (actual changeset + node
-- outcomes rather than orchestrator claims), emits an `override` node when a
-- rewrite happens, then routes via lifecycle.handle_exit.

local ctx = require("ctx")
local changeset_repo = require("changeset_repo")
local nodes_reader = require("nodes_reader")
local nodes_writer = require("nodes_writer")
local runners = require("runners")
local state_machine = require("state_machine")
local lifecycle = require("lifecycle")

local M = {}

local P = state_machine.PHASES
local S = state_machine.SIGNALS

local function allowed_signals_for(phase)
    local signals = runners.exit_signals_for(phase)
    local set = {}
    for _, s in ipairs(signals) do set[s] = true end
    return set, signals
end

local function count_changeset_changes(task_id)
    local cs = changeset_repo.active_for_task(task_id)
    if not cs or not cs.changeset_id then return 0, nil end
    local count = changeset_repo.count_changes(cs.changeset_id) or 0
    return count, cs
end

local function latest_integrate_run(task_id)
    local row = nodes_reader.latest_of_type(task_id, "integrate_stage",
        { discriminator = "run" })
    return row
end


-- Verify the exit signal against authoritative state. Returns
-- (new_signal, new_summary, override_reason). If override_reason is nil,
-- the signal is trusted as-is.
local function verify(task_id, phase, signal, summary)
    if phase == P.IMPLEMENT and
        (signal == S.STAGED or signal == S.PUSHED or signal == S.ALREADY_DONE) then
        local changes, cs = count_changeset_changes(task_id)
        if changes == 0 and (signal == S.STAGED or signal == S.PUSHED) then
            if cs and changeset_repo.has_merged_for_task(task_id) then
                return S.ALREADY_DONE,
                    "implement produced no new changes after a prior successful merge; "
                    .. "rewritten to already_done to avoid sending an empty workspace through review/integrate. "
                    .. "Original summary: " .. tostring(summary),
                    "implement zero-edit recovery after merged changeset"
            end
            return S.STUCK,
                "implement exited " .. signal .. " but produced zero changes since spawn; rewritten to stuck. "
                .. "Original summary: " .. tostring(summary),
                "implement staged with changeset_changes=0"
        end

        if changes == 0 and signal == S.ALREADY_DONE and
            not changeset_repo.has_merged_for_task(task_id) then
            return S.STUCK,
                "implement exited already_done with zero changes and no merged changeset for this task; "
                .. "rewritten to stuck because finish would falsely mark the task shipped. "
                .. "Original summary: " .. tostring(summary),
                "implement already_done without merged changeset"
        end

        return signal, summary, nil
    end

    if phase == P.INTEGRATE and signal == S.OK then
        local run = latest_integrate_run(task_id)
        if not run or run.status ~= "passed" then
            return S.FAIL,
                "integrate claimed ok but latest integrate_stage(run) is not passed; rewritten to fail. "
                .. "Original summary: " .. tostring(summary),
                "integrate_stage(run) status != passed"
        end
        return signal, summary, nil
    end

    if phase == P.REVIEW and signal == S.APPROVED then
        local cs = changeset_repo.active_for_task(task_id)
        if not cs then
            return S.BUGS,
                "review approved but task has no live changeset (rejected or dropped); rewritten to bugs. "
                .. "Original summary: " .. tostring(summary),
                "no live changeset for review"
        end
        return signal, summary, nil
    end

    if phase == P.TEST and signal == S.APPROVED then
        local merged = changeset_repo.has_merged_for_task(task_id)
        if not merged then
            return S.BUGS,
                "test approved but task has no merged changeset; rewritten to bugs. "
                .. "Original summary: " .. tostring(summary),
                "no merged changeset on record"
        end
        local run = latest_integrate_run(task_id)
        if not run or run.status ~= "passed" then
            return S.BUGS,
                "test approved but latest integrate_stage(run) is not passed; rewritten to bugs. "
                .. "Original summary: " .. tostring(summary),
                "integrate_stage(run) status != passed"
        end
        -- UI verification (screenshot_ui for frontend changes) is a
        -- prompt-level obligation enforced by test_orchestrator's instructions
        -- and its review_ui delegate. We do NOT system-gate it here:
        -- delegate tool_calls are recorded under the child dataflow, so a
        -- node-table check produces false negatives that bounce healthy
        -- tasks into a loop. Trust the orchestrator's reported probes; the
        -- agent prompt is the right enforcement layer.
        return signal, summary, nil
    end

    return signal, summary, nil
end

local function emit_override(task_id, phase, original_signal, new_signal, reason)
    nodes_writer.record({
        task_id        = task_id,
        type           = "override",
        discriminator  = original_signal,
        title          = "Signal rewritten: " .. original_signal .. " -> " .. new_signal,
        content        = reason,
        content_type   = "text/plain",
        status         = "passed",
        visibility     = "user",
        metadata       = {
            phase            = phase,
            original_signal  = original_signal,
            new_signal       = new_signal,
            reason           = reason,
        },
    })
end

-- finish({status, summary, ...}) -> advance passthrough, err
function M.handle(task_id, current_phase, result)
    if not task_id or task_id == "" then
        return nil, "finish: task_id missing from context"
    end
    if type(result) ~= "table" then
        return nil, "finish: result table required"
    end

    local phase = current_phase or P.DESIGN
    if not state_machine.is_valid_phase(phase) then
        return nil, "finish: invalid phase '" .. tostring(phase) .. "'"
    end

    local signal = result.status
    if not signal or signal == "" then
        return nil, "finish: result.status (signal) required"
    end

    local allowed, signal_list = allowed_signals_for(phase)
    if not allowed[signal] then
        return nil, "finish: signal '" .. tostring(signal) ..
            "' not allowed for phase '" .. phase .. "' (allowed: " ..
            table.concat(signal_list, ", ") .. ")"
    end

    -- ask_user is a self-loop — no verification required, the phase stays
    -- paused until respond() re-spawns it.
    if signal ~= S.ASK_USER then
        local new_signal, new_summary, reason = verify(task_id, phase, signal, result.summary)
        if reason then
            emit_override(task_id, phase, signal, new_signal, reason)
            result = { status = new_signal, summary = new_summary }
        end
    end

    return lifecycle.handle_exit(task_id, phase, result)
end

function M.handler(result)
    local advanced, err = M.handle(ctx.get("task_id"), ctx.get("phase"), result)
    if err then
        return { error = err, ok = false }
    end
    return advanced
end

return M
