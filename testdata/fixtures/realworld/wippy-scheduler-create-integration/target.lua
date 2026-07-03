local security = require("security")
local schedule_calculator = require("schedule_calculator")
local schedule_repo = require("schedule_repo")

type Input = {
    title: string?,
    user_id: string?,
    context: {[string]: any}?,
}

type State = {
    schedule_id: string,
    trace_id: string?,
}

type RunResult = {
    state: State,
    metadata: {
        title: string,
        actor_scope: string?,
    },
}

local M = {}
M.Input = Input
M.RunResult = RunResult

local function is_str(value: any): boolean
    return type(value) == "string" and value ~= ""
end

function M.run(input: Input, actor: security.Actor, scope: security.Scope): (RunResult?, string?)
    local title = is_str(input.title) and input.title or "repro"
    local schedule_expression = "1h"
    local now_str = "2026-06-23T12:00:00Z"
    local next_run = schedule_calculator.next_interval_run(schedule_expression, nil, now_str)

    local actor_id = actor:id()
    local actor_metadata = actor:meta()
    local user_id = is_str(input.user_id) and input.user_id or actor_id
    local actor_scope = scope.name
    local user_ctx: {[string]: any} = input.context or {}
    local trace_id: string? = nil
    local raw_trace_id = user_ctx.trace_id
    if type(raw_trace_id) == "string" then
        trace_id = raw_trace_id
    end

    local task_args: schedule_repo.TaskArgs = {
        title = title,
        agent_id = nil,
        agent_ref = "test_agent",
        agent_title = nil,
        agent_icon = nil,
        kb_ids = nil,
        max_iterations = nil,
        actor_id = actor_id,
        actor_metadata = actor_metadata,
        actor_scope = actor_scope,
    }

    local schedule_id, err = schedule_repo.create({
        description = title,
        class = "automation",
        user_id = user_id,
        actor_id = actor_id,
        actor_scope = actor_scope,
        actor_metadata = actor_metadata,
        task_implementation_id = "x:y",
        task_args = task_args,
        task_context = user_ctx,
        schedule_type = "interval",
        schedule_expression = schedule_expression,
        next_run_at = next_run,
        timeout_seconds = 300,
        max_retries = 3,
        enabled = true,
    })
    if err then
        return nil, err
    end
    if not schedule_id then
        return nil, "missing schedule id"
    end

    local state: State = {
        schedule_id = schedule_id,
        trace_id = trace_id,
    }
    return {
        state = state,
        metadata = {
            title = title,
            actor_scope = actor_scope,
        },
    }, nil
end

return M
