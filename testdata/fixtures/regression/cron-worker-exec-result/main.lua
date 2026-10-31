-- Source: kickside.cron.service:worker in spiralscout/foundation.
-- The result branch and both calls below retain the source expressions.
local cron_types = require("cron_types")

local function execute_task(task: cron_types.ScheduleData, _deps: cron_types.Dependencies): cron_types.TaskExecutionResult
    return { task_id = task.id, result = nil, duration_ms = 0 }
end

local function determine_completion_action(_task: cron_types.ScheduleData, _exec_result: cron_types.TaskExecutionResult): (string, string?)
    return "complete", nil
end

local function handle_completion_action(_task: cron_types.ScheduleData, _action: string, _reason: string?, _deps: cron_types.Dependencies, _stats: cron_types.WorkerStats, _exec_result: cron_types.TaskExecutionResult?): boolean
    return true
end

local function process_body(task: cron_types.ScheduleData, deps: cron_types.Dependencies, stats: cron_types.WorkerStats)
    local task_id = task.id
    local exec_result
    if task.hydration_error then
        exec_result = {
            task_id = task_id,
            result = nil,
            error = "Corrupt task payload: " .. tostring(task.hydration_error),
            retriable = false,
            duration_ms = 0,
        }
    else
        local task_executor = deps.execute_task or execute_task
        local exec_ok, exec_result_or_err = pcall(task_executor, task, deps)
        if exec_ok then
            exec_result = exec_result_or_err
        else
            exec_result = {
                task_id = task_id,
                result = nil,
                error = "Task processor raised: " .. tostring(exec_result_or_err),
                retriable = true,
                duration_ms = 0,
            }
        end
    end
    local action, reason = determine_completion_action(task, exec_result)
    local action_success = handle_completion_action(task, action, reason, deps, stats, exec_result)
    return action_success
end

return process_body
