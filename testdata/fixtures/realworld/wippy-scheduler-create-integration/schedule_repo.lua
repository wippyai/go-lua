local security = require("security")

type TaskArgs = {
    title: string,
    agent_id: string?,
    agent_ref: string,
    agent_title: string?,
    agent_icon: string?,
    kb_ids: {string}?,
    max_iterations: integer?,
    actor_id: string,
    actor_metadata: security.ActorMeta,
    actor_scope: string?,
}

type ScheduleRecord = {
    description: string,
    class: "automation",
    user_id: string,
    actor_id: string,
    actor_scope: string?,
    actor_metadata: security.ActorMeta,
    task_implementation_id: string,
    task_args: TaskArgs,
    task_context: {[string]: any},
    schedule_type: "interval",
    schedule_expression: string,
    next_run_at: string,
    timeout_seconds: integer,
    max_retries: integer,
    enabled: boolean,
}

local M = {}
M.TaskArgs = TaskArgs
M.ScheduleRecord = ScheduleRecord

function M.create(record: ScheduleRecord): (string?, string?)
    if record.task_args.agent_ref == "" then
        return nil, "missing agent ref"
    end
    return "schedule:" .. record.user_id .. ":" .. record.task_args.agent_ref, nil
end

return M
